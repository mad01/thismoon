package web

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/services/csl/internal/hybrid"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// HybridRequest carries the parsed parameters for a hybrid search.
type HybridRequest struct {
	Query  string
	Limit  int
	RRFK   int
	Repo   string
	Lang   string
	Expand int
}

// HybridHit is one fused result handed to the API layer. A zero LexRank or
// SemRank means the file was absent from that backend's results.
type HybridHit struct {
	Repo     string
	Path     string
	Score    float64
	LexRank  int
	LexLine  int
	LexText  string
	SemRank  int
	SemStart int
	SemEnd   int
	SemScore float32
	Snippet  string
}

// HybridResult is the outcome of a hybrid search. When SemanticAvailable is
// false, Note explains why and hits are lexical-only (degraded, not an error).
type HybridResult struct {
	SemanticAvailable bool
	Note              string
	Hits              []HybridHit
}

// HybridSearch runs a hybrid query: lexical results from Search, semantic
// results from SemanticSearch, fused by Reciprocal Rank Fusion. A missing
// semantic index is reported as SemanticAvailable=false with a note; the fuse
// still runs with lexical-only input.
func (s *Service) HybridSearch(ctx context.Context, req HybridRequest) (HybridResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	k := req.RRFK
	if k <= 0 {
		k = hybrid.DefaultK
	}

	lexMatches, err := s.Search(ctx, search.SearchOptions{
		Pattern:    req.Query,
		RepoFilter: req.Repo,
		Lang:       req.Lang,
		Limit:      limit,
		OutputMode: "content",
	})
	if err != nil {
		return HybridResult{}, fmt.Errorf("hybrid lexical search: %w", err)
	}

	semResult, err := s.SemanticSearch(ctx, SemanticRequest{
		Query:  req.Query,
		K:      limit,
		Repo:   req.Repo,
		Lang:   req.Lang,
		Expand: req.Expand,
	})
	if err != nil {
		return HybridResult{}, fmt.Errorf("hybrid semantic search: %w", err)
	}

	// Convert SemanticHit → semantic.Result for Fuse. When semantic is
	// unavailable, semResult.Hits is empty, so Fuse returns lexical-only.
	semResults := make([]semantic.Result, len(semResult.Hits))
	for i, h := range semResult.Hits {
		semResults[i] = semantic.Result{
			Hit: semantic.Hit{
				Repo:      h.Repo,
				Path:      h.Path,
				Lang:      h.Lang,
				Kind:      h.Kind,
				StartLine: h.StartLine,
				EndLine:   h.EndLine,
				Score:     h.Score,
			},
			Snippet: h.Snippet,
		}
	}

	fused := hybrid.Fuse(lexMatches, semResults, k, limit)

	hits := make([]HybridHit, len(fused))
	for i, f := range fused {
		hits[i] = HybridHit{
			Repo:     f.Repo,
			Path:     f.Path,
			Score:    f.Score,
			LexRank:  f.LexRank,
			LexLine:  f.LexLine,
			LexText:  f.LexText,
			SemRank:  f.SemRank,
			SemStart: f.SemStart,
			SemEnd:   f.SemEnd,
			SemScore: f.SemScore,
			Snippet:  f.Snippet,
		}
	}

	return HybridResult{
		SemanticAvailable: semResult.Available,
		Note:              semResult.Note,
		Hits:              hits,
	}, nil
}
