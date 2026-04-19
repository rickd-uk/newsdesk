# Newsdesk Article Viewer — Project Status

**Last updated:** 2026-04-19  
**Live URL:** https://newsdesk.rickd.dev  
**Local dev:** `go run -tags fts5 . -db articles.db -addr :9090`

---

## What This Is

A read-only Go web app serving articles from a SQLite database populated by a separate scraper (`scrape-adv`). Single self-contained binary — no Node.js, no build pipeline.

---

## Stack

| Layer | Technology |
|---|---|
| Backend | Go `net/http`, `html/template`, `embed.FS` |
| Database | SQLite via `mattn/go-sqlite3` (CGO), FTS5 full-text search |
| Frontend | HTMX 1.9.12 (served locally from `/static/htmx.min.js`), vanilla JS, vanilla CSS |
| Deployment | Hetzner VPS, systemd, nginx, Let's Encrypt |

**Critical:** Every `go build`, `go run`, `go test` must include `-tags fts5`.

---

## Features Implemented

### Core
- Article feed with infinite scroll (HTMX intersection sentinel)
- Full-text search via FTS5 (title / body / tags, individually selectable)
- Article modal with font picker (Serif/Sans/Mono), size adjustment, copy-to-clipboard
- Live filtering with URL sync (`HX-Push-Url`)

### Filters
- Site pill filter
- Category pill filter with hierarchy (group → subgroup → pill)
- Category pills filtered by selected site (`data-sites` attribute)
- Author text filter
- Date range filter (From / To, stacked layout)
- Hide read checkbox
- Favorites-only checkbox
- Filter badge showing active filter count
- Mobile: full-height slide-in drawer with backdrop

### Read Tracking (server-persisted)
- `article_reads` table created on startup
- Article marked as read only when user scrolls to the **bottom** of the article
- Read cards: `opacity: 0.45`, `filter: grayscale(40%)`, green top border, green `✓ Read` badge in meta row
- `↩` icon button in modal toolbar (disabled until article is fully read, re-disables after marking unread)
- "Mark unread" removes read state from DB and card immediately

### Favorites
- `article_favorites` table created on startup
- `☆` / `★` icon button in modal toolbar (star only, no text)
- Favorited cards get a gold top border and `★` in meta row
- "Favorites only" filter in filter panel

### Recent Search History
- Last 12 searches saved to `localStorage` (`av-recent-searches`)
- Last 12 authors saved to `localStorage` (`av-recent-authors`)
- Dropdown appears on focus; click to reuse; "Clear history" button
- Search suggestions positioned fixed below search bar; author suggestions inline in filter panel

### UI / UX
- Dark theme, no framework
- Compact view toggle (⛇ button)
- Responsive: single-column feed on mobile
- Modal is bottom sheet on mobile (slides up from bottom)
- `← Back` button at bottom of modal on mobile (no scroll-to-top needed to close)
- Date range: "All dates" pill → click reveals From/To stacked inputs → `✕ Clear` button
- Close drawer button: icon only (`✕`), large tap target, top-right aligned
- Original article link: `↗` icon only (no text)
- Font/size preferences persisted in `localStorage`
- Compact view preference persisted in `localStorage`

---

## Database Schema (created by scraper, read-only by viewer)

```sql
CREATE TABLE articles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    url          TEXT,
    fingerprint  TEXT UNIQUE,
    title        TEXT,
    author       TEXT,
    publish_date TEXT,   -- may be ISO (YYYY-MM-DD) or freeform ("Sun 12 Apr 2026 09.00 BST")
    tags         TEXT,   -- comma-separated
    content      TEXT,   -- plain text, paragraphs separated by \n\n
    scraped_at   DATETIME,
    category     TEXT,   -- underscore hierarchy e.g. "news_japan_history"
    site         TEXT    -- e.g. "The Japan Times", "The Guardian"
)
```

App-created tables:
```sql
CREATE TABLE article_reads    (article_id INTEGER PRIMARY KEY, read_at DATETIME DEFAULT CURRENT_TIMESTAMP)
CREATE TABLE article_favorites(article_id INTEGER PRIMARY KEY, favorited_at DATETIME DEFAULT CURRENT_TIMESTAMP)
```

---

## Known Issues / Gotchas Fixed

### Non-ISO publish_date breaks date filtering
**Sites affected:** The Guardian (stores dates as `"Sun 12 Apr 2026 09.00 BST"`)  
**Fix applied:** All date comparisons use `date(a.publish_date)` — returns NULL for non-ISO dates, which pass through the filter rather than being excluded.  
**Permanent fix needed:** Scraper should store dates as `YYYY-MM-DD`.

### Mobile filter panel taps closing drawer immediately
**Cause:** `#filter-panel` was inside `<header>` (stacking context z-index 100). The backdrop was a sibling at z-index 149, rendering on top of the panel.  
**Fix:** Moved `#filter-panel` outside `<header>` into the root stacking context. Panel z-index 150 now correctly beats backdrop z-index 149.

### HTMX CDN failure
**Fix:** HTMX 1.9.12 now served locally from `/static/htmx.min.js`. No external CDN dependency.

### Read state never triggering
**Cause:** `e.detail.elt.dataset.id` in `htmx:afterSwap` was unreliable.  
**Fix:** ID now read from `document.querySelector('#modal-container .modal').dataset.articleId` — always present from the modal template.

---

## File Structure

```
article-viewer/
├── main.go                          # Entry point, routing, embed, DB init
├── db.go                            # All DB logic, FTS, read/favorite tables
├── db_test.go                       # DB layer tests (in-memory SQLite)
├── handlers.go                      # HTTP handlers, PageData, CardsData structs
├── handlers_test.go                 # Handler tests
├── go.mod / go.sum
├── templates/
│   ├── index.html                   # Full page shell, filter panel
│   └── partials/
│       ├── cards.html               # Card grid + infinite scroll sentinel
│       └── modal.html               # Article reading modal
├── static/
│   ├── style.css                    # All styles (dark theme)
│   ├── app.js                       # All client JS (vanilla)
│   └── htmx.min.js                  # HTMX 1.9.12 (local copy)
├── docs/
│   ├── PROJECT-STATUS.md            # This file
│   └── VPS-DEPLOYMENT.md            # Full deployment + troubleshooting guide
└── deploy/
    ├── newsdesk.service             # systemd unit
    └── nginx-article-viewer.conf    # nginx reverse proxy config
```

---

## VPS Deployment

**Directory:** `/opt/newsdesk/`  
**Binary:** `news-desk`  
**Service:** `newsdesk`  
**Port:** `127.0.0.1:8181` (nginx proxies 80/443)

### Update workflow
```bash
# Local machine
rsync -av \
  --exclude='articles.db' --exclude='articles.db-wal' --exclude='articles.db-shm' \
  --exclude='article-viewer' \
  /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer/ \
  rick@hz-hel:/opt/newsdesk/

# VPS
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
sudo systemctl restart newsdesk
```

Full guide: `docs/VPS-DEPLOYMENT.md`

---

## Tests

```bash
go test -tags fts5 ./...
```

17 tests across `db_test.go` and `handlers_test.go`. All passing.

---

## Possible Next Steps (not started)

- Scraper fix: store `publish_date` as ISO `YYYY-MM-DD` for all sources
- Tag click → filter by tag
- Keyboard navigation (j/k between cards, Enter to open)
- Article sharing / export
- Per-site unread counts in the site pill labels
