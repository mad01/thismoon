package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

// testInfo is the build metadata the test servers report on /version.
var testInfo = buildinfo.Info{
	Version:   "test",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "catalog/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

func fixtureCatalog() *catalog.Catalog {
	return catalog.NewCatalog([]catalog.Entity{
		{Kind: catalog.KindSystem, Metadata: catalog.Metadata{Name: "dotfiles", Description: "tooling"}, Spec: catalog.Spec{Owner: "mad01"}},
		{Kind: catalog.KindComponent, Metadata: catalog.Metadata{Name: "present"}, Spec: catalog.Spec{Owner: "mad01", System: "dotfiles", Type: "cli"}},
		{Kind: catalog.KindComponent, Metadata: catalog.Metadata{Name: "csl"}, Spec: catalog.Spec{Owner: "alice", System: "code-search-local"}},
	})
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadOnlyEndpoints(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()

	tests := []struct {
		name      string
		path      string
		wantCount int
	}{
		{"entities", "/api/entities", 3},
		{"systems", "/api/systems", 1},
		{"components", "/api/components", 2},
		{"search by owner", "/api/search?owner=mad01", 2},
		{"search by text", "/api/search?q=pres", 1},
		{"search by kind", "/api/search?kind=System", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doGet(t, h, tt.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
			}
			var got []catalog.Entity
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("got %d entities, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestHandleSystem(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	rec := doGet(t, h, "/api/systems/dotfiles")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var view systemView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.System.Metadata.Name != "dotfiles" {
		t.Errorf("system = %q", view.System.Metadata.Name)
	}
	if len(view.Components) != 1 || view.Components[0].Metadata.Name != "present" {
		t.Errorf("components = %+v", view.Components)
	}
}

func TestHandleComponent_NotFound(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	rec := doGet(t, h, "/api/components/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleOwners(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	rec := doGet(t, h, "/api/owners")
	var owners []string
	_ = json.Unmarshal(rec.Body.Bytes(), &owners)
	if len(owners) != 2 {
		t.Errorf("owners = %v, want 2", owners)
	}
}

func TestHealthz(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	rec := doGet(t, h, "/healthz")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}

// newTestServer builds a server backed by a real registry + repo on disk so
// refresh and add (which re-scan) can be exercised end to end.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	repo := t.TempDir()
	writeServiceInfo(t, filepath.Join(repo, "service-info.yaml"), `
kind: System
metadata:
  name: demo
spec:
  owner: mad01
`)
	registry := filepath.Join(t.TempDir(), "registry.yaml")
	if err := os.WriteFile(registry, []byte("sources:\n  - path: "+repo+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), registry, testInfo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, repo
}

func writeServiceInfo(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleRefresh(t *testing.T) {
	s, repo := newTestServer(t)
	h := s.Handler()

	// Add a component file on disk after the server has loaded.
	writeServiceInfo(t, filepath.Join(repo, "tool", "service-info.yaml"), `
kind: Component
metadata:
  name: demo-tool
spec:
  owner: mad01
  system: demo
`)
	// Before refresh, the new component is absent.
	if rec := doGet(t, h, "/api/components"); strings.Contains(rec.Body.String(), "demo-tool") {
		t.Fatal("component visible before refresh")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", rec.Code)
	}

	if rec := doGet(t, h, "/api/components"); !strings.Contains(rec.Body.String(), "demo-tool") {
		t.Error("component not visible after refresh")
	}
}

func TestHandleAdd(t *testing.T) {
	s, repo := newTestServer(t)
	h := s.Handler()

	body := `{"dir":"` + filepath.Join(repo, "newtool") + `","kind":"Component","name":"newtool","owner":"mad01","type":"cli","system":"demo"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/entities", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}
	// File written and visible after the implicit reload.
	if _, err := os.Stat(filepath.Join(repo, "newtool", "service-info.yaml")); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if rec := doGet(t, h, "/api/components"); !strings.Contains(rec.Body.String(), "newtool") {
		t.Error("added component not visible")
	}
}

func TestHandleAdd_RejectsOutsideRoots(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()

	outside := filepath.Join(t.TempDir(), "evil")
	body := `{"dir":"` + outside + `","kind":"Component","name":"evil","owner":"mad01","system":"demo"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/entities", strings.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestWebkitServed asserts the shared webkit chrome is served at /webkit/ so the
// SPA shell can link /webkit/webkit.css and /webkit/webkit.js.
func TestWebkitServed(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	for _, path := range []string{"/webkit/webkit.css", "/webkit/webkit.js"} {
		rec := doGet(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
	}
}

// TestIndexUsesWebkit asserts the served SPA shell wires up the webkit chrome.
func TestIndexUsesWebkit(t *testing.T) {
	h := newServerWithCatalog(fixtureCatalog(), nil).Handler()
	body := doGet(t, h, "/").Body.String()
	for _, want := range []string{"<wk-header", "/webkit/webkit.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("index body missing %q", want)
		}
	}
}

func TestWithinRoots(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		dir  string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "sub"), true},
		{filepath.Join(root, "a", "b"), true},
		{filepath.Dir(root), false},
		{"/etc", false},
	}
	for _, tt := range tests {
		if got := withinRoots(tt.dir, []string{root}); got != tt.want {
			t.Errorf("withinRoots(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

// TestNewMissingRegistry asserts a fresh install (no registry file) still gets
// a serving server with an empty catalog, while a malformed registry stays a
// hard error.
func TestNewMissingRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")

	srv, err := New(context.Background(), path, testInfo)
	if err != nil {
		t.Fatalf("New with missing registry: %v, want nil error", err)
	}
	rec := doGet(t, srv.Handler(), "/api/entities")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/entities = %d, want %d", rec.Code, http.StatusOK)
	}
	var entities []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &entities); err != nil {
		t.Fatalf("decode entities: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestNewMalformedRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	if err := os.WriteFile(path, []byte("sources: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), path, testInfo); err == nil {
		t.Fatal("New with malformed registry: nil error, want parse error")
	}
}

// TestVersionEndpoint pins the cross-tool build metadata contract: the four
// keys, the injected values, and the headers ralph and status probe with.
func TestVersionEndpoint(t *testing.T) {
	s := newServerWithCatalog(fixtureCatalog(), nil)
	s.info = testInfo

	rec := doGet(t, s.Handler(), "/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
	}
	want := map[string]string{
		"version":    testInfo.Version,
		"commit":     testInfo.Commit,
		"tag":        testInfo.Tag,
		"build_time": testInfo.BuildTime,
	}
	if len(got) != len(want) {
		t.Errorf("version body = %q, want exactly the keys %v", rec.Body.String(), want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("version[%q] = %q, want %q", k, got[k], v)
		}
	}
}
