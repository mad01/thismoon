package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
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

// preflight is the browser's OPTIONS before a cross-origin JSON POST.
func preflight(h http.Handler, origin, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, target, nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestPreflightAnswersForOwnPages pins that a page on the allowlist can post
// JSON to any route: its preflight gets 204 with the permission on, from the
// wrapper, without a route of its own for OPTIONS.
func TestPreflightAnswersForOwnPages(t *testing.T) {
	f := newFakeSpeaker()
	h := guardHandler(t, f)
	want := map[string]string{
		"Access-Control-Allow-Origin":  "http://present.this",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type",
		"Access-Control-Max-Age":       "86400",
	}
	for _, target := range []string{"/read", "/doc/0123456789abcdef/prepare", "/v1/audio/speech"} {
		rec := preflight(h, "http://present.this", target)
		if rec.Code != http.StatusNoContent {
			t.Errorf("OPTIONS %s = %d %s, want 204", target, rec.Code, rec.Body.String())
		}
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("OPTIONS %s: %s = %q, want %q", target, k, got, v)
			}
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("preflights ran %d syntheses, want none", n)
	}
}

// A foreign origin's preflight is refused outright, with no permission on
// it, so the browser never sends the request behind it.
func TestPreflightRefusesForeignOrigin(t *testing.T) {
	h := guardHandler(t, newFakeSpeaker())
	rec := preflight(h, "https://evil.example", "/read")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("preflight status = %d, want 403", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty for a foreign origin", got)
	}
}

// guardCase is one request through the served handler.
type guardCase struct {
	name, method, target string
	body, contentType    string
	headers              map[string]string
}

func (c guardCase) serve(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.target, strings.NewReader(c.body))
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func guardHandler(t *testing.T, f *fakeSpeaker) http.Handler {
	t.Helper()
	return handler(Config{
		Speaker:  f,
		Health:   tts.NewHealth("fake", "model"),
		Info:     testInfo,
		CacheDir: t.TempDir(),
	})
}

// Headers a browser sends: a fetch from an allowlisted sibling page, a
// fetch from the page itself, and a link followed from anywhere.
var (
	fromPresent = map[string]string{
		"Origin": "http://present.this", "Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors",
	}
	fromSpeakPage = map[string]string{
		"Origin": "http://127.0.0.1:7425", "Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors",
	}
	navigation = map[string]string{
		"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document",
	}
	evilOrigin = map[string]string{
		"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "no-cors",
	}
)

// TestHandlerAllowsOwnPages pins what the cross-site guard must keep
// working: tools without a browser, the page itself, allowlisted siblings
// and navigations.
func TestHandlerAllowsOwnPages(t *testing.T) {
	f := newFakeSpeaker()
	f.open()
	h := guardHandler(t, f)
	speech := `{"input":"hi"}`
	blocks := `{"sections":[{"blocks":["Hi there."]}]}`
	for _, c := range []guardCase{
		{name: "curl speech", method: http.MethodPost, target: "/v1/audio/speech", body: speech},
		{"page speech", http.MethodPost, "/v1/audio/speech", speech, "", fromSpeakPage},
		{"present speech", http.MethodPost, "/v1/audio/speech", speech, "", fromPresent},
		{"present preflight", http.MethodOptions, "/v1/audio/speech", "", "", fromPresent},
		{"present read preflight", http.MethodOptions, "/read", "", "", fromPresent},
		{"present register", http.MethodPost, "/read", blocks, "application/json", fromPresent},
		{
			"present prepare preflight", http.MethodOptions, "/doc/0123456789abcdef/prepare", "",
			"", fromPresent,
		},
		{"present probe", http.MethodGet, "/", "", "", fromPresent},
		{"present healthz", http.MethodGet, "/healthz", "", "", fromPresent},
		{"present enginez", http.MethodGet, "/enginez", "", "", fromPresent},
		{"navigation", http.MethodGet, "/", "", "", navigation},
		{name: "doctor", method: http.MethodGet, target: "/version"},
	} {
		if rec := c.serve(h); rec.Code >= http.StatusBadRequest {
			t.Errorf("%s: %s %s = %d %s, want it served", c.name, c.method, c.target, rec.Code,
				rec.Body.String())
		}
	}
}

// TestHandlerRefusesOtherSites pins the guard: a page that is not this
// machine's own cannot spend synthesis through a request the browser sends
// without a preflight, whether or not it may read the answer.
func TestHandlerRefusesOtherSites(t *testing.T) {
	f := newFakeSpeaker()
	f.open()
	h := guardHandler(t, f)
	key := audiocache.Key("fake ", 0, "hi")
	for _, c := range []guardCase{
		{
			"text/plain speech", http.MethodPost, "/v1/audio/speech", `{"input":"hi"}`,
			"text/plain", evilOrigin,
		},
		{
			"form post", http.MethodPost, "/read", "--b\r\nContent-Disposition: form-data; " +
				"name=\"doc\"; filename=\"a.md\"\r\n\r\nHello there.\r\n--b--\r\n",
			"multipart/form-data; boundary=b", evilOrigin,
		},
		{
			"register", http.MethodPost, "/read", `{"sections":[{"blocks":["Hi there."]}]}`,
			"application/json", evilOrigin,
		},
		{"read preflight", http.MethodOptions, "/read", "", "", evilOrigin},
		{"prepare", http.MethodPost, "/doc/0123456789abcdef/prepare", "", "", evilOrigin},
		{
			"opaque origin", http.MethodPost, "/v1/audio/speech", `{"input":"hi"}`, "",
			map[string]string{"Origin": "null"},
		},
		{"audio element", http.MethodGet, "/audio/" + key, "", "", map[string]string{
			"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "no-cors", "Sec-Fetch-Dest": "audio",
		}},
		{
			"image of a download", http.MethodGet, "/doc/0123456789abcdef/audio", "", "",
			map[string]string{
				"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "no-cors",
				"Sec-Fetch-Dest": "image",
			},
		},
	} {
		rec := c.serve(h)
		var body errorBody
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusForbidden || body.Error.Message == "" {
			t.Errorf("%s: %s %s = %d %s, want 403 with an error message", c.name, c.method,
				c.target, rec.Code, rec.Body.String())
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("refused requests ran %d syntheses, want none", n)
	}
}
