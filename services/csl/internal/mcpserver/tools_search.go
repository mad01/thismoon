package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

const (
	defaultSearchLimit  = 50
	defaultSearchOutput = "files_with_matches"
	contentOutputMode   = "content"
	filesOutputMode     = "files_with_matches"
)

// searchInput is the typed input for the csl_search tool.
type searchInput struct {
	Query         string `json:"query"                    jsonschema:"zoekt query — literal / regex / 'phrase' / AND (space) / OR (|) / NOT (-) / repo: / file: / lang: / case:yes"`
	Repo          string `json:"repo,omitempty"           jsonschema:"restrict to repo names matching this case-insensitive regex (a plain substring also works)"`
	Lang          string `json:"lang,omitempty"           jsonschema:"restrict to files of this language (e.g. go, swift, python)"`
	File          string `json:"file,omitempty"           jsonschema:"restrict to file paths matching this regex (e.g. paths ending in .go)"`
	OutputMode    string `json:"output_mode,omitempty"    jsonschema:"files_with_matches (default) returns unique file paths; content returns matching lines with context"`
	ContextLines  int    `json:"context_lines,omitempty"  jsonschema:"number of context lines around each match in content mode (default 0); ignored in files_with_matches mode"`
	Limit         int    `json:"limit,omitempty"          jsonschema:"maximum number of file results (default 50)"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"force case-sensitive matching; default is smart case (case-insensitive unless the query has uppercase)"`
}

// searchMatchFile is one entry returned in files_with_matches mode.
type searchMatchFile struct {
	Repo string `json:"repo"`
	Path string `json:"path" jsonschema:"file path relative to the repo root"`
}

// searchMatchLine is one entry returned in content mode.
type searchMatchLine struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
	Before string `json:"before,omitempty" jsonschema:"context lines before the match (only set when context_lines > 0)"`
	After  string `json:"after,omitempty"  jsonschema:"context lines after the match"`
}

// searchOutput is the typed output of the csl_search tool. Exactly one of
// Files / Lines is populated depending on OutputMode.
type searchOutput struct {
	OutputMode     string            `json:"output_mode"               jsonschema:"echoes the output mode actually used (files_with_matches or content)"`
	Files          []searchMatchFile `json:"files,omitempty"           jsonschema:"unique file paths that matched; set when output_mode is files_with_matches"`
	Lines          []searchMatchLine `json:"lines,omitempty"           jsonschema:"matching lines; set when output_mode is content"`
	Total          int               `json:"total"                     jsonschema:"total number of match records returned (files or lines)"`
	Truncated      bool              `json:"truncated"                 jsonschema:"true if results were capped by limit; more matches exist — refine your query or increase limit"`
	TotalAvailable int               `json:"total_available,omitempty" jsonschema:"total matches available before truncation (only set when truncated is true)"`
}

// countInput is the typed input for the csl_count tool.
type countInput struct {
	Query   string `json:"query"              jsonschema:"zoekt query (same syntax as csl_search)"`
	Repo    string `json:"repo,omitempty"     jsonschema:"restrict to repo names matching this case-insensitive regex (a plain substring also works)"`
	Lang    string `json:"lang,omitempty"     jsonschema:"restrict to files of this language"`
	GroupBy string `json:"group_by,omitempty" jsonschema:"group matches by: repo or language; empty returns a single total"`
}

// countGroup is one entry in the csl_count result.
type countGroup struct {
	Group string `json:"group" jsonschema:"the repo name or language, depending on group_by"`
	Count int    `json:"count"`
}

// countOutput is the typed output of the csl_count tool.
type countOutput struct {
	Total  int          `json:"total"            jsonschema:"total match count across all groups"`
	Groups []countGroup `json:"groups,omitempty" jsonschema:"per-group counts; empty when group_by is not set"`
}

// queryValidateInput is the typed input for the csl_query_validate tool.
type queryValidateInput struct {
	Query string `json:"query" jsonschema:"the zoekt query string to validate"`
}

// queryValidateOutput is the typed output of the csl_query_validate tool.
type queryValidateOutput struct {
	Valid  bool   `json:"valid"`
	Parsed string `json:"parsed,omitempty" jsonschema:"string representation of the parsed query tree when valid"`
	Error  string `json:"error,omitempty"  jsonschema:"parse error message when not valid"`
	Hint   string `json:"hint,omitempty"   jsonschema:"suggestion for fixing the query when not valid"`
}

func registerSearchTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_search",
		Description: "Search code across locally checked-out git repos using zoekt query syntax. " +
			"Use whenever the task involves finding where a symbol, function, pattern, or string is used — 'where is X defined', 'find all Y', 'does any of my projects use Z', 'show me every TODO in the Go code'. " +
			"Defaults to returning unique matching file paths (files_with_matches); set output_mode to 'content' to get matching lines with optional context. " +
			"Query syntax: literal substring, regex, \"quoted phrase\", AND (space), OR (|), NOT (-), repo:name, f:\\.go$, lang:go, case:yes. " +
			"AND is strict: all terms must appear in the SAME FILE. Use 1-2 terms and narrow with repo:/f:/lang: filters, not 3+ chained terms. " +
			"Use | or lowercase 'or' for OR — uppercase OR is treated as a literal string, and spaces around | break it (a | b is three AND terms, not OR). " +
			"Filter prefixes: repo: (not r:), f: (not file:). Prefer the dedicated repo/lang/file params over inline filter syntax — the repo param is case-insensitive, while an inline repo: filter is raw zoekt (case-sensitive regex). " +
			"Defaults and caps: limit 50 files, context_lines 0; content mode returns at most 300 lines per call — when capped, truncated=true and total_available says how many matched, so narrow the query or paginate with filters. " +
			"The results come from a persistent in-memory zoekt index maintained by the csl search daemon, so calls are fast across a session.",
	}, handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_count",
		Description: "Count matches of a zoekt query across locally checked-out repos. " +
			"Accepts the same zoekt query syntax as csl_search (including repo:/f:/lang: filters and AND/OR/NOT). " +
			"Use for cross-repo tallies like 'how many TODOs across my Go projects' or 'which language has the most calls to fmt.Errorf'. " +
			"Set group_by to 'repo' or 'language' for a breakdown; leave it empty for a single total.",
	}, handleCount)

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_query_validate",
		Description: "Validate a zoekt query and return its parsed tree or a parse error with a fixing hint. " +
			"Use whenever a query returns zero results or behaves unexpectedly — the parsed tree shows exactly how zoekt interpreted your terms. " +
			"Also useful for debugging regex escaping like \\.go$.",
	}, handleQueryValidate)
}

func handleSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, searchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, searchOutput{}, fmt.Errorf("query is required")
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		outputMode = defaultSearchOutput
	}
	if outputMode != filesOutputMode && outputMode != contentOutputMode {
		return nil, searchOutput{}, fmt.Errorf(
			"output_mode must be %q or %q, got %q",
			filesOutputMode, contentOutputMode, outputMode,
		)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("resolve index dir: %w", err)
	}
	socketPath := daemon.DefaultSocketPath()

	cfg, err := config.Load()
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("load csl config: %w", err)
	}

	repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("walk repos: %w", err)
	}
	if len(repos) == 0 {
		return nil, searchOutput{OutputMode: outputMode}, nil
	}

	repoNames := make(map[string]string, len(repos))
	for _, r := range repos {
		repoNames[r.Name] = r.Path
	}

	opts := search.SearchOptions{
		Pattern:       in.Query,
		RepoFilter:    insensitiveRepoFilter(in.Repo),
		FileFilter:    in.File,
		Lang:          in.Lang,
		CaseSensitive: in.CaseSensitive,
		Limit:         limit,
		ContextLines:  in.ContextLines,
		OutputMode:    outputMode,
	}

	matches, err := runSearch(ctx, indexDir, socketPath, opts, repoNames)
	if err != nil {
		return nil, searchOutput{}, err
	}

	return nil, buildSearchOutput(outputMode, limit, matches), nil
}

// runSearch mirrors the daemon-first-then-fallback pattern from
// internal/cli/search.go, minus the indexing / progress output (the daemon
// handles that, and MCP clients do not benefit from progress logs written
// to stderr during a single JSON-RPC call).
func runSearch(
	ctx context.Context,
	indexDir, socketPath string,
	opts search.SearchOptions,
	repoNames map[string]string,
) ([]search.Match, error) {
	if err := daemon.EnsureDaemon(indexDir, socketPath); err == nil {
		matches, err := daemon.SearchVia(ctx, socketPath, opts, repoNames)
		if err == nil {
			return matches, nil
		}
		if !errors.Is(err, daemon.ErrDaemonNotRunning) {
			return nil, err
		}
	}
	return search.Search(ctx, indexDir, opts, repoNames)
}

const maxContentLines = 300

func buildSearchOutput(mode string, limit int, matches []search.Match) searchOutput {
	out := searchOutput{OutputMode: mode}
	if len(matches) == 0 {
		return out
	}

	if mode == contentOutputMode {
		lines := make([]searchMatchLine, 0, len(matches))
		for _, m := range matches {
			lines = append(lines, searchMatchLine{
				Repo:   m.Repo,
				Path:   m.File,
				Line:   m.Line,
				Column: m.Column,
				Text:   m.Text,
				Before: m.Before,
				After:  m.After,
			})
		}
		if len(lines) > maxContentLines {
			out.Truncated = true
			out.TotalAvailable = len(lines)
			lines = lines[:maxContentLines]
		}
		out.Lines = lines
		out.Total = len(lines)
		return out
	}

	// files_with_matches: collapse to unique file paths.
	seen := make(map[string]struct{}, len(matches))
	files := make([]searchMatchFile, 0, len(matches))
	for _, m := range matches {
		key := m.Repo + "/" + m.File
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		files = append(files, searchMatchFile{Repo: m.Repo, Path: m.File})
	}
	if len(files) >= limit {
		out.Truncated = true
		out.TotalAvailable = len(files)
		files = files[:limit]
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
		return nil, countOutput{}, fmt.Errorf("query is required")
	}
	if in.GroupBy != "" && in.GroupBy != "repo" && in.GroupBy != "language" {
		return nil, countOutput{}, fmt.Errorf(
			"group_by must be 'repo', 'language', or empty, got %q",
			in.GroupBy,
		)
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, countOutput{}, fmt.Errorf("resolve index dir: %w", err)
	}
	socketPath := daemon.DefaultSocketPath()

	opts := search.CountOptions{
		Pattern:    in.Query,
		RepoFilter: insensitiveRepoFilter(in.Repo),
		Lang:       in.Lang,
		GroupBy:    in.GroupBy,
	}

	var (
		results []search.CountResult
		total   int
	)

	if err := daemon.EnsureDaemon(indexDir, socketPath); err == nil {
		results, total, err = daemon.CountVia(ctx, socketPath, opts)
		if err != nil {
			if !errors.Is(err, daemon.ErrDaemonNotRunning) {
				return nil, countOutput{}, fmt.Errorf("count: %w", err)
			}
			results, total, err = search.Count(ctx, indexDir, opts)
			if err != nil {
				return nil, countOutput{}, fmt.Errorf("count: %w", err)
			}
		}
	} else {
		results, total, err = search.Count(ctx, indexDir, opts)
		if err != nil {
			return nil, countOutput{}, fmt.Errorf("count: %w", err)
		}
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
		return nil, queryValidateOutput{}, fmt.Errorf("query is required")
	}
	info := search.ValidateQuery(in.Query)
	return nil, queryValidateOutput{
		Valid:  info.Valid,
		Parsed: info.Parsed,
		Error:  info.Error,
		Hint:   info.Hint,
	}, nil
}
