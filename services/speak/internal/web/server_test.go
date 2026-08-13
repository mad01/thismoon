package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

// testInfo is the build metadata the test mux reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test-sha",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "speak/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

func newTestMux(t *testing.T, ttsURL string) *http.ServeMux {
	t.Helper()
	mux, err := NewMux(ttsURL, testInfo)
	if err != nil {
		t.Fatal(err)
	}
	return mux
}

// TestVersion pins the cross-tool build metadata contract: the four keys, the
// injected values, and the headers ralph and status probe with.
func TestVersion(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var keys map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.Bytes(), err)
	}
	want := map[string]string{
		"version":    testInfo.Version,
		"commit":     testInfo.Commit,
		"tag":        testInfo.Tag,
		"build_time": testInfo.BuildTime,
	}
	if len(keys) != len(want) {
		t.Errorf("version body = %q, want exactly the keys %v", rec.Body.Bytes(), want)
	}
	for k, v := range want {
		if keys[k] != v {
			t.Errorf("version[%q] = %q, want %q", k, keys[k], v)
		}
	}
}

func TestIndexServesShell(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	// The shell is chrome only: header + a mount point + the webkit/app scripts.
	// The upload form and <wk-read-aloud> are built client-side by app.js now.
	for _, want := range []string{`id="app"`, "/app.js", "/webkit/webkit.js", "wk-header"} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
}

func TestAppJSServesJavaScript(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /app.js: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("GET /app.js Content-Type = %q, want application/javascript", ct)
	}
	body := rec.Body.String()
	// The client logic that left the shell: the upload form posts to /read and
	// builds <wk-read-aloud> over the rendered sections.
	for _, want := range []string{"/read", "wk-read-aloud"} {
		if !strings.Contains(body, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
}

func TestReadReturnsRenderedJSON(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("doc", "notes.md")
	_, _ = fw.Write([]byte("## Hello\n\nworld paragraph\n"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/read", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /read: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST /read Content-Type = %q, want application/json", ct)
	}
	var got readResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /read response: %v (body: %s)", err, rec.Body.String())
	}
	if got.Name != "notes.md" {
		t.Errorf("name = %q, want notes.md", got.Name)
	}
	for _, want := range []string{`<section class="doc-section">`, "<h2>Hello</h2>", "world paragraph"} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("rendered content missing %q", want)
		}
	}
}

func TestSpeechProxyForwardsAndAddsCORS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("upstream path = %q, want /v1/audio/speech", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "af_heart") {
			t.Errorf("upstream body = %q, want voice payload", b)
		}
		// Real mlx-audio reflects the request origin; the proxy must strip
		// this or the response carries two ACAO values and browsers reject it.
		w.Header().Set("Access-Control-Allow-Origin", "http://present.this")
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer upstream.Close()

	mux := newTestMux(t, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"af_heart"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != "*" {
		t.Errorf("ACAO values = %v, want exactly [*]", got)
	}
	if rec.Body.String() != "RIFFfake" {
		t.Errorf("body = %q, want upstream audio passthrough", rec.Body.String())
	}
}

func TestEnginezReportsUpstreamState(t *testing.T) {
	// Reachable upstream — even a 404 response means the engine is up.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	mux := newTestMux(t, upstream.URL)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/enginez", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("enginez with live upstream: status = %d, want 204", rec.Code)
	}

	// Dead upstream — connection refused maps to 502.
	upstream.Close()
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/enginez", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("enginez with dead upstream: status = %d, want 502", rec.Code)
	}
}

func TestSpeechPreflightAnsweredLocally(t *testing.T) {
	// Upstream that fails the test if the preflight is forwarded.
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("OPTIONS preflight must not reach the upstream")
	}))
	defer upstream.Close()

	mux := newTestMux(t, upstream.URL)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/audio/speech", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("Allow-Methods = %q, want POST", got)
	}
}
