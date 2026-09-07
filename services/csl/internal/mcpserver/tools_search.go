package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	Query         string `json:"query"                    jsonschema:"zoekt query: literal / regex / 'phrase' / AND (space) / OR (|) / NOT (-) / repo: / file: / lang: / case:yes"`
	Repo          string `json:"repo,omitempty"           jsonschema:"restrict to repo names matching this case-insensitive regex (a plain substring also works)"`
	Lang          string `json:"lang,omitempty"           jsonschema:"restrict to files of this language (e.g. go, swift, python)"`
	File          string `json:"file,omitempty"           jsonschema:"restrict to file paths matching this regex (e.g. paths ending in .go)"`
	OutputMode    string `json:"output_mode,omitempty"    jsonschema:"files_with_matches (default) returns unique file paths; content returns matching lines with context"`
	ContextLines  int    `json:"context_lines,omitempty"  jsonschema:"number of context lines around each match in content mode (default 0); ignored in files_with_matches mode"`
	Limit         int    `json:"limit,omitempty"          jsonschema:"maximum number of file results (default 50)"`
	Offset        int    `json:"offset,omitempty"         jsonschema:"skip this many ranked file results before applying limit, to page past the first limit results (default 0); ranking is stable across calls, so successive pages line up"`
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
	OutputMode     string            `json:"output_mode"                jsonschema:"echoes the output mode actually used (files_with_matches or content)"`
	Files          []searchMatchFile `json:"files,omitempty"            jsonschema:"unique file paths that matched; set when output_mode is files_with_matches"`
	Lines          []searchMatchLine `json:"lines,omitempty"            jsonschema:"matching lines; set when output_mode is content"`
	Total          int               `json:"total"                      jsonschema:"total number of match records returned (files or lines)"`
	Offset         int               `json:"offset,omitempty"           jsonschema:"the ranked-file offset this page started at; request the next page with offset + limit"`
	Truncated      bool              `json:"truncated"                  jsonschema:"true if limit capped the results; more matches exist, so page with offset (offset + limit), refine your query, or increase limit"`
	TotalAvailable int               `json:"total_available,omitempty"  jsonschema:"matches in this page before limit capped it (only set when truncated is true); not a grand total across pages"`
	ZeroHint       *searchZeroHint   `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: how the query was parsed, how many repos the filters covered, and index age; read it before assuming the code doesn't exist"`
}

// searchZeroHint explains a zero-hit search so an agent can tell a genuinely
// empty result from a malformed query or a stale index. Additive: it appears
// only when the search returned nothing, and never changes non-empty output.
type searchZeroHint struct {
	ParsedQuery     string   `json:"parsed_query,omitempty"      jsonschema:"the effective query (filters folded in) as zoekt parsed it; check that terms and operators mean what you intended"`
	ReposSearched   int      `json:"repos_searched"              jsonschema:"repos the repo filter matched that are also present in the search index (only indexed repos can produce hits); 0 means the filter or index coverage is the problem, not the query"`
	ReposDiscovered int      `json:"repos_discovered"            jsonschema:"git repos discovered under the configured dirs"`
	ReposIndexed    int      `json:"repos_indexed,omitempty"     jsonschema:"repos present in the search index; a repo discovered but not indexed is invisible to search until indexed"`
	NewestIndexedAt string   `json:"newest_indexed_at,omitempty" jsonschema:"most recent per-repo index time (RFC3339)"`
	OldestIndexedAt string   `json:"oldest_indexed_at,omitempty" jsonschema:"least recent per-repo index time (RFC3339); very old means some repo's index is stale"`
	Notes           []string `json:"notes,omitempty"             jsonschema:"targeted suggestions for this query (known syntax traps, filter mismatches)"`
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
	Groups []countGroup `json:"groups,omitempty" jsonschema:"per-group counts; empty when group_by isn't set"`
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
			"Use whenever the task involves finding where a symbol, function, pattern, or string is used: 'where is X defined', 'find all Y', 'does any of my projects use Z', 'show me every TODO in the Go code'. " +
			"Defaults to returning unique matching file paths (files_with_matches); set output_mode to 'content' to get matching lines with optional context. " +
			"Query syntax: literal substring, regex, \"quoted phrase\", AND (space), OR (|), NOT (-), repo:name, f:\\.go$, lang:go, case:yes. " +
			"AND is strict: all terms must appear in the SAME FILE. Use 1-2 terms and narrow with repo:/f:/lang: filters, not 3+ chained terms. " +
			"Use | or lowercase 'or' for OR; uppercase OR is treated as a literal string, and spaces around | break it (a | b is three AND terms, not OR). " +
			"Filter prefixes: repo: (not r:), f: (not file:). Prefer the dedicated repo/lang/file params over inline filter syntax: the repo param is case-insensitive, while an inline repo: filter is raw zoekt (case-sensitive regex). " +
			"Defaults and caps: limit 50 files, context_lines 0; content mode returns at most 300 lines per call. When capped, truncated=true; page with offset (next page = offset + limit), narrow the query, or raise limit. " +
			"On zero results the response carries zero_result_hint (the query as zoekt parsed it, repos the filters covered, index age, known syntax traps); read it before retrying or concluding the code doesn't exist. " +
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
			"Use whenever a query returns zero results or behaves unexpectedly: the parsed tree shows exactly how zoekt interpreted your terms. " +
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
	offset := in.Offset
	if offset < 0 {
		offset = 0
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

	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("walk repos: %w", err)
	}
	if len(repos) == 0 {
		return nil, searchOutput{OutputMode: outputMode, ZeroHint: &searchZeroHint{
			Notes: []string{
				"no git repos discovered under the configured dirs; run 'csl config' to check dirs and index.hosts",
			},
		}}, nil
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
		Offset:        offset,
		ContextLines:  in.ContextLines,
		OutputMode:    outputMode,
	}

	matches, err := runSearch(ctx, indexDir, socketPath, opts, repoNames)
	if err != nil {
		return nil, searchOutput{}, err
	}

	out := buildSearchOutput(outputMode, limit, offset, matches)
	if out.Total == 0 {
		out.ZeroHint = buildZeroHint(in, opts, repos, indexDir)
	}
	return nil, out, nil
}

// buildZeroHint assembles the zero_result_hint payload for a search that ran
// cleanly but matched nothing. Best-effort: any piece that cannot be computed
// is omitted rather than failing the response.
func buildZeroHint(
	in searchInput,
	opts search.SearchOptions,
	repos []finder.Repo,
	indexDir string,
) *searchZeroHint {
	hint := &searchZeroHint{
		ReposDiscovered: len(repos),
		Notes:           append(queryTrapNotes(in.Query), overConstraintNotes(in)...),
	}

	if info := search.ValidateQuery(search.BuildQueryString(opts)); info.Valid {
		hint.ParsedQuery = info.Parsed
	}

	indexed := make(map[string]struct{})
	if state, err := search.LoadState(indexDir); err == nil {
		hint.ReposIndexed = len(state.Repos)
		var newest, oldest time.Time
		for path, rs := range state.Repos {
			indexed[path] = struct{}{}
			if rs.IndexedAt.IsZero() {
				continue
			}
			if newest.IsZero() || rs.IndexedAt.After(newest) {
				newest = rs.IndexedAt
			}
			if oldest.IsZero() || rs.IndexedAt.Before(oldest) {
				oldest = rs.IndexedAt
			}
		}
		if !newest.IsZero() {
			hint.NewestIndexedAt = newest.Format(time.RFC3339)
			hint.OldestIndexedAt = oldest.Format(time.RFC3339)
		}
	}

	searched, notes := repoFilterHint(in.Repo, repos, indexed)
	hint.ReposSearched = searched
	hint.Notes = append(hint.Notes, notes...)
	return hint
}

// repoFilterHint reports how many discovered repos the repo filter matches
// AND the index actually covers — zoekt cannot return hits from a repo that
// is discovered on disk but not yet indexed. It mirrors the case-insensitive
// matching the repo tools use; the whitespace case is called out instead of
// diagnosed, because the search itself space-splits the filter into separate
// zoekt terms and no repo-name count describes what actually ran.
func repoFilterHint(
	repoFilter string,
	repos []finder.Repo,
	indexed map[string]struct{},
) (int, []string) {
	countIndexed := func(rs []finder.Repo) (n int) {
		for _, r := range rs {
			if _, ok := indexed[r.Path]; ok {
				n++
			}
		}
		return n
	}

	if repoFilter == "" {
		return countIndexed(repos), nil
	}
	if strings.ContainsAny(repoFilter, " \t") {
		return 0, []string{fmt.Sprintf(
			"repo filter %q contains whitespace; the search splits it into separate zoekt terms, so it is not matched as one repo name — use a regex without spaces",
			repoFilter,
		)}
	}

	re, err := finder.CompileMatcher(repoFilter)
	if err != nil {
		return 0, nil
	}
	var matched []finder.Repo
	for _, r := range repos {
		if re.MatchString(r.Name) {
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 {
		return 0, []string{fmt.Sprintf(
			"repo filter %q matched none of the %d locally discovered repos; check the name with csl_repo_lookup — if the repo is not checked out locally, csl cannot see it, so search it where it is hosted instead of retrying here",
			repoFilter,
			len(repos),
		)}
	}
	searched := countIndexed(matched)
	if unindexed := len(matched) - searched; unindexed > 0 {
		return searched, []string{fmt.Sprintf(
			"%d of the %d repos matching the filter are not in the search index yet and are invisible to search; run csl_repo_reindex on them",
			unindexed,
			len(matched),
		)}
	}
	return searched, nil
}

// queryTrapNotes flags known zoekt syntax traps present in the raw query that
// commonly explain a surprising zero-hit result. Quoted phrases are stripped
// first: an ' OR ' inside a "quoted literal" is content, not an operator.
func queryTrapNotes(query string) []string {
	query = stripQuoted(query)
	var notes []string
	if strings.Contains(query, " | ") {
		notes = append(notes,
			"'a | b' parses as three AND terms, not OR; write a|b with no spaces")
	}
	if strings.Contains(query, " OR ") {
		notes = append(notes,
			"uppercase OR is a literal search term; use | with no spaces or lowercase 'or'")
	}
	return notes
}

// overConstraintNotes flags the query shapes that most often explain a
// zero-hit search: 3+ AND terms, a verbatim-only quoted phrase, and a stacked
// file filter. Sessions loosen these one guess at a time over long refinement
// chains; naming them up front is what shortens the chain.
func overConstraintNotes(in searchInput) []string {
	var notes []string
	if n := andTermCount(in.Query); n >= 3 {
		notes = append(notes, fmt.Sprintf(
			"query has %d AND terms that must ALL appear in the same file — retry with 1-2 key terms, or join alternatives as a|b (no spaces)",
			n,
		))
	}
	for _, span := range quotedSpans(in.Query) {
		if strings.ContainsAny(span, " \t") {
			notes = append(
				notes,
				"a \"quoted phrase\" matches only that exact text verbatim — drop the quotes to match the words as separate AND terms",
			)
			break
		}
	}
	if in.File != "" {
		notes = append(
			notes,
			"the file filter is the most common over-constraint — retry without it before loosening the query",
		)
	}
	return notes
}

// andTermCount counts the AND terms zoekt will require in the same file:
// bare whitespace-separated tokens plus one per quoted phrase. Filters
// (repo:/f:/lang:), negations, OR groups, and the or operator don't count —
// they narrow or widen, but they are not another required term.
func andTermCount(query string) int {
	n := len(quotedSpans(query))
	for tok := range strings.FieldsSeq(stripQuoted(query)) {
		if strings.Contains(tok, ":") || strings.HasPrefix(tok, "-") ||
			strings.Contains(tok, "|") || strings.EqualFold(tok, "or") {
			continue
		}
		n++
	}
	return n
}

// quotedSpans returns the contents of every complete double-quoted span in
// the query, in order. An unclosed quote yields no span for its tail.
func quotedSpans(s string) []string {
	var spans []string
	for {
		i := strings.Index(s, `"`)
		if i < 0 {
			return spans
		}
		s = s[i+1:]
		j := strings.Index(s, `"`)
		if j < 0 {
			return spans
		}
		spans = append(spans, s[:j])
		s = s[j+1:]
	}
}

// stripQuoted removes double-quoted spans from a query so trap sniffing does
// not fire on operators that appear inside a quoted literal phrase.
func stripQuoted(s string) string {
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			b.WriteRune(r)
		}
	}
	return b.String()
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
