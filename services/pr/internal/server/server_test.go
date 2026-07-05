package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/pr/internal/config"
	"github.com/mad01/thismoon/services/pr/internal/github"
	"github.com/mad01/thismoon/services/pr/internal/store"
)

// testMux builds a mux backed by a store holding one open PR, so /api/prs has
// data while the page route still serves the static shell.
func testMux(t *testing.T) *http.ServeMux {
	t.Helper()
	st, err := store.Load(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	st.Put("github.com/mad01/dotfiles", &store.RepoState{
		PRs: []store.CachedPR{{
			Number:    1,
			Title:     "Test PR title",
			State:     "open",
			HTMLURL:   "https://github.com/mad01/dotfiles/pull/1",
			Author:    "alice",
			CreatedAt: time.Now().Add(-time.Hour),
			HeadRef:   "feat/x",
			BaseRef:   "main",
		}},
		FetchedAt: time.Now(),
	})
	cfg := &config.Config{
		Sources: []config.Source{{Host: "github.com", Owner: "mad01", Repos: []string{"dotfiles"}}},
	}
	client := github.NewClient()
	poller := newPoller(cfg, client, st)
	return newMux(poller, client, cfg, "test-sha")
}

func TestPageServesShell(t *testing.T) {
	rec := httptest.NewRecorder()
	testMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	// The page is a static shell: chrome + an empty mount point + the client
	// scripts. The PR list is rendered in the browser, so no PR data is inlined.
	for _, want := range []string{`id="app"`, "/app.js", "/webkit/webkit.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	for _, absent := range []string{"Test PR title", "feat/x", "alice"} {
		if strings.Contains(body, absent) {
			t.Errorf("shell should not inline PR data, found %q", absent)
		}
	}
}

func TestAppJS(t *testing.T) {
	rec := httptest.NewRecorder()
	testMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /app.js = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/prs") {
		t.Error("app.js should fetch /api/prs")
	}
}

func TestAPIPRs(t *testing.T) {
	rec := httptest.NewRecorder()
	testMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/api/prs", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /api/prs = %d", rec.Code)
	}
	var snap []RepoPRs
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 || len(snap[0].PRs) != 1 || snap[0].PRs[0].Title != "Test PR title" {
		t.Fatalf("snapshot = %+v, want 1 repo with 1 PR titled Test PR title", snap)
	}

	// app.js renders from these field names, so the JSON shape is the contract.
	body := rec.Body.String()
	for _, want := range []string{
		`"Repo"`, `"Owner"`, `"Name"`, `"Host"`, `"PRs"`,
		`"number"`, `"title"`, `"state"`, `"html_url"`, `"author"`,
		`"created_at"`, `"head_ref"`, `"base_ref"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api/prs JSON missing field %s", want)
		}
	}
}

func TestDetailServesShell(t *testing.T) {
	rec := httptest.NewRecorder()
	testMux(t).ServeHTTP(rec,
		httptest.NewRequest("GET", "/pr/github.com/mad01/dotfiles/1", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /pr/... = %d", rec.Code)
	}
	body := rec.Body.String()
	// Like the list page, the detail page is a static shell: chrome + an empty
	// mount point + detail.js. The PR is fetched and rendered in the browser, so
	// no PR data is inlined.
	for _, want := range []string{`id="detail-app"`, "/detail.js", "/webkit/webkit.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail shell missing %q", want)
		}
	}
}

func TestDetailJS(t *testing.T) {
	rec := httptest.NewRecorder()
	testMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/detail.js", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /detail.js = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/pr/") {
		t.Error("detail.js should fetch /api/pr/")
	}
}

func TestVersionAndHealthz(t *testing.T) {
	mux := testMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	var v map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v["version"] != "test-sha" {
		t.Errorf("version = %q", v["version"])
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 204 {
		t.Errorf("GET /healthz = %d, want 204", rec.Code)
	}
}
