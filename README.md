# Newsdesk Article Viewer

A fast, read-only web app for browsing articles stored in a SQLite database. Designed to pair with a separate scraper that populates the DB. Single self-contained binary — no Node.js, no build pipeline.

## Features

- Full-text search across title, content, and tags (FTS5)
- Filter by site, category, author, and date range
- Category hierarchy display (e.g. `news_japan_history` → News › Japan › history)
- Infinite scroll pagination
- Article reading modal with font/size preferences
- Local signup/login with cookie-based sessions
- Read tracking — user-specific, auto-marked as read when opened; hidden from feed by default
- Favorites — user-specific starred articles; filter to show only favorites
- Compact card view for dense browsing
- Mobile-friendly with touch-optimised controls
- Dark theme

## Requirements

- Go 1.21+
- `gcc` (for CGO — `mattn/go-sqlite3` bundles SQLite source)

## Running locally

```bash
go run -tags fts5 . -db articles.db -addr :9090
```

Flags:
- `-db`   path to SQLite database (default `articles.db`)
- `-addr` listen address (default `:8080`)

## Building

```bash
go build -tags fts5 -o news-desk .
```

> **`-tags fts5` is required** on every build, run, and test command. Without it, SQLite compiles without FTS5 and all search queries fail at runtime with no warning.

## Testing

```bash
go test -tags fts5 ./...
```

## Deployment

See [CLAUDE.md](CLAUDE.md) for the full Hetzner VPS deployment guide covering dependencies, systemd service, nginx reverse proxy, and HTTPS setup.

## Tech stack

| Layer | Technology |
|-------|-----------|
| Backend | Go `net/http` + `html/template` |
| Database | SQLite via `mattn/go-sqlite3` (CGO, FTS5) |
| Frontend | HTMX 1.9.12 + vanilla JS |
| Styles | Plain CSS (dark theme, no framework) |
| Assets | Embedded in binary via `embed.FS` |

## Database

The `articles` table is written by a separate scraper. This app is read-only with respect to `articles`. It creates additional tables for auth and per-user state:

- `articles_fts` — FTS5 full-text search index (populated on startup)
- `users` — local accounts
- `user_sessions` — login sessions
- `article_reads` — tracks which articles a given user has read
- `article_favorites` — tracks which articles a given user has starred

If the app starts against an older database that still has global `article_reads` or `article_favorites` tables, they are renamed to `*_legacy_global` and new user-scoped tables are created.

## Project layout

```
├── main.go                  # Entry point, routing
├── db.go                    # Database layer
├── handlers.go              # HTTP handlers and template data
├── templates/
│   ├── index.html           # Full page shell
│   └── partials/
│       ├── cards.html       # Article card grid + infinite scroll
│       └── modal.html       # Article reading modal
├── static/
│   ├── style.css            # All styles
│   └── app.js               # All client-side JS
└── deploy/
    ├── newsdesk.service     # systemd unit
    └── nginx-article-viewer.conf
```
