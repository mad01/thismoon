package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
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
	Query         string `json:"query"                    jsonschema:"zoekt query: literal / regex / 'phrase' / AND (space) / OR (|) / NOT (-) / sym: (definitions only) / repo: / file: / lang: / case:yes"`
	Repo          string `json:"repo,omitempty"           jsonschema:"restrict to repo names matching this case-insensitive regex (a plain substring also works)"`
	Lang          string `json:"lang,omitempty"           jsonschema:"restrict to files of this language (e.g. go, swift, python)"`
	File          string `json:"file,omitempty"           jsonschema:"restrict to file paths matching this regex (e.g. paths ending in .go)"`
	OutputMode    string `json:"output_mode,omitempty"    jsonschema:"files_with_matches (default) returns unique file paths; content returns matching lines with context"`
	ContextLines  int    `json:"context_lines,omitempty"  jsonschema:"number of context lines around each match in content mode (default 0); ignored in files_with_matches mode"`
	Limit         int    `json:"limit,omitempty"          jsonschema:"maximum number of file results (default 50)"`
	Offset        int    `json:"offset,omitempty"         jsonschema:"skip this many ranked file results before applying limit, to page past the first limit results (default 0); ranking is stable across calls, so successive pages line up"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"force case-sensitive matching; default is smart case (case-insensitive unless the query has uppercase)"`
	formatParam
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
	Kind   string `json:"kind,omitempty"   jsonschema:"symbol kind of the definition on this line (function, method, struct, class, ...); set only for sym: hits"`
	Parent string `json:"parent,omitempty" jsonschema:"enclosing declaration of the definition (receiver type, class, message); set only for sym: hits"`
}

// searchOutput is the typed output of the csl_search tool. Exactly one of
// Files / Lines is populated depending on OutputMode.
type searchOutput struct {
	OutputMode     string            `json:"output_mode"                jsonschema:"echoes the output mode actually used (files_with_matches or content)"`
	Files          []searchMatchFile `json:"files,omitempty"            jsonschema:"unique file paths that matched; set when output_mode is files_with_matches"`
	Lines          []searchMatchLine `json:"lines,omitempty"            jsonschema:"matching lines; set when output_mode is content"`
	Total          int               `json:"total"                      jsonschema:"total number of match records returned (files or lines)"`
	Offset         int               `json:"offset,omitempty"           jsonschema:"the ranked-file offset this page started at; request the next page with offset + limit"`
	Truncated      bool              `json:"truncated"                  jsonschema:"true if limit capped the results; more matches exist, so page with offset (offset + limit), refine your query, or increase limit"`
	TotalAvailable int               `json:"total_available,omitempty"  jsonschema:"matches in this page before limit capped it (only set when truncated is true); not a grand total across pages"`
	RelaxedQuery   string            `json:"relaxed_query,omitempty"    jsonschema:"set when the original query matched nothing and the search reran once without the AND terms that match no file at all; the query these results come from"`
	DroppedTerms   []string          `json:"dropped_terms,omitempty"    jsonschema:"the AND terms removed to form relaxed_query, each matching zero files under the same filters; do not add them back"`
	ZeroHint       *lexical.ZeroHint `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: how the query was parsed, files per AND term, how many repos the filters covered, and index age; read it before assuming the code doesn't exist"`
}

// countInput is the typed input for the csl_count tool.
type countInput struct {
	Query   string `json:"query"              jsonschema:"zoekt query (same syntax as csl_search)"`
	Repo    string `json:"repo,omitempty"     jsonschema:"restrict to repo names matching this case-insensitive regex (a plain substring also works)"`
	Lang    string `json:"lang,omitempty"     jsonschema:"restrict to files of this language"`
	GroupBy string `json:"group_by,omitempty" jsonschema:"group matches by: repo or language; empty returns a single total"`
	formatParam
}

// countGroup is one entry in the csl_count result.
type countGroup struct {
	Group string `json:"group" jsonschema:"the repo name or language, depending on group_by"`
	Count int    `json:"count"`
}

// countOutput is the typed output of the csl_count tool.
type countOutput struct {
	Total  int          `json:"total"            jsonschema:"total match count across all groups"`
	Groups []countGroup `json:"groups,omitempty" jsonschema:"per-group counts; empty when group_by isn't set"`
}

// queryValidateInput is the typed input for the csl_query_validate tool.
type queryValidateInput struct {
	Query string `json:"query" jsonschema:"the zoekt query string to validate"`
	formatParam
}

// queryValidateOutput is the typed output of the csl_query_validate tool.
type queryValidateOutput struct {
	Valid   bool     `json:"valid"`
	Parsed  string   `json:"parsed,omitempty"  jsonschema:"string representation of the parsed query tree when valid"`
	Error   string   `json:"error,omitempty"   jsonschema:"parse error message when not valid"`
	Hint    string   `json:"hint,omitempty"    jsonschema:"suggestion for fixing the query when not valid, or a trap spotted in a valid one"`
	Terms   []string `json:"terms,omitempty"   jsonschema:"top-level AND terms, each of which must match in the same file; a zero-result csl_search counts these and drops the ones matching no file"`
	Filters []string `json:"filters,omitempty" jsonschema:"filter atoms and negations (repo:, f:, lang:, sym:, case:, -term) that narrow the search and are never dropped"`
}

// parallelCallsTail ends the call-site nudge on a read-only tool's
// description, after a "Several independent <items>: " lead-in naming what
// the caller has several of. The model reads it while deciding how many calls
// to send this turn.
const parallelCallsTail = "send them as parallel calls in one turn, not one per turn; " +
	"this tool is read-only and runs concurrently."

func registerSearchTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_search",
		Description: "Search code across locally checked-out git repos using zoekt query syntax. " +
			"Use whenever the task involves finding where a symbol, function, pattern, or string is used: 'where is X defined', 'find all Y', 'does any of my projects use Z', 'show me every TODO in the Go code'. " +
			"Defaults to returning unique matching file paths (files_with_matches); set output_mode to 'content' to get matching lines with optional context. " +
			"Query syntax: literal substring, regex, \"quoted phrase\", AND (space), OR (|), NOT (-), repo:name, f:\\.go$, lang:go, case:yes. " +
			"sym:Name matches symbol definitions only (function, method, type, class, field names as tree-sitter extracts them) and skips call sites and comments; in content mode each sym: hit carries kind (and parent for nested definitions). Plain queries already rank a definition's file above its call sites. " +
			"Two names: sym:Foo|Bar (or sym:Foo or sym:Bar); sym:Foo|sym:Bar is one regexp and is refused with the fix. " +
			"AND is strict: all terms must appear in the SAME FILE. Use 1-2 terms and narrow with repo:/f:/lang: filters, not 3+ chained terms. " +
			"Use | or lowercase 'or' for OR; uppercase OR is treated as a literal string, and spaces around | break it (a | b is three AND terms, not OR). " +
			"Filter prefixes: repo: (not r:), f: (not file:). Prefer the dedicated repo/lang/file params over inline filter syntax: the repo param is case-insensitive, while an inline repo: filter is raw zoekt (case-sensitive regex). " +
			"Defaults and caps: limit 50 files, context_lines 0; content mode returns at most 300 lines per call. When capped, truncated=true; page with offset (next page = offset + limit), narrow the query, or raise limit. " +
			"On zero results the response carries zero_result_hint: the query as zoekt parsed it, term_counts (files matching each AND term alone), repos the filters covered, index age, and known traps; read it before retrying or concluding the code doesn't exist. " +
			"When some AND terms match no file and others do, the search reruns once without them and the response carries relaxed_query and dropped_terms; do not add a dropped term back. " +
			"A query that is empty, only quotes, or has an unbalanced quote is rejected with the fix instead of searched. " +
			"The results come from a persistent in-memory zoekt index maintained by the csl search daemon, so calls are fast across a session. " +
			"Set response_format to pick the encoding (text, the default, is ripgrep-style: `repo/path` headers, `LINE:match`, `LINE-context`; json restores the structured object); every csl tool accepts it. " +
			"Several independent queries: " + parallelCallsTail,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
		Meta: mcp.Meta{alwaysLoadKey: true},
	}, handleSearch, renderSearchText)

	addFormattedTool(s, &mcp.Tool{
		Name: "csl_count",
		Description: "Count matches of a zoekt query across locally checked-out repos. " +
			"Accepts the same zoekt query syntax as csl_search (including repo:/f:/lang: filters and AND/OR/NOT). " +
			"Use for cross-repo tallies like 'how many TODOs across my Go projects' or 'which language has the most calls to fmt.Errorf'. " +
			"Set group_by to 'repo' or 'language' for a breakdown; leave it empty for a single total.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, handleCount, renderCountText)

	addFormattedTool(s, &mcp.Tool{
		Name: "csl_query_validate",
		Description: "Validate a zoekt query and return its parsed tree or a parse error with a fixing hint. " +
			"Use whenever a query returns zero results or behaves unexpectedly: the parsed tree shows exactly how zoekt interpreted your terms, and terms/filters show the split csl_search's zero-result diagnosis works from (each term must match in the same file; filters are never dropped). " +
			"A malformed query (empty, only quotes, unbalanced quote) or a sym: term zoekt cannot run (sym:(Foo|Bar)+, sym:Foo|sym:Bar) reports valid=false with the fix, the same check csl_search applies before searching. " +
			"Also useful for debugging regex escaping like \\.go$.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, handleQueryValidate, nil)
}

func handleSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, searchOutput, error) {
	req, err := newSearchRequest(in)
	if err != nil {
		return nil, searchOutput{}, err
	}
	env, err := newSearchEnv()
	if err != nil {
		return nil, searchOutput{}, err
	}
	out, err := env.search(ctx, req)
	if err != nil {
		return nil, searchOutput{}, err
	}
	return nil, out, nil
}

// noReposNote is the whole answer when discovery finds no repos at all.
const noReposNote = "no git repos discovered under the configured dirs; run 'csl config' to check dirs and index.hosts"

// searchRequest is a csl_search input that passed the shape check, with its
// output mode, limit, and offset defaulted.
type searchRequest struct {
	in     searchInput
	mode   string
	limit  int
	offset int
}

// newSearchRequest refuses a malformed query or output mode and applies the
// defaults. It needs no config or index, so a bad input is refused before
// any setup runs.
func newSearchRequest(in searchInput) (searchRequest, error) {
	if m := lexical.CheckShape(in.Query, mcpSurface); m != nil {
		return searchRequest{}, m
	}
	r := searchRequest{in: in, mode: in.OutputMode, limit: in.Limit, offset: max(in.Offset, 0)}
	if r.mode == "" {
		r.mode = defaultSearchOutput
	}
	if r.mode != filesOutputMode && r.mode != contentOutputMode {
		return searchRequest{}, fmt.Errorf(
			"output_mode must be %q or %q, got %q",
			filesOutputMode, contentOutputMode, r.mode,
		)
	}
	if r.limit <= 0 {
		r.limit = defaultSearchLimit
	}
	return r, nil
}

// searchEnv is the setup a lexical search runs against: the backend, the
// repos discovery found, their checkout paths by name, and the index they
// search. csl_search builds one per call; csl_search_multi shares one across
// its queries.
type searchEnv struct {
	backend   lexical.Backend
	repos     []finder.Repo
	repoPaths map[string]string
	indexDir  string
}

// newSearchEnv resolves the index, loads the config, and discovers the
// repos. Its backend asks the daemon first and searches in-process without
// one.
func newSearchEnv() (searchEnv, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return searchEnv{}, fmt.Errorf("resolve index dir: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return searchEnv{}, fmt.Errorf("load csl config: %w", err)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return searchEnv{}, fmt.Errorf("walk repos: %w", err)
	}
	repoPaths := make(map[string]string, len(repos))
	for _, r := range repos {
		repoPaths[r.Name] = r.Path
	}
	return searchEnv{
		backend: lexical.Daemon{
			Local:      lexical.Local{IndexDir: indexDir, RepoNames: repoPaths},
			SocketPath: daemon.DefaultSocketPath(),
		},
		repos:     repos,
		repoPaths: repoPaths,
		indexDir:  indexDir,
	}, nil
}

// search runs one checked request: the search, the page shaped the way
// csl_search returns it, and on an empty page the zero-result diagnosis with
// its one relaxation.
func (e searchEnv) search(ctx context.Context, r searchRequest) (searchOutput, error) {
	if len(e.repos) == 0 {
		return searchOutput{OutputMode: r.mode, ZeroHint: &lexical.ZeroHint{
			Notes: []string{noReposNote},
		}}, nil
	}
	opts := search.SearchOptions{
		Pattern:       r.in.Query,
		RepoFilter:    lexical.RepoFilter(r.in.Repo),
		FileFilter:    r.in.File,
		Lang:          r.in.Lang,
		CaseSensitive: r.in.CaseSensitive,
		Limit:         r.limit,
		Offset:        r.offset,
		ContextLines:  r.in.ContextLines,
		OutputMode:    r.mode,
	}
	matches, err := e.backend.Search(ctx, opts)
	if err != nil {
		return searchOutput{}, err
	}
	if out := buildSearchOutput(r.mode, r.limit, r.offset, matches); out.Total > 0 {
		return out, nil
	}
	req := lexical.Request{Options: opts, Repo: r.in.Repo, Repos: e.repos, IndexDir: e.indexDir}
	return explainZero(ctx, e.backend, req), nil
}

// mcpSurface points the csl_search hint notes at the MCP tools, and the
// malformed-query fixes at the query parameter every lexical tool takes.
var mcpSurface = lexical.Surface{
	QueryArg:   "query",
	RepoLookup: "csl_repo_lookup",
	Reindex:    "csl_repo_reindex",
	ParamNote:  paramNameNote,
}

// explainZero shapes the outcome of an empty first page as csl_search output:
// the relaxed rerun's page when lexical.ExplainZero found one, else an empty
// page carrying the zero-result hint.
func explainZero(ctx context.Context, b lexical.Backend, req lexical.Request) searchOutput {
	opts := req.Options
	res := lexical.ExplainZero(ctx, b, mcpSurface, req)
	if res.Hint != nil {
		return searchOutput{OutputMode: opts.OutputMode, Offset: opts.Offset, ZeroHint: res.Hint}
	}
	out := buildSearchOutput(opts.OutputMode, opts.Limit, 0, res.Matches)
	out.RelaxedQuery = res.RelaxedQuery
	out.DroppedTerms = res.DroppedTerms
	return out
}

const maxContentLines = 300

func buildSearchOutput(mode string, limit, offset int, matches []search.Match) searchOutput {
	out := searchOutput{OutputMode: mode, Offset: offset}
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
				Kind:   m.Kind,
				Parent: m.Parent,
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
	if len(files) >= limit && offset+limit < search.RankUniverse {
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
	if m := lexical.CheckShape(in.Query, mcpSurface); m != nil {
		return nil, countOutput{}, m
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
		RepoFilter: lexical.RepoFilter(in.Repo),
		Lang:       in.Lang,
		GroupBy:    in.GroupBy,
	}
	backend := lexical.Daemon{Local: lexical.Local{IndexDir: indexDir}, SocketPath: socketPath}
	results, total, err := backend.Count(ctx, opts)
	if err != nil {
		return nil, countOutput{}, fmt.Errorf("count: %w", err)
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
	if m := lexical.CheckShape(in.Query, mcpSurface); m != nil {
		return nil, queryValidateOutput{Error: m.Problem, Hint: m.Fix}, nil
	}
	info := search.ValidateQuery(in.Query)
	out := queryValidateOutput{
		Valid:  info.Valid,
		Parsed: info.Parsed,
		Error:  info.Error,
		Hint:   info.Hint,
	}
	if !info.Valid {
		return nil, out, nil
	}
	parts := lexical.SplitTerms(in.Query)
	out.Terms, out.Filters = parts.Terms, parts.Fixed
	if note := paramNameNote(in.Query); note != "" {
		out.Hint = note
	}
	return nil, out, nil
}
