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

func TestHandleSignup(t *testing.T) {
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

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("want %q session cookie, got %#v", sessionCookieName, cookies)
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
