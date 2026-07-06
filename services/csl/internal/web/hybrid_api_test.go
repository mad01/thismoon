package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

func TestHybridSearchMissingQuery(t *testing.T) {
	// q is validated before the service is touched, so a nil-svc server suffices.
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hybrid_search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHybridSearchHits(t *testing.T) {
	fake := &fakeSearcher{
		repos: []finder.Repo{{
			Name:   "mad01/thismoon",
			Host:   "github.com",
			Remote: "git@github.com:mad01/thismoon.git",
		}},
		hybridResult: HybridResult{
			SemanticAvailable: true,
			Hits: []HybridHit{{
				Repo:     "mad01/thismoon",
				Path:     "internal/hybrid/fuse.go",
				Score:    0.0164,
				LexRank:  1,
				LexLine:  45,
				LexText:  "func Fuse(lexical []search.Match, sem []semantic.Result, k, limit int) []FusedHit {",
				SemRank:  2,
				SemStart: 1,
				SemEnd:   113,
				SemScore: 0.72,
				Snippet:  "// Package hybrid fuses lexical and semantic search",
			}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/hybrid_search?q=reciprocal+rank+fusion", nil)
	rec := httptest.NewRecorder()
	serverWith(fake).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp hybridResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.SemanticAvailable || len(resp.Hits) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	h := resp.Hits[0]
	if h.Path != "internal/hybrid/fuse.go" {
		t.Errorf("path = %q, want internal/hybrid/fuse.go", h.Path)
	}
	if h.LexRank != 1 || h.SemRank != 2 {
		t.Errorf("ranks wrong: lex=%d sem=%d, want lex=1 sem=2", h.LexRank, h.SemRank)
	}
	if h.Score == 0 {
		t.Errorf("score should be non-zero")
	}
	if h.Snippet == "" {
		t.Errorf("expected a snippet")
	}
	// repo metadata is present, so a remote source link should be built.
	if h.FileURL == "" {
		t.Errorf("expected a remote URL on the hit, got empty")
	}
}
