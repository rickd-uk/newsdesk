# Newsdesk Auth + Deployment Guide

This is the practical reference for running, testing, and deploying Newsdesk now that it supports local accounts.

It is written around the current live setup:

- App dir: `/opt/newsdesk`
- Service: `newsdesk`
- Reverse proxy: nginx
- App bind: `127.0.0.1:8181`
- SSH host alias: `hz-hel`
- Local deploy command: `./deploy/push.sh`

## What Changed

Newsdesk now supports:

- optional account signup
- login with username or email
- per-user read tracking
- per-user starred articles

Anonymous users can still:

- open the site
- search articles
- browse and read articles

Accounts are only needed for saved/read state.

## Data Model Notes

The app now creates and uses:

- `users`
- `user_sessions`
- `article_reads`
- `article_favorites`

On first startup against an older DB, it migrates the old global tables by renaming them:

- `article_reads` -> `article_reads_legacy_global`
- `article_favorites` -> `article_favorites_legacy_global`

## Local Development

Run locally:

```bash
go run -tags fts5 . -db articles.db -addr :9090
```

Run tests:

```bash
env GOCACHE=/tmp/go-build-cache go test -tags fts5 ./...
```

## Manual Browser Test

Check these flows:

1. Open the site without logging in.
2. Confirm search and browsing work.
3. Confirm read/save actions are presented as optional account features.
4. Open the auth modal.
5. Sign up with:
   - required username
   - optional email
   - password of at least 8 chars
6. Open an article and scroll to the bottom.
7. Confirm it becomes read for that account.
8. Star an article and confirm it persists after refresh.
9. Log out and confirm anonymous browsing still works.
10. Log in again with username or email and confirm saved state returns.

## Standard Deploy

From your local machine:

```bash
./deploy/push.sh
```

What it does:

1. `rsync`s code to `/opt/newsdesk`
2. does not overwrite the live SQLite DB
3. builds `news-desk` with `-tags fts5`
4. fixes binary and DB permissions
5. restarts `newsdesk`
6. prints service status

You will still be prompted for your VPS `sudo` password. That is expected.

## Database Sync

Code deploys do not replace the live SQLite database.

To push your local `articles.db` to the VPS:

```bash
./deploy/push-db.sh
```

What it does:

1. uploads local `articles.db` to `/opt/newsdesk/articles.db.new`
2. stops `newsdesk`
3. rotates the current live DB into a timestamped backup
4. installs the new DB
5. fixes ownership on the DB file
6. starts `newsdesk`
7. prints service status
8. prunes old `pre-sync` DB backups, keeping the newest 3 by default

If you want a different local DB path:

```bash
NEWSDESK_LOCAL_DB=/path/to/articles.db ./deploy/push-db.sh
```

If you want to keep more backup generations:

```bash
NEWSDESK_BACKUP_KEEP=5 ./deploy/push-db.sh
```

## First Auth-Enabled Deploy

On the first deploy that includes auth changes, the deploy script also:

1. creates a timestamped backup of:
   - `articles.db`
   - `articles.db-wal`
   - `articles.db-shm`
2. starts the app
3. lets the app migrate old global read/star tables into legacy names

Expected journal lines:

```text
migrated legacy global table article_reads to article_reads_legacy_global
migrated legacy global table article_favorites to article_favorites_legacy_global
article-viewer listening on 127.0.0.1:8181
```

## Live Service Checks

Check service state:

```bash
sudo systemctl status newsdesk --no-pager -l
```

Check recent app logs:

```bash
sudo journalctl -u newsdesk -n 100 --no-pager -l
```

Check the app is listening locally:

```bash
curl -I http://127.0.0.1:8181/
```

Check nginx:

```bash
sudo nginx -t
sudo systemctl reload nginx
sudo tail -n 50 /var/log/nginx/error.log
```

## Common Failure: `502 Bad Gateway`

If the site shows `502 Bad Gateway`, check the app directly:

```bash
curl -I http://127.0.0.1:8181/
```

If that fails, the Go app is not running properly. Inspect:

```bash
sudo systemctl status newsdesk --no-pager -l
sudo journalctl -u newsdesk -n 50 --no-pager -l
```

## Common Failure: `attempt to write a readonly database`

This is the most important operational issue with SQLite in WAL mode.

Symptoms:

- service loops under systemd
- nginx returns `502`
- journal shows:

```text
init users table: attempt to write a readonly database
```

Cause:

`www-data` cannot write one or more of:

- `/opt/newsdesk`
- `articles.db`
- `articles.db-wal`
- `articles.db-shm`

Fix:

```bash
cd /opt/newsdesk

sudo systemctl stop newsdesk

sudo chown rick:www-data /opt/newsdesk
sudo chmod 775 /opt/newsdesk

sudo chown www-data:www-data articles.db
sudo chmod 664 articles.db

sudo chown www-data:www-data articles.db-wal articles.db-shm 2>/dev/null || true
sudo chmod 664 articles.db-wal articles.db-shm 2>/dev/null || true

sudo systemctl start newsdesk
sudo systemctl status newsdesk --no-pager -l
sudo journalctl -u newsdesk -n 20 --no-pager -l
```

The deploy script has been updated to handle this automatically on future deploys.

## If Deploy Succeeds But Service Fails

Run this exact sequence on the VPS:

```bash
cd /opt/newsdesk
sudo systemctl restart newsdesk
sudo systemctl status newsdesk --no-pager -l
sudo journalctl -u newsdesk -n 50 --no-pager -l
curl -I http://127.0.0.1:8181/
```

That will tell you whether the issue is:

- app startup
- DB permissions
- local listener
- nginx proxying

## Manual Build On VPS

If you ever need to bypass the deploy script:

```bash
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
sudo chown www-data:www-data /opt/newsdesk/news-desk
sudo chmod 755 /opt/newsdesk/news-desk
sudo systemctl restart newsdesk
sudo systemctl status newsdesk --no-pager -l
```

## Useful Commands

Deploy:

```bash
./deploy/push.sh
```

SSH to VPS:

```bash
ssh hz-hel
```

Service status:

```bash
sudo systemctl status newsdesk --no-pager -l
```

Logs:

```bash
sudo journalctl -u newsdesk -n 100 --no-pager -l
```

Local upstream test:

```bash
curl -I http://127.0.0.1:8181/
```

## Recommended Deploy Habit

Use this sequence every time:

1. Run tests locally.
2. Deploy with `./deploy/push.sh`
3. Enter sudo password when prompted.
4. If the site fails, check:
   - `systemctl status`
   - `journalctl`
   - `curl -I http://127.0.0.1:8181/`

That is the shortest reliable loop.
