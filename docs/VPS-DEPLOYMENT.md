# VPS Deployment Guide

Deployment target: Hetzner VPS (Ubuntu/Debian).  
Live domain: `newsdesk.rickd.dev`  
All app files live in `/opt/newsdesk/`.

---

## Table of Contents

1. [First-time server setup](#1-first-time-server-setup)
2. [Copy source and database](#2-copy-source-and-database)
3. [Build the binary](#3-build-the-binary)
4. [Systemd service](#4-systemd-service)
5. [Nginx reverse proxy](#5-nginx-reverse-proxy)
6. [HTTPS with Let's Encrypt](#6-https-with-lets-encrypt)
7. [Routine update workflow](#7-routine-update-workflow)
8. [Service management cheatsheet](#8-service-management-cheatsheet)
9. [Common errors and fixes](#9-common-errors-and-fixes)

---

## 1. First-time server setup

### Install dependencies

```bash
sudo apt update
sudo apt install -y git gcc nginx certbot python3-certbot-nginx
```

`gcc` is required because `mattn/go-sqlite3` compiles SQLite from source via CGO. No other non-Go dependency is needed.

### Install Go 1.21+

Check if the distro version is new enough:

```bash
go version   # need 1.21 or later
```

If not, install manually:

```bash
wget https://go.dev/dl/go1.21.13.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.13.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version   # should print go1.21.x
```

### Create the deployment directory

```bash
sudo mkdir -p /opt/newsdesk
sudo chown rick:rick /opt/newsdesk   # or your SSH user
```

---

## 2. Copy source and database

Run from your **local machine**. The `--exclude` flags prevent overwriting the live database.

```bash
rsync -av \
  --exclude='articles.db' \
  --exclude='articles.db-wal' \
  --exclude='articles.db-shm' \
  --exclude='article-viewer' \
  /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer/ \
  rick@hz-hel:/opt/newsdesk/
```

To copy the database for the first time (or to replace it):

```bash
scp articles.db rick@hz-hel:/opt/newsdesk/articles.db
```

---

## 3. Build the binary

**On the VPS:**

```bash
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
```

> **Critical:** `-tags fts5` is required on every build. Without it, SQLite compiles without FTS5 support and all full-text search queries fail at runtime with no compile-time warning.

First build takes ~1–2 minutes (CGO + SQLite compilation). Subsequent builds are faster thanks to Go's build cache.

Set correct ownership after building:

```bash
sudo chown www-data:www-data /opt/newsdesk/news-desk
sudo chmod 755 /opt/newsdesk/news-desk
sudo chown www-data:www-data /opt/newsdesk/articles.db
```

---

## 4. Systemd service

The service file is at `deploy/newsdesk.service` in the repo.

### Install

```bash
sudo cp /opt/newsdesk/deploy/newsdesk.service /etc/systemd/system/newsdesk.service
sudo systemctl daemon-reload
sudo systemctl enable newsdesk
sudo systemctl start newsdesk
sudo systemctl status newsdesk
```

### Service file contents

```ini
[Unit]
Description=Newsdesk Article Viewer
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/newsdesk
ExecStart=/opt/newsdesk/news-desk -db /opt/newsdesk/articles.db -addr 127.0.0.1:8181
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

The binary listens on `127.0.0.1:8181` (localhost only). Nginx proxies public traffic to it.

---

## 5. Nginx reverse proxy

### Install the config

Edit the domain name first:

```bash
nano /opt/newsdesk/deploy/nginx-article-viewer.conf
# Set: server_name newsdesk.rickd.dev;
```

Then install:

```bash
sudo cp /opt/newsdesk/deploy/nginx-article-viewer.conf \
    /etc/nginx/sites-available/newsdesk
sudo ln -s /etc/nginx/sites-available/newsdesk \
    /etc/nginx/sites-enabled/newsdesk
sudo nginx -t        # must print "syntax is ok"
sudo systemctl reload nginx
```

### Config contents (after editing domain)

```nginx
server {
    listen 80;
    listen [::]:80;
    server_name newsdesk.rickd.dev;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        proxy_pass         http://127.0.0.1:8181;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 30s;
    }
}
```

> The `/.well-known/acme-challenge/` block must come **before** the proxy block, otherwise certbot's HTTP challenge will be proxied to the app (which returns 404) instead of being served from disk.

> `listen [::]:80;` enables IPv6. Required if your VPS has an IPv6 address — certbot validates whichever IP DNS resolves to, and the challenge will fail if nginx only listens on IPv4.

---

## 6. HTTPS with Let's Encrypt

```bash
sudo certbot --nginx -d newsdesk.rickd.dev
```

Certbot automatically edits the nginx config to add `listen 443 ssl` and sets up auto-renewal. After running, verify HTTPS works:

```bash
curl -I https://newsdesk.rickd.dev
```

Auto-renewal runs via a systemd timer or cron. Test the renewal process:

```bash
sudo certbot renew --dry-run
```

---

## 7. Routine update workflow

Use this every time you push code changes.

### One-command deploy

From your local machine:

```bash
./deploy/push.sh
```

What it does:

- `rsync`s the repo to `/opt/newsdesk` without overwriting the live SQLite DB
- runs `go build -tags fts5 -o news-desk .` on the VPS
- restarts the `newsdesk` systemd service
- creates a one-time timestamped backup of `articles.db`, `articles.db-wal`, and `articles.db-shm` before the first auth-enabled startup migration

By default the script connects to the SSH host alias `hz-hel`. Override it only if needed:

Optional overrides:

```bash
NEWSDESK_HOST=rick@hz-hel \
NEWSDESK_REMOTE_DIR=/opt/newsdesk \
NEWSDESK_SERVICE=newsdesk \
./deploy/push.sh
```

To replace the live database with your local `articles.db`:

```bash
./deploy/push-db.sh
```

### Manual workflow

If you ever want to do it step by step, the old commands are still valid.

**From your local machine:**

```bash
rsync -av \
  --exclude='articles.db' \
  --exclude='articles.db-wal' \
  --exclude='articles.db-shm' \
  --exclude='article-viewer' \
  /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer/ \
  rick@hz-hel:/opt/newsdesk/
```

**On the VPS:**

```bash
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
sudo systemctl restart newsdesk
sudo systemctl status newsdesk
```

To update the database only (from local machine):

```bash
scp articles.db rick@hz-hel:/opt/newsdesk/articles.db
# Then restart the service to rebuild the FTS index:
ssh rick@hz-hel "sudo systemctl restart newsdesk"
```

> The FTS5 full-text search index is built once at startup from the articles table. New articles scraped after the server starts are not searchable until the service is restarted.

---

## 8. Service management cheatsheet

```bash
sudo systemctl status newsdesk       # current status
sudo systemctl start newsdesk        # start
sudo systemctl stop newsdesk         # stop
sudo systemctl restart newsdesk      # restart (use after every build)
sudo systemctl reload newsdesk       # not useful (binary doesn't support config reload)
sudo journalctl -u newsdesk -f       # follow live logs
sudo journalctl -u newsdesk -n 50    # last 50 log lines
```

---

## 9. Common errors and fixes

### `bind: address already in use`

Another process is on port 8181. Find and kill it:

```bash
ss -tlnp | grep 8181
sudo kill <pid>
```

Or just restart the service — systemd will handle it:

```bash
sudo systemctl restart newsdesk
```

---

### Systemd unit file parse error: `bad unit setting`

Caused by leading spaces or a multi-line `ExecStart`. The unit file must have no indentation and `ExecStart` on a single line.

Fix by rewriting the file cleanly:

```bash
sudo tee /etc/systemd/system/newsdesk.service << 'EOF'
[Unit]
Description=Newsdesk Article Viewer
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/newsdesk
ExecStart=/opt/newsdesk/news-desk -db /opt/newsdesk/articles.db -addr 127.0.0.1:8181
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl restart newsdesk
```

---

### Certbot fails: `Connection refused` or `Timeout during connect`

**Cause A — nginx not listening on IPv6.**  
If your VPS has an IPv6 address and DNS resolves to it, nginx must listen on `[::]:80`.

Add to the `server` block in your nginx config:

```nginx
listen [::]:80;
```

Then: `sudo nginx -t && sudo systemctl reload nginx`

**Cause B — ACME challenge proxied to the app.**  
The proxy `location /` block must not catch the certbot challenge path.

Add this block **before** `location /`:

```nginx
location /.well-known/acme-challenge/ {
    root /var/www/html;
}
```

Then: `sudo nginx -t && sudo systemctl reload nginx && sudo certbot --nginx -d newsdesk.rickd.dev`

---

### Articles from one source don't appear when date filter is active

**Cause:** The scraper stored `publish_date` in a non-ISO format (e.g. `"Sun 12 Apr 2026 09.00 BST"` instead of `"2026-04-12"`). SQLite date comparisons on text columns use lexicographic order — `'S' > '2'` — so every article with a non-ISO date fails the `<= date_to` condition.

**Fix** (already applied in `db.go`): wrap the column in SQLite's `date()` function. Non-ISO dates return `NULL` from `date()` and are treated as "passes the filter" rather than excluded:

```sql
AND (date(a.publish_date) IS NULL OR date(a.publish_date) >= ?)
AND (date(a.publish_date) IS NULL OR date(a.publish_date) <= ?)
```

The permanent fix is to have the scraper store dates in `YYYY-MM-DD` format.

---

### Mobile filter panel closes immediately when tapping inside it

**Cause:** The filter panel `<div>` was a child of `<header id="top-bar">` (which creates a stacking context at `z-index: 100`). The backdrop overlay was a sibling at `z-index: 149`. Since 149 > 100, the backdrop rendered on top of the entire header including the panel, swallowing every tap and firing `toggleFilters()`.

**Fix** (already applied in `index.html`): move `#filter-panel` outside `<header>` so it participates in the root stacking context. Its own `z-index: 150` then correctly beats the backdrop's `149`.

---

### FTS search returns no results / `no such table: articles_fts`

The FTS index is created by `InitFTS()` at startup. If the server was started without `-tags fts5` during build, SQLite lacks FTS5 support and the table was never created.

Rebuild with the correct tag:

```bash
go build -tags fts5 -o news-desk .
sudo systemctl restart newsdesk
```

---

### `www-data` permission denied on `articles.db`

```bash
sudo chown www-data:www-data /opt/newsdesk/articles.db
sudo chown www-data:www-data /opt/newsdesk/news-desk
sudo chmod 755 /opt/newsdesk/news-desk
```

---

## File locations on VPS

| File | Path |
|------|------|
| Binary | `/opt/newsdesk/news-desk` |
| Database | `/opt/newsdesk/articles.db` |
| Source | `/opt/newsdesk/` |
| systemd unit | `/etc/systemd/system/newsdesk.service` |
| nginx config | `/etc/nginx/sites-available/newsdesk` |
| nginx symlink | `/etc/nginx/sites-enabled/newsdesk` |
| TLS certificates | `/etc/letsencrypt/live/newsdesk.rickd.dev/` |
