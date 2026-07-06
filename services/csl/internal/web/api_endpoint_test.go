package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// fakeSearcher is an in-memory searcher for handler tests.
type fakeSearcher struct {
	matches      []search.Match
	repos        []finder.Repo
	read         *ReadResult
	searchErr    error
	readErr      error
	semResult    SemanticResult
	semErr       error
	hybridResult HybridResult
	hybridErr    error
}

func (f *fakeSearcher) Search(_ context.Context, _ search.SearchOptions) ([]search.Match, error) {
	return f.matches, f.searchErr
}
func (f *fakeSearcher) SemanticSearch(_ context.Context, _ SemanticRequest) (SemanticResult, error) {
	return f.semResult, f.semErr
}
func (f *fakeSearcher) HybridSearch(_ context.Context, _ HybridRequest) (HybridResult, error) {
	return f.hybridResult, f.hybridErr
}
func (f *fakeSearcher) Repos() ([]finder.Repo, error) { return f.repos, nil }
func (f *fakeSearcher) ReadFile(_, _ string, _, _ int) (*ReadResult, error) {
	return f.read, f.readErr
}

func serverWith(f *fakeSearcher) http.Handler {
	return (&Server{svc: f}).Handler()
}

func TestSearchEndpointGroupsAndTruncates(t *testing.T) {
	fake := &fakeSearcher{
		matches: matchesAcrossFiles("mad01/thismoon", 3),
		repos:   []finder.Repo{{Name: "mad01/thismoon", Host: "github.com"}},
	}
	// limit=3 with 3 files → truncated.
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=foo&limit=3", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
	if resp.Files != 3 {
		t.Errorf("Files = %d, want 3", resp.Files)
	}
	if resp.Limit != 3 {
		t.Errorf("Limit = %d, want 3 (echoed from request)", resp.Limit)
	}
	if !resp.Truncated {
		t.Errorf("Truncated = false, want true (files == limit)")
	}
	if len(resp.Repos) != 1 || resp.Repos[0].Host != "github.com" {
		t.Fatalf("repo grouping wrong: %+v", resp.Repos)
	}
	if got := resp.Repos[0].Files[0].Matches[0].RemoteURL; got == "" {
		t.Errorf("expected a remote URL on the match, got empty")
	}
}

func TestSearchEndpointNotTruncated(t *testing.T) {
	fake := &fakeSearcher{
		matches: matchesAcrossFiles("mad01/thismoon", 2),
		repos:   []finder.Repo{{Name: "mad01/thismoon", Host: "github.com"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=foo&limit=50", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Truncated {
		t.Errorf("Truncated = true, want false (2 files < limit 50)")
	}
}

func TestSearchEndpointEmptyReposIsArray(t *testing.T) {
	fake := &fakeSearcher{
		repos: []finder.Repo{{Name: "mad01/thismoon", Host: "github.com"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=nope", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	// repos must be [] not null so the UI can call data.repos.length.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["repos"]) != "[]" {
		t.Errorf("repos = %s, want []", raw["repos"])
	}
}

func TestSearchEndpointLimitClampedToMax(t *testing.T) {
	fake := &fakeSearcher{repos: []finder.Repo{{Name: "r", Host: "github.com"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=foo&limit=99999", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	var resp searchResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Limit != maxLimit {
		t.Errorf("Limit = %d, want clamped to %d", resp.Limit, maxLimit)
	}
}

func TestReadEndpoint(t *testing.T) {
	fake := &fakeSearcher{read: &ReadResult{
		Repo:  "mad01/thismoon",
		Path:  "main.go",
		Lines: []FileLine{{Number: 1, Text: "package main"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/read?repo=x&file=main.go", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var rr ReadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &rr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rr.Lines) != 1 || rr.Lines[0].Text != "package main" {
		t.Errorf("unexpected read result: %+v", rr)
	}
}

func TestReposEndpoint(t *testing.T) {
	fake := &fakeSearcher{repos: []finder.Repo{
		{
			Name:   "mad01/thismoon",
			Host:   "github.com",
			Remote: "git@github.com:mad01/thismoon.git",
		},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var repos []repoJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &repos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(repos) != 1 || repos[0].Host != "github.com" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}
