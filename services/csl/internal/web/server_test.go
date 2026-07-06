package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer builds a Server with no Service. Routes that don't touch search
// (pages, healthz, and the missing-query branch of /api/search) are exercisable
// without a filesystem-backed index.
func newTestServer() http.Handler {
	return (&Server{}).Handler()
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want ok", rec.Body.String())
	}
}

func TestPagesServed(t *testing.T) {
	for _, path := range []string{"/"} {
		rec := httptest.NewRecorder()
		newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s content-type = %q, want text/html", path, ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<wk-header") {
			t.Errorf("%s body missing <wk-header> chrome", path)
		}
		if !strings.Contains(body, "/webkit/webkit.js") {
			t.Errorf("%s body missing /webkit/webkit.js script", path)
		}
	}
}

// TestWebkitServed asserts the shared webkit chrome is served at /webkit/ so the
// pages can link /webkit/webkit.css and /webkit/webkit.js.
func TestWebkitServed(t *testing.T) {
	for _, path := range []string{"/webkit/webkit.css", "/webkit/webkit.js"} {
		rec := httptest.NewRecorder()
		newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestSearchMissingQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("body = %q, want JSON error", rec.Body.String())
	}
}

func TestReadMissingParams(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/read?repo=x", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAssetsServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ".repo-group") {
		t.Errorf("app.css missing csl content styles")
	}
}
