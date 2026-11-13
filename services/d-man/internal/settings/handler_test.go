package settings

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPath = "/__this/settings.json"

func newHandler(t *testing.T) (http.Handler, *Store) {
	t.Helper()
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	store := Open(filepath.Join(t.TempDir(), "ui-settings.json"), logger)
	return Handler(store, logger), store
}

func newPatch(host, origin, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "http://"+host+testPath, strings.NewReader(body))
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertNoCORS(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for name := range rec.Header() {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), "Access-Control-") {
			t.Errorf("response carries %s: %q", name, rec.Header().Get(name))
		}
	}
}

func decodeDocument(t *testing.T, rec *httptest.ResponseRecorder) Document {
	t.Helper()
	var doc Document
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return doc
}

func TestGetServesDocumentWithoutCORS(t *testing.T) {
	h, _ := newHandler(t)
	req := httptest.NewRequest(http.MethodGet, "http://csl.this"+testPath, nil)
	req.Host = "csl.this"
	req.Header.Set("Origin", "http://present.this")
	rec := serve(h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"v":1,"settings":{}}` {
		t.Errorf("body = %q, want the empty document", got)
	}
	for header, want := range map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	assertNoCORS(t, rec)
}

func TestPatchSameOrigin(t *testing.T) {
	h, store := newHandler(t)
	for _, origin := range []string{"http://csl.this", "HTTP://CSL.THIS"} {
		rec := serve(h, newPatch("csl.this", origin,
			`{"v":1,"settings":{"webkit-theme":"dark","unknown":{"x":1}}}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("origin %q: code = %d (%s), want 200", origin, rec.Code, rec.Body.String())
		}
		doc := decodeDocument(t, rec)
		if !maps.Equal(doc.Settings, map[string]string{"webkit-theme": "dark"}) {
			t.Errorf("origin %q: document = %v, want the unknown key dropped", origin, doc.Settings)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("origin %q: Cache-Control = %q", origin, rec.Header().Get("Cache-Control"))
		}
		assertNoCORS(t, rec)
	}
	if got := store.Document().Settings["webkit-theme"]; got != "dark" {
		t.Errorf("store = %q, want dark", got)
	}
}

// The request Host keeps its port, so the Origin must carry the same one.
func TestPatchSameOriginWithPort(t *testing.T) {
	h, _ := newHandler(t)
	rec := serve(h, newPatch("csl.this:18080", "http://csl.this:18080",
		`{"v":1,"settings":{"webkit-size":"18"}}`))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d (%s), want 200", rec.Code, rec.Body.String())
	}
}

func TestPatchOverTLSWantsHTTPSOrigin(t *testing.T) {
	h, _ := newHandler(t)
	body := `{"v":1,"settings":{"webkit-size":"18"}}`

	req := newPatch("csl.this", "https://csl.this", body)
	req.TLS = &tls.ConnectionState{}
	if rec := serve(h, req); rec.Code != http.StatusOK {
		t.Errorf("https origin over TLS: code = %d, want 200", rec.Code)
	}

	req = newPatch("csl.this", "http://csl.this", body)
	req.TLS = &tls.ConnectionState{}
	if rec := serve(h, req); rec.Code != http.StatusForbidden {
		t.Errorf("http origin over TLS: code = %d, want 403", rec.Code)
	}
}

func TestPatchRefusesOtherOrigins(t *testing.T) {
	h, store := newHandler(t)
	body := `{"v":1,"settings":{"webkit-theme":"dark"}}`
	for _, origin := range []string{
		"",                       // no Origin
		"null",                   // opaque
		"http://present.this",    // another .this host
		"http://localhost:7424",  // a localhost port
		"http://127.0.0.1",       // loopback
		"https://csl.this",       // wrong scheme
		"http://csl.this:8080",   // wrong port
		"http://csl.this/",       // not a serialized origin
		"http://csl.this.evil",   // suffix trick
		"http://evil.csl.this",   // subdomain
		"http://csl.this@evil",   // userinfo trick
		"https://example.com",    // the internet
		"http://csl.this, x",     // smuggled list
		"chrome-extension://abc", // non-web scheme
	} {
		rec := serve(h, newPatch("csl.this", origin, body))
		if rec.Code != http.StatusForbidden {
			t.Errorf("origin %q: code = %d, want 403", origin, rec.Code)
		}
		assertNoCORS(t, rec)
	}
	if got := store.Document().Settings; len(got) != 0 {
		t.Errorf("store = %v, want untouched", got)
	}
}

func TestPatchWantsJSONContentType(t *testing.T) {
	h, _ := newHandler(t)
	for _, ct := range []string{
		"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data",
	} {
		req := newPatch("csl.this", "http://csl.this", `{"v":1,"settings":{}}`)
		req.Header.Set("Content-Type", ct)
		if rec := serve(h, req); rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: code = %d, want 415", ct, rec.Code)
		}
	}
	req := newPatch("csl.this", "http://csl.this", `{"v":1,"settings":{}}`)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if rec := serve(h, req); rec.Code != http.StatusOK {
		t.Errorf("Content-Type with charset: code = %d, want 200", rec.Code)
	}
}

func TestPatchRefusesOversizeBody(t *testing.T) {
	h, store := newHandler(t)
	pad := strings.Repeat("x", maxBody)
	body := `{"v":1,"settings":{"webkit-theme":"dark","pad":"` + pad + `"}}`
	rec := serve(h, newPatch("csl.this", "http://csl.this", body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("code = %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "body over 4 KiB") {
		t.Errorf("body = %q, want the 4 KiB limit named", rec.Body.String())
	}
	if got := store.Document().Settings; len(got) != 0 {
		t.Errorf("store = %v, want untouched", got)
	}
}

func TestPatchRefusesBadBodies(t *testing.T) {
	h, store := newHandler(t)
	if _, err := store.Patch(map[string]string{"webkit-theme": "light"}); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"bad json":         `{"v":1,`,
		"trailing data":    `{"v":1,"settings":{}} {}`,
		"no version":       `{"settings":{"webkit-theme":"dark"}}`,
		"version 2":        `{"v":2,"settings":{"webkit-theme":"dark"}}`,
		"bad value":        `{"v":1,"settings":{"webkit-theme":"dark","webkit-size":"25"}}`,
		"unknown family":   `{"v":1,"settings":{"webkit-palette-dark":"dracula"}}`,
		"non-string value": `{"v":1,"settings":{"webkit-size":18}}`,
	} {
		rec := serve(h, newPatch("csl.this", "http://csl.this", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", name, rec.Code)
		}
	}
	if got := store.Document().Settings; !maps.Equal(
		got,
		map[string]string{"webkit-theme": "light"},
	) {
		t.Errorf("store = %v, want unchanged", got)
	}
}

func TestOtherMethodsAre405(t *testing.T) {
	h, _ := newHandler(t)
	for _, method := range []string{
		http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead,
	} {
		req := httptest.NewRequest(method, "http://csl.this"+testPath, nil)
		req.Host = "csl.this"
		req.Header.Set("Origin", "http://csl.this")
		req.Header.Set("Access-Control-Request-Method", "PATCH")
		rec := serve(h, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: code = %d, want 405", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != "GET, PATCH" {
			t.Errorf("%s: Allow = %q, want \"GET, PATCH\"", method, got)
		}
		assertNoCORS(t, rec)
	}
}

// A directory at the store's path makes every save fail: the rename over it
// is refused, even as root.
func TestPatchSaveFailureIs500(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui-settings.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	store := Open(path, logger)
	h := Handler(store, logger)
	rec := serve(h, newPatch("csl.this", "http://csl.this",
		`{"v":1,"settings":{"webkit-theme":"dark"}}`))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", rec.Code)
	}
	if got := store.Document().Settings; len(got) != 0 {
		t.Errorf("store = %v, want unchanged", got)
	}
	if !strings.Contains(buf.String(), "save settings") {
		t.Errorf("logged %q, want the save error", buf.String())
	}
}
