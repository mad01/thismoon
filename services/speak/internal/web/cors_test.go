package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSAllowlist is the contract: the speech endpoint drives this
// machine's TTS engine, so only this machine's own pages may fetch from it
// cross-origin. Everything else gets no CORS headers at all, which is what
// stops an arbitrary page the user is browsing from using the engine.
func TestCORSAllowlist(t *testing.T) {
	allowed := []string{
		"http://present.this",
		"https://csl.this",
		"http://speak.this:80",
		"http://localhost:7423",
		"http://127.0.0.1:5173",
		"http://[::1]:8080",
		"http://LOCALHOST:3000",
	}
	denied := []string{
		"",
		"null",
		"https://evil.example",
		"http://this",
		"http://notlocalhost",
		"file://",
		"http://present.this.evil.example",
		"javascript:alert(1)",
	}

	for _, origin := range allowed {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", origin)
		setCORS(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q: ACAO = %q, want it reflected", origin, got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Errorf("origin %q: Vary = %q, want Origin", origin, got)
		}
	}

	for _, origin := range denied {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		setCORS(rec, req)
		for _, h := range []string{
			"Access-Control-Allow-Origin", "Access-Control-Allow-Methods",
			"Access-Control-Allow-Headers", "Access-Control-Max-Age", "Vary",
		} {
			if got := rec.Header().Get(h); got != "" {
				t.Errorf("origin %q: %s = %q, want no CORS headers", origin, h, got)
			}
		}
	}
}

// A disallowed origin's preflight still answers 204 — it just carries no
// permission, so the browser refuses the follow-up request.
func TestSpeechPreflightDeniesForeignOrigin(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodOptions, "/v1/audio/speech", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty for a foreign origin", got)
	}
}
