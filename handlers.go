package main

import (
	"embed"
	"html/template"
	"log"
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
	Sites           []string
	CategoryGroups  []CategoryGroup
	Q            string
	Author       string
	DateFrom     string
	DateTo       string
	SearchTitle  bool
	SearchBody   bool
	SearchTags   bool
	SelectedSite string
	SelectedCat  string
	ShowRead      bool
	FavoritesOnly bool
	Cards         CardsData
}

type CardsData struct {
	Articles   []Article
	TotalCount int
	Q          string
	Author     string
	DateFrom   string
	DateTo     string
	Fields     []string
	Site       string
	Category   string
	ShowRead      bool
	FavoritesOnly bool
	NextOffset    int
	HasMore       bool
}

func parseFields(r *http.Request) []string {
	f := r.URL.Query()["fields"]
	if len(f) == 0 {
		return nil // nil = all columns
	}
	return f
}

func hasField(fields []string, name string) bool {
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}

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
		"fmtCount": func(n int) string {
			s := strconv.Itoa(n)
			for i := len(s) - 3; i > 0; i -= 3 {
				s = s[:i] + "," + s[i:]
			}
			return s
		},
		"buildQuery": func(d CardsData) template.URL {
			params := url.Values{}
			if d.Q != "" {
				params.Set("q", d.Q)
			}
			if d.Site != "" {
				params.Set("site", d.Site)
			}
			if d.Category != "" {
				params.Set("category", d.Category)
			}
			if d.Author != "" {
				params.Set("author", d.Author)
			}
			if d.DateFrom != "" {
				params.Set("date_from", d.DateFrom)
			}
			if d.DateTo != "" {
				params.Set("date_to", d.DateTo)
			}
			for _, f := range d.Fields {
				params.Add("fields", f)
			}
			if d.ShowRead {
				params.Set("show_read", "1")
			}
			if d.FavoritesOnly {
				params.Set("favorites_only", "1")
			}
			params.Set("offset", strconv.Itoa(d.NextOffset))
			return template.URL("/articles?" + params.Encode())
		},
	}
}

// mustParseTemplates parses from disk — used in tests (no embed.FS available).
func mustParseTemplates() *template.Template {
	tmpl, err := template.New("").Funcs(buildFuncMap()).ParseGlob("templates/*.html")
	if err != nil {
		panic("parse templates: " + err.Error())
	}
	// partials directory may not exist yet during early tasks — tolerate absence only
	if _, err2 := tmpl.ParseGlob("templates/partials/*.html"); err2 != nil {
		if !strings.Contains(err2.Error(), "pattern matches no files") {
			panic("parse partials: " + err2.Error())
		}
	}
	return tmpl
}

// mustParseTemplatesFS parses from embedded filesystem — used in production.
func mustParseTemplatesFS(fsys embed.FS) (*template.Template, error) {
	return template.New("").Funcs(buildFuncMap()).ParseFS(fsys, "templates/*.html", "templates/partials/*.html")
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")
	author := r.URL.Query().Get("author")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	fields := parseFields(r)
	showRead      := r.URL.Query().Get("show_read") == "1"
	favoritesOnly := r.URL.Query().Get("favorites_only") == "1"

	qp := QueryParams{
		Q: q, Site: site, Category: cat,
		Author: author, DateFrom: dateFrom, DateTo: dateTo,
		Fields: fields, HideRead: !showRead, FavoritesOnly: favoritesOnly,
	}
	sites, _ := s.db.GetSites()
	catInfos, _ := s.db.GetCategoryInfos()
	articles, _ := s.db.QueryArticles(QueryParams{Limit: pageSize, Offset: 0,
		Q: qp.Q, Site: qp.Site, Category: qp.Category,
		Author: qp.Author, DateFrom: qp.DateFrom, DateTo: qp.DateTo,
		Fields: qp.Fields, HideRead: qp.HideRead, FavoritesOnly: qp.FavoritesOnly})
	total, _ := s.db.CountArticles(qp)

	data := PageData{
		Sites:          sites,
		CategoryGroups: BuildCategoryTree(catInfos),
		Q:            q,
		Author:       author,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
		SearchTitle:  hasField(fields, "title"),
		SearchBody:   hasField(fields, "body"),
		SearchTags:   hasField(fields, "tags"),
		SelectedSite: site,
		SelectedCat:  cat,
		ShowRead:      showRead,
		FavoritesOnly: favoritesOnly,
		Cards: CardsData{
			Articles:      articles,
			TotalCount:    total,
			Q:             q,
			Author:        author,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
			Fields:        fields,
			Site:          site,
			Category:      cat,
			ShowRead:      showRead,
			FavoritesOnly: favoritesOnly,
			NextOffset:    pageSize,
			HasMore:       len(articles) == pageSize,
		},
	}
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("execute index.html: %v", err)
	}
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")
	author := r.URL.Query().Get("author")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	fields := parseFields(r)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	showRead      := r.URL.Query().Get("show_read") == "1"
	favoritesOnly := r.URL.Query().Get("favorites_only") == "1"

	qp := QueryParams{Q: q, Site: site, Category: cat,
		Author: author, DateFrom: dateFrom, DateTo: dateTo,
		Fields: fields, HideRead: !showRead, FavoritesOnly: favoritesOnly}
	articles, _ := s.db.QueryArticles(QueryParams{Limit: pageSize, Offset: offset,
		Q: qp.Q, Site: qp.Site, Category: qp.Category,
		Author: qp.Author, DateFrom: qp.DateFrom, DateTo: qp.DateTo,
		Fields: qp.Fields, HideRead: qp.HideRead, FavoritesOnly: qp.FavoritesOnly})
	total, _ := s.db.CountArticles(qp)

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
		if author != "" {
			params.Set("author", author)
		}
		if dateFrom != "" {
			params.Set("date_from", dateFrom)
		}
		if dateTo != "" {
			params.Set("date_to", dateTo)
		}
		for _, f := range fields {
			params.Add("fields", f)
		}
		if showRead {
			params.Set("show_read", "1")
		}
		if favoritesOnly {
			params.Set("favorites_only", "1")
		}
		pushURL := "/"
		if len(params) > 0 {
			pushURL += "?" + params.Encode()
		}
		w.Header().Set("HX-Push-Url", pushURL)
	}

	data := CardsData{
		Articles:   articles,
		TotalCount: total,
		Q:          q,
		Author:     author,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Fields:     fields,
		Site:       site,
		Category:   cat,
		ShowRead:      showRead,
		FavoritesOnly: favoritesOnly,
		NextOffset:    offset + pageSize,
		HasMore:    len(articles) == pageSize,
	}
	if err := s.tmpl.ExecuteTemplate(w, "cards", data); err != nil {
		log.Printf("execute cards: %v", err)
	}
}

func (s *Server) handleArticleDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/read"):
		s.handleMarkRead(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/unread"):
		s.handleMarkUnread(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/favorite"):
		s.handleMarkFavorite(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/unfavorite"):
		s.handleUnmarkFavorite(w, r)
	default:
		s.handleArticle(w, r)
	}
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
	if err := s.tmpl.ExecuteTemplate(w, "modal", article); err != nil {
		log.Printf("execute modal: %v", err)
	}
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/read")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkRead(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/unread")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkUnread(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkFavorite(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/favorite")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkFavorite(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnmarkFavorite(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/unfavorite")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.UnmarkFavorite(id)
	w.WriteHeader(http.StatusNoContent)
}
