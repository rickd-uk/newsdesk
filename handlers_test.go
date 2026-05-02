package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	db := testDB(t)
	tmpl := mustParseTemplates()
	return &Server{db: db, tmpl: tmpl}
}

func testSessionCookie(t *testing.T, srv *Server, username string) *http.Cookie {
	t.Helper()
	user := testUser(t, srv.db, username)
	token, err := srv.db.CreateSession(user.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func TestHandleIndexRequiresLogin(t *testing.T) {
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
	if !strings.Contains(body, "Log in to view articles") {
		t.Error("want login prompt in response body")
	}
	if strings.Contains(body, "Japan Times") || strings.Contains(body, "Article One") {
		t.Error("unauthenticated index should not render article filters or articles")
	}
}

func TestHandleIndexAuthenticated(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(testSessionCookie(t, srv, "indexuser"))
	w := httptest.NewRecorder()
	srv.handleIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
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
	req.AddCookie(testSessionCookie(t, srv, "articlesuser"))
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
	req.AddCookie(testSessionCookie(t, srv, "searchuser"))
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
	req.AddCookie(testSessionCookie(t, srv, "filteruser"))
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
	req.AddCookie(testSessionCookie(t, srv, "articleuser"))
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
	req.AddCookie(testSessionCookie(t, srv, "notfounduser"))
	w := httptest.NewRecorder()
	srv.handleArticle(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleArticle_BadID(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/article/abc", nil)
	req.AddCookie(testSessionCookie(t, srv, "badiduser"))
	w := httptest.NewRecorder()
	srv.handleArticle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleSignup(t *testing.T) {
	srv := testServer(t)
	srv.signupEnabled = true
	form := url.Values{
		"username": {"user1"},
		"email":    {""},
		"password": {"password123"},
		"next":     {"/"},
	}
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSignup(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("want %q session cookie, got %#v", sessionCookieName, cookies)
	}
}

func TestHandleSignup_Disabled(t *testing.T) {
	srv := testServer(t)
	form := url.Values{
		"username": {"user1"},
		"email":    {""},
		"password": {"password123"},
		"next":     {"/"},
	}
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSignup(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

func TestHandleArticles_RequiresLogin(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/articles", nil)
	w := httptest.NewRecorder()

	srv.handleArticles(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestHandleArticle_RequiresLogin(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("GET", "/article/1", nil)
	w := httptest.NewRecorder()

	srv.handleArticle(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestHandleMarkRead_RequiresLogin(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest("POST", "/article/1/read", nil)
	w := httptest.NewRecorder()

	srv.handleMarkRead(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestHandleSaveNote(t *testing.T) {
	srv := testServer(t)
	cookie := testSessionCookie(t, srv, "notehandler")
	form := url.Values{"note": {"handler saved note"}}
	req := httptest.NewRequest("POST", "/article/1/note", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()

	srv.handleSaveNote(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	user, err := srv.currentUser(req)
	if err != nil || user == nil {
		t.Fatalf("currentUser: user=%#v err=%v", user, err)
	}
	article, err := srv.db.GetArticleByID(1, user.ID)
	if err != nil {
		t.Fatalf("GetArticleByID: %v", err)
	}
	if article.Note != "handler saved note" {
		t.Fatalf("want saved note, got %q", article.Note)
	}
}
