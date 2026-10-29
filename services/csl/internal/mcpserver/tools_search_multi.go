package mcpserver

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/repopath"
)

// csl_search_multi runs several independent csl_search queries in one call.
// Each query takes csl_search's own path (newSearchRequest, then
// searchEnv.search), against one repo discovery and one backend shared by
// the whole call.

const (
	// maxMultiQueries caps the queries one call may carry.
	maxMultiQueries = 10
	// multiSearchWorkers bounds how many of a call's queries run at once.
	multiSearchWorkers = 4
	// topHitLines is how much of the top-ranked file read_top_hit inlines,
	// well under csl_read's maxReadLines.
	topHitLines = 60
	// multiLineBudget is csl_search's per-call line cap, applied to one whole
	// csl_search_multi call: content lines and inlined lines share it.
	multiLineBudget = maxContentLines
)

// topHitCutNote replaces a top hit the line budget left no room for.
const topHitCutNote = "cut by the shared line budget; read it with csl_read"

// searchMultiQuery is one query of a csl_search_multi call: csl_search's
// parameters without offset, plus read_top_hit.
type searchMultiQuery struct {
	Query         string `json:"query"                    jsonschema:"zoekt query, same syntax as csl_search"`
	Repo          string `json:"repo,omitempty"           jsonschema:"restrict to repo names matching this case-insensitive regex"`
	Lang          string `json:"lang,omitempty"           jsonschema:"restrict to files of this language"`
	File          string `json:"file,omitempty"           jsonschema:"restrict to file paths matching this regex"`
	OutputMode    string `json:"output_mode,omitempty"    jsonschema:"files_with_matches (default) or content"`
	ContextLines  int    `json:"context_lines,omitempty"  jsonschema:"context lines around each match in content mode (default 0)"`
	Limit         int    `json:"limit,omitempty"          jsonschema:"maximum number of file results (default 50)"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"force case-sensitive matching; default is smart case"`
	ReadTopHit    bool   `json:"read_top_hit,omitempty"   jsonschema:"also return the first 60 lines of the top-ranked file as top_hit, so the read that would follow the search comes back in this call"`
}

// searchInput maps the query onto csl_search's input.
func (q searchMultiQuery) searchInput() searchInput {
	return searchInput{
		Query:         q.Query,
		Repo:          q.Repo,
		Lang:          q.Lang,
		File:          q.File,
		OutputMode:    q.OutputMode,
		ContextLines:  q.ContextLines,
		Limit:         q.Limit,
		CaseSensitive: q.CaseSensitive,
	}
}

// searchMultiInput is the typed input for the csl_search_multi tool.
type searchMultiInput struct {
	Queries []searchMultiQuery `json:"queries" jsonschema:"1 to 10 independent queries; each gets its own entry in results, in the order sent"`
	formatParam
}

// searchMultiResult is one query's entry. A query that failed carries only
// Error; every other entry carries the csl_search output fields, flattened.
type searchMultiResult struct {
	Query string `json:"query"                   jsonschema:"the query this entry answers"`
	Error string `json:"error,omitempty"         jsonschema:"why this query failed; the other entries are unaffected"`
	*searchOutput
	TopHit      *readOutput `json:"top_hit,omitempty"       jsonschema:"the first lines of the top-ranked file; set when read_top_hit was requested and the query matched"`
	TopHitError string      `json:"top_hit_error,omitempty" jsonschema:"why the top-ranked file could not be read"`
}

// searchMultiOutput is the typed output of the csl_search_multi tool.
type searchMultiOutput struct {
	Results   []searchMultiResult `json:"results"        jsonschema:"one entry per query, in the order sent"`
	Truncated bool                `json:"truncated"      jsonschema:"true if any entry's results were capped; that entry carries truncated and total_available"`
	Note      string              `json:"note,omitempty" jsonschema:"set when the shared line budget cut an entry, saying how to get the rest"`
}

func registerSearchMultiTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_search_multi",
		Description: "Run several independent zoekt searches in one call and get one entry per query, in the order sent. " +
			"Use it when you have two or more queries that do not depend on each other's results (several symbols, patterns or repos); for a single query call csl_search. " +
			"Each query takes csl_search's parameters except offset (query, repo, file, lang, output_mode, limit, context_lines, case_sensitive) and gets csl_search's treatment: a malformed query is refused with the fix, a zero-hit query carries zero_result_hint, a relaxed rerun carries relaxed_query and dropped_terms, and a capped one carries truncated and total_available. " +
			"A query that fails carries its error in its own entry and does not fail the others. " +
			"Set read_top_hit on a query to inline the first 60 lines of its top-ranked file as top_hit, so a search and the read that follows it take one call. " +
			"At most 10 queries per call. Content lines and top_hit lines share a 300-line budget per call, split evenly across the queries that need it; to see more of a cut entry, rerun that query alone with csl_search. " +
			"Query syntax and pitfalls are csl_search's: space is AND within one file, a|b is OR, sym:Name finds definitions.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, handleSearchMulti, renderSearchMultiText)
}

func handleSearchMulti(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in searchMultiInput,
) (*mcp.CallToolResult, searchMultiOutput, error) {
	if err := checkMultiQueryCount(len(in.Queries)); err != nil {
		return nil, searchMultiOutput{}, err
	}
	env, err := newMultiSearchEnv()
	if err != nil {
		return nil, searchMultiOutput{}, err
	}
	return nil, runSearchMulti(ctx, env, in.Queries), nil
}

// checkMultiQueryCount refuses an empty call and one past maxMultiQueries.
func checkMultiQueryCount(n int) error {
	switch {
	case n == 0:
		return fmt.Errorf(
			"queries is empty; send 1 to %d queries, or call csl_search for one",
			maxMultiQueries,
		)
	case n > maxMultiQueries:
		return fmt.Errorf(
			"csl_search_multi takes at most %d queries per call, got %d; split them across calls",
			maxMultiQueries,
			n,
		)
	}
	return nil
}

// ensureDaemon starts the search daemon when nothing answers. Tests replace
// it, since starting the daemon from a test binary runs the test binary.
var ensureDaemon = daemon.EnsureDaemon

// newMultiSearchEnv does csl_search's setup once for the batch and starts the
// daemon before the fan-out, so the workers find it running instead of each
// racing to start one. When that start fails the whole call searches
// in-process: the daemon backend would retry the start for every query and
// every zero-result count, each waiting up to a second.
func newMultiSearchEnv() (searchEnv, error) {
	env, err := newSearchEnv()
	if err != nil {
		return searchEnv{}, err
	}
	if len(env.repos) == 0 {
		return env, nil
	}
	if err := ensureDaemon(env.indexDir, daemon.DefaultSocketPath()); err != nil {
		env.backend = lexical.Local{IndexDir: env.indexDir, RepoNames: env.repoPaths}
	}
	return env, nil
}

// runSearchMulti runs the queries on at most multiSearchWorkers goroutines,
// then fits the results into the call's line budget. A failing query lands
// in its own entry, so no worker returns an error. A panic does too: errgroup
// does not recover one and neither does the MCP SDK, so one bad query would
// otherwise take csl mcp down.
func runSearchMulti(
	ctx context.Context,
	env searchEnv,
	queries []searchMultiQuery,
) searchMultiOutput {
	results := make([]searchMultiResult, len(queries))
	var g errgroup.Group
	g.SetLimit(multiSearchWorkers)
	for i, q := range queries {
		g.Go(func() error {
			defer func() {
				if p := recover(); p != nil {
					// The entry carries the value; the stack goes to stderr, the
					// only place a stdio MCP server can leave it.
					fmt.Fprintf(os.Stderr, "csl_search_multi: panic in query %q: %v\n%s", q.Query, p, debug.Stack())
					results[i] = searchMultiResult{
						Query: q.Query,
						Error: fmt.Sprintf("internal error: %v", p),
					}
				}
			}()
			results[i] = answerQuery(ctx, env, q)
			return nil
		})
	}
	_ = g.Wait()
	return fitLineBudget(results, multiLineBudget)
}

// answerQuery answers one query: the search csl_search runs, then the
// top-hit read when asked. Every query reads the first page. A query still
// queued when the call is cancelled does not start.
func answerQuery(ctx context.Context, env searchEnv, q searchMultiQuery) searchMultiResult {
	res := searchMultiResult{Query: q.Query}
	if err := ctx.Err(); err != nil {
		res.Error = err.Error()
		return res
	}
	req, err := newSearchRequest(q.searchInput())
	var out searchOutput
	if err == nil {
		out, err = env.search(ctx, req)
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.searchOutput = &out
	if q.ReadTopHit {
		res.TopHit, res.TopHitError = readTopHit(env.repoPaths, out)
	}
	return res
}

// readTopHit reads the first topHitLines lines of the page's top-ranked
// file, refusing a path that leaves the repo the way csl_read does. A page
// with no file yields nothing; a read failure comes back as text for the
// entry, since the search itself succeeded.
func readTopHit(repoPaths map[string]string, out searchOutput) (*readOutput, string) {
	repo, file, ok := topHit(out)
	if !ok {
		return nil, ""
	}
	root, ok := repoPaths[repo]
	if !ok {
		return nil, fmt.Sprintf("repo %s is not among the discovered repos", repo)
	}
	path, err := repopath.Resolve(root, "file", file)
	if err != nil {
		return nil, err.Error()
	}
	lines, total, err := readLines(path, 1, topHitLines)
	if err != nil {
		return nil, err.Error()
	}
	return &readOutput{
		Repo:       repo,
		Path:       file,
		Lines:      lines,
		TotalLines: total,
		Truncated:  total > len(lines),
	}, ""
}

// topHit names the first file of a page: zoekt ranks files, and content
// lines arrive grouped by file in that order.
func topHit(out searchOutput) (repo, file string, ok bool) {
	switch {
	case len(out.Files) > 0:
		return out.Files[0].Repo, out.Files[0].Path, true
	case len(out.Lines) > 0:
		return out.Lines[0].Repo, out.Lines[0].Path, true
	}
	return "", "", false
}

// fitLineBudget cuts the entries' content lines and top-hit lines down to
// budget in total, shared by shareLines. Within an entry the search lines
// come first, since they are the answer, and the top hit gets the rest. A
// cut page is marked the way csl_search marks a capped one.
func fitLineBudget(results []searchMultiResult, budget int) searchMultiOutput {
	needs := make([]int, len(results))
	for i, r := range results {
		needs[i] = entryLines(r)
	}
	allot := shareLines(needs, budget)

	out := searchMultiOutput{Results: results}
	cut := false
	for i := range results {
		r := &results[i]
		if allot[i] < needs[i] {
			cutEntry(r, allot[i])
			cut = true
		}
		if r.searchOutput != nil && r.Truncated {
			out.Truncated = true
		}
	}
	if cut {
		out.Note = fmt.Sprintf(
			"the %d-line budget is shared by every query in one call, so some entries were cut; rerun a cut query alone with csl_search for its full page",
			budget,
		)
	}
	return out
}

// entryLines counts the lines an entry would print against the budget:
// content lines and top-hit lines. File paths are bounded by limit instead.
func entryLines(r searchMultiResult) int {
	n := 0
	if r.searchOutput != nil {
		n += len(r.Lines)
	}
	if r.TopHit != nil {
		n += len(r.TopHit.Lines)
	}
	return n
}

// cutEntry keeps the first allot lines of an entry, content lines first. A
// top hit with no lines left is dropped and says so, since an empty one
// would read as an empty file.
func cutEntry(r *searchMultiResult, allot int) {
	if r.searchOutput != nil {
		if len(r.Lines) > allot {
			if !r.Truncated {
				r.Truncated = true
				r.TotalAvailable = len(r.Lines)
			}
			r.Lines = r.Lines[:allot]
			r.Total = allot
		}
		allot -= len(r.Lines)
	}
	if r.TopHit == nil || len(r.TopHit.Lines) <= allot {
		return
	}
	if allot <= 0 {
		r.TopHit = nil
		r.TopHitError = topHitCutNote
		return
	}
	r.TopHit.Lines = r.TopHit.Lines[:allot]
	r.TopHit.Truncated = true
}

// shareLines splits budget across needs so that an entry needing less than
// an even share keeps all of it, and what it leaves goes to the others in
// the next round. Lines left over after the even split go to the earliest
// entries, one each.
func shareLines(needs []int, budget int) []int {
	allot := make([]int, len(needs))
	var open []int
	for i, n := range needs {
		if n > 0 {
			open = append(open, i)
		}
	}
	for len(open) > 0 && budget > 0 {
		share := max(budget/len(open), 1)
		var still []int
		for _, i := range open {
			give := min(needs[i]-allot[i], share, budget)
			allot[i] += give
			budget -= give
			if allot[i] < needs[i] {
				still = append(still, i)
			}
		}
		open = still
	}
	return allot
}

// renderSearchMultiText renders each entry under a header that gives its
// place in the call and its query, with csl_search's text form for the page
// and csl_read's for the top hit. The budget note, when set, closes it.
func renderSearchMultiText(out searchMultiOutput) string {
	sections := make([]string, 0, len(out.Results)+1)
	for i, r := range out.Results {
		sections = append(sections, renderMultiEntry(i+1, len(out.Results), r))
	}
	if out.Note != "" {
		sections = append(sections, "note: "+out.Note)
	}
	return strings.Join(sections, "\n\n")
}

// renderMultiEntry renders one entry: its header, then the error, or the
// page followed by the top hit.
func renderMultiEntry(n, of int, r searchMultiResult) string {
	lines := []string{fmt.Sprintf("=== query %d/%d: %s", n, of, r.Query)}
	if r.Error != "" {
		return strings.Join(append(lines, "error: "+r.Error), "\n")
	}
	if r.searchOutput != nil {
		lines = append(lines, renderSearchText(*r.searchOutput))
	}
	switch {
	case r.TopHit != nil:
		lines = append(lines, "", "top hit: "+renderReadText(*r.TopHit))
	case r.TopHitError != "":
		lines = append(lines, "", "top hit: "+r.TopHitError)
	}
	return strings.Join(lines, "\n")
}
