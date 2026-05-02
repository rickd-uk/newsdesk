package main

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const pageSize = 20

type Server struct {
	db            *DB
	tmpl          *template.Template
	signupEnabled bool
}

type PageData struct {
	Sites          []string
	CategoryGroups []CategoryGroup
	CurrentUser    *User
	AuthError      string
	AuthMode       string
	SignupEnabled  bool
	CurrentPath    string
	Q              string
	Author         string
	DateFrom       string
	DateTo         string
	SearchTitle    bool
	SearchBody     bool
	SearchTags     bool
	SearchNotes    bool
	SelectedSite   string
	SelectedCat    string
	HideRead       bool
	ReadsOnly      bool
	FavoritesOnly  bool
	ArchivedOnly   bool
	HideNotes      bool
	Cards          CardsData
}

type CardsData struct {
	Articles      []Article
	TotalCount    int
	Q             string
	Author        string
	DateFrom      string
	DateTo        string
	Fields        []string
	Site          string
	Category      string
	HideRead      bool
	ReadsOnly     bool
	FavoritesOnly bool
	ArchivedOnly  bool
	HideNotes     bool
	NextOffset    int
	HasMore       bool
}

type ModalData struct {
	Article     *Article
	CurrentUser *User
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
			if d.HideRead {
				params.Set("hide_read", "1")
			}
			if d.ReadsOnly {
				params.Set("reads_only", "1")
			}
			if d.FavoritesOnly {
				params.Set("favorites_only", "1")
			}
			if d.ArchivedOnly {
				params.Set("archived_only", "1")
			}
			if d.HideNotes {
				params.Set("hide_notes", "1")
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
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		mode := r.URL.Query().Get("auth_mode")
		if mode == "" || !s.signupEnabled {
			mode = "login"
		}
		data := PageData{
			CurrentUser:   nil,
			AuthError:     r.URL.Query().Get("auth_error"),
			AuthMode:      mode,
			SignupEnabled: s.signupEnabled,
			CurrentPath:   currentPath(r),
		}
		if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("execute index.html: %v", err)
		}
		return
	}
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")
	author := r.URL.Query().Get("author")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	fields := parseFields(r)
	hideRead := user != nil && r.URL.Query().Get("hide_read") == "1"
	readsOnly := user != nil && r.URL.Query().Get("reads_only") == "1"
	favoritesOnly := user != nil && r.URL.Query().Get("favorites_only") == "1"
	archivedOnly := user != nil && r.URL.Query().Get("archived_only") == "1"
	hideNotes := user != nil && r.URL.Query().Get("hide_notes") == "1"
	if readsOnly {
		hideRead = false
	}

	userID := 0
	if user != nil {
		userID = user.ID
	}

	qp := QueryParams{
		UserID: userID,
		Q:      q, Site: site, Category: cat,
		Author: author, DateFrom: dateFrom, DateTo: dateTo,
		Fields: fields, HideRead: hideRead, ReadsOnly: readsOnly,
		FavoritesOnly: favoritesOnly, ArchivedOnly: archivedOnly, HideNotes: hideNotes,
	}
	sites, _ := s.db.GetSites()
	catInfos, _ := s.db.GetCategoryInfos()
	articles, _ := s.db.QueryArticles(QueryParams{Limit: pageSize, Offset: 0,
		UserID: qp.UserID,
		Q:      qp.Q, Site: qp.Site, Category: qp.Category,
		Author: qp.Author, DateFrom: qp.DateFrom, DateTo: qp.DateTo,
		Fields: qp.Fields, HideRead: qp.HideRead, ReadsOnly: qp.ReadsOnly,
		FavoritesOnly: qp.FavoritesOnly, ArchivedOnly: qp.ArchivedOnly, HideNotes: qp.HideNotes})
	total, _ := s.db.CountArticles(qp)

	data := PageData{
		Sites:          sites,
		CategoryGroups: BuildCategoryTree(catInfos),
		CurrentUser:    user,
		AuthError:      r.URL.Query().Get("auth_error"),
		AuthMode:       r.URL.Query().Get("auth_mode"),
		SignupEnabled:  s.signupEnabled,
		CurrentPath:    currentPath(r),
		Q:              q,
		Author:         author,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		SearchTitle:    hasField(fields, "title"),
		SearchBody:     hasField(fields, "body"),
		SearchTags:     hasField(fields, "tags"),
		SearchNotes:    hasField(fields, "notes"),
		SelectedSite:   site,
		SelectedCat:    cat,
		HideRead:       hideRead,
		ReadsOnly:      readsOnly,
		FavoritesOnly:  favoritesOnly,
		ArchivedOnly:   archivedOnly,
		HideNotes:      hideNotes,
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
			HideRead:      hideRead,
			ReadsOnly:     readsOnly,
			FavoritesOnly: favoritesOnly,
			ArchivedOnly:  archivedOnly,
			HideNotes:     hideNotes,
			NextOffset:    pageSize,
			HasMore:       len(articles) == pageSize,
		},
	}
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("execute index.html: %v", err)
	}
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	site := r.URL.Query().Get("site")
	cat := r.URL.Query().Get("category")
	author := r.URL.Query().Get("author")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	fields := parseFields(r)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	hideRead := user != nil && r.URL.Query().Get("hide_read") == "1"
	readsOnly := user != nil && r.URL.Query().Get("reads_only") == "1"
	favoritesOnly := user != nil && r.URL.Query().Get("favorites_only") == "1"
	archivedOnly := user != nil && r.URL.Query().Get("archived_only") == "1"
	hideNotes := user != nil && r.URL.Query().Get("hide_notes") == "1"
	if readsOnly {
		hideRead = false
	}

	userID := 0
	if user != nil {
		userID = user.ID
	}
	qp := QueryParams{UserID: userID, Q: q, Site: site, Category: cat,
		Author: author, DateFrom: dateFrom, DateTo: dateTo,
		Fields: fields, HideRead: hideRead, ReadsOnly: readsOnly,
		FavoritesOnly: favoritesOnly, ArchivedOnly: archivedOnly, HideNotes: hideNotes}
	articles, _ := s.db.QueryArticles(QueryParams{Limit: pageSize, Offset: offset,
		UserID: qp.UserID,
		Q:      qp.Q, Site: qp.Site, Category: qp.Category,
		Author: qp.Author, DateFrom: qp.DateFrom, DateTo: qp.DateTo,
		Fields: qp.Fields, HideRead: qp.HideRead, ReadsOnly: qp.ReadsOnly,
		FavoritesOnly: qp.FavoritesOnly, ArchivedOnly: qp.ArchivedOnly, HideNotes: qp.HideNotes})
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
		if hideRead {
			params.Set("hide_read", "1")
		}
		if readsOnly {
			params.Set("reads_only", "1")
		}
		if favoritesOnly {
			params.Set("favorites_only", "1")
		}
		if archivedOnly {
			params.Set("archived_only", "1")
		}
		if hideNotes {
			params.Set("hide_notes", "1")
		}
		pushURL := "/"
		if len(params) > 0 {
			pushURL += "?" + params.Encode()
		}
		w.Header().Set("HX-Push-Url", pushURL)
	}

	data := CardsData{
		Articles:      articles,
		TotalCount:    total,
		Q:             q,
		Author:        author,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Fields:        fields,
		Site:          site,
		Category:      cat,
		HideRead:      hideRead,
		ReadsOnly:     readsOnly,
		FavoritesOnly: favoritesOnly,
		ArchivedOnly:  archivedOnly,
		HideNotes:     hideNotes,
		NextOffset:    offset + pageSize,
		HasMore:       len(articles) == pageSize,
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
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/archive"):
		s.handleMarkArchived(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/unarchive"):
		s.handleUnmarkArchived(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/note"):
		s.handleSaveNote(w, r)
	default:
		s.handleArticle(w, r)
	}
}

func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/article/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	userID := 0
	if user != nil {
		userID = user.ID
	}
	article, err := s.db.GetArticleByID(id, userID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.tmpl.ExecuteTemplate(w, "modal", ModalData{Article: article, CurrentUser: user}); err != nil {
		log.Printf("execute modal: %v", err)
	}
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/read")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkRead(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/unread")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkUnread(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkFavorite(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/favorite")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkFavorite(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnmarkFavorite(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/unfavorite")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.UnmarkFavorite(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkArchived(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/archive")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.MarkArchived(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnmarkArchived(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/unarchive")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.db.UnmarkArchived(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/article/"), "/note")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.db.SaveNote(user.ID, id, r.FormValue("note")); err != nil {
		http.Error(w, "save note failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.signupEnabled {
		http.Error(w, "signups disabled", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user, err := s.db.CreateUser(r.FormValue("username"), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		s.redirectAuthError(w, r, err.Error(), "signup")
		return
	}
	token, err := s.db.CreateSession(user.ID)
	if err != nil {
		http.Error(w, "session create failed", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token)
	http.Redirect(w, r, authRedirectTarget(r, false), http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user, err := s.db.AuthenticateUser(r.FormValue("identifier"), r.FormValue("password"))
	if err != nil {
		if errors.Is(err, errInvalidCredentials) {
			s.redirectAuthError(w, r, "invalid username, email, or password", "login")
			return
		}
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	token, err := s.db.CreateSession(user.ID)
	if err != nil {
		http.Error(w, "session create failed", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token)
	http.Redirect(w, r, authRedirectTarget(r, false), http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	if cookie != nil {
		_ = s.db.DeleteSession(cookie.Value)
	}
	clearSessionCookie(w)
	http.Redirect(w, r, authRedirectTarget(r, false), http.StatusSeeOther)
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return nil, false
	}
	if user == nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return nil, false
	}
	return user, true
}

func authRedirectTarget(r *http.Request, withError bool) string {
	target := r.FormValue("next")
	if target == "" || !strings.HasPrefix(target, "/") {
		target = "/"
	}
	if !withError {
		u, err := url.Parse(target)
		if err == nil {
			q := u.Query()
			q.Del("auth_error")
			q.Del("auth_mode")
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return target
}

func (s *Server) redirectAuthError(w http.ResponseWriter, r *http.Request, msg, mode string) {
	target := authRedirectTarget(r, true)
	u, err := url.Parse(target)
	if err != nil {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	q := u.Query()
	q.Set("auth_error", msg)
	if mode != "" {
		q.Set("auth_mode", mode)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func currentPath(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}
