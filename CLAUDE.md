# Article Viewer — CLAUDE.md

A read-only Go web app that serves articles from a SQLite database populated by a separate scraper (`scrape-adv`). No Node.js. No build pipeline. Single self-contained binary.

---

## Critical: Build Tag

**Every `go build`, `go run`, and `go test` command must include `-tags fts5`.**

```bash
go build -tags fts5 -o article-viewer .
go run   -tags fts5 . -db articles.db -addr :9090
go test  -tags fts5 ./...
```

Without `-tags fts5`, `mattn/go-sqlite3` compiles SQLite without FTS5 support and all full-text search queries fail at runtime with no compile-time warning.

---

## Running Locally

```bash
go run -tags fts5 . -db articles.db -addr :9090
```

Flags:
- `-db`   path to SQLite database (default `articles.db`)
- `-addr` listen address (default `:8080`)

Hard-refresh the browser (`Ctrl+Shift+R`) after CSS/JS changes — the binary embeds static files so they are fresh on each restart, but browsers cache aggressively.

---

## Project Structure

```
article-viewer/
├── main.go                          # Entry point, routing, embed directive
├── db.go                            # All database logic
├── db_test.go                       # DB layer tests
├── handlers.go                      # HTTP handlers, template data structs
├── handlers_test.go                 # Handler tests
├── go.mod                           # module article-viewer, go 1.21
├── templates/
│   ├── index.html                   # Full page shell
│   └── partials/
│       ├── cards.html               # Feed card grid + infinite scroll sentinel
│       └── modal.html               # Article reading modal
├── static/
│   ├── style.css                    # All styles (dark theme, no framework)
│   └── app.js                       # All client JS (vanilla, no framework)
└── deploy/
    ├── article-viewer.service       # systemd unit
    └── nginx-article-viewer.conf    # nginx reverse proxy config
```

---

## Architecture

### Backend
- **`net/http`** standard library only — no router framework
- **`html/template`** for server-side rendering
- **`embed.FS`** — `templates/` and `static/` are compiled into the binary at build time via `//go:embed templates static` in `main.go`. No files need to be present at runtime.
- **`mattn/go-sqlite3`** — CGO driver. Requires `gcc` to build. Bundles SQLite source (no system SQLite dependency needed).

### Frontend
- **HTMX 1.9.12** (loaded from CDN) — handles live search, filter pills, infinite scroll, modal loading. No page reloads.
- **Vanilla JS** (`static/app.js`) — filter panel toggle, compact view, pill logic, font/size preferences, clipboard copy.
- **No JavaScript framework, no build step.**

### Routes
| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| GET | `/` | `handleIndex` | Full page. Accepts same query params as `/articles`. |
| GET | `/articles` | `handleArticles` | Returns `cards` partial. Used by HTMX for all feed updates. |
| GET | `/article/{id}` | `handleArticle` | Returns `modal` partial. Loaded into modal container by HTMX. |
| GET | `/static/` | `http.FileServer` | Serves embedded static assets. |

---

## Database

### Source
The database is written by a separate scraper (`scrape-adv`) using Playwright. The viewer is **read-only** — it never writes to `articles`.

### Schema (articles table — created by scraper, not this app)
```sql
CREATE TABLE articles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    url          TEXT,
    fingerprint  TEXT UNIQUE,
    title        TEXT,
    author       TEXT,
    publish_date TEXT,   -- stored as YYYY-MM-DD text; date comparisons rely on ISO sort order
    tags         TEXT,   -- comma-separated
    content      TEXT,   -- plain text, paragraphs separated by \n\n
    scraped_at   DATETIME,
    category     TEXT,   -- underscore-encoded hierarchy, e.g. "news_japan_history"
    site         TEXT    -- e.g. "Japan Times", "The Guardian"
)
```

### FTS5 Index
`InitFTS()` creates a **standalone** (not content-table) FTS5 virtual table on first startup:
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, body, tags)
```
- Column `body` maps to `articles.content` (FTS5 column name differs from table column name).
- Populated once from the `articles` table if empty. **New articles scraped after startup are not searchable until the server is restarted.**
- FTS5 column filter syntax used for scoped search: `title:term*`, `{title tags}:term*`, etc.
- All search queries append `*` for prefix matching.

### Key DB Functions
| Function | Description |
|----------|-------------|
| `OpenDB(path)` | Opens SQLite with `busy_timeout=5000`, WAL mode, `MaxOpenConns=1` |
| `InitFTS()` | Creates and populates FTS5 index on first run |
| `GetSites()` | `SELECT DISTINCT site` ordered alphabetically |
| `GetCategoryInfos()` | Returns `[]CategoryInfo{Name, Sites}` — `Sites` is `GROUP_CONCAT(DISTINCT site)` for dynamic pill filtering |
| `QueryArticles(p)` | Full filtered query with FTS or plain SQL path, supports all filter params |
| `CountArticles(p)` | Same filter logic as `QueryArticles` but returns `COUNT(*)` — used for result count display |
| `BuildCategoryTree([]CategoryInfo)` | Pure function; converts flat category names into `[]CategoryGroup` tree for display |

---

## Category Hierarchy

Categories use `_` as a hierarchy separator. `BuildCategoryTree` (in `db.go`) parses them using `strings.SplitN(name, "_", 3)`:

| Raw name | Group | SubGroup | Pill label |
|---|---|---|---|
| `business_economy` | Business | — | economy |
| `business_tech` | Business | — | tech |
| `news_japan_history` | News | Japan | history |
| `news_japan_science-health` | News | Japan | science-health |
| `life_lifestyle` | Life | — | lifestyle |

The full original name (e.g. `news_japan_history`) is always used as the `data-value` attribute and DB filter value. Display labels are derived portions only.

---

## Data Structures

### QueryParams (db.go)
```go
type QueryParams struct {
    Q        string
    Site     string
    Category string   // full original name e.g. "news_japan_history"
    Author   string   // LIKE %author%
    DateFrom string   // YYYY-MM-DD, compared with >= against publish_date
    DateTo   string   // YYYY-MM-DD, compared with <=
    Fields   []string // "title","body","tags" — nil/empty means search all columns
    Offset   int
    Limit    int      // defaults to 20 if <= 0
}
```

### PageData (handlers.go)
Passed to `index.html` on full page renders only.
```go
type PageData struct {
    Sites          []string
    CategoryGroups []CategoryGroup   // pre-built hierarchy tree
    Q, Author, DateFrom, DateTo string
    SearchTitle, SearchBody, SearchTags bool  // checkbox initial state
    SelectedSite, SelectedCat string
    Cards CardsData
}
```

### CardsData (handlers.go)
Passed to the `cards` partial for both initial render and HTMX responses.
```go
type CardsData struct {
    Articles   []Article
    TotalCount int       // total matching rows (ignores LIMIT/OFFSET)
    Q, Author, DateFrom, DateTo string
    Fields     []string
    Site, Category string
    NextOffset int
    HasMore    bool
}
```

---

## Template System

Templates are parsed from `embed.FS` in production (`mustParseTemplatesFS`) and from disk in tests (`mustParseTemplates`).

### Template functions (defined in `buildFuncMap`)
| Function | Signature | Purpose |
|----------|-----------|---------|
| `excerpt` | `(string) string` | First paragraph or 200 chars of content |
| `splitParagraphs` | `(string) []string` | Splits `\n\n`-separated content into paragraphs |
| `splitTags` | `(string) []string` | Splits `", "`-separated tag string |
| `buildQuery` | `(CardsData) template.URL` | Builds `/articles?...` URL for infinite scroll sentinel; returns `template.URL` to prevent double-encoding |
| `fmtCount` | `(int) string` | Formats integer with comma separators (e.g. `4386` → `"4,386"`) |

### HTMX patterns
- **Live search**: `hx-trigger="keyup changed delay:300ms, search"` on `#search-input`; `hx-include="#filter-panel [name]"` picks up all filter state.
- **Infinite scroll**: `scroll-sentinel` div with `hx-trigger="intersect once"` and `hx-swap="outerHTML"` replaces itself with the next batch.
- **Modal**: cards have `hx-get="/article/{id}" hx-target="#modal-container" hx-swap="innerHTML"`; JS listens for `htmx:afterSwap` to open the overlay.
- **Result count OOB**: `cards.html` includes `<div id="result-count" hx-swap-oob="true">` — HTMX extracts this and updates the count element outside `#feed`. A CSS rule `#feed .result-count { display: none }` hides the duplicate that appears in the initial inline render.
- **Browser history**: `handleArticles` sets `HX-Push-Url` response header (offset=0 only) so the URL reflects current filters.

---

## Filter Panel (Frontend)

The filter panel (`#filter-panel`) is hidden by default and toggled by the `☰` button (`#filter-btn`). All filter inputs live inside the panel so `hx-include="#filter-panel [name]"` on the search input captures everything.

### Filter inputs inside `#filter-panel`
| Element | `name` attribute | Type |
|---------|-----------------|------|
| `#site-filter` | `site` | hidden |
| `#cat-filter` | `category` | hidden |
| `#date-from` | `date_from` | date |
| `#date-to` | `date_to` | date |
| `#author-input` | `author` | text |
| Search-in checkboxes | `fields` | checkbox (multi-value) |

### `fields` param behaviour
- `fields` absent (or all three checked) → FTS searches all columns.
- `fields=title` only → `title:term*` FTS query.
- `fields=title&fields=tags` → `{title tags}:term*`.
- All three checked sends all three values; server treats len==3 same as len==0 (all).

### `fireFeedRefresh()` (app.js)
Central function called by all filter controls (pills, checkboxes, date/author inputs). Reads current state from `#search-input` and all `#filter-panel [name]` elements, builds a URL, and fires `htmx.ajax('GET', ...)`.

### Category pill filtering
`filterCategoryPills(selectedSite)` runs after every site pill toggle:
1. Hides individual `.pill` buttons whose `data-sites` doesn't include the selected site.
2. Hides `.cat-subgroup` divs where all pills are hidden.
3. Hides `.cat-group` divs where all pills are hidden (removes the group label too).
4. Clears the active category if its pill was hidden.

### Date range UI
Two-state design: shows "All dates" pill by default; click reveals date inputs. `clearDates()` resets inputs and collapses back to "All dates". Activated automatically on page load if `date_from`/`date_to` are in the URL.

---

## Client-Side State (localStorage)

| Key | Values | Purpose |
|-----|--------|---------|
| `av-font` | `serif` / `sans` / `mono` | Modal reading font |
| `av-fontSize` | `"12"`–`"28"` | Modal font size in px |
| `av-compact` | `"1"` / `""` | Compact card view on/off |

Preferences are applied in `applyPrefs()` (called after modal opens) and `applyCompactPref()` (called on DOMContentLoaded).

---

## View Modes

The `⛇` button (`#view-btn`) in the top bar toggles `.compact` on `#feed`:

- **Normal**: `minmax(280px, 1fr)` grid, full card with excerpt + author.
- **Compact**: `minmax(220px, 1fr)` grid, card shows only meta + title (excerpt and author hidden via CSS). Fits ~8 columns on a wide screen without feeling squashed.

---

## Tests

```bash
go test -tags fts5 ./...
```

**17 tests** across two files:
- `db_test.go` — uses an in-memory SQLite DB seeded with 3 fixture articles across 2 sites and 3 categories. Tests `GetSites`, `GetCategoryInfos`, `QueryArticles` (no filter, site filter, search, offset, combined filter), `GetArticleByID` (found and not-found).
- `handlers_test.go` — uses `mustParseTemplates()` (disk parse, not embed). Tests `handleIndex` (200 and 404), `handleArticles` (no filter, search, site filter), `handleArticle` (found, not-found, bad ID).

`mustParseTemplates()` (disk) is test-only. `mustParseTemplatesFS(embed.FS)` is production. Both use the same `buildFuncMap()`.

---

## Deployment (Hetzner VPS)

**Deployment directory:** `/opt/newsdesk/`
**Binary name:** `news-desk`
**Service name:** `newsdesk`

### 1. Install dependencies on the VPS

```bash
sudo apt update
sudo apt install -y git gcc golang-go
```

If you need a newer Go than the distro provides (recommended — get 1.21+):

```bash
wget https://go.dev/dl/go1.21.13.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.13.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

`gcc` is the only non-Go dependency — `mattn/go-sqlite3` bundles SQLite source and compiles it itself.

### 2. Copy source to the VPS

From your local machine:

```bash
rsync -av --exclude='articles.db' \
  /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer/ \
  rick@hz-hel:/opt/newsdesk/
```

Then copy the database separately:

```bash
scp articles.db rick@hz-hel:/opt/newsdesk/articles.db
```

### 3. Build the binary on the VPS

```bash
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
```

This takes a minute the first time (CGO + SQLite compilation). Subsequent builds are faster.

### 4. Set correct ownership

```bash
sudo chown -R www-data:www-data /opt/newsdesk
sudo chmod 755 /opt/newsdesk/news-desk
```

### 5. Install the systemd service

Edit the service file first if needed (check `ExecStart` points to correct DB path):

```bash
sudo nvim /opt/newsdesk/deploy/newsdesk.service
```

Then install and start it:

```bash
sudo cp /opt/newsdesk/deploy/newsdesk.service /etc/systemd/system/newsdesk.service
sudo systemctl daemon-reload
sudo systemctl enable newsdesk
sudo systemctl start newsdesk
sudo systemctl status newsdesk
```

### Service management

```bash
sudo systemctl status newsdesk
sudo systemctl restart newsdesk
sudo systemctl stop newsdesk
sudo journalctl -u newsdesk -f
```

### 6. Configure nginx

Edit the nginx config to set your real domain:

```bash
nano /opt/newsdesk/deploy/nginx-article-viewer.conf
# Change: server_name articles.yourdomain.com;
```

Install it:

```bash
sudo cp /opt/newsdesk/deploy/nginx-article-viewer.conf /etc/nginx/sites-available/newsdesk
sudo ln -s /etc/nginx/sites-available/newsdesk /etc/nginx/sites-enabled/newsdesk
sudo nginx -t
sudo systemctl reload nginx
```

### 7. HTTPS (Let's Encrypt)

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d articles.yourdomain.com
```

Certbot automatically updates the nginx config and sets up auto-renewal.

### Update workflow

```bash
# From local machine — sync source, skip the database
rsync -av --exclude='articles.db' \
  /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer/ \
  rick@hz-hel:/opt/newsdesk/

# On VPS
cd /opt/newsdesk
go build -tags fts5 -o news-desk .
sudo systemctl restart newsdesk
```

The binary embeds all templates and static files — only `news-desk` and `articles.db` are needed at runtime.

### File locations on VPS

| File | Path |
|------|------|
| Binary | `/opt/newsdesk/news-desk` |
| Database | `/opt/newsdesk/articles.db` |
| systemd unit | `/etc/systemd/system/newsdesk.service` |
| nginx config | `/etc/nginx/sites-enabled/newsdesk` |

Server listens on `127.0.0.1:8181`. nginx proxies from port 80/443.

---

## Common Gotchas

- **FTS5 index is static.** New articles scraped after server start are not searchable. Restart to rebuild the index.
- **WAL mode with `MaxOpenConns=1`.** The DB is read-only in practice. One connection prevents WAL locking issues.
- **`buildQuery` returns `template.URL`**, not `string`. This prevents `html/template` from double-escaping the URL in `hx-get` attributes. Do not change the return type.
- **`data-value` on pill buttons, not inline onclick args.** Site/category names can contain apostrophes or special characters. JS reads `el.dataset.value`, never a string argument passed through onclick.
- **`GROUP_CONCAT(DISTINCT site)` in `GetCategoryInfos`.** The `DISTINCT` keyword in aggregate functions requires SQLite 3.x — available on any modern system.
- **`publish_date` is a TEXT column** in ISO `YYYY-MM-DD` format. Date range comparisons (`>=`, `<=`) work correctly because ISO dates sort lexicographically. Non-ISO formats in the DB will break date filtering silently.
- **OOB result count.** `cards.html` outputs `<div id="result-count" hx-swap-oob="true">` — HTMX strips this from the main response and updates the element in `<main>`. The duplicate that lands inside `#feed` during initial inline render is hidden by `#feed .result-count { display: none }` in CSS. Do not remove that CSS rule.
- **Template parse errors are fatal.** `mustParseTemplatesFS` panics on any template error. Syntax errors in templates will prevent the server from starting.
- **`fields` checkbox param**: when all three are unchecked, no `fields` params are sent — server treats this as "search all" (same as all checked). This is intentional.
