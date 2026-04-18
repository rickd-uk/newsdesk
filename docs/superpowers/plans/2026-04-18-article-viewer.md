# Article Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go + HTMX web app that serves articles from `articles.db` with full-text search, site/category filters, infinite scroll, and a modal reader with font selection and copy-to-clipboard.

**Architecture:** Single Go binary using `net/http` + `html/template` with all assets embedded via `embed.FS`. HTMX handles infinite scroll, live search, and modal loading with near-zero custom JS. SQLite FTS5 powers full-text search.

**Tech Stack:** Go stdlib, `mattn/go-sqlite3` (CGO), HTMX 1.9.x (CDN), vanilla JS (~80 lines), CSS (dark theme, responsive)

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | Flags, embed declarations, DB init, FTS5 setup, HTTP server wiring |
| `db.go` | `Article` struct, `DB` type, `OpenDB`, `InitFTS`, `GetSites`, `GetCategories`, `QueryArticles`, `GetArticleByID` |
| `handlers.go` | `Server` type, `handleIndex`, `handleArticles`, `handleArticle`, template data structs, template funcs |
| `db_test.go` | DB layer tests using in-memory SQLite |
| `handlers_test.go` | HTTP handler tests using `httptest` |
| `templates/index.html` | Full HTML shell — top bar, filter form, feed container, modal overlay |
| `templates/cards.html` | `{{define "cards"}}` — article card grid + infinite scroll sentinel |
| `templates/modal.html` | `{{define "modal"}}` — article reader with toolbar |
| `static/style.css` | Dark theme, grid layout, pills, modal, responsive |
| `static/app.js` | Font/size picker, localStorage, copy button, modal open/close, pill toggle, HTMX event hooks |

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `templates/index.html` (empty placeholder)
- Create: `templates/cards.html` (empty placeholder)
- Create: `templates/modal.html` (empty placeholder)
- Create: `static/style.css` (empty placeholder)
- Create: `static/app.js` (empty placeholder)

- [ ] **Step 1: Initialise Go module**

```bash
cd /home/rick/Documents/D/WEB/_LIVE_/TESTING/article-viewer
go mod init article-viewer
```

Expected: `go.mod` created with `module article-viewer` and `go 1.21` (or current version).

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/mattn/go-sqlite3
```

Expected: `go.sum` created, `go.mod` updated with `require github.com/mattn/go-sqlite3`.

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p templates static
```

- [ ] **Step 4: Create placeholder files so `embed.FS` compiles**

Create `templates/index.html`:
```html
<!DOCTYPE html><html><body>placeholder</body></html>
```

Create `templates/cards.html`:
```html
{{define "cards"}}{{end}}
```

Create `templates/modal.html`:
```html
{{define "modal"}}{{end}}
```

Create `static/style.css`:
```css
/* placeholder */
```

Create `static/app.js`:
```javascript
// placeholder
```

- [ ] **Step 5: Create `.gitignore`**

```
articles.db
user_data/
*.db-shm
*.db-wal
article-viewer
.superpowers/
```

- [ ] **Step 6: Commit**

```bash
git init
git add go.mod go.sum .gitignore templates/ static/
git commit -m "feat: scaffold article-viewer Go module"
```

---

## Task 2: Database Layer (TDD)

**Files:**
- Create: `db.go`
- Create: `db_test.go`

- [ ] **Step 1: Write the failing tests**

Create `db_test.go`:
```go
package main

import (
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT, fingerprint TEXT UNIQUE,
		title TEXT, author TEXT, publish_date TEXT,
		tags TEXT, content TEXT, scraped_at DATETIME,
		category TEXT, site TEXT
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO articles
		(site, url, fingerprint, title, author, publish_date, tags, content, category) VALUES
		('Japan Times','http://ex.com/1','fp1','Article One','Auth A','2026-04-01','tech,science','First content paragraph.\n\nSecond paragraph.','tech'),
		('The Guardian','http://ex.com/2','fp2','Article Two','','2026-04-02','','Guardian content here','uk-news'),
		('Japan Times','http://ex.com/3','fp3','Article Three','Auth B','2026-03-15','','Old article content','health')`)
	if err != nil {
		t.Fatalf("insert fixtures: %v", err)
	}
	if err := db.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}
	return db
}

func TestGetSites(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	sites, err := db.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("want 2 sites, got %d: %v", len(sites), sites)
	}
}

func TestGetCategories(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	cats, err := db.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("want 3 categories, got %d: %v", len(cats), cats)
	}
}

func TestQueryArticles_NoFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("want 3 articles, got %d", len(arts))
	}
	// Newest first
	if arts[0].PublishDate != "2026-04-02" {
		t.Fatalf("want newest first, got %s", arts[0].PublishDate)
	}
}

func TestQueryArticles_SiteFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Site: "Japan Times", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("want 2 Japan Times articles, got %d", len(arts))
	}
}

func TestQueryArticles_Search(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Q: "Guardian", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles search: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 result for 'Guardian', got %d", len(arts))
	}
}

func TestQueryArticles_Offset(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Offset: 2, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article at offset 2, got %d", len(arts))
	}
}

func TestGetArticleByID(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	a, err := db.GetArticleByID(1)
	if err != nil {
		t.Fatalf("GetArticleByID: %v", err)
	}
	if a.Title != "Article One" {
		t.Fatalf("want 'Article One', got %q", a.Title)
	}
}

func TestGetArticleByID_NotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	_, err := db.GetArticleByID(9999)
	if err == nil {
		t.Fatal("want error for missing ID, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run TestGetSites
```

Expected: `cannot find package` or `undefined: OpenDB` — confirms tests are wired but implementation is missing.

- [ ] **Step 3: Implement `db.go`**

Create `db.go`:
```go
package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Article struct {
	ID          int
	Site        string
	URL         string
	Category    string
	Title       string
	Author      string
	PublishDate string
	Tags        string
	Content     string
	ScrapedAt   string
}

type QueryParams struct {
	Q        string
	Site     string
	Category string
	Offset   int
	Limit    int
}

type DB struct {
	*sql.DB
}

func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL;")
	return &DB{db}, nil
}

func (db *DB) InitFTS() error {
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts
		USING fts5(title, content, tags, content=articles, content_rowid=id)`)
	if err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM articles_fts").Scan(&count)
	if count == 0 {
		_, err = db.Exec(`INSERT INTO articles_fts(rowid, title, content, tags)
			SELECT id, COALESCE(title,''), COALESCE(content,''), COALESCE(tags,'')
			FROM articles`)
		if err != nil {
			return fmt.Errorf("populate fts: %w", err)
		}
	}
	return nil
}

func (db *DB) GetSites() ([]string, error) {
	rows, err := db.Query(
		"SELECT DISTINCT site FROM articles WHERE site != '' ORDER BY site")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out, nil
}

func (db *DB) GetCategories() ([]string, error) {
	rows, err := db.Query(
		"SELECT DISTINCT category FROM articles WHERE category != '' ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out = append(out, c)
	}
	return out, nil
}

func (db *DB) QueryArticles(p QueryParams) ([]Article, error) {
	cols := `COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
		COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
		COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
		COALESCE(a.scraped_at,'')`

	var (
		rows *sql.Rows
		err  error
	)

	if p.Q != "" {
		q := `SELECT ` + cols + `
			FROM articles_fts f
			JOIN articles a ON a.id = f.rowid
			WHERE articles_fts MATCH ?`
		args := []interface{}{strings.TrimSpace(p.Q) + "*"}
		if p.Site != "" {
			q += " AND a.site = ?"
			args = append(args, p.Site)
		}
		if p.Category != "" {
			q += " AND a.category = ?"
			args = append(args, p.Category)
		}
		q += " ORDER BY a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
		args = append(args, p.Limit, p.Offset)
		rows, err = db.Query(q, args...)
	} else {
		q := `SELECT ` + strings.ReplaceAll(cols, "a.", "") + `
			FROM articles a WHERE 1=1`
		// Rewrite cols without alias for non-join query
		q = `SELECT COALESCE(id,0), COALESCE(site,''), COALESCE(url,''),
			COALESCE(category,''), COALESCE(title,''), COALESCE(author,''),
			COALESCE(publish_date,''), COALESCE(tags,''), COALESCE(content,''),
			COALESCE(scraped_at,'') FROM articles WHERE 1=1`
		args := []interface{}{}
		if p.Site != "" {
			q += " AND site = ?"
			args = append(args, p.Site)
		}
		if p.Category != "" {
			q += " AND category = ?"
			args = append(args, p.Category)
		}
		q += " ORDER BY publish_date DESC, scraped_at DESC LIMIT ? OFFSET ?"
		args = append(args, p.Limit, p.Offset)
		rows, err = db.Query(q, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
			&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) GetArticleByID(id int) (*Article, error) {
	row := db.QueryRow(`SELECT COALESCE(id,0), COALESCE(site,''), COALESCE(url,''),
		COALESCE(category,''), COALESCE(title,''), COALESCE(author,''),
		COALESCE(publish_date,''), COALESCE(tags,''), COALESCE(content,''),
		COALESCE(scraped_at,'') FROM articles WHERE id = ?`, id)
	var a Article
	if err := row.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
		&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./... -v -run "TestGet|TestQuery"
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add db.go db_test.go
git commit -m "feat: add database layer with FTS5 search"
```

---

## Task 3: Handlers + main.go (TDD)

**Files:**
- Create: `handlers.go`
- Create: `main.go`
- Create: `handlers_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `handlers_test.go`:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	db := testDB(t)
	tmpl := mustParseTemplates()
	return &Server{db: db, tmpl: tmpl}
}

func TestHandleIndex(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Article Viewer") {
		t.Error("want 'Article Viewer' in response body")
	}
	if !strings.Contains(body, "Japan Times") {
		t.Error("want site pill 'Japan Times' in response body")
	}
}

func TestHandleIndex_404(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleIndex(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleArticles_NoFilter(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/articles", nil)
	w := httptest.NewRecorder()
	srv.handleArticles(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Article One") {
		t.Error("want 'Article One' in cards response")
	}
}

func TestHandleArticles_Search(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/articles?q=Guardian", nil)
	w := httptest.NewRecorder()
	srv.handleArticles(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Article Two") {
		t.Error("want 'Article Two' in search results for 'Guardian'")
	}
	if strings.Contains(body, "Article One") {
		t.Error("want 'Article One' excluded from 'Guardian' search")
	}
}

func TestHandleArticles_SiteFilter(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/articles?site=Japan+Times", nil)
	w := httptest.NewRecorder()
	srv.handleArticles(w, req)
	body := w.Body.String()
	if strings.Contains(body, "Article Two") {
		t.Error("Guardian article should not appear in Japan Times filter")
	}
}

func TestHandleArticle(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/article/1", nil)
	w := httptest.NewRecorder()
	srv.handleArticle(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Article One") {
		t.Error("want 'Article One' in modal response")
	}
}

func TestHandleArticle_NotFound(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/article/9999", nil)
	w := httptest.NewRecorder()
	srv.handleArticle(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleArticle_BadID(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/article/abc", nil)
	w := httptest.NewRecorder()
	srv.handleArticle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./... -run "TestHandle"
```

Expected: compile error — `Server`, `mustParseTemplates` undefined.

- [ ] **Step 3: Implement `handlers.go`**

Create `handlers.go`:
```go
package main

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const pageSize = 20

type Server struct {
	db   *DB
	tmpl *template.Template
}

type PageData struct {
	Sites       []string
	Categories  []string
	Q           string
	SelectedSite string
	SelectedCat  string
	Cards       CardsData
}

type CardsData struct {
	Articles   []Article
	Q          string
	Site       string
	Category   string
	NextOffset int
	HasMore    bool
}

func mustParseTemplates() *template.Template {
	funcMap := template.FuncMap{
		"excerpt": func(s string) string {
			// First paragraph only, max 200 chars
			if idx := strings.Index(s, "\n\n"); idx > 0 && idx < 300 {
				s = s[:idx]
			}
			s = strings.TrimSpace(s)
			if len(s) > 200 {
				if idx := strings.LastIndex(s[:200], " "); idx > 0 {
					return s[:idx] + "…"
				}
				return s[:200] + "…"
			}
			return s
		},
		"splitParagraphs": func(s string) []string {
			parts := strings.Split(s, "\n\n")
			var out []string
			for _, p := range parts {
				p = strings.TrimSpace(strings.ReplaceAll(p, "\n", " "))
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		},
		"splitTags": func(s string) []string {
			parts := strings.Split(s, ", ")
			var out []string
			for _, t := range parts {
				t = strings.TrimSpace(t)
				if t != "" {
					out = append(out, t)
				}
			}
			return out
		},
		"buildQuery": func(q, site, category string, offset int) string {
			params := url.Values{}
			if q != "" {
				params.Set("q", q)
			}
			if site != "" {
				params.Set("site", site)
			}
			if category != "" {
				params.Set("category", category)
			}
			params.Set("offset", strconv.Itoa(offset))
			return params.Encode()
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		panic("parse templates: " + err.Error())
	}
	tmpl, err = tmpl.ParseGlob("templates/partials/*.html")
	if err != nil {
		panic("parse partials: " + err.Error())
	}
	return tmpl
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")

	sites, _ := s.db.GetSites()
	cats, _ := s.db.GetCategories()
	articles, _ := s.db.QueryArticles(QueryParams{
		Q: q, Site: site, Category: cat, Offset: 0, Limit: pageSize,
	})

	data := PageData{
		Sites:        sites,
		Categories:   cats,
		Q:            q,
		SelectedSite: site,
		SelectedCat:  cat,
		Cards: CardsData{
			Articles:   articles,
			Q:          q,
			Site:       site,
			Category:   cat,
			NextOffset: pageSize,
			HasMore:    len(articles) == pageSize,
		},
	}
	s.tmpl.ExecuteTemplate(w, "index.html", data)
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	articles, _ := s.db.QueryArticles(QueryParams{
		Q: q, Site: site, Category: cat, Offset: offset, Limit: pageSize,
	})

	// Push URL so browser history reflects current filters (offset=0 only)
	if offset == 0 {
		params := url.Values{}
		if q != "" {
			params.Set("q", q)
		}
		if site != "" {
			params.Set("site", site)
		}
		if cat != "" {
			params.Set("category", cat)
		}
		pushURL := "/"
		if len(params) > 0 {
			pushURL += "?" + params.Encode()
		}
		w.Header().Set("HX-Push-Url", pushURL)
	}

	data := CardsData{
		Articles:   articles,
		Q:          q,
		Site:       site,
		Category:   cat,
		NextOffset: offset + pageSize,
		HasMore:    len(articles) == pageSize,
	}
	s.tmpl.ExecuteTemplate(w, "cards", data)
}

func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/article/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	article, err := s.db.GetArticleByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.tmpl.ExecuteTemplate(w, "modal", article)
}
```

- [ ] **Step 4: Implement `main.go`**

Create `main.go`:
```go
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

//go:embed templates static
var embeddedFiles embed.FS

func main() {
	dbPath := flag.String("db", "articles.db", "Path to SQLite database")
	addr := flag.String("addr", ":8080", "Address to listen on")
	flag.Parse()

	db, err := OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.InitFTS(); err != nil {
		log.Fatalf("init fts: %v", err)
	}

	funcMap := template.FuncMap{} // populated in mustParseTemplatesFS
	tmpl, err := mustParseTemplatesFS(embeddedFiles, funcMap)
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	srv := &Server{db: db, tmpl: tmpl}

	staticFS, _ := fs.Sub(embeddedFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/articles", srv.handleArticles)
	http.HandleFunc("/article/", srv.handleArticle)
	http.HandleFunc("/", srv.handleIndex)

	fmt.Printf("article-viewer listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
```

**Note:** `mustParseTemplates` in handlers.go uses `ParseGlob` (for tests with real files). For production, `main.go` uses `embed.FS`. Refactor `handlers.go` to expose `buildFuncMap()` and replace `mustParseTemplates` with `mustParseTemplatesFS`:

Add to `handlers.go` (replace `mustParseTemplates`):
```go
func buildFuncMap() template.FuncMap {
	return template.FuncMap{
		"excerpt": func(s string) string {
			if idx := strings.Index(s, "\n\n"); idx > 0 && idx < 300 {
				s = s[:idx]
			}
			s = strings.TrimSpace(s)
			if len(s) > 200 {
				if idx := strings.LastIndex(s[:200], " "); idx > 0 {
					return s[:idx] + "…"
				}
				return s[:200] + "…"
			}
			return s
		},
		"splitParagraphs": func(s string) []string {
			parts := strings.Split(s, "\n\n")
			var out []string
			for _, p := range parts {
				p = strings.TrimSpace(strings.ReplaceAll(p, "\n", " "))
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		},
		"splitTags": func(s string) []string {
			parts := strings.Split(s, ", ")
			var out []string
			for _, t := range parts {
				t = strings.TrimSpace(t)
				if t != "" {
					out = append(out, t)
				}
			}
			return out
		},
		"buildQuery": func(q, site, category string, offset int) string {
			params := url.Values{}
			if q != "" {
				params.Set("q", q)
			}
			if site != "" {
				params.Set("site", site)
			}
			if category != "" {
				params.Set("category", category)
			}
			params.Set("offset", strconv.Itoa(offset))
			return params.Encode()
		},
	}
}

func mustParseTemplates() *template.Template {
	tmpl, err := template.New("").Funcs(buildFuncMap()).ParseGlob("templates/*.html")
	if err != nil {
		panic("parse templates: " + err.Error())
	}
	tmpl, err = tmpl.ParseGlob("templates/partials/*.html")
	if err != nil {
		panic("parse partials: " + err.Error())
	}
	return tmpl
}

func mustParseTemplatesFS(fsys embed.FS, _ template.FuncMap) (*template.Template, error) {
	tmpl, err := template.New("").Funcs(buildFuncMap()).ParseFS(fsys, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}
```

Update `main.go` import to drop the unused `funcMap` variable:
```go
tmpl, err := mustParseTemplatesFS(embeddedFiles, nil)
```

And update the signature: `func mustParseTemplatesFS(fsys embed.FS, _ template.FuncMap) (*template.Template, error)` — the second param can be removed entirely for cleanliness:
```go
func mustParseTemplatesFS(fsys embed.FS) (*template.Template, error) {
	return template.New("").Funcs(buildFuncMap()).ParseFS(fsys, "templates/*.html", "templates/partials/*.html")
}
```

And in `main.go`:
```go
tmpl, err := mustParseTemplatesFS(embeddedFiles)
```

- [ ] **Step 5: Run tests**

```bash
go test ./... -v -run "TestHandle"
```

Expected: All handler tests PASS. (Templates are placeholder files — responses may be empty but status codes are correct.)

- [ ] **Step 6: Verify it compiles**

```bash
go build -o article-viewer .
```

Expected: binary produced with no errors.

- [ ] **Step 7: Commit**

```bash
git add handlers.go handlers_test.go main.go
git commit -m "feat: add HTTP handlers and main server wiring"
```

---

## Task 4: HTML Templates

**Files:**
- Modify: `templates/index.html`
- Create: `templates/partials/cards.html` (move from placeholder location)
- Create: `templates/partials/modal.html` (move from placeholder location)

**Note:** Move `cards.html` and `modal.html` from `templates/` to `templates/partials/` and delete the originals. Update `mustParseTemplates` glob if needed (it already parses both globs).

- [ ] **Step 1: Create `templates/partials/` directory and move partials**

```bash
mkdir -p templates/partials
```

Delete `templates/cards.html` and `templates/modal.html` (they're placeholder files — replace with the real versions below).

- [ ] **Step 2: Write `templates/partials/cards.html`**

```html
{{define "cards"}}
{{range .Articles}}
<div class="card"
     hx-get="/article/{{.ID}}"
     hx-target="#modal-container"
     hx-swap="innerHTML">
    <div class="card-meta">
        {{if .Site}}<span class="site">{{.Site}}</span>{{end}}
        {{if .Category}}<span class="category">{{.Category}}</span>{{end}}
        {{if .PublishDate}}<span class="date">{{.PublishDate}}</span>{{end}}
    </div>
    <h3 class="card-title">{{.Title}}</h3>
    {{if .Author}}<div class="card-author">{{.Author}}</div>{{end}}
    <p class="card-excerpt">{{excerpt .Content}}</p>
</div>
{{end}}
{{if .HasMore}}
<div class="scroll-sentinel"
     hx-get="/articles?{{buildQuery .Q .Site .Category .NextOffset}}"
     hx-trigger="intersect once"
     hx-swap="outerHTML"
     hx-target="this"></div>
{{end}}
{{end}}
```

- [ ] **Step 3: Write `templates/partials/modal.html`**

```html
{{define "modal"}}
<div class="modal">
    <div class="modal-header">
        <div class="modal-title-group">
            <h2 class="modal-title">{{.Title}}</h2>
            <div class="modal-meta">
                {{if .Site}}<span class="badge site">{{.Site}}</span>{{end}}
                {{if .Category}}<span class="badge category">{{.Category}}</span>{{end}}
                {{if .PublishDate}}<span class="date">{{.PublishDate}}</span>{{end}}
                {{if .Author}}<span class="author">by {{.Author}}</span>{{end}}
            </div>
        </div>
        <button class="close-btn" onclick="dismissModal()">✕</button>
    </div>
    <div class="modal-toolbar">
        <div class="font-picker">
            <span class="label">Font</span>
            <button class="font-btn" data-font="serif" onclick="setFont('serif')">Serif</button>
            <button class="font-btn" data-font="sans"  onclick="setFont('sans')">Sans</button>
            <button class="font-btn" data-font="mono"  onclick="setFont('mono')">Mono</button>
        </div>
        <div class="size-picker">
            <button class="size-btn" onclick="adjustSize(-1)">A−</button>
            <button class="size-btn" onclick="adjustSize(1)">A+</button>
        </div>
        <button class="copy-btn" id="copy-btn" onclick="copyContent()">Copy</button>
        <a class="original-link" href="{{.URL}}" target="_blank" rel="noopener noreferrer">↗ Original</a>
    </div>
    <div class="modal-body" id="article-content">
        {{range splitParagraphs .Content}}
        <p>{{.}}</p>
        {{end}}
    </div>
    {{if .Tags}}
    <div class="modal-tags">
        {{range splitTags .Tags}}
        <span class="tag">{{.}}</span>
        {{end}}
    </div>
    {{end}}
</div>
{{end}}
```

- [ ] **Step 4: Write `templates/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Article Viewer</title>
    <link rel="stylesheet" href="/static/style.css">
    <script src="https://unpkg.com/htmx.org@1.9.12" defer></script>
    <script src="/static/app.js" defer></script>
</head>
<body>
    <header id="top-bar">
        <div class="top-inner">
            <input
                type="search"
                id="search-input"
                name="q"
                value="{{.Q}}"
                placeholder="Search articles…"
                autocomplete="off"
                hx-get="/articles"
                hx-trigger="keyup changed delay:300ms, search"
                hx-target="#feed"
                hx-swap="innerHTML"
                hx-include="#site-filter,#cat-filter"
            >
            <input type="hidden" id="site-filter" name="site" value="{{.SelectedSite}}">
            <input type="hidden" id="cat-filter"  name="category" value="{{.SelectedCat}}">
        </div>
        <div class="pills-row">
            <div class="pills" id="site-pills">
                {{range .Sites}}
                <button type="button"
                        class="pill{{if eq $.SelectedSite .}} active{{end}}"
                        onclick="togglePill(this,'site-filter','{{.}}')">{{.}}</button>
                {{end}}
            </div>
            <div class="pills pills-cats" id="cat-pills">
                {{range .Categories}}
                <button type="button"
                        class="pill{{if eq $.SelectedCat .}} active{{end}}"
                        onclick="togglePill(this,'cat-filter','{{.}}')">{{.}}</button>
                {{end}}
            </div>
        </div>
    </header>

    <main>
        <div id="feed">
            {{template "cards" .Cards}}
        </div>
    </main>

    <div id="modal-overlay">
        <div id="modal-container"></div>
    </div>
</body>
</html>
```

- [ ] **Step 5: Build and verify no template errors**

```bash
go build -o article-viewer . && ./article-viewer -db articles.db
```

Open `http://localhost:8080` — page should load (cards may be empty if templates are incomplete, but no 500 errors).

Stop server with Ctrl+C.

- [ ] **Step 6: Commit**

```bash
git add templates/
git commit -m "feat: add HTML templates (index, cards, modal)"
```

---

## Task 5: CSS

**Files:**
- Modify: `static/style.css`

- [ ] **Step 1: Write `static/style.css`**

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
    --bg:        #0f0f1a;
    --surface:   #1a1a2e;
    --surface2:  #252540;
    --border:    #2a2a4a;
    --accent:    #7c88ff;
    --text:      #e0e0e0;
    --muted:     #888;
    --danger:    #ff6b6b;
}

html { font-size: 16px; }
body { background: var(--bg); color: var(--text); font-family: system-ui, sans-serif; min-height: 100vh; }

/* ── TOP BAR ── */
#top-bar {
    position: sticky; top: 0; z-index: 100;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    padding: 0.75rem 1rem 0;
}

.top-inner {
    display: flex; gap: 0.5rem; align-items: center;
    margin-bottom: 0.6rem;
}

#search-input {
    flex: 1;
    background: var(--surface2); border: 1px solid var(--border);
    border-radius: 8px; padding: 0.5rem 0.75rem;
    color: var(--text); font-size: 0.95rem; outline: none;
}
#search-input:focus { border-color: var(--accent); }

/* ── PILLS ── */
.pills-row {
    display: flex; gap: 0.5rem; flex-wrap: wrap;
    padding-bottom: 0.6rem;
    overflow-x: auto; -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
}
.pills-row::-webkit-scrollbar { display: none; }

.pills { display: flex; gap: 0.4rem; flex-wrap: nowrap; }
.pills-cats { border-left: 1px solid var(--border); padding-left: 0.5rem; }

.pill {
    background: var(--surface2); border: 1px solid var(--border);
    border-radius: 999px; padding: 0.25rem 0.75rem;
    color: var(--muted); font-size: 0.8rem; cursor: pointer;
    white-space: nowrap; transition: all 0.15s;
}
.pill:hover { border-color: var(--accent); color: var(--text); }
.pill.active {
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    border-color: var(--accent); color: var(--accent);
}

/* ── FEED / CARD GRID ── */
main { padding: 1rem; max-width: 1200px; margin: 0 auto; }

#feed {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 1rem;
    align-items: start;
}

.card {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 10px; padding: 1rem;
    cursor: pointer; transition: border-color 0.15s, transform 0.1s;
    border-top: 3px solid var(--border);
}
.card:hover { border-top-color: var(--accent); transform: translateY(-2px); }

.card-meta {
    display: flex; flex-wrap: wrap; gap: 0.4rem;
    font-size: 0.75rem; color: var(--muted); margin-bottom: 0.5rem;
}
.card-meta .site  { color: var(--accent); }
.card-meta .category { text-transform: capitalize; }

.card-title {
    font-size: 1rem; font-weight: 600; line-height: 1.4;
    color: var(--text); margin-bottom: 0.4rem;
}
.card-author { font-size: 0.8rem; color: var(--muted); margin-bottom: 0.5rem; }
.card-excerpt { font-size: 0.85rem; color: #aaa; line-height: 1.5;
    display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }

.scroll-sentinel { height: 1px; }

/* ── MODAL OVERLAY ── */
#modal-overlay {
    display: none; position: fixed; inset: 0; z-index: 200;
    background: rgba(0,0,0,0.7); backdrop-filter: blur(4px);
    overflow-y: auto; padding: 2rem 1rem;
}
#modal-overlay.open { display: flex; align-items: flex-start; justify-content: center; }

#modal-container { width: 100%; max-width: 780px; }

/* ── MODAL ── */
.modal {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 12px; overflow: hidden;
}

.modal-header {
    display: flex; gap: 1rem; align-items: flex-start;
    padding: 1.25rem 1.25rem 1rem;
    border-bottom: 1px solid var(--border);
}
.modal-title-group { flex: 1; }
.modal-title { font-size: 1.25rem; font-weight: 700; line-height: 1.4; margin-bottom: 0.5rem; }
.modal-meta { display: flex; flex-wrap: wrap; gap: 0.5rem; font-size: 0.8rem; color: var(--muted); }
.modal-meta .badge {
    background: var(--surface2); border-radius: 4px;
    padding: 0.1rem 0.5rem; font-size: 0.75rem;
}
.modal-meta .site { color: var(--accent); }

.close-btn {
    background: none; border: none; color: var(--muted);
    font-size: 1.25rem; cursor: pointer; padding: 0.25rem; flex-shrink: 0;
}
.close-btn:hover { color: var(--text); }

/* ── MODAL TOOLBAR ── */
.modal-toolbar {
    display: flex; align-items: center; flex-wrap: wrap; gap: 0.5rem;
    padding: 0.6rem 1.25rem;
    border-bottom: 1px solid var(--border);
    background: var(--surface2);
}
.label { font-size: 0.75rem; color: var(--muted); }

.font-picker, .size-picker { display: flex; align-items: center; gap: 0.3rem; }

.font-btn, .size-btn {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 5px; padding: 0.2rem 0.5rem;
    color: var(--muted); font-size: 0.8rem; cursor: pointer;
    transition: all 0.15s;
}
.font-btn:hover, .size-btn:hover { border-color: var(--accent); color: var(--text); }
.font-btn.active { background: color-mix(in srgb, var(--accent) 15%, transparent); border-color: var(--accent); color: var(--accent); }

.copy-btn {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 5px; padding: 0.2rem 0.6rem;
    color: var(--muted); font-size: 0.8rem; cursor: pointer;
    transition: all 0.15s; margin-left: auto;
}
.copy-btn:hover { border-color: var(--accent); color: var(--text); }

.original-link {
    color: var(--accent); font-size: 0.8rem; text-decoration: none;
}
.original-link:hover { text-decoration: underline; }

/* ── MODAL BODY ── */
.modal-body {
    padding: 1.5rem 1.5rem 1rem;
    font-family: Georgia, serif;
    font-size: 1rem; line-height: 1.8; color: #d0d0d0;
    max-height: 65vh; overflow-y: auto;
}
.modal-body p { margin-bottom: 1.1em; }

.modal-tags {
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border);
    display: flex; flex-wrap: wrap; gap: 0.4rem;
}
.tag {
    background: var(--surface2); border-radius: 999px;
    padding: 0.2rem 0.65rem; font-size: 0.75rem; color: var(--muted);
}

/* ── RESPONSIVE ── */
@media (max-width: 600px) {
    #top-bar { padding: 0.6rem 0.75rem 0; }
    main { padding: 0.75rem; }
    #feed { grid-template-columns: 1fr; gap: 0.75rem; }
    #modal-overlay { padding: 0; align-items: flex-end; }
    #modal-container { max-width: 100%; width: 100%; }
    .modal { border-radius: 16px 16px 0 0; max-height: 93vh; overflow-y: auto; }
    .modal-body { max-height: none; }
    .modal-title { font-size: 1.1rem; }
    .pills-cats { border-left: none; padding-left: 0; border-top: 1px solid var(--border); padding-top: 0.4rem; }
}
```

- [ ] **Step 2: Build and check visual**

```bash
go build -o article-viewer . && ./article-viewer -db articles.db
```

Open `http://localhost:8080` — should show dark-themed page with top bar, pills row, and article cards.

Stop server with Ctrl+C.

- [ ] **Step 3: Commit**

```bash
git add static/style.css
git commit -m "feat: add dark theme CSS with responsive layout"
```

---

## Task 6: JavaScript

**Files:**
- Modify: `static/app.js`

- [ ] **Step 1: Write `static/app.js`**

```javascript
// Font families indexed by name
const FONTS = {
    serif: 'Georgia, "Times New Roman", serif',
    sans:  'system-ui, -apple-system, sans-serif',
    mono:  '"Courier New", Courier, monospace',
};

// ── Font & size preference ──

function setFont(name) {
    const content = document.getElementById('article-content');
    if (!content) return;
    content.style.fontFamily = FONTS[name] || FONTS.serif;
    document.querySelectorAll('.font-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.font === name));
    localStorage.setItem('av-font', name);
}

function adjustSize(delta) {
    const content = document.getElementById('article-content');
    if (!content) return;
    const current = parseInt(localStorage.getItem('av-fontSize') || '16', 10);
    const next = Math.min(Math.max(current + delta * 2, 12), 28);
    content.style.fontSize = next + 'px';
    localStorage.setItem('av-fontSize', String(next));
}

function applyPrefs() {
    const font = localStorage.getItem('av-font') || 'serif';
    const size = localStorage.getItem('av-fontSize') || '16';
    setFont(font);
    const content = document.getElementById('article-content');
    if (content) content.style.fontSize = size + 'px';
}

// ── Modal open / close ──

function dismissModal() {
    const overlay = document.getElementById('modal-overlay');
    overlay.classList.remove('open');
    document.body.style.overflow = '';
    document.getElementById('modal-container').innerHTML = '';
}

// Click backdrop to close
document.getElementById('modal-overlay').addEventListener('click', function(e) {
    if (e.target === this) dismissModal();
});

// ESC to close
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') dismissModal();
});

// Open modal and apply prefs after HTMX loads content
document.body.addEventListener('htmx:afterSwap', function(e) {
    if (e.detail.target.id === 'modal-container') {
        document.getElementById('modal-overlay').classList.add('open');
        document.body.style.overflow = 'hidden';
        applyPrefs();
    }
});

// ── Copy article content ──

function copyContent() {
    const content = document.getElementById('article-content');
    if (!content) return;
    navigator.clipboard.writeText(content.innerText).then(function() {
        const btn = document.getElementById('copy-btn');
        const original = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(function() { btn.textContent = original; }, 2000);
    });
}

// ── Pill filter toggle ──

function togglePill(el, inputId, value) {
    const input = document.getElementById(inputId);
    const pills = el.closest('.pills').querySelectorAll('.pill');

    if (input.value === value) {
        // Deselect: already active
        input.value = '';
        el.classList.remove('active');
    } else {
        // Select: deactivate siblings, activate this one
        pills.forEach(function(p) { p.classList.remove('active'); });
        input.value = value;
        el.classList.add('active');
    }

    // Fire HTMX request with current filter state
    const params = new URLSearchParams({
        q:        document.getElementById('search-input').value,
        site:     document.getElementById('site-filter').value,
        category: document.getElementById('cat-filter').value,
        offset:   '0',
    });
    // Remove empty params
    for (const [k, v] of [...params]) { if (!v) params.delete(k); }

    htmx.ajax('GET', '/articles?' + params.toString(), {
        target: '#feed',
        swap:   'innerHTML',
    });
}
```

- [ ] **Step 2: Build and smoke test**

```bash
go build -o article-viewer . && ./article-viewer -db articles.db
```

Open `http://localhost:8080`:
- Type in search box — cards should update after 300ms
- Click a site pill — cards should filter to that site
- Click an article card — modal should open with full content
- Click font buttons — font should change
- Click A− / A+ — font size should change
- Click Copy — button should briefly say "Copied!"
- Click ✕ or press ESC — modal should close
- On mobile viewport (DevTools) — modal should appear as bottom sheet

Stop server with Ctrl+C.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add static/app.js
git commit -m "feat: add JS for font picker, copy button, pill toggle, modal"
```

---

## Task 7: Deployment

**Files:**
- Create: `deploy/article-viewer.service`
- Create: `deploy/nginx-article-viewer.conf`

- [ ] **Step 1: Build production binary**

On the VPS or cross-compiled for Linux amd64:
```bash
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o article-viewer .
```

Copy binary to VPS:
```bash
scp article-viewer user@your-vps:/opt/article-viewer/
```

- [ ] **Step 2: Create `deploy/article-viewer.service`**

```ini
[Unit]
Description=Article Viewer
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/article-viewer
ExecStart=/opt/article-viewer/article-viewer -db /path/to/articles.db -addr 127.0.0.1:8181
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Replace `/path/to/articles.db` with the actual path on your VPS (e.g. `/home/rick/scrape-adv/articles.db`).

- [ ] **Step 3: Create `deploy/nginx-article-viewer.conf`**

```nginx
server {
    listen 80;
    server_name articles.yourdomain.com;

    location / {
        proxy_pass         http://127.0.0.1:8181;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 30s;
    }
}
```

- [ ] **Step 4: Install on VPS**

```bash
# On the VPS
sudo cp deploy/article-viewer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable article-viewer
sudo systemctl start article-viewer
sudo systemctl status article-viewer   # expect: active (running)

sudo cp deploy/nginx-article-viewer.conf /etc/nginx/sites-available/article-viewer
sudo ln -s /etc/nginx/sites-available/article-viewer /etc/nginx/sites-enabled/
sudo nginx -t                           # expect: syntax ok
sudo systemctl reload nginx
```

- [ ] **Step 5: Verify**

```
curl http://articles.yourdomain.com/ | grep "Article Viewer"
```

Expected: HTML containing `Article Viewer`.

- [ ] **Step 6: Commit deploy configs**

```bash
git add deploy/
git commit -m "feat: add systemd service and nginx config for deployment"
```

---

## Self-Review Notes

- **FTS5 init on read-only DB:** Covered — DB is opened read-write so `InitFTS` can create the virtual table; all handler queries are reads only.
- **Missing data:** `{{if .Author}}`, `{{if .PublishDate}}`, `{{if .Tags}}` guards in all templates — nothing rendered for empty fields.
- **Infinite scroll sentinel ID:** Uses class `scroll-sentinel` (not ID) to avoid duplicate ID issues across multiple HTMX appends.
- **URL state on refresh:** `handleArticles` sets `HX-Push-Url` for offset=0 requests so `/?q=...&site=...` is bookmarkable and correctly restores filter state on refresh.
- **Dynamic sites/categories:** `GetSites` and `GetCategories` run `SELECT DISTINCT` at request time — new scraped sites/categories appear automatically.
- **Mobile modal:** `position:fixed; inset:0` bottom-sheet styling at ≤600px via media query.
