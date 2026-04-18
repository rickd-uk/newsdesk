package main

import (
	"embed"
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
	Sites        []string
	Categories   []string
	Q            string
	SelectedSite string
	SelectedCat  string
	Cards        CardsData
}

type CardsData struct {
	Articles   []Article
	Q          string
	Site       string
	Category   string
	NextOffset int
	HasMore    bool
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

// mustParseTemplates parses from disk — used in tests (no embed.FS available).
func mustParseTemplates() *template.Template {
	tmpl, err := template.New("").Funcs(buildFuncMap()).ParseGlob("templates/*.html")
	if err != nil {
		panic("parse templates: " + err.Error())
	}
	// partials may not exist yet during early task execution
	if _, err2 := tmpl.ParseGlob("templates/partials/*.html"); err2 == nil {
		// parsed successfully — nothing to do
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
