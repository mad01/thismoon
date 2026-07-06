package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

func TestSemanticSearchMissingQuery(t *testing.T) {
	// q is validated before the service is touched, so a nil-svc server suffices.
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/semantic_search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSemanticSearchUnavailableNote(t *testing.T) {
	fake := &fakeSearcher{
		repos:     []finder.Repo{{Name: "mad01/thismoon", Host: "github.com"}},
		semResult: SemanticResult{Available: false, Note: semanticNotBuiltNote},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/semantic_search?q=retry+a+request", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp semanticResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Available {
		t.Errorf("Available = true, want false")
	}
	if resp.Note != semanticNotBuiltNote {
		t.Errorf("Note = %q, want build hint", resp.Note)
	}
	if resp.Query != "retry a request" {
		t.Errorf("Query = %q, want echoed", resp.Query)
	}
	// hits must marshal as [] not null so the UI can read data.hits.length.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if string(raw["hits"]) != "[]" {
		t.Errorf("hits = %s, want []", raw["hits"])
	}
}

func TestSemanticSearchHits(t *testing.T) {
	fake := &fakeSearcher{
		repos: []finder.Repo{{
			Name:   "mad01/thismoon",
			Host:   "github.com",
			Remote: "git@github.com:mad01/thismoon.git",
		}},
		semResult: SemanticResult{Available: true, Hits: []SemanticHit{{
			Repo:      "mad01/thismoon",
			Path:      "internal/web/server.go",
			Lang:      "go",
			Kind:      "func",
			StartLine: 35,
			EndLine:   45,
			Score:     0.87,
			Snippet:   "func (s *Server) Handler() http.Handler {",
		}}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/semantic_search?q=build+http+routes&k=5", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp semanticResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Available || len(resp.Hits) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	h := resp.Hits[0]
	if h.Path != "internal/web/server.go" || h.StartLine != 35 || h.EndLine != 45 {
		t.Errorf("hit location wrong: %+v", h)
	}
	if h.Kind != "func" || h.Score != 0.87 {
		t.Errorf("hit kind/score wrong: %+v", h)
	}
	if h.Snippet == "" {
		t.Errorf("expected a snippet")
	}
	// repo metadata is present, so a remote source link should be built.
	if h.FileURL == "" {
		t.Errorf("expected a remote URL on the hit, got empty")
	}
}
