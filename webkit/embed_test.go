package webkit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// serve runs a request through the mounted handler the way consumers mount it:
//
//	mux.Handle("GET /webkit/", webkit.Handler())
func serve(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	webkit.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHandlerServesCSS(t *testing.T) {
	rec := serve(t, "/webkit/webkit.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/webkit.css: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "css") {
		t.Errorf("Content-Type = %q, want to contain \"css\"", ct)
	}
	body := rec.Body.String()
	// Component spec: every component must be styled by its selector in webkit.css.
	// Adding a row here first (RED) drives porting the component's CSS (GREEN).
	for _, want := range []string{
		// palette
		"--page-width", "--terracotta", "--mono",
		// existing components
		"wk-card", "wk-panel", "wk-badge", "wk-search", "wk-table",
		// interactive: filter chip + segmented control
		"wk-badge[variant=\"filter\"]", "wk-badge[active]", "wk-search:has(", "wk-seg",
		// content blocks
		"wk-kv", "wk-section", "wk-callout", "wk-progress", "wk-toc",
		// rich markdown / prose + code block (issue #37)
		".wk-prose", ".wk-code-block", ".wk-code-lang", ".wk-code-copy", ".token.keyword",
		// standalone code block (outside .wk-prose): padding + code reset + token colors
		"pre.wk-code-block > code", ".wk-code-block .token.keyword",
		// terminal-style frame: pinned dark palette + header bar traffic dots
		"--code-bg", ".wk-code-controls::before",
		// overlays
		"wk-modal", "wk-modal-body", "wk-form", "wk-toast",
		// danger button variant (destructive confirm dialogs)
		"wk-button[variant=\"danger\"]",
		// ⌘K site picker
		".wk-cmdk-overlay", ".wk-cmdk-item", ".ctrl-kbd",
		// feature guide (help modal)
		".wk-help-row", ".wk-help-close", ".wk-help-section-title",
		// read aloud, plus prepared mode's page bar, section badge and text buttons
		"wk-read-aloud", ".wk-ra-btn", ".wk-ra-sentence",
		".wk-ra-bar", ".wk-ra-bar-actions", "wk-badge.wk-ra-state", ".wk-ra-btn.wk-ra-text",
		// a11y
		"prefers-reduced-motion", ":focus-visible",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("webkit.css missing expected token %q", want)
		}
	}
}

func TestHandlerServesJS(t *testing.T) {
	rec := serve(t, "/webkit/webkit.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/webkit.js: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want to contain \"javascript\"", ct)
	}
	body := rec.Body.String()
	// Every JS-registered custom element must self-register via customElements.define.
	for _, want := range []string{
		"Webkit", "init", "wk-header",
		"wk-seg", "wk-kv", "wk-section", "wk-callout",
		"wk-progress", "wk-toc", "wk-modal", "wk-form", "wk-toast",
		"wk-read-aloud", "v1/audio/speech",
		// prepared mode: the attribute, speak's document API, the status UI classes
		`hasAttribute("prepare")`, `"/read"`, "/prepare", "data-ra-chunk", "wk-ra-bar", "wk-ra-state",
		// ⌘K site picker
		"wk-cmdk:open", "__this/sites.json", "webkit-cmdk",
		// feature guide (help modal)
		"webkit-help", "data-wk-help", "Feature guide",
		// rich markdown / prose (issue #37): client-side highlight + copy enhancer,
		// covering both .wk-prose bodies and standalone pre.wk-code-block
		"enhanceProse", "wk-code-copy", "wk-prose",
		`pre.wk-code-block > code[class*="language-"]`,
		// version-poll soft-reload (issue #1)
		"/webkit/version",
		// shared client render helpers (CSR migration): escapeHtml + el + poll
		"escapeHtml", "createTextNode", "&amp;", "request failed (",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("webkit.js missing expected token %q", want)
		}
	}
}

func TestHandlerRevalidatesAssets(t *testing.T) {
	rec := serve(t, "/webkit/webkit.css")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf(
			"Cache-Control = %q, want no-cache (stale-asset bug: immutable on unversioned URLs)",
			cc,
		)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag missing on asset response")
	}

	req := httptest.NewRequest(http.MethodGet, "/webkit/webkit.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	webkit.Handler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("conditional GET: status = %d, want 304", rec2.Code)
	}
}

// TestAssetETagIsContentHash verifies the cache-busting fix (issue #1): the
// asset ETag is the content hash, not the pinned pseudo-version, and
// GET /webkit/version returns that exact same value so the in-page poll can
// detect a redeploy.
func TestAssetETagIsContentHash(t *testing.T) {
	asset := serve(t, "/webkit/webkit.css")
	etag := strings.Trim(asset.Header().Get("ETag"), `"`)
	if etag == "" {
		t.Fatal("asset ETag missing")
	}
	// Content hash is a 16-char lowercase hex digest, never "(devel)" or a
	// pseudo-version with slashes/dots.
	if len(etag) != 16 {
		t.Errorf("ETag = %q, want a 16-char content hash", etag)
	}
	if strings.ContainsAny(etag, "/.-") || etag == "devel" {
		t.Errorf("ETag = %q looks like a version string, want a content hash", etag)
	}

	verRec := serve(t, "/webkit/version")
	var ver map[string]string
	if err := json.Unmarshal(verRec.Body.Bytes(), &ver); err != nil {
		t.Fatalf("version body not JSON: %v", err)
	}
	if ver["version"] != etag {
		t.Errorf(
			"/webkit/version version = %q, want it to equal the asset ETag %q",
			ver["version"],
			etag,
		)
	}
}

func TestHandlerServesVersion(t *testing.T) {
	rec := serve(t, "/webkit/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/version: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, want to contain \"json\"", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want \"no-store\"", cc)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v (body=%q)", err, rec.Body.String())
	}
	if got["module"] != "github.com/mad01/thismoon/webkit" {
		t.Errorf("module = %q, want %q", got["module"], "github.com/mad01/thismoon/webkit")
	}
	// The version is a content hash over the embedded dist/ bytes; assert it
	// is present and non-empty rather than pinning a specific digest.
	if got["version"] == "" {
		t.Errorf("version is empty, want a non-empty string")
	}
}

// TestHandlerServesBootJS verifies the FOUC-guard boot snippet is served as a
// standalone classic script at GET /webkit/boot.js (single-sourced from
// src/boot.snippet.js via the build), so consumers can <script src> it instead
// of hand-pasting the inline IIFE.
func TestHandlerServesBootJS(t *testing.T) {
	rec := serve(t, "/webkit/boot.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/boot.js: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want to contain \"javascript\"", ct)
	}
	body := rec.Body.String()
	// Must be the runnable IIFE (no module syntax) that applies persisted theme.
	for _, want := range []string{
		"(function(){", "webkit-theme", "data-theme", "webkit-size", "fontSize",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("boot.js missing expected token %q", want)
		}
	}
	if strings.Contains(body, "export") || strings.Contains(body, "import") {
		t.Errorf("boot.js must be a classic script, found module syntax")
	}
}

// TestBootJSInEmbeddedFS confirms boot.js is actually committed into the
// embedded dist FS (not just emitted at build time).
func TestBootJSInEmbeddedFS(t *testing.T) {
	f, err := webkit.FS().Open("boot.js")
	if err != nil {
		t.Fatalf("FS().Open(\"boot.js\"): %v", err)
	}
	_ = f.Close()
}

// TestBootScript verifies the Go helper emits exactly the tag consumers inject
// in <head> before webkit.js.
func TestBootScript(t *testing.T) {
	if got := string(webkit.BootScript()); got != `<script src="/webkit/boot.js"></script>` {
		t.Errorf("BootScript() = %q", got)
	}
}

// TestMount verifies the convenience wiring serves assets at the /webkit/ prefix.
func TestMount(t *testing.T) {
	mux := http.NewServeMux()
	webkit.Mount(mux)
	req := httptest.NewRequest(http.MethodGet, "/webkit/boot.js", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Mount: GET /webkit/boot.js status = %d, want 200", rec.Code)
	}
}

func TestHandlerUnknownPath404(t *testing.T) {
	rec := serve(t, "/webkit/nope.txt")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /webkit/nope.txt: status = %d, want 404", rec.Code)
	}
}
