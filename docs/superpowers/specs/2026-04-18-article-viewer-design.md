# Article Viewer — Design Spec
Date: 2026-04-18

## Overview

A fast, simple Go web application for searching and reading articles stored in the scraper's SQLite database (`articles.db`). Built with Go + HTMX. Deployed as a systemd service behind nginx on Hetzner VPS.

---

## Architecture

- **Language / runtime:** Go, single binary
- **HTTP:** stdlib `net/http` + `html/template`
- **Static assets:** embedded via `embed.FS` (CSS, JS, fonts)
- **Database:** read-write connection to existing `articles.db` (SQLite, `mattn/go-sqlite3`) — write access needed only on startup to create the FTS5 table; all handler queries are reads
- **Interactivity:** HTMX (~14kb, no build step) — infinite scroll, live search, modal loading
- **Font preference:** persisted in `localStorage` via ~10 lines of vanilla JS

**Directory layout:**
```
article-viewer/
  main.go
  db.go
  handlers.go
  templates/
    index.html
    partials/
      cards.html      # article card list fragment
      modal.html      # article reader fragment
      filters.html    # site/category pills fragment
  static/
    style.css
    app.js
  docs/superpowers/specs/
```

---

## Database

The app opens `articles.db` read-only. On startup it creates (if absent) one FTS5 virtual table:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts
  USING fts5(title, content, tags, content=articles, content_rowid=id);
```

If the FTS table is empty on first run, it is populated via `INSERT INTO articles_fts SELECT id, title, content, tags FROM articles`.

### Queries

| Operation | SQL strategy |
|---|---|
| Full-text search | `articles_fts MATCH ?` joined back to `articles` |
| Site filter | `WHERE site = ?` |
| Category filter | `WHERE category = ?` |
| Combined | FTS match + AND clauses for site/category |
| Pagination | `LIMIT 20 OFFSET ?` |
| Single article | `SELECT * FROM articles WHERE id = ?` |
| Filter options | `SELECT DISTINCT site FROM articles` / `SELECT DISTINCT category FROM articles` |

---

## Routes

| Method + Path | Purpose | Returns |
|---|---|---|
| `GET /` | Main page | Full HTML shell |
| `GET /articles?q=&site=&category=&offset=0` | Article cards | HTML fragment (HTMX) |
| `GET /article/{id}` | Single article modal | HTML fragment (HTMX) |

Filter options (sites + categories) are injected server-side into the initial `GET /` response — no separate endpoint needed.

---

## UI

### Top Bar
- Search input: `hx-get="/articles"`, `hx-trigger="keyup changed delay:300ms"`, `hx-target="#feed"`, `hx-push-url="true"`
- Site pills: each is a link/button with `hx-get="/articles"`, active state toggled; clicking an active pill deselects it
- Category pills: same pattern as site pills
- All filters compose — site + category + search can all be active simultaneously

### Card Grid
- 2-column grid on desktop, 1-column on mobile (CSS grid, `auto-fill`)
- Each card shows: **title** (required), site, category, date, 2-line content excerpt
- Author shown only if non-empty; date shown only if non-empty — no "Unknown" fallbacks
- Cards are `hx-get="/article/{id}"`, `hx-target="#modal-container"`, `hx-swap="innerHTML"`

### Infinite Scroll
- A sentinel `<div id="scroll-sentinel">` rendered after each card batch
- `hx-get="/articles?offset=N&..."`, `hx-trigger="intersect once"`, `hx-swap="outerHTML"`
- If fewer than 20 results returned, sentinel is omitted (end of feed)

### Article Reader Modal
- Rendered into `#modal-container` via HTMX on card click
- **Toolbar:** Serif / Sans / Mono font buttons · A− / A+ size · Copy button · ↗ Original link
- **Copy button:** uses `navigator.clipboard.writeText(articleText)`, brief "Copied!" feedback
- **Close:** ✕ button or ESC key clears `#modal-container`
- **Mobile:** modal is `position:fixed; inset:0` (full-screen sheet)
- **Font + size:** saved to `localStorage`, restored on page load via `app.js`

### Missing Data Handling
- `author`, `publish_date`, `tags`: if empty string or NULL, the element is not rendered at all
- No "Unknown", "N/A", or placeholder text anywhere in the UI

---

## Deployment

- **Binary:** `go build -o article-viewer .`
- **Config flags:** `-db /path/to/articles.db`, `-addr :8080`
- **systemd unit:** `/etc/systemd/system/article-viewer.service`
- **nginx:** reverse proxy `proxy_pass http://127.0.0.1:8080`
- The app uses `_busy_timeout=5000` and WAL journal mode — SQLite WAL allows concurrent reads alongside the scraper's writes without locking issues

---

## Out of Scope

- Authentication / login
- Writing to the database
- Mobile app / API clients
- Dark/light theme toggle (dark only)
- Article sorting options (default order: `publish_date DESC`, `scraped_at DESC` as tiebreaker)
