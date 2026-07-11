package web

import (
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// defaultSemanticK is the result count used when the request omits k.
const defaultSemanticK = 10

// semanticNotBuiltNote is surfaced (with Available=false) when the semantic
// index or embedding model is not ready, so the UI can show a build hint
// instead of an error.
const semanticNotBuiltNote = "semantic index not built — run: csl index --semantic-all"

// SemanticRequest carries the parsed parameters for a semantic search.
type SemanticRequest struct {
	Query  string
	K      int
	Repo   string
	Lang   string
	Expand int
}

// SemanticHit is one semantic match handed to the API layer.
type SemanticHit struct {
	Repo      string
	Path      string
	Lang      string
	Kind      string
	StartLine int
	EndLine   int
	Score     float32
	Snippet   string
}

// SemanticResult is the outcome of a semantic search. When Available is false,
// Note explains why (e.g. the index has not been built) and Hits is empty.
type SemanticResult struct {
	Available bool
	Note      string
	Hits      []SemanticHit
}

// SemanticSearch runs a semantic (vector) query, mirroring the lexical
// daemon-first-with-in-process-fallback behaviour (see Service.Search and
// internal/mcpserver/tools_semantic.go). A missing index or model is reported
// as Available=false with a build note, not an error.
func (s *Service) SemanticSearch(ctx context.Context, req SemanticRequest) (SemanticResult, error) {
	k := req.K
	if k <= 0 {
		k = defaultSemanticK
	}

	indexDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return SemanticResult{}, fmt.Errorf("resolve semantic index dir: %w", err)
	}
	filter := semanticFilter(req.Repo, req.Lang)

	// Daemon-first: the daemon serves both lexical and semantic from the warm
	// index, so ensure it via the lexical index dir like the lexical path.
	if err := daemon.EnsureDaemon(s.indexDir, s.socketPath); err == nil {
		res, fallback, derr := s.semanticViaDaemon(req, k, filter)
		if derr == nil {
			return res, nil
		}
		if !fallback {
			return SemanticResult{}, derr
		}
		// fallback == true signals ErrDaemonNotRunning → fall through.
	}

	return s.semanticInProcess(ctx, indexDir, req.Query, k, filter, req.Expand)
}

// semanticViaDaemon queries the daemon. The bool return is true when the caller
// should fall back to in-process (daemon unreachable); on a real RPC error it
// is false with the error.
func (s *Service) semanticViaDaemon(
	req SemanticRequest,
	k int,
	filter semantic.Filter,
) (SemanticResult, bool, error) {
	resp, err := daemon.SemanticSearchVia(s.socketPath, &pb.SemanticSearchRequest{
		Query:  req.Query,
		K:      int32(k),
		Repos:  filter.Repos,
		Langs:  filter.Langs,
		Expand: int32(req.Expand),
	})
	if err != nil {
		if errors.Is(err, daemon.ErrDaemonNotRunning) {
			return SemanticResult{}, true, err
		}
		return SemanticResult{}, false, fmt.Errorf("semantic search: %w", err)
	}
	if !resp.Available {
		return SemanticResult{Available: false, Note: semanticNotBuiltNote}, false, nil
	}
	return SemanticResult{Available: true, Hits: hitsFromProto(resp.Hits)}, false, nil
}

// semanticInProcess runs the query without the daemon. An empty index is
// reported as unavailable (with a build note), not an error.
func (s *Service) semanticInProcess(
	ctx context.Context,
	indexDir, query string,
	k int,
	filter semantic.Filter,
	expand int,
) (SemanticResult, error) {
	emb := semantic.NewDefaultEmbedder()
	results, err := semantic.SearchInProcess(ctx, indexDir, emb, query, k, filter, expand)
	if err != nil {
		return SemanticResult{}, fmt.Errorf("semantic search: %w", err)
	}
	if results == nil {
		return SemanticResult{Available: false, Note: semanticNotBuiltNote}, nil
	}
	return SemanticResult{Available: true, Hits: hitsFromResults(results)}, nil
}

// semanticFilter builds a semantic.Filter from single repo/lang values, omitting
// empty ones.
func semanticFilter(repo, lang string) semantic.Filter {
	var f semantic.Filter
	if repo != "" {
		f.Repos = []string{repo}
	}
	if lang != "" {
		f.Langs = []string{lang}
	}
	return f
}

func hitsFromResults(results []semantic.Result) []SemanticHit {
	hits := make([]SemanticHit, len(results))
	for i, r := range results {
		hits[i] = SemanticHit{
			Repo:      r.Repo,
			Path:      r.Path,
			Lang:      r.Lang,
			Kind:      r.Kind,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
			Snippet:   r.Snippet,
		}
	}
	return hits
}

func hitsFromProto(in []*pb.SemanticHit) []SemanticHit {
	hits := make([]SemanticHit, len(in))
	for i, h := range in {
		hits[i] = SemanticHit{
			Repo:      h.Repo,
			Path:      h.File,
			Lang:      h.Lang,
			Kind:      h.Kind,
			StartLine: int(h.StartLine),
			EndLine:   int(h.EndLine),
			Score:     h.Score,
			Snippet:   h.Snippet,
		}
	}
	return hits
}
