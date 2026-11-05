package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

// ---------------------------------------------------------------------------
// brain_semantic_search
// ---------------------------------------------------------------------------

type semanticSearchInput struct {
	Query      string `json:"query"                     jsonschema:"natural-language query — vector search returns conceptually similar passages, not just exact-string matches"`
	K          int    `json:"k,omitempty"               jsonschema:"number of results to return (default 20)"`
	Tier       string `json:"tier,omitempty"            jsonschema:"restrict to a single vault tier: inbox, notes, outputs, or MOCs"`
	Tag        string `json:"tag,omitempty"             jsonschema:"restrict to notes whose frontmatter tags list contains this tag"`
	Type       string `json:"type,omitempty"            jsonschema:"restrict to notes with this frontmatter type"`
	Status     string `json:"status,omitempty"          jsonschema:"restrict to notes with this frontmatter status"`
	Evidence   string `json:"evidence,omitempty"        jsonschema:"restrict to notes with this evidence tier (A/B/C/D/mixed)"`
	OutputMode string `json:"output_mode,omitempty"     jsonschema:"snippets (default) returns one result per chunk with body excerpt; paths returns unique note paths"`
	Vault      string `json:"vault,omitempty"           jsonschema:"restrict to this configured vault; default searches every vault"`
	// IncludeRevoked keeps notes default search hides.
	IncludeRevoked bool `json:"include_revoked,omitempty" jsonschema:"include superseded, retracted and withdrawn notes, which are hidden by default"`
}

// ---------------------------------------------------------------------------
// brain_hybrid_search
// ---------------------------------------------------------------------------

type hybridSearchInput struct {
	Query    string `json:"query"                     jsonschema:"natural-language query — fused literal (zoekt) + semantic (vector) results via RRF"`
	K        int    `json:"k,omitempty"               jsonschema:"final number of results to return after fusion (default 20)"`
	KZoekt   int    `json:"k_zoekt,omitempty"         jsonschema:"candidates pulled from the literal index before fusion (default 30)"`
	KVector  int    `json:"k_vector,omitempty"        jsonschema:"candidates pulled from the semantic index before fusion (default 30)"`
	Tier     string `json:"tier,omitempty"            jsonschema:"restrict to a single vault tier"`
	Tag      string `json:"tag,omitempty"             jsonschema:"restrict by frontmatter tag"`
	Type     string `json:"type,omitempty"            jsonschema:"restrict by frontmatter type"`
	Status   string `json:"status,omitempty"          jsonschema:"restrict by frontmatter status"`
	Evidence string `json:"evidence,omitempty"        jsonschema:"restrict by evidence tier (A/B/C/D/mixed)"`
	Rerank   *bool  `json:"rerank,omitempty"          jsonschema:"opt-in LLM reranker (slower; defaults to semantic.rerank_default in config)"`
	Vault    string `json:"vault,omitempty"           jsonschema:"restrict to this configured vault; default searches every vault"`
	// IncludeRevoked keeps notes default search hides.
	IncludeRevoked bool `json:"include_revoked,omitempty" jsonschema:"include superseded, retracted and withdrawn notes, which are hidden by default"`
}

// ---------------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------------

func registerSemanticTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_semantic_search",
		Description: "Vector (semantic) search across the vault. " +
			"Use whenever the task is conceptual rather than exact-string — 'notes that argue against X', " +
			"'anything about graceful shutdown patterns', 'what have I written that resembles this passage'. " +
			"Filters: tier (inbox/notes/outputs/MOCs), tag, type, status, evidence (A/B/C/D/mixed). " +
			"Superseded, retracted and withdrawn notes are hidden unless include_revoked is set or status names them. " +
			"Requires semantic.enabled=true in the brain config and a running local Ollama; a stale index is refreshed first, and warnings say when it could not be. " +
			"Prefer brain_search for exact-string lookups, brain_hybrid_search when you want both signals fused.",
	}, h.handleSemanticSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_hybrid_search",
		Description: "Hybrid search: fuses literal (zoekt) and semantic (vector) results via Reciprocal Rank Fusion (RRF k=60). " +
			"Use as the default 'find notes about X' tool when both keywords and concepts matter — usually the right pick over brain_search alone. " +
			"Set rerank=true to additionally pass the top candidates through a local LLM scorer (1–5s slower). " +
			"Same filter vocabulary as brain_semantic_search. Requires semantic.enabled=true; when Ollama is unreachable the hits are literal only and warnings say so.",
	}, h.handleHybridSearch)
}

func (h *handlers) handleSemanticSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in semanticSearchInput,
) (*mcp.CallToolResult, semantic.SearchResult, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, semantic.SearchResult{}, fmt.Errorf("brain_semantic_search: query is required")
	}
	mode := in.OutputMode
	if mode == "" {
		mode = semantic.ModeSnippets
	}
	if mode != semantic.ModeSnippets && mode != semantic.ModePaths {
		return nil, semantic.SearchResult{}, fmt.Errorf(
			"brain_semantic_search: output_mode must be 'snippets' or 'paths', got %q",
			mode,
		)
	}
	lib, set, err := h.vaults(in.Vault)
	if err != nil {
		return nil, semantic.SearchResult{}, fmt.Errorf("brain_semantic_search: %w", err)
	}
	res, err := lib.SemanticSearch(ctx, set, library.SemanticQuery{
		Text: in.Query,
		Mode: mode,
		Filter: semantic.QueryOptions{
			K:        in.K,
			Tier:     in.Tier,
			Tag:      in.Tag,
			Type:     in.Type,
			Status:   in.Status,
			Evidence: in.Evidence,
		},
		IncludeRevoked: in.IncludeRevoked,
	})
	if err != nil {
		return nil, semantic.SearchResult{}, fmt.Errorf("brain_semantic_search: %w", err)
	}
	return nil, res, nil
}

func (h *handlers) handleHybridSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in hybridSearchInput,
) (*mcp.CallToolResult, hybrid.Result, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, hybrid.Result{}, fmt.Errorf("brain_hybrid_search: query is required")
	}
	lib, set, err := h.vaults(in.Vault)
	if err != nil {
		return nil, hybrid.Result{}, fmt.Errorf("brain_hybrid_search: %w", err)
	}
	rerankFlag := lib.Config().Semantic.RerankDefault
	if in.Rerank != nil {
		rerankFlag = *in.Rerank
	}
	res, err := lib.HybridSearch(ctx, set, library.HybridQuery{
		Text:     in.Query,
		K:        in.K,
		KZoekt:   in.KZoekt,
		KVector:  in.KVector,
		Tier:     in.Tier,
		Tag:      in.Tag,
		Type:     in.Type,
		Status:   in.Status,
		Evidence: in.Evidence,
		Rerank:   rerankFlag,

		IncludeRevoked: in.IncludeRevoked,
	})
	if err != nil {
		return nil, hybrid.Result{}, fmt.Errorf("brain_hybrid_search: %w", err)
	}
	return nil, res, nil
}
