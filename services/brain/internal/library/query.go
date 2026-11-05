package library

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/rerank"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Defaults for the queries when the caller passes zero.
const (
	defaultSearchLimit = 50
	defaultK           = 20
	defaultKCandidates = 30
	// minRerank is the fewest fused hits worth an LLM rerank.
	minRerank = 4
)

// Search runs a zoekt query against each vault's literal index, refreshing
// stale ones first, and merges the hits by score into at most opts.Limit
// notes. Every hit carries its vault.
func (l *Library) Search(
	ctx context.Context,
	vaults *vault.Set,
	opts search.SearchOptions,
) (search.Result, error) {
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}
	if opts.OutputMode == "" {
		opts.OutputMode = search.ModeFiles
	}
	lists := make([][]search.Match, 0, len(vaults.Vaults()))
	for _, v := range vaults.Vaults() {
		dir, err := l.literalDir(v)
		if err != nil {
			return search.Result{}, err
		}
		matches, err := search.Search(ctx, dir, opts)
		if err != nil {
			return search.Result{}, err
		}
		for i := range matches {
			matches[i].Vault = v.Name
		}
		lists = append(lists, matches)
	}
	merged := search.MergeMatches(lists, opts.Limit)
	return search.NewResult(l.roots(), opts.OutputMode, merged), nil
}

// Count totals a zoekt query across vaults. GroupBy may be tier, tag or
// vault.
func (l *Library) Count(
	ctx context.Context,
	vaults *vault.Set,
	opts search.CountOptions,
) (search.Counts, error) {
	group := opts.GroupBy
	if group == search.GroupVault {
		opts.GroupBy = ""
	}
	res := search.Counts{}
	var lists [][]search.CountResult
	for _, v := range vaults.Vaults() {
		dir, err := l.literalDir(v)
		if err != nil {
			return search.Counts{}, err
		}
		groups, total, err := search.Count(ctx, dir, opts)
		if err != nil {
			return search.Counts{}, err
		}
		res.Total += total
		if group == search.GroupVault {
			groups = []search.CountResult{{Group: v.Name, Count: total}}
		}
		lists = append(lists, groups)
	}
	if group != "" {
		res.Groups = search.MergeCounts(lists...)
	}
	return res, nil
}

// SemanticQuery is one semantic search across vaults.
type SemanticQuery struct {
	// Text is the natural-language query.
	Text string
	// Mode is semantic.ModeSnippets (default) or semantic.ModePaths.
	Mode string
	// Filter narrows the hits; Filter.K caps them (default 20).
	Filter semantic.QueryOptions
}

// SemanticSearch runs a vector query against each vault's semantic index,
// refreshing stale indexes first, and merges the hits by score.
func (l *Library) SemanticSearch(
	ctx context.Context,
	vaults *vault.Set,
	q SemanticQuery,
) (semantic.SearchResult, error) {
	if !l.cfg.Semantic.Enabled {
		return semantic.SearchResult{}, ErrSemanticDisabled
	}
	if strings.TrimSpace(q.Text) == "" {
		return semantic.SearchResult{}, errors.New("query is required")
	}
	if q.Filter.K <= 0 {
		q.Filter.K = defaultK
	}
	if q.Mode == "" {
		q.Mode = semantic.ModeSnippets
	}
	warnings, err := l.refreshSemantic(ctx, vaults)
	if err != nil {
		return semantic.SearchResult{}, err
	}
	hits, err := l.semanticHits(ctx, vaults, q.Text, q.Filter)
	if err != nil {
		return semantic.SearchResult{}, err
	}
	res := semantic.NewSearchResult(q.Mode, hits)
	res.Warnings = warnings
	return res, nil
}

// semanticHits queries each vault's semantic index and merges the hits.
func (l *Library) semanticHits(
	ctx context.Context,
	vaults *vault.Set,
	text string,
	filter semantic.QueryOptions,
) ([]semantic.Hit, error) {
	lists := make([][]semantic.Hit, 0, len(vaults.Vaults()))
	for _, v := range vaults.Vaults() {
		idx, err := l.openSemantic(v)
		if err != nil {
			return nil, fmt.Errorf("open semantic index of %s: %w", v.Name, err)
		}
		hits, err := idx.Query(ctx, text, filter)
		if err != nil {
			return nil, err
		}
		for i := range hits {
			hits[i].Vault = v.Name
		}
		lists = append(lists, hits)
	}
	return semantic.MergeHits(lists, filter.K), nil
}

// HybridQuery is one hybrid (literal + semantic) search across vaults.
type HybridQuery struct {
	Text     string
	K        int // final hits, default 20
	KZoekt   int // literal candidates before fusion, default 30
	KVector  int // semantic candidates before fusion, default 30
	Tier     string
	Tag      string
	Type     string
	Status   string
	Evidence string
	Rerank   bool
}

func (q *HybridQuery) applyDefaults() {
	if q.K <= 0 {
		q.K = defaultK
	}
	if q.KZoekt <= 0 {
		q.KZoekt = defaultKCandidates
	}
	if q.KVector <= 0 {
		q.KVector = defaultKCandidates
	}
}

// HybridSearch fuses literal and semantic hits by Reciprocal Rank Fusion,
// then optionally reranks them with the LLM. When the semantic side fails
// (Ollama unreachable, say) the result fuses literal hits alone and says so
// in Warnings.
func (l *Library) HybridSearch(
	ctx context.Context,
	vaults *vault.Set,
	q HybridQuery,
) (hybrid.Result, error) {
	if !l.cfg.Semantic.Enabled {
		return hybrid.Result{}, ErrSemanticDisabled
	}
	if strings.TrimSpace(q.Text) == "" {
		return hybrid.Result{}, errors.New("query is required")
	}
	q.applyDefaults()
	var tier vault.Tier
	if q.Tier != "" {
		t, err := vault.ParseTier(q.Tier)
		if err != nil {
			return hybrid.Result{}, err
		}
		tier = t
	}

	warnings, err := l.refreshSemantic(ctx, vaults)
	if err != nil {
		return hybrid.Result{}, err
	}
	semHits, err := l.semanticHits(ctx, vaults, q.Text, semantic.QueryOptions{
		K:        q.KVector,
		Tier:     q.Tier,
		Tag:      q.Tag,
		Type:     q.Type,
		Status:   q.Status,
		Evidence: q.Evidence,
	})
	if err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("semantic search failed, hits are literal only: %v", err),
		)
		semHits = nil
	}

	literal, err := l.Search(ctx, vaults, search.SearchOptions{
		Pattern: q.Text,
		Tag:     q.Tag,
		Type:    q.Type,
		Status:  q.Status,
		Tier:    tier,
		Limit:   q.KZoekt,
	})
	if err != nil {
		return hybrid.Result{}, fmt.Errorf("literal query: %w", err)
	}

	hits := fuse(literal.Files, semHits, q.K)
	if q.Rerank && len(hits) >= minRerank {
		hits = l.rerank(ctx, q.Text, hits)
	}
	return hybrid.NewResult(hits, warnings), nil
}

// hitKey identifies a note across vaults in the fused lists.
func hitKey(vaultName, path string) string { return vaultName + ":" + path }

// fuse RRF-fuses literal notes and semantic chunks by note, keeping the best
// chunk of each note as its representative, and returns the top k.
func fuse(literal []search.FileHit, semHits []semantic.Hit, k int) []hybrid.ResultHit {
	notes := make(map[string]hybrid.ResultHit)
	literalIDs := make([]string, 0, len(literal))
	for _, f := range literal {
		key := hitKey(f.Vault, f.Path)
		literalIDs = append(literalIDs, key)
		notes[key] = hybrid.ResultHit{Vault: f.Vault, Path: f.Path, Evidence: f.Evidence}
	}
	// semHits arrive best first, so the first chunk of a note is its best.
	best := make(map[string]bool)
	vectorIDs := make([]string, 0, len(semHits))
	for _, h := range semHits {
		key := hitKey(h.Vault, h.Path)
		if best[key] {
			continue
		}
		best[key] = true
		vectorIDs = append(vectorIDs, key)
		notes[key] = hybrid.ResultHit{
			Vault:       h.Vault,
			Path:        h.Path,
			Title:       h.Title,
			HeadingPath: h.HeadingPath,
			Snippet:     h.Snippet,
			Tier:        h.Tier,
			Tags:        h.Tags,
			Type:        h.Type,
			Status:      h.Status,
			Evidence:    h.Evidence,
		}
	}
	fused := hybrid.Fuse([]hybrid.Input{
		{Source: "zoekt", IDs: literalIDs},
		{Source: "vector", IDs: vectorIDs},
	}, hybrid.DefaultK)
	out := make([]hybrid.ResultHit, 0, min(k, len(fused)))
	for _, f := range fused {
		if len(out) >= k {
			break
		}
		h := notes[f.ID]
		h.Score = f.Score
		h.Sources = f.Sources
		out = append(out, h)
	}
	return out
}

// rerank reorders hits by the LLM scorer. A scorer failure keeps the fused
// order: the rerank is an optional refinement, not a requirement.
func (l *Library) rerank(
	ctx context.Context,
	query string,
	hits []hybrid.ResultHit,
) []hybrid.ResultHit {
	r := rerank.New(l.cfg.Semantic.OllamaURL, l.cfg.Semantic.ChatModel)
	cands := make([]rerank.Candidate, 0, len(hits))
	byKey := make(map[string]hybrid.ResultHit, len(hits))
	for i, h := range hits {
		key := hitKey(h.Vault, h.Path)
		byKey[key] = h
		snippet := h.Snippet
		if snippet == "" {
			snippet = h.Path
		}
		cands = append(cands, rerank.Candidate{
			ID:            key,
			Snippet:       snippet,
			RetrievalRank: i + 1,
			Score:         h.Score,
		})
	}
	scored, err := r.Score(ctx, query, cands)
	if err != nil {
		return hits
	}
	out := make([]hybrid.ResultHit, 0, len(scored))
	for _, s := range scored {
		if h, ok := byKey[s.ID]; ok {
			h.Score = s.Final
			out = append(out, h)
		}
	}
	return out
}
