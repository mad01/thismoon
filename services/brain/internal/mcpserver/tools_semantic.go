package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/embedding"
	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/rerank"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// ---------------------------------------------------------------------------
// brain_semantic_search
// ---------------------------------------------------------------------------

type semanticSearchInput struct {
	Query      string `json:"query"                 jsonschema:"natural-language query — vector search returns conceptually similar passages, not just exact-string matches"`
	K          int    `json:"k,omitempty"           jsonschema:"number of results to return (default 20)"`
	Tier       string `json:"tier,omitempty"        jsonschema:"restrict to a single vault tier: inbox, notes, outputs, or MOCs"`
	Tag        string `json:"tag,omitempty"         jsonschema:"restrict to notes whose frontmatter tags list contains this tag"`
	Type       string `json:"type,omitempty"        jsonschema:"restrict to notes with this frontmatter type"`
	Status     string `json:"status,omitempty"      jsonschema:"restrict to notes with this frontmatter status"`
	Evidence   string `json:"evidence,omitempty"    jsonschema:"restrict to notes with this evidence tier (A/B/C/D/mixed)"`
	OutputMode string `json:"output_mode,omitempty" jsonschema:"snippets (default) returns one result per chunk with body excerpt; paths returns unique note paths"`
}

type semanticSearchHit struct {
	Path        string   `json:"path"                   jsonschema:"vault-relative note path"`
	Title       string   `json:"title,omitempty"        jsonschema:"note title (frontmatter or filename stem)"`
	HeadingPath string   `json:"heading_path,omitempty" jsonschema:"' › '-joined heading hierarchy above this chunk"`
	Score       float32  `json:"score"                  jsonschema:"cosine similarity in [-1,1]; higher is better"`
	Snippet     string   `json:"snippet,omitempty"      jsonschema:"chunk body excerpt; only set in snippets mode"`
	Tier        string   `json:"tier,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Evidence    string   `json:"evidence,omitempty"     jsonschema:"trust tier of the source note (A/B/C/D/mixed)"`
}

type semanticSearchOutput struct {
	OutputMode string              `json:"output_mode"`
	Hits       []semanticSearchHit `json:"hits"`
	Total      int                 `json:"total"`
}

// ---------------------------------------------------------------------------
// brain_hybrid_search
// ---------------------------------------------------------------------------

type hybridSearchInput struct {
	Query    string `json:"query"              jsonschema:"natural-language query — fused literal (zoekt) + semantic (vector) results via RRF"`
	K        int    `json:"k,omitempty"        jsonschema:"final number of results to return after fusion (default 20)"`
	KZoekt   int    `json:"k_zoekt,omitempty"  jsonschema:"candidates pulled from the literal index before fusion (default 30)"`
	KVector  int    `json:"k_vector,omitempty" jsonschema:"candidates pulled from the semantic index before fusion (default 30)"`
	Tier     string `json:"tier,omitempty"     jsonschema:"restrict to a single vault tier"`
	Tag      string `json:"tag,omitempty"      jsonschema:"restrict by frontmatter tag"`
	Type     string `json:"type,omitempty"     jsonschema:"restrict by frontmatter type"`
	Status   string `json:"status,omitempty"   jsonschema:"restrict by frontmatter status"`
	Evidence string `json:"evidence,omitempty" jsonschema:"restrict by evidence tier (A/B/C/D/mixed)"`
	Rerank   *bool  `json:"rerank,omitempty"   jsonschema:"opt-in LLM reranker (slower; defaults to semantic.rerank_default in config)"`
}

type hybridSearchHit struct {
	Path        string   `json:"path"`
	Title       string   `json:"title,omitempty"`
	HeadingPath string   `json:"heading_path,omitempty"`
	Score       float64  `json:"score"                  jsonschema:"post-fusion (and post-rerank when enabled) score; higher is better"`
	Sources     []string `json:"sources"                jsonschema:"which retrievers found this hit: zoekt, vector, or both"`
	Snippet     string   `json:"snippet,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Evidence    string   `json:"evidence,omitempty"`
}

type hybridSearchOutput struct {
	Hits  []hybridSearchHit `json:"hits"`
	Total int               `json:"total"`
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func registerSemanticTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_semantic_search",
		Description: "Vector (semantic) search across the vault. " +
			"Use whenever the task is conceptual rather than exact-string — 'notes that argue against X', " +
			"'anything about graceful shutdown patterns', 'what have I written that resembles this passage'. " +
			"Filters: tier (inbox/notes/outputs/MOCs), tag, type, status, evidence (A/B/C/D/mixed). " +
			"Requires semantic.enabled=true in ~/.config/brain/config.yaml and a running local Ollama. " +
			"Prefer brain_search for exact-string lookups, brain_hybrid_search when you want both signals fused.",
	}, handleSemanticSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_hybrid_search",
		Description: "Hybrid search: fuses literal (zoekt) and semantic (vector) results via Reciprocal Rank Fusion (RRF k=60). " +
			"Use as the default 'find notes about X' tool when both keywords and concepts matter — usually the right pick over brain_search alone. " +
			"Set rerank=true to additionally pass the top candidates through a local LLM scorer (1–5s slower). " +
			"Same filter vocabulary as brain_semantic_search. Requires semantic.enabled=true and a running Ollama.",
	}, handleHybridSearch)
}

func handleSemanticSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in semanticSearchInput,
) (*mcp.CallToolResult, semanticSearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, semanticSearchOutput{}, fmt.Errorf("brain_semantic_search: query is required")
	}
	mode := in.OutputMode
	if mode == "" {
		mode = "snippets"
	}
	if mode != "snippets" && mode != "paths" {
		return nil, semanticSearchOutput{}, fmt.Errorf(
			"brain_semantic_search: output_mode must be 'snippets' or 'paths', got %q",
			mode,
		)
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("brain_semantic_search: %w", err)
	}
	if !cfg.Semantic.Enabled {
		return nil, semanticSearchOutput{}, fmt.Errorf(
			"brain_semantic_search: semantic layer is disabled (set semantic.enabled=true in ~/.config/brain/config.yaml)",
		)
	}
	if err := ensureSemanticIndex(ctx, cfg); err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("brain_semantic_search: %w", err)
	}
	idx, err := openSemanticForMCP(cfg)
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("brain_semantic_search: %w", err)
	}
	k := in.K
	if k <= 0 {
		k = 20
	}
	hits, err := idx.Query(ctx, in.Query, semantic.QueryOptions{
		K:        k,
		Tier:     in.Tier,
		Tag:      in.Tag,
		Type:     in.Type,
		Status:   in.Status,
		Evidence: in.Evidence,
	})
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("brain_semantic_search: %w", err)
	}

	out := semanticSearchOutput{OutputMode: mode}
	if mode == "paths" {
		seen := make(map[string]struct{})
		for _, h := range hits {
			if _, ok := seen[h.Path]; ok {
				continue
			}
			seen[h.Path] = struct{}{}
			out.Hits = append(out.Hits, semanticSearchHit{
				Path:     h.Path,
				Title:    h.Title,
				Score:    h.Score,
				Tier:     h.Tier,
				Tags:     h.Tags,
				Type:     h.Type,
				Status:   h.Status,
				Evidence: h.Evidence,
			})
		}
	} else {
		for _, h := range hits {
			out.Hits = append(out.Hits, semanticSearchHit{
				Path:        h.Path,
				Title:       h.Title,
				HeadingPath: h.HeadingPath,
				Score:       h.Score,
				Snippet:     h.Snippet,
				Tier:        h.Tier,
				Tags:        h.Tags,
				Type:        h.Type,
				Status:      h.Status,
				Evidence:    h.Evidence,
			})
		}
	}
	out.Total = len(out.Hits)
	return nil, out, nil
}

func handleHybridSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in hybridSearchInput,
) (*mcp.CallToolResult, hybridSearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, hybridSearchOutput{}, fmt.Errorf("brain_hybrid_search: query is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, hybridSearchOutput{}, fmt.Errorf("brain_hybrid_search: %w", err)
	}
	if !cfg.Semantic.Enabled {
		return nil, hybridSearchOutput{}, fmt.Errorf(
			"brain_hybrid_search: semantic layer is disabled (set semantic.enabled=true in ~/.config/brain/config.yaml)",
		)
	}
	if err := ensureSemanticIndex(ctx, cfg); err != nil {
		return nil, hybridSearchOutput{}, fmt.Errorf("brain_hybrid_search: %w", err)
	}

	rerankFlag := cfg.Semantic.RerankDefault
	if in.Rerank != nil {
		rerankFlag = *in.Rerank
	}
	out, err := runHybridForMCP(ctx, cfg, hybridMCPOptions{
		Query:    in.Query,
		K:        defaultIfZero(in.K, 20),
		KZoekt:   defaultIfZero(in.KZoekt, 30),
		KVector:  defaultIfZero(in.KVector, 30),
		Tier:     in.Tier,
		Tag:      in.Tag,
		Type:     in.Type,
		Status:   in.Status,
		Evidence: in.Evidence,
		Rerank:   rerankFlag,
	})
	if err != nil {
		return nil, hybridSearchOutput{}, fmt.Errorf("brain_hybrid_search: %w", err)
	}
	resp := hybridSearchOutput{Hits: make([]hybridSearchHit, 0, len(out))}
	for _, h := range out {
		resp.Hits = append(resp.Hits, hybridSearchHit{
			Path:        h.Path,
			Title:       h.Title,
			HeadingPath: h.HeadingPath,
			Score:       h.Score,
			Sources:     h.Sources,
			Snippet:     h.Snippet,
			Tier:        h.Tier,
			Tags:        h.Tags,
			Type:        h.Type,
			Status:      h.Status,
			Evidence:    h.Evidence,
		})
	}
	resp.Total = len(resp.Hits)
	return nil, resp, nil
}

// ---------------------------------------------------------------------------
// shared helpers (internal — not exported, but lifted out so the CLI's
// runHybridQuery and the MCP handler don't drift)
// ---------------------------------------------------------------------------

type hybridMCPOptions struct {
	Query    string
	K        int
	KZoekt   int
	KVector  int
	Tier     string
	Tag      string
	Type     string
	Status   string
	Evidence string
	Rerank   bool
}

type hybridMCPHit struct {
	Path        string
	Title       string
	HeadingPath string
	Snippet     string
	Score       float64
	Sources     []string
	Tier        string
	Tags        []string
	Type        string
	Status      string
	Evidence    string
}

func runHybridForMCP(
	ctx context.Context,
	cfg *config.Config,
	in hybridMCPOptions,
) ([]hybridMCPHit, error) {
	idx, err := openSemanticForMCP(cfg)
	if err != nil {
		return nil, fmt.Errorf("semantic open: %w", err)
	}
	semHits, err := idx.Query(ctx, in.Query, semantic.QueryOptions{
		K:        in.KVector,
		Tier:     in.Tier,
		Tag:      in.Tag,
		Type:     in.Type,
		Status:   in.Status,
		Evidence: in.Evidence,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic query: %w", err)
	}

	indexDir, err := ensureIndex(cfg)
	if err != nil {
		return nil, fmt.Errorf("ensure literal index: %w", err)
	}
	var tier vault.Tier
	if in.Tier != "" {
		t, perr := vault.ParseTier(in.Tier)
		if perr != nil {
			return nil, fmt.Errorf("tier: %w", perr)
		}
		tier = t
	}
	zMatches, err := search.Search(ctx, indexDir, search.SearchOptions{
		Pattern: in.Query,
		Tag:     in.Tag,
		Type:    in.Type,
		Status:  in.Status,
		Tier:    tier,
		Limit:   in.KZoekt,
	})
	if err != nil {
		return nil, fmt.Errorf("literal query: %w", err)
	}

	zoektPaths := dedupeMatchPaths(zMatches)

	semByPath := make(map[string]semantic.Hit)
	semOrder := make([]string, 0, len(semHits))
	for _, h := range semHits {
		if cur, ok := semByPath[h.Path]; !ok || h.Score > cur.Score {
			if !ok {
				semOrder = append(semOrder, h.Path)
			}
			semByPath[h.Path] = h
		}
	}
	for i := 1; i < len(semOrder); i++ {
		for j := i; j > 0 && semByPath[semOrder[j]].Score > semByPath[semOrder[j-1]].Score; j-- {
			semOrder[j], semOrder[j-1] = semOrder[j-1], semOrder[j]
		}
	}

	fused := hybrid.Fuse([]hybrid.Input{
		{Source: "zoekt", IDs: zoektPaths},
		{Source: "vector", IDs: semOrder},
	}, hybrid.DefaultK)

	out := make([]hybridMCPHit, 0, in.K)
	for _, f := range fused {
		if len(out) >= in.K {
			break
		}
		hh := hybridMCPHit{Path: f.ID, Score: f.Score, Sources: f.Sources}
		if sh, ok := semByPath[f.ID]; ok {
			hh.Title = sh.Title
			hh.HeadingPath = sh.HeadingPath
			hh.Snippet = sh.Snippet
			hh.Tier = sh.Tier
			hh.Tags = sh.Tags
			hh.Type = sh.Type
			hh.Status = sh.Status
			hh.Evidence = sh.Evidence
		}
		out = append(out, hh)
	}

	if in.Rerank && len(out) > 3 {
		r := rerank.New(cfg.Semantic.OllamaURL, cfg.Semantic.ChatModel)
		cands := make([]rerank.Candidate, 0, len(out))
		for i, h := range out {
			snippet := h.Snippet
			if snippet == "" {
				snippet = h.Path
			}
			cands = append(cands, rerank.Candidate{
				ID:            h.Path,
				Snippet:       snippet,
				RetrievalRank: i + 1,
				Score:         h.Score,
			})
		}
		scored, err := r.Score(ctx, in.Query, cands)
		if err != nil {
			return out, nil //nolint:nilerr // preserve fused list on rerank failure
		}
		byID := make(map[string]hybridMCPHit, len(out))
		for _, h := range out {
			byID[h.Path] = h
		}
		reordered := make([]hybridMCPHit, 0, len(scored))
		for _, s := range scored {
			if h, ok := byID[s.ID]; ok {
				h.Score = s.Final
				reordered = append(reordered, h)
			}
		}
		out = reordered
	}

	return out, nil
}

func dedupeMatchPaths(ms []search.Match) []string {
	seen := make(map[string]struct{}, len(ms))
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if _, ok := seen[m.Path]; ok {
			continue
		}
		seen[m.Path] = struct{}{}
		out = append(out, m.Path)
	}
	return out
}

// semanticEmbedFn returns the embedding function used by the semantic MCP
// tools. Production binds to Ollama via the embedding package; tests swap
// in a deterministic stub so the suite never touches the network.
var semanticEmbedFn = func(cfg *config.Config) semantic.EmbedFunc {
	c := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel)
	return c.EmbedFunc()
}

// openSemanticForMCP is the MCP-handler equivalent of the CLI's
// openSemanticIndex helper. Lives here to keep cli/ and mcpserver/ free of
// cross-package handles.
func openSemanticForMCP(cfg *config.Config) (*semantic.Index, error) {
	dir, err := cfg.Semantic.ResolvedIndexDir()
	if err != nil {
		return nil, err
	}
	return semantic.Open(dir, semanticEmbedFn(cfg))
}

// ensureSemanticIndex makes sure the chromem-go collection is in sync with
// the vault before each MCP query. Mirrors ensureIndex's pattern: compare
// fingerprints, run an incremental upsert when stale, persist new state.
func ensureSemanticIndex(ctx context.Context, cfg *config.Config) error {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return fmt.Errorf("resolve index dir: %w", err)
	}
	fingerprint, _, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return fmt.Errorf("fingerprint vault: %w", err)
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return fmt.Errorf("load index state: %w", err)
	}
	if !state.IsSemanticStale(fingerprint) {
		return nil
	}
	idx, err := openSemanticForMCP(cfg)
	if err != nil {
		return fmt.Errorf("open semantic: %w", err)
	}
	opts := chunk.FromTokens(cfg.Semantic.ChunkSize, cfg.Semantic.ChunkOverlap)
	if err := semantic.IndexVault(ctx, idx, cfg.Vault, opts, nil); err != nil {
		return fmt.Errorf("semantic vault: %w", err)
	}
	state.SetSemanticFingerprint(fingerprint, time.Now().UTC())
	_ = state.Save(indexDir)
	return nil
}

func defaultIfZero(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}
