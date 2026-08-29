package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// TestSitesCORSAllowlist is the contract on the one endpoint d-man answers
// itself: the site list names this machine's local services, so only this
// machine's own pages may read it cross-origin. Everything else gets no CORS
// headers, which is what stops a page the user is browsing from enumerating
// the local services.
func TestSitesCORSAllowlist(t *testing.T) {
	sites := []config.Site{
		{Name: "csl", Host: "csl.this", URL: "http://csl.this/", Backend: "127.0.0.1:9"},
	}
	h, err := New(map[string]string{"csl.this": "127.0.0.1:9"}, sites, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.probe = alwaysUp

	allowed := []string{
		"http://present.this",
		"https://csl.this",
		"http://localhost:7424",
		"http://127.0.0.1:5173",
		"http://[::1]:8080",
	}
	denied := []string{
		"null",
		"https://evil.example",
		"http://this",
		"http://notlocalhost",
		"http://csl.this.evil.example",
	}

	get := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://csl.this"+SitesPath, nil)
		req.Host = "csl.this"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, origin := range allowed {
		rec := get(origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q: ACAO = %q, want it reflected", origin, got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Errorf("origin %q: Vary = %q, want Origin", origin, got)
		}
	}

	for _, origin := range denied {
		rec := get(origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q: ACAO = %q, want no CORS header", origin, got)
		}
		if got := rec.Header().Get("Vary"); got != "" {
			t.Errorf("origin %q: Vary = %q, want none", origin, got)
		}
		// The body is still served: a same-origin page behind d-man sends no
		// Origin at all, and the browser is what enforces the rest.
		if rec.Code != http.StatusOK {
			t.Errorf("origin %q: code = %d, want 200", origin, rec.Code)
		}
	}

	// No Origin header (same-origin fetch, curl): no CORS headers, still served.
	rec := get("")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no Origin: ACAO = %q, want none", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("no Origin: code = %d, want 200", rec.Code)
	}
}
