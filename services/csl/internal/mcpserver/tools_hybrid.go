package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/hybrid"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

const defaultHybridLimit = 50

// hybridSearchInput is the typed input for the csl_hybrid_search tool.
type hybridSearchInput struct {
	Query  string `json:"query"            jsonschema:"what you are looking for; matched both literally (lexical) and by meaning (semantic)"`
	Repo   string `json:"repo,omitempty"   jsonschema:"restrict to a repo: the lexical side treats this as a case-insensitive regex, the semantic side as a case-sensitive substring; a plain repo name like 'mad01/csl' satisfies both"`
	Lang   string `json:"lang,omitempty"   jsonschema:"restrict to a single language (e.g. go, typescript, python)"`
	Limit  int    `json:"limit,omitempty"  jsonschema:"maximum number of fused file results to return (default 50)"`
	RRFK   int    `json:"rrf_k,omitempty"  jsonschema:"Reciprocal Rank Fusion smoothing constant (default 60); lower favors top-ranked outliers, higithostr favors cross-backend consensus"`
	Expand int    `json:"expand,omitempty" jsonschema:"extra lines of source context around each semantic chunk snippet (default 0)"`
}

// hybridHit is one fused, file-level result from csl_hybrid_search. It carries
// the evidence from each backend the file appeared in; a zero lex_rank or
// sem_rank means the file was absent from that backend's results.
type hybridHit struct {
	Repo  string  `json:"repo"`
	Path  string  `json:"path"  jsonschema:"file path relative to the repo root"`
	Score float64 `json:"score" jsonschema:"fused RRF score; higithostr is a stronger combined match"`

	LexRank int    `json:"lex_rank"           jsonschema:"1-based rank in the lexical (zoekt) results; 0 if the file didn't match lexically"`
	LexLine int    `json:"lex_line,omitempty" jsonschema:"line of the first lexical match"`
	LexText string `json:"lex_text,omitempty" jsonschema:"text of the first lexically matching line"`

	SemRank  int     `json:"sem_rank"            jsonschema:"1-based rank in the semantic results; 0 if the file didn't match semantically"`
	SemStart int     `json:"sem_start,omitempty" jsonschema:"start line of the matched semantic chunk"`
	SemEnd   int     `json:"sem_end,omitempty"   jsonschema:"end line of the matched semantic chunk"`
	SemScore float32 `json:"sem_score,omitempty" jsonschema:"cosine similarity of the semantic chunk in [0,1]"`
	Snippet  string  `json:"snippet,omitempty"   jsonschema:"the matched semantic source chunk"`
}

// hybridSearchOutput is the typed output of csl_hybrid_search.
type hybridSearchOutput struct {
	SemanticAvailable bool        `json:"semantic_available" jsonschema:"false when the semantic index/model isn't ready; results are then lexical-only (see note)"`
	Hits              []hybridHit `json:"hits,omitempty"`
	Note              string      `json:"note,omitempty"     jsonschema:"guidance shown when semantic search is unavailable"`
}

func registerHybridTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_hybrid_search",
		Description: "Search code by combining lexical (exact/regex) and semantic (meaning) search and fusing the two rankings with Reciprocal Rank Fusion. " +
			"Prefer this as the default code search when you want BOTH exact-match safety and meaning-based recall, e.g. a natural-language description that also contains a likely literal token ('where do we retry failed HTTP requests'). " +
			"Use csl_search instead for pure regex/exact lookups, and csl_semantic_search for pure meaning-only queries when you have no useful literal terms. " +
			"Returns file-level hits ranked by fused score, each carrying its lexical line and/or semantic chunk as evidence. " +
			"Filter by repo (substring) or lang (single language); tune fusion with rrf_k (default 60, limit default 50). " +
			"Runs both backends per call, so the semantic cold-start cost (first-ever model download ~90 MB, model load on a cold daemon; see csl_semantic_search) applies here too. " +
			"If the semantic index isn't built (csl index --semantic-all), results degrade to lexical-only with semantic_available=false.",
	}, handleHybridSearch)
}

func handleHybridSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in hybridSearchInput,
) (*mcp.CallToolResult, hybridSearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, hybridSearchOutput{}, fmt.Errorf("query is required")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultHybridLimit
	}

	lexMatches, err := hybridLexical(ctx, in, limit)
	if err != nil {
		return nil, hybridSearchOutput{}, err
	}

	semResults, available, note, err := hybridSemantic(ctx, in, limit)
	if err != nil {
		return nil, hybridSearchOutput{}, err
	}

	fused := hybrid.Fuse(lexMatches, semResults, in.RRFK, limit)
	return nil, hybridSearchOutput{
		SemanticAvailable: available,
		Hits:              hybridHits(fused),
		Note:              note,
	}, nil
}

// hybridLexical runs the lexical backend in content mode (so fusion has ranked
// per-line matches), reusing the same daemon-first path as csl_search.
func hybridLexical(ctx context.Context, in hybridSearchInput, limit int) ([]search.Match, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, fmt.Errorf("resolve index dir: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load csl config: %w", err)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, fmt.Errorf("walk repos: %w", err)
	}
	if len(repos) == 0 {
		return nil, nil
	}
	repoNames := make(map[string]string, len(repos))
	for _, r := range repos {
		repoNames[r.Name] = r.Path
	}

	opts := search.SearchOptions{
		Pattern:    in.Query,
		RepoFilter: insensitiveRepoFilter(in.Repo),
		Lang:       in.Lang,
		Limit:      limit,
		OutputMode: contentOutputMode,
	}
	return runSearch(ctx, indexDir, daemon.DefaultSocketPath(), opts, repoNames)
}

// hybridSemantic reuses the csl_semantic_search handler so the daemon-first
// path and the "index not built" degradation are shared. It returns the hits as
// semantic.Result for fusion, whether semantic was available, and any note.
func hybridSemantic(
	ctx context.Context,
	in hybridSearchInput,
	limit int,
) ([]semantic.Result, bool, string, error) {
	_, out, err := handleSemanticSearch(ctx, nil, semanticSearchInput{
		Query:  in.Query,
		Repo:   in.Repo,
		Lang:   in.Lang,
		K:      limit,
		Expand: in.Expand,
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("semantic search: %w", err)
	}
	if !out.Available {
		return nil, false, out.Note, nil
	}
	return resultsFromHits(out.Hits), true, "", nil
}

// resultsFromHits adapts the MCP semantic hits back into semantic.Result for the
// pure fusion function. RepoPath is not carried by the MCP type; the lexical
// side fills it for any overlapping file.
func resultsFromHits(hits []semanticHit) []semantic.Result {
	out := make([]semantic.Result, len(hits))
	for i, h := range hits {
		out[i] = semantic.Result{
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
	return out
}

func hybridHits(fused []hybrid.FusedHit) []hybridHit {
	hits := make([]hybridHit, len(fused))
	for i, f := range fused {
		hits[i] = hybridHit{
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
	return hits
}
