package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

const (
	defaultSearchLimit  = 50
	defaultSearchOutput = "files_with_matches"
	contentOutputMode   = "content"
	filesOutputMode     = "files_with_matches"
)

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

type searchMatchFile struct {
	Path     string `json:"path"               jsonschema:"vault-relative note path, e.g. notes/topic/foo.md"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
}

type searchMatchLine struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Text     string `json:"text"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
}

type searchOutput struct {
	OutputMode string            `json:"output_mode"     jsonschema:"echoes the output mode actually used"`
	Files      []searchMatchFile `json:"files,omitempty" jsonschema:"unique file paths that matched; set when output_mode is files_with_matches"`
	Lines      []searchMatchLine `json:"lines,omitempty" jsonschema:"matching lines; set when output_mode is content"`
	Total      int               `json:"total"           jsonschema:"total number of match records returned"`
}

type countInput struct {
	Query   string `json:"query"              jsonschema:"zoekt query (same syntax as brain_search)"`
	Tier    string `json:"tier,omitempty"     jsonschema:"restrict to a single vault tier"`
	GroupBy string `json:"group_by,omitempty" jsonschema:"group matches by: tier or tag; empty returns a single total"`
}

type countGroup struct {
	Group string `json:"group" jsonschema:"the tier name or tag, depending on group_by"`
	Count int    `json:"count"`
}

type countOutput struct {
	Total  int          `json:"total"            jsonschema:"total match count"`
	Groups []countGroup `json:"groups,omitempty" jsonschema:"per-group counts; empty when group_by is not set"`
}

type queryValidateInput struct {
	Query string `json:"query" jsonschema:"the zoekt query string to validate"`
}

type queryValidateOutput struct {
	Valid  bool   `json:"valid"`
	Parsed string `json:"parsed,omitempty" jsonschema:"string representation of the parsed query tree when valid"`
	Error  string `json:"error,omitempty"  jsonschema:"parse error message when not valid"`
	Hint   string `json:"hint,omitempty"   jsonschema:"suggestion for fixing the query when not valid"`
}

func registerSearchTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_search",
		Description: "Search across your personal knowledge vault (markdown notes) using zoekt query syntax. " +
			"Use whenever the task involves finding notes by text — 'where did I write about X', 'find every note mentioning Y', 'what have I captured on Z'. " +
			"Filter with tier (inbox/notes/outputs/MOCs), tag, type, or status. " +
			"Defaults to files_with_matches (unique note paths); set output_mode=content to get matching lines with optional context_lines. " +
			"Query syntax: literal, regex, \"phrase\", AND (space), OR (|), NOT (-), file:regex, lang:markdown, case:yes. " +
			"The index auto-refreshes when the vault has changed since the last call.",
	}, handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_count",
		Description: "Count matches of a zoekt query across the vault. " +
			"Use for tallies like 'how many notes mention kubernetes', 'which tag has the most matches', or summaries across tiers. " +
			"Set group_by to 'tier' or 'tag' for a breakdown; leave empty for a single total.",
	}, handleCount)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_query_validate",
		Description: "Validate a zoekt query and return its parsed tree or a parse error with a fixing hint. " +
			"Use to debug a complex query before invoking brain_search, especially when dealing with escaped regex metacharacters like \\.md$.",
	}, handleQueryValidate)
}

func handleSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, searchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, searchOutput{}, fmt.Errorf("brain_search: query is required")
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		outputMode = defaultSearchOutput
	}
	if outputMode != filesOutputMode && outputMode != contentOutputMode {
		return nil, searchOutput{}, fmt.Errorf(
			"brain_search: output_mode must be %q or %q, got %q",
			filesOutputMode, contentOutputMode, outputMode,
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
			return nil, searchOutput{}, fmt.Errorf("brain_search: %w", err)
		}
		tier = t
	}

	cfg, _, err := loadVault()
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("brain_search: %w", err)
	}
	indexDir, err := ensureIndex(cfg)
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("brain_search: %w", err)
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
		return nil, searchOutput{}, fmt.Errorf("brain_search: %w", err)
	}

	return nil, buildSearchOutput(outputMode, matches, cfg), nil
}

// evidenceForPath reads the frontmatter of a vault-relative path and returns
// its evidence tier. Caches results so each path is parsed at most once per
// call. Returns empty string on any error (evidence is best-effort metadata).
func evidenceForPath(cfg *config.Config, cache map[string]string, relPath string) string {
	if ev, ok := cache[relPath]; ok {
		return ev
	}
	abs := filepath.Join(cfg.Vault, filepath.FromSlash(relPath))
	data, err := os.ReadFile(abs)
	if err != nil {
		cache[relPath] = ""
		return ""
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		cache[relPath] = ""
		return ""
	}
	ev := markdown.EvidenceOf(fm)
	cache[relPath] = ev
	return ev
}

func buildSearchOutput(mode string, matches []search.Match, cfg *config.Config) searchOutput {
	out := searchOutput{OutputMode: mode}
	if len(matches) == 0 {
		return out
	}
	evCache := make(map[string]string)
	if mode == contentOutputMode {
		lines := make([]searchMatchLine, 0, len(matches))
		for _, m := range matches {
			lines = append(lines, searchMatchLine{
				Path:     m.Path,
				Line:     m.Line,
				Column:   m.Column,
				Text:     m.Text,
				Before:   m.Before,
				After:    m.After,
				Evidence: evidenceForPath(cfg, evCache, m.Path),
			})
		}
		out.Lines = lines
		out.Total = len(lines)
		return out
	}
	seen := make(map[string]struct{}, len(matches))
	files := make([]searchMatchFile, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m.Path]; ok {
			continue
		}
		seen[m.Path] = struct{}{}
		files = append(files, searchMatchFile{
			Path:     m.Path,
			Evidence: evidenceForPath(cfg, evCache, m.Path),
		})
	}
	out.Files = files
	out.Total = len(files)
	return out
}

func handleCount(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in countInput,
) (*mcp.CallToolResult, countOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, countOutput{}, fmt.Errorf("brain_count: query is required")
	}
	if in.GroupBy != "" && in.GroupBy != "tier" && in.GroupBy != "tag" {
		return nil, countOutput{}, fmt.Errorf(
			"brain_count: group_by must be 'tier', 'tag', or empty, got %q",
			in.GroupBy,
		)
	}

	var tier vault.Tier
	if in.Tier != "" {
		t, err := vault.ParseTier(in.Tier)
		if err != nil {
			return nil, countOutput{}, fmt.Errorf("brain_count: %w", err)
		}
		tier = t
	}

	cfg, _, err := loadVault()
	if err != nil {
		return nil, countOutput{}, fmt.Errorf("brain_count: %w", err)
	}
	indexDir, err := ensureIndex(cfg)
	if err != nil {
		return nil, countOutput{}, fmt.Errorf("brain_count: %w", err)
	}

	opts := search.CountOptions{
		Pattern: in.Query,
		Tier:    tier,
		GroupBy: in.GroupBy,
	}
	results, total, err := search.Count(ctx, indexDir, opts)
	if err != nil {
		return nil, countOutput{}, fmt.Errorf("brain_count: %w", err)
	}

	groups := make([]countGroup, 0, len(results))
	for _, r := range results {
		groups = append(groups, countGroup{Group: r.Group, Count: r.Count})
	}
	return nil, countOutput{Total: total, Groups: groups}, nil
}

func handleQueryValidate(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in queryValidateInput,
) (*mcp.CallToolResult, queryValidateOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, queryValidateOutput{}, fmt.Errorf("brain_query_validate: query is required")
	}
	info := search.ValidateQuery(in.Query)
	return nil, queryValidateOutput{
		Valid:  info.Valid,
		Parsed: info.Parsed,
		Error:  info.Error,
		Hint:   info.Hint,
	}, nil
}
