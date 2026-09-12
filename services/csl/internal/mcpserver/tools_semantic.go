package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

const defaultSemanticK = 10

// semanticNotBuiltNote is shown when the semantic index has not been built yet.
const semanticNotBuiltNote = "semantic index not built — run: csl index --semantic-all"

// semanticSearchInput is the typed input for the csl_semantic_search tool.
type semanticSearchInput struct {
	Query  string `json:"query"            jsonschema:"natural-language description of the code you are looking for; matched by meaning/intent, not exact text"`
	Repo   string `json:"repo,omitempty"   jsonschema:"restrict to repo names containing this substring (case-sensitive; unlike the other csl tools this isn't a regex)"`
	Lang   string `json:"lang,omitempty"   jsonschema:"restrict to a single language (e.g. go, typescript, python)"`
	K      int    `json:"k,omitempty"      jsonschema:"maximum number of results to return (default 10)"`
	Expand int    `json:"expand,omitempty" jsonschema:"extra lines of source context to include above and below each matched chunk (default 0)"`
	formatParam
}

// semanticHit is one result returned by csl_semantic_search.
type semanticHit struct {
	Repo      string  `json:"repo"`
	Path      string  `json:"path"              jsonschema:"file path relative to the repo root"`
	Lang      string  `json:"lang,omitempty"`
	Kind      string  `json:"kind,omitempty"    jsonschema:"the chunk kind, e.g. func, type, method, or window"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"             jsonschema:"cosine similarity in [0,1]; higithostr is closer in meaning"`
	Snippet   string  `json:"snippet,omitempty" jsonschema:"the matched source text, widened by expand lines"`
}

// semanticSearchOutput is the typed output of csl_semantic_search.
type semanticSearchOutput struct {
	Available bool          `json:"available"      jsonschema:"false when the semantic index or embedding model isn't ready; see note"`
	Hits      []semanticHit `json:"hits,omitempty"`
	Note      string        `json:"note,omitempty" jsonschema:"guidance when results are unavailable"`
}

func registerSemanticTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_semantic_search",
		Description: "Find code by MEANING/intent across locally checked-out repos using vector embeddings. " +
			"Complements csl_search (which does lexical/exact/regex matching): use csl_semantic_search when you don't know the exact symbol or wording, for natural-language questions like 'where do we retry failed HTTP requests' or 'code that parses config files', and for queries that should match synonyms and paraphrases rather than literal strings. " +
			"Returns the top matching code chunks ranked by cosine similarity, each with its source snippet (widen it with expand). " +
			"Filter by repo (substring) or lang (single language). " +
			"Code is chunked by tree-sitter declarations for parseable languages and by 120-line windows for everything else, so hits in parseable languages correspond to whole declarations while other files return coarser windows. " +
			"Costs: embedding runs via a local Ollama server (jina-code-v2 by default, pulled with 'ollama pull'), so Ollama must be running; a cold query pays ~1-2s model load, then the model stays warm for 20 minutes. " +
			"Requires a semantic index built with 'csl index --semantic-all'; if it isn't built, the tool returns available=false with a note instead of an error.",
	}, withFormat(handleSemanticSearch, renderSemanticText))
}

func handleSemanticSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in semanticSearchInput,
) (*mcp.CallToolResult, semanticSearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, semanticSearchOutput{}, fmt.Errorf("query is required")
	}
	k := in.K
	if k <= 0 {
		k = defaultSemanticK
	}

	indexDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("resolve semantic index dir: %w", err)
	}
	filter := semanticFilter(in.Repo, in.Lang)

	// Daemon-first: the daemon loads both the lexical and semantic indexes, so
	// we ensure it via the lexical index dir like the other tools. semantic
	// cannot import daemon (cycle), so the fallback is duplicated here.
	socketPath := daemon.DefaultSocketPath()
	if lexDir, lexErr := search.DefaultIndexDir(); lexErr == nil {
		if err := daemon.EnsureDaemon(lexDir, socketPath); err == nil {
			out, ok, derr := semanticSearchViaDaemon(socketPath, in, k, filter)
			if derr == nil {
				return nil, out, nil
			}
			if !ok {
				return nil, semanticSearchOutput{}, derr
			}
			// ok == true && derr != nil signals ErrDaemonNotRunning → fall through.
		}
	}

	return semanticSearchInProcess(ctx, indexDir, in.Query, k, filter, in.Expand)
}

// semanticSearchViaDaemon queries the daemon. The bool return is true when the
// caller should fall back to in-process (daemon unreachable); on a real RPC
// error it is false with the error.
func semanticSearchViaDaemon(
	socketPath string,
	in semanticSearchInput,
	k int,
	filter semantic.Filter,
) (semanticSearchOutput, bool, error) {
	resp, err := daemon.SemanticSearchVia(socketPath, &pb.SemanticSearchRequest{
		Query:  in.Query,
		K:      int32(k),
		Repos:  filter.Repos,
		Langs:  filter.Langs,
		Expand: int32(in.Expand),
	})
	if err != nil {
		if errors.Is(err, daemon.ErrDaemonNotRunning) {
			return semanticSearchOutput{}, true, err
		}
		return semanticSearchOutput{}, false, fmt.Errorf("semantic search: %w", err)
	}
	if !resp.Available {
		return semanticSearchOutput{Available: false, Note: semanticNotBuiltNote}, false, nil
	}
	return semanticSearchOutput{Available: true, Hits: hitsFromProto(resp.Hits)}, false, nil
}

// semanticSearchInProcess runs the query without the daemon. An unbuilt index
// is reported as unavailable (with a build note), not an error.
func semanticSearchInProcess(
	ctx context.Context,
	indexDir, query string,
	k int,
	filter semantic.Filter,
	expand int,
) (*mcp.CallToolResult, semanticSearchOutput, error) {
	emb := semantic.NewDefaultEmbedder()
	results, err := semantic.SearchInProcess(ctx, indexDir, emb, query, k, filter, expand)
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("semantic search: %w", err)
	}
	if results == nil {
		return nil, semanticSearchOutput{Available: false, Note: semanticNotBuiltNote}, nil
	}
	return nil, semanticSearchOutput{Available: true, Hits: hitsFromResults(results)}, nil
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

func hitsFromResults(results []semantic.Result) []semanticHit {
	hits := make([]semanticHit, len(results))
	for i, r := range results {
		hits[i] = semanticHit{
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

func hitsFromProto(in []*pb.SemanticHit) []semanticHit {
	hits := make([]semanticHit, len(in))
	for i, h := range in {
		hits[i] = semanticHit{
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
