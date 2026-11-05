package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

const defaultSearchLimit = 50

// searchInput is the typed input for the brain_search tool.
type searchInput struct {
	Query         string `json:"query"                    jsonschema:"zoekt query — literal / regex / 'phrase' / AND (space) / OR (|) / NOT (-) / file: / lang: / case:yes"`
	Tier          string `json:"tier,omitempty"           jsonschema:"restrict to a single vault tier: inbox, notes, outputs, or MOCs"`
	Tag           string `json:"tag,omitempty"            jsonschema:"restrict to notes whose frontmatter tags list contains this tag"`
	Type          string `json:"type,omitempty"           jsonschema:"restrict to notes with this frontmatter type (e.g. note, source, moc, output)"`
	Status        string `json:"status,omitempty"         jsonschema:"restrict to notes with this frontmatter status (e.g. raw, validated, archived)"`
	Lang          string `json:"lang,omitempty"           jsonschema:"restrict to files of this language (usually markdown)"`
	File          string `json:"file,omitempty"           jsonschema:"restrict to vault-relative file paths matching this regex"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"force case-sensitive matching; default is smart case"`
	Limit         int    `json:"limit,omitempty"          jsonschema:"maximum number of file results (default 50)"`
	OutputMode    string `json:"output_mode,omitempty"    jsonschema:"files_with_matches (default) returns unique file paths; content returns matching lines with optional context"`
	ContextLines  int    `json:"context_lines,omitempty"  jsonschema:"number of context lines around each match in content mode; ignored in files_with_matches mode"`
}

type countInput struct {
	Query   string `json:"query"              jsonschema:"zoekt query (same syntax as brain_search)"`
	Tier    string `json:"tier,omitempty"     jsonschema:"restrict to a single vault tier"`
	GroupBy string `json:"group_by,omitempty" jsonschema:"group matches by: tier or tag; empty returns a single total"`
}

type queryValidateInput struct {
	Query string `json:"query" jsonschema:"the zoekt query string to validate"`
}

func registerSearchTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_search",
		Description: "Search across your personal knowledge vault (markdown notes) using zoekt query syntax. " +
			"Use whenever the task involves finding notes by text — 'where did I write about X', 'find every note mentioning Y', 'what have I captured on Z'. " +
			"Filter with tier (inbox/notes/outputs/MOCs), tag, type, or status. " +
			"Defaults to files_with_matches (unique note paths); set output_mode=content to get matching lines with optional context_lines. " +
			"Query syntax: literal, regex, \"phrase\", AND (space), OR (|), NOT (-), file:regex, lang:markdown, case:yes. " +
			"The index auto-refreshes when the vault has changed since the last call.",
	}, h.handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_count",
		Description: "Count matches of a zoekt query across the vault. " +
			"Use for tallies like 'how many notes mention kubernetes', 'which tag has the most matches', or summaries across tiers. " +
			"Set group_by to 'tier' or 'tag' for a breakdown; leave empty for a single total.",
	}, h.handleCount)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_query_validate",
		Description: "Validate a zoekt query and return its parsed tree or a parse error with a fixing hint. " +
			"Use to debug a complex query before invoking brain_search, especially when dealing with escaped regex metacharacters like \\.md$.",
	}, h.handleQueryValidate)
}

func (h *handlers) handleSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, search.Result, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, search.Result{}, fmt.Errorf("brain_search: query is required")
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		outputMode = search.ModeFiles
	}
	if outputMode != search.ModeFiles && outputMode != search.ModeContent {
		return nil, search.Result{}, fmt.Errorf(
			"brain_search: output_mode must be %q or %q, got %q",
			search.ModeFiles, search.ModeContent, outputMode,
		)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	var tier vault.Tier
	if in.Tier != "" {
		t, err := vault.ParseTier(in.Tier)
		if err != nil {
			return nil, search.Result{}, fmt.Errorf("brain_search: %w", err)
		}
		tier = t
	}

	cfg, _, err := h.loadVault()
	if err != nil {
		return nil, search.Result{}, fmt.Errorf("brain_search: %w", err)
	}
	indexDir, err := ensureIndex(cfg)
	if err != nil {
		return nil, search.Result{}, fmt.Errorf("brain_search: %w", err)
	}

	opts := search.SearchOptions{
		Pattern:       in.Query,
		File:          in.File,
		Tag:           in.Tag,
		Type:          in.Type,
		Status:        in.Status,
		Tier:          tier,
		Lang:          in.Lang,
		CaseSensitive: in.CaseSensitive,
		Limit:         limit,
		ContextLines:  in.ContextLines,
		OutputMode:    outputMode,
	}

	matches, err := search.Search(ctx, indexDir, opts)
	if err != nil {
		return nil, search.Result{}, fmt.Errorf("brain_search: %w", err)
	}

	return nil, search.NewResult(cfg.Vault, outputMode, matches), nil
}

func (h *handlers) handleCount(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in countInput,
) (*mcp.CallToolResult, search.Counts, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, search.Counts{}, fmt.Errorf("brain_count: query is required")
	}
	if in.GroupBy != "" && in.GroupBy != "tier" && in.GroupBy != "tag" {
		return nil, search.Counts{}, fmt.Errorf(
			"brain_count: group_by must be 'tier', 'tag', or empty, got %q",
			in.GroupBy,
		)
	}

	var tier vault.Tier
	if in.Tier != "" {
		t, err := vault.ParseTier(in.Tier)
		if err != nil {
			return nil, search.Counts{}, fmt.Errorf("brain_count: %w", err)
		}
		tier = t
	}

	cfg, _, err := h.loadVault()
	if err != nil {
		return nil, search.Counts{}, fmt.Errorf("brain_count: %w", err)
	}
	indexDir, err := ensureIndex(cfg)
	if err != nil {
		return nil, search.Counts{}, fmt.Errorf("brain_count: %w", err)
	}

	opts := search.CountOptions{
		Pattern: in.Query,
		Tier:    tier,
		GroupBy: in.GroupBy,
	}
	results, total, err := search.Count(ctx, indexDir, opts)
	if err != nil {
		return nil, search.Counts{}, fmt.Errorf("brain_count: %w", err)
	}

	return nil, search.Counts{Total: total, Groups: results}, nil
}

func (h *handlers) handleQueryValidate(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in queryValidateInput,
) (*mcp.CallToolResult, search.QueryInfo, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, search.QueryInfo{}, fmt.Errorf("brain_query_validate: query is required")
	}
	return nil, search.ValidateQuery(in.Query), nil
}
