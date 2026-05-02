# Newsdesk Guide

Newsdesk is a Go + SQLite article browser for reading, searching, filtering, and saving articles collected by a separate scraper. It is designed as a small self-contained web app: the backend is Go `net/http`, templates are embedded into the binary, the frontend is HTMX plus vanilla JavaScript, and article/search data lives in SQLite.

This guide covers how to use, run, develop, deploy, and troubleshoot the current software.

## Quick Start

Run locally:

```bash
go run -tags fts5 . -db articles.db -addr :9090
```

Open:

```text
http://127.0.0.1:9090/
```

Run tests:

```bash
go test -tags fts5 ./...
```

Build a binary:

```bash
go build -tags fts5 -o news-desk .
```

The `-tags fts5` build tag is required for local runs, tests, and production builds. Without it, SQLite will not include FTS5 support and search/startup can fail with `no such module: fts5`.

## What Newsdesk Does

Newsdesk provides:

- Article card feed with infinite scroll pagination.
- Full-text search over title, content, and tags.
- Filters for site, category, author, and date range.
- Search field selection for title, content, and tags.
- Article reading modal with font and size controls.
- Optional accounts for saved state.
- Per-user read tracking.
- Per-user favorites.
- Per-user archive state.
- Compact card view.
- Mobile filter drawer and touch-friendly controls.
- Dark theme.

Anonymous users can browse, search, filter, and read articles. Accounts are only required for persistent read/favorite/archive state.

## Requirements

Install:

- Go 1.21 or newer.
- `gcc`, because `github.com/mattn/go-sqlite3` uses CGO.
- A SQLite database containing an `articles` table.

For production deployment, the current helper scripts assume:

- SSH host alias: `hz-hel`
- Remote app directory: `/opt/newsdesk`
- Systemd service: `newsdesk`
- Runtime user: `www-data`
- App bind address: `127.0.0.1:8181`
- Nginx reverse proxy in front of the Go app

These defaults can be overridden with environment variables described later.

## Project Layout

```text
main.go                         entry point, route registration, embedded files
handlers.go                     HTTP handlers and template view models
db.go                           article query/search/state database layer
auth.go                         users, sessions, password hashing, cookies
templates/index.html            main page shell
templates/partials/cards.html   article card grid and infinite-scroll sentinel
templates/partials/modal.html   article reading modal
static/app.js                   browser behavior, HTMX hooks, filters, modals
static/style.css                full UI styling
deploy/push.sh                  code deploy helper
deploy/push-db.sh               database deploy helper
deploy/remote-restart.sh        remote build/restart helper
deploy/newsdesk.service         systemd unit
deploy/nginx-article-viewer.conf nginx reverse proxy example
docs/                           supporting deployment/status docs
sessions.txt                    working handoff notes
```

## Runtime Flags

Newsdesk has two flags:

```text
-db    path to SQLite database, default articles.db
-addr  listen address, default :8080
```

Examples:

```bash
go run -tags fts5 . -db articles.db -addr :9090
go run -tags fts5 . -db /opt/newsdesk/articles.db -addr 127.0.0.1:8181
```

## User Interface

### Feed

The home page shows article cards. Each card includes site, category, publication date, title, author, and an excerpt.

Scrolling to the bottom loads the next page of articles via HTMX. The page size is currently `20`, defined by `pageSize` in `handlers.go`.

### Search

Use the top search input to search articles. Search is backed by SQLite FTS5 and is prefix-oriented: each term is converted into an FTS prefix term.

The filter panel lets you choose which fields are searched:

- Title
- Content
- Tags

If no field is selected in the URL/query state, the app treats search as searching all fields.

### Filters

The filter button opens the filter panel. Available filters:

- Site
- Category
- Author
- Date range
- Hide read
- Latest reads
- Favorites only
- Archived only

Read, favorite, and archive filters only apply when logged in. Anonymous users do not have user-specific state.

On mobile, the filter panel is a left-side drawer. While the mobile drawer is open, body scrolling is intentionally locked. The scroll lock is released when the drawer closes.

### Article Modal

Clicking a card opens the article modal. The modal includes:

- Article title and metadata.
- Article body.
- Font selector.
- Font size controls.
- Copy button.
- Original article link when present.
- Read/favorite/archive controls for logged-in users.

For logged-in users, opening an article and scrolling to the bottom marks it as read.

### Accounts

Accounts are optional. Anonymous browsing remains available.

Signup requires:

- Username.
- Password of at least 8 characters.

Email is optional.

Login accepts either username or email plus password.

## HTTP Routes

Current routes:

```text
GET  /                         full page
GET  /articles                 card partial for search/filter/infinite scroll
GET  /article/{id}             article modal partial
POST /article/{id}/read        mark article read, login required
POST /article/{id}/unread      mark article unread, login required
POST /article/{id}/favorite    mark favorite, login required
POST /article/{id}/unfavorite  unmark favorite, login required
POST /article/{id}/archive     archive article, login required
POST /article/{id}/unarchive   unarchive article, login required
POST /signup                   create account and session
POST /login                    create session
POST /logout                   delete session
GET  /static/...               embedded static files
```

`/articles` returns the `cards` template partial, not a full page. It is used by search, filters, and infinite scroll.

## Database

### Source Data

Newsdesk expects a scraper or other external process to populate the `articles` table. Newsdesk reads articles but does not scrape them.

The app expects article fields corresponding to:

```text
id
site
url
category
title
author
publish_date
tags
content
scraped_at
```

The app creates and maintains auxiliary tables for search, auth, and user state.

### App-Created Tables

Newsdesk creates:

```text
articles_fts        SQLite FTS5 index for title/body/tags
users               local accounts
user_sessions       cookie sessions
article_reads       per-user read state
article_favorites   per-user starred state
article_archives    per-user archived state
```

`articles_fts` is rebuilt on startup if the row counts or row IDs do not match `articles`.

### Legacy Migration

Older versions used global article state. On startup, if an old state table exists without `user_id`, Newsdesk renames it:

```text
article_reads     -> article_reads_legacy_global
article_favorites -> article_favorites_legacy_global
article_archives  -> article_archives_legacy_global, if applicable
```

It then creates new user-scoped state tables.

### SQLite WAL Mode

`OpenDB` enables WAL mode:

```sql
PRAGMA journal_mode=WAL;
```

In production this means the runtime user must be able to write:

```text
/opt/newsdesk
/opt/newsdesk/articles.db
/opt/newsdesk/articles.db-wal
/opt/newsdesk/articles.db-shm
```

Readonly database errors are usually permission errors on one of those paths.

## Local Development

Use:

```bash
go run -tags fts5 . -db articles.db -addr :9090
```

Then open:

```text
http://127.0.0.1:9090/
```

Recommended local checks before deploy:

```bash
go test -tags fts5 ./...
node --check static/app.js
```

If using an alternate Go cache:

```bash
env GOCACHE=/tmp/go-build-cache-newsdesk go test -tags fts5 ./...
```

## Manual Browser Test Checklist

Before deploying UI/auth changes, check:

1. Open the site while logged out.
2. Scroll down far enough to load more cards.
3. Search for a common term.
4. Toggle field filters for title/content/tags.
5. Filter by site and category.
6. Filter by author.
7. Set and clear a date range.
8. Open an article modal.
9. Close it by button, backdrop, and Escape.
10. Open the auth modal and close it.
11. Sign up with a test account.
12. Open an article and scroll to the bottom.
13. Confirm the card is marked read.
14. Mark unread.
15. Favorite and unfavorite.
16. Archive and unarchive.
17. Refresh and confirm user state persists.
18. Log out and confirm anonymous browsing still works.
19. On mobile width, open and close the filter drawer.
20. Confirm page scroll is not locked after closing overlays/drawers.

## Deployment

### Code Deploy

Use:

```bash
./deploy/push.sh
```

Default target:

```text
hz-hel:/opt/newsdesk
```

What it does:

- Rsyncs the repo to the remote app directory.
- Excludes local DB files, backups, build outputs, and Git metadata.
- Runs `deploy/remote-restart.sh` remotely.
- Builds `news-desk` with `go build -tags fts5 -o news-desk .`.
- Restarts the `newsdesk` systemd service.
- Prints service status.

Override defaults:

```bash
NEWSDESK_HOST=my-host ./deploy/push.sh
NEWSDESK_REMOTE_DIR=/opt/other-newsdesk ./deploy/push.sh
NEWSDESK_SERVICE=other-service ./deploy/push.sh
NEWSDESK_BUILD_CMD='go build -tags fts5 -o news-desk .' ./deploy/push.sh
```

You can also pass the host as the first argument:

```bash
./deploy/push.sh hz-hel
```

### Database Deploy

Code deploys do not replace the live `articles.db`.

To replace the live database with local `articles.db`:

```bash
./deploy/push-db.sh
```

What it does:

- Uploads the local DB to `/opt/newsdesk/articles.db.new`.
- Stops the service.
- Moves the current live DB and sidecars to timestamped pre-sync backups.
- Installs the uploaded DB as `articles.db`.
- Fixes directory and DB ownership/permissions.
- Removes stale WAL/SHM sidecars.
- Starts the service.
- Prints service status.
- Prunes old pre-sync backups.

Use a different local DB:

```bash
NEWSDESK_LOCAL_DB=/path/to/articles.db ./deploy/push-db.sh
```

Keep more backup generations:

```bash
NEWSDESK_BACKUP_KEEP=5 ./deploy/push-db.sh
```

### Systemd

The provided unit runs:

```text
/opt/newsdesk/news-desk -db /opt/newsdesk/articles.db -addr 127.0.0.1:8181
```

As:

```text
User=www-data
WorkingDirectory=/opt/newsdesk
```

Useful commands on the VPS:

```bash
sudo systemctl status newsdesk --no-pager -l
sudo systemctl restart newsdesk
sudo journalctl -u newsdesk -n 100 --no-pager -l
```

### Nginx

The example nginx config proxies public traffic to:

```text
http://127.0.0.1:8181
```

Check and reload nginx:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

Check app directly from the VPS:

```bash
curl -I http://127.0.0.1:8181/
```

## Troubleshooting

### `no such module: fts5`

Cause: the binary or test command was built without the `fts5` tag.

Fix:

```bash
go run -tags fts5 . -db articles.db -addr :9090
go test -tags fts5 ./...
go build -tags fts5 -o news-desk .
```

### `attempt to write a readonly database`

Cause: the service user cannot write the SQLite DB, sidecar files, or containing directory.

Fix on VPS:

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
sudo journalctl -u newsdesk -n 50 --no-pager -l
```

### `502 Bad Gateway`

First check whether the app is running:

```bash
curl -I http://127.0.0.1:8181/
```

If that fails:

```bash
sudo systemctl status newsdesk --no-pager -l
sudo journalctl -u newsdesk -n 100 --no-pager -l
```

If the app responds locally, check nginx:

```bash
sudo nginx -t
sudo tail -n 50 /var/log/nginx/error.log
```

### Search Returns Nothing Unexpectedly

Check:

- Was the binary built with `-tags fts5`?
- Does `articles` contain rows?
- Does `articles_fts` have matching rows?

Useful SQLite checks:

```bash
sqlite3 articles.db 'SELECT COUNT(*) FROM articles;'
sqlite3 articles.db 'SELECT COUNT(*) FROM articles_fts;'
sqlite3 articles.db 'SELECT COUNT(*) FROM articles a LEFT JOIN articles_fts f ON f.rowid=a.id WHERE f.rowid IS NULL;'
```

The app repairs the FTS index at startup if it detects missing or orphaned rows.

### Page Cannot Scroll

The frontend intentionally locks body scrolling when overlays are open:

- article modal
- auth modal
- mobile filter drawer

If scrolling stays locked, inspect `document.body.style.overflow` and the open state of:

```text
#modal-overlay
#auth-overlay
#filter-panel
```

The current scroll-lock code lives in `static/app.js`:

```text
filterDrawerLocksPage()
updateBodyScrollLock()
```

### Login Required on State Actions

Read, unread, favorite, unfavorite, archive, and unarchive routes require a logged-in user. Anonymous users can still browse and read; they just do not get persistent state.

## Maintenance Notes

### Updating the Article Database

The scraper or external process owns article ingestion. Once a refreshed DB exists locally, push it with:

```bash
./deploy/push-db.sh
```

This is intentionally separate from code deploys so app changes do not accidentally replace production data.

### Backup Retention

Deploy scripts prune old generated backups. Default retention is `3`.

Override:

```bash
NEWSDESK_BACKUP_KEEP=5 ./deploy/push.sh
NEWSDESK_BACKUP_KEEP=5 ./deploy/push-db.sh
```

### Dirty Working Tree

This repo often has active local changes. Before larger edits or deploys, check:

```bash
git status --short
git diff --stat
```

Do not assume unrelated modified files belong to the current task.

## Important Implementation Details

- Templates and static assets are embedded with Go `embed.FS`.
- The app uses plain `html/template`, not a frontend build system.
- HTMX powers card replacement, modal loading, and infinite scroll.
- `static/app.js` owns UI state such as filters, local preferences, scroll lock, and modal behavior.
- `static/style.css` owns all layout and responsive behavior.
- SQLite connections are limited to one open connection with `SetMaxOpenConns(1)`.
- Session cookies are backed by hashed session tokens in `user_sessions`.
- Passwords are salted and hashed in `auth.go`.
- User article state tables use `(user_id, article_id)` as the primary key.

## Safe Change Checklist

For backend changes:

```bash
go test -tags fts5 ./...
```

For frontend JavaScript changes:

```bash
node --check static/app.js
```

For template/CSS/UI behavior:

- Run the app locally.
- Test anonymous browsing.
- Test logged-in browsing.
- Test mobile width.
- Test infinite scroll.
- Test article modal open/close.

For deployment changes:

- Read `deploy/push.sh`.
- Read `deploy/remote-restart.sh`.
- Check service status after deploy.
- Check app direct with `curl -I http://127.0.0.1:8181/`.
- Check nginx if direct app works but public site fails.
