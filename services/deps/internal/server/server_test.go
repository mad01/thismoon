package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test-sha",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "deps/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

// testServer builds a Server over a temp store seeded with one flagged dep and
// one clean dep. The engine is nil — the page/API/version routes under test
// never reach it (only the POST scan/check routes do).
func testServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]store.Dependency{
		{
			Ecosystem: "Go", Name: "golang.org/x/net", Version: "0.17.0",
			Repo: "/home/u/code/widget", Direct: true, Imported: true,
			Advisories: []store.Advisory{
				{ID: "GO-2024-0001", Summary: "header smuggling", Severity: "high", FixedVersion: "0.23.0"},
			},
		},
		{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Repo: "/home/u/code/widget", Direct: true, Imported: true},
	}); err != nil {
		t.Fatal(err)
	}
	return New(st, nil, testInfo)
}

func TestPageServesShell(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	// The page is a static shell: chrome + an empty mount point + the client
	// scripts. The body is rendered in the browser, so no dep data inlined.
	for _, want := range []string{`id="app"`, "/app.js", "/webkit/webkit.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	for _, absent := range []string{"golang.org/x/net", "GO-2024-0001", "Active findings"} {
		if strings.Contains(body, absent) {
			t.Errorf("shell should not inline dep data, found %q", absent)
		}
	}
}

func TestAppJS(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /app.js = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/deps") {
		t.Error("app.js should fetch /api/deps")
	}
}

func TestAPIDeps(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/deps", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /api/deps = %d", rec.Code)
	}
	var got struct {
		ScannedAt    string `json:"scanned_at"`
		Total        int    `json:"total"`
		FlaggedCount int    `json:"flagged_count"`
		Deps         []struct {
			Ecosystem  string `json:"ecosystem"`
			Name       string `json:"name"`
			Version    string `json:"version"`
			Repo       string `json:"repo"`
			Direct     bool   `json:"direct"`
			Advisories []struct {
				ID           string `json:"id"`
				FixedVersion string `json:"fixed_version"`
				Key          string `json:"key"`
				Resolved     bool   `json:"resolved"`
			} `json:"advisories"`
		} `json:"deps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.FlaggedCount != 1 {
		t.Errorf("api/deps = total %d / flagged %d, want 2 / 1", got.Total, got.FlaggedCount)
	}
	if len(got.Deps) != 2 {
		t.Fatalf("api/deps returned %d deps, want 2", len(got.Deps))
	}

	// app.js renders from these field names, so the JSON shape is the contract.
	body := rec.Body.String()
	for _, want := range []string{
		`"scanned_at"`, `"total"`, `"flagged_count"`, `"deps"`,
		`"ecosystem"`, `"name"`, `"version"`, `"repo"`, `"direct"`,
		`"advisories"`, `"id"`, `"fixed_version"`, `"key"`, `"resolved"`,
		`"imported"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api/deps JSON missing field %s", want)
		}
	}
}

// TestAPIDepsExcludesNonImportedFromFlagged confirms a flagged-but-not-imported
// (graph-only transitive) dep does not count toward flagged_count — it isn't a
// real exposure, so it's informational only.
func TestAPIDepsExcludesNonImportedFromFlagged(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]store.Dependency{{
		Ecosystem: "Go", Name: "github.com/graph/only", Version: "1.0.0",
		Repo: "/home/u/code/widget", Imported: false,
		Advisories: []store.Advisory{{ID: "GO-2024-9999", FixedVersion: "1.1.0"}},
	}}); err != nil {
		t.Fatal(err)
	}
	h := New(st, nil, testInfo).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/deps", nil))

	var got struct {
		Total        int `json:"total"`
		FlaggedCount int `json:"flagged_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Errorf("total = %d, want 1", got.Total)
	}
	if got.FlaggedCount != 0 {
		t.Errorf("flagged_count = %d, want 0 (graph-only dep excluded)", got.FlaggedCount)
	}
}

// TestVersionAndHealthz pins the cross-tool build metadata contract: the four
// keys, the injected values, and the headers ralph and status probe with.
func TestVersionAndHealthz(t *testing.T) {
	h := testServer(t).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var v map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"version":    testInfo.Version,
		"commit":     testInfo.Commit,
		"tag":        testInfo.Tag,
		"build_time": testInfo.BuildTime,
	}
	if len(v) != len(want) {
		t.Errorf("version body = %q, want exactly the keys %v", rec.Body.Bytes(), want)
	}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("version[%q] = %q, want %q", k, v[k], w)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 204 {
		t.Errorf("GET /healthz = %d, want 204", rec.Code)
	}
}
