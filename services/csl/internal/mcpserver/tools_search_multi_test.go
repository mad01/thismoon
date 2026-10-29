package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// multiFake is a searchBackend the concurrent workers can share: results and
// errors keyed by pattern, per-term file counts for the zero-result
// diagnosis, and the most searches it saw in flight at once.
type multiFake struct {
	mu       sync.Mutex
	results  map[string][]search.Match
	errs     map[string]error
	files    map[string]int
	inFlight int
	peak     int
}

func (f *multiFake) search(_ context.Context, opts search.SearchOptions) ([]search.Match, error) {
	f.mu.Lock()
	f.inFlight++
	f.peak = max(f.peak, f.inFlight)
	f.mu.Unlock()

	// Hold the slot briefly so overlapping workers register in peak.
	time.Sleep(2 * time.Millisecond)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
	return f.results[opts.Pattern], f.errs[opts.Pattern]
}

func (f *multiFake) count(
	_ context.Context,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	term := strings.Fields(opts.Pattern)[1] // after the type:filename atom
	return nil, f.files[term], nil
}

// multiEnv builds the shared call state around a fake backend, with every
// repo rooted at root.
func multiEnv(t *testing.T, b searchBackend, root string, names ...string) multiSearchEnv {
	t.Helper()
	env := multiSearchEnv{backend: b, repoPaths: map[string]string{}, indexDir: t.TempDir()}
	for _, name := range names {
		env.repos = append(env.repos, finder.Repo{Name: name, Path: root})
		env.repoPaths[name] = root
	}
	return env
}

func TestSearchMulti_OneEntryPerQueryInOrder(t *testing.T) {
	fb := &multiFake{results: map[string][]search.Match{
		"alpha": makeFileMatches(2, "org/repo"),
		"beta":  makeMatches(3, "org/repo", "beta.go"),
		"gamma": makeFileMatches(1, "org/other"),
	}}
	env := multiEnv(t, fb, t.TempDir(), "org/repo", "org/other")

	out := runSearchMulti(context.Background(), env, []searchMultiQuery{
		{Query: "alpha"},
		{Query: "beta", OutputMode: contentOutputMode},
		{Query: "gamma", Repo: "other"},
	})

	var got []string
	for _, r := range out.Results {
		got = append(got, r.Query)
		if r.Error != "" {
			t.Errorf("%s: unexpected error %q", r.Query, r.Error)
		}
	}
	if want := []string{"alpha", "beta", "gamma"}; !slices.Equal(got, want) {
		t.Fatalf("result order = %v, want %v", got, want)
	}
	if n := len(out.Results[0].Files); n != 2 {
		t.Errorf("alpha: %d files, want 2", n)
	}
	if beta := out.Results[1]; beta.OutputMode != contentOutputMode || len(beta.Lines) != 3 {
		t.Errorf(
			"beta: mode %q with %d lines, want content with 3",
			beta.OutputMode,
			len(beta.Lines),
		)
	}
	if n := len(out.Results[2].Files); n != 1 {
		t.Errorf("gamma: %d files, want 1", n)
	}
	if out.Truncated || out.Note != "" {
		t.Errorf("truncated=%v note=%q, want neither for small results", out.Truncated, out.Note)
	}
}

func TestSearchMulti_FailingQueryStaysInItsEntry(t *testing.T) {
	fb := &multiFake{
		results: map[string][]search.Match{"fine": makeFileMatches(1, "org/repo")},
		errs:    map[string]error{"boom": errors.New("index unavailable")},
		files:   map[string]int{"retry": 3, "backoff": 2},
	}
	env := multiEnv(t, fb, t.TempDir(), "org/repo")

	out := runSearchMulti(context.Background(), env, []searchMultiQuery{
		{Query: `"`},
		{Query: "ok", OutputMode: "lines"},
		{Query: "boom"},
		{Query: "retry backoff"},
		{Query: "fine"},
	})
	if len(out.Results) != 5 {
		t.Fatalf("got %d entries, want 5", len(out.Results))
	}

	wantErr := []string{"only quote characters", "output_mode must be", "index unavailable"}
	for i, want := range wantErr {
		r := out.Results[i]
		if !strings.Contains(r.Error, want) || r.searchOutput != nil {
			t.Errorf("%q: error %q with output %v, want only an error containing %q",
				r.Query, r.Error, r.searchOutput, want)
		}
	}

	zero := out.Results[3]
	if zero.Error != "" || zero.ZeroHint == nil {
		t.Fatalf(
			"zero-hit query: error %q hint %v, want a zero_result_hint",
			zero.Error,
			zero.ZeroHint,
		)
	}
	if got := termCountsLine(zero.ZeroHint.TermCounts); got != "retry=3, backoff=2" {
		t.Errorf("zero-hit term counts = %q, want retry=3, backoff=2", got)
	}

	if fine := out.Results[4]; fine.Error != "" || len(fine.Files) != 1 {
		t.Errorf("fine: error %q with %d files, want 1 file", fine.Error, len(fine.Files))
	}
}

func TestSearchMulti_NoReposAnswersLikeSearch(t *testing.T) {
	env := multiEnv(t, &multiFake{}, t.TempDir())
	out := runSearchMulti(context.Background(), env, []searchMultiQuery{{Query: "x"}})
	hint := out.Results[0].ZeroHint
	if hint == nil || !slices.Contains(hint.Notes, noReposNote) {
		t.Errorf("hint = %v, want the no-repos note", hint)
	}
}

func TestSearchMulti_WorkersAreBounded(t *testing.T) {
	fb := &multiFake{results: map[string][]search.Match{}}
	queries := make([]searchMultiQuery, maxMultiQueries)
	for i := range queries {
		q := fmt.Sprintf("q%d", i)
		fb.results[q] = makeFileMatches(1, "org/repo")
		queries[i] = searchMultiQuery{Query: q}
	}
	env := multiEnv(t, fb, t.TempDir(), "org/repo")

	out := runSearchMulti(context.Background(), env, queries)
	if len(out.Results) != maxMultiQueries {
		t.Fatalf("got %d entries, want %d", len(out.Results), maxMultiQueries)
	}
	if fb.peak > multiSearchWorkers {
		t.Errorf("%d searches ran at once, want at most %d", fb.peak, multiSearchWorkers)
	}
}

func TestCheckMultiQueryCount(t *testing.T) {
	tests := []struct {
		n       int
		wantErr string
	}{
		{0, "queries is empty"},
		{1, ""},
		{maxMultiQueries, ""},
		{maxMultiQueries + 1, "at most 10 queries per call, got 11"},
	}
	for _, tt := range tests {
		err := checkMultiQueryCount(tt.n)
		switch {
		case tt.wantErr == "" && err != nil:
			t.Errorf("%d queries: unexpected error %v", tt.n, err)
		case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
			t.Errorf("%d queries: error %v, want one containing %q", tt.n, err, tt.wantErr)
		}
	}
}

// TestSearchMulti_QueryCapOverMCP sends one query too many through a live
// server and expects the whole call refused as a tool error.
func TestSearchMulti_QueryCapOverMCP(t *testing.T) {
	writeConfig(t, "")
	ctx, session := connect(t, New("test", Options{}))

	queries := make([]map[string]any, maxMultiQueries+1)
	for i := range queries {
		queries[i] = map[string]any{"query": fmt.Sprintf("q%d", i)}
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "csl_search_multi",
		Arguments: map[string]any{"queries": queries},
	})
	if err != nil {
		t.Fatalf("call csl_search_multi: %v", err)
	}
	if !res.IsError || !strings.Contains(textOf(t, res), "at most 10 queries per call") {
		t.Errorf("IsError=%v content=%q, want the per-call cap error", res.IsError, textOf(t, res))
	}
}

func TestSearchMulti_ReadTopHit(t *testing.T) {
	root := t.TempDir()
	var body strings.Builder
	for i := range 100 {
		fmt.Fprintf(&body, "line %d\n", i+1)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	fb := &multiFake{results: map[string][]search.Match{
		"needle": {
			{Repo: "org/repo", File: "main.go", Line: 40},
			{Repo: "org/repo", File: "other.go", Line: 1},
		},
		"plain":   makeFileMatches(1, "org/repo"),
		"missing": {{Repo: "org/repo", File: "gone.go", Line: 1}},
	}}
	env := multiEnv(t, fb, root, "org/repo")

	out := runSearchMulti(context.Background(), env, []searchMultiQuery{
		{Query: "needle", ReadTopHit: true},
		{Query: "plain"},
		{Query: "nothing", ReadTopHit: true},
		{Query: "missing", ReadTopHit: true},
	})

	top := out.Results[0].TopHit
	if top == nil {
		t.Fatalf("needle: no top_hit (error %q)", out.Results[0].TopHitError)
	}
	if top.Path != "main.go" || len(top.Lines) != topHitLines || top.Lines[0].Number != 1 {
		t.Errorf("top_hit = %s with %d lines from %d, want main.go lines 1-%d",
			top.Path, len(top.Lines), top.Lines[0].Number, topHitLines)
	}
	if top.TotalLines != 100 || !top.Truncated {
		t.Errorf(
			"top_hit total_lines=%d truncated=%v, want 100 and true",
			top.TotalLines,
			top.Truncated,
		)
	}
	if out.Results[1].TopHit != nil {
		t.Error("plain: top_hit set without read_top_hit")
	}
	if r := out.Results[2]; r.TopHit != nil || r.TopHitError != "" || r.Error != "" {
		t.Errorf(
			"zero-hit read_top_hit: top_hit %v, top_hit_error %q, error %q; want none",
			r.TopHit,
			r.TopHitError,
			r.Error,
		)
	}
	if r := out.Results[3]; r.Error != "" || !strings.Contains(r.TopHitError, "gone.go") {
		t.Errorf(
			"unreadable top hit: error %q, top_hit_error %q; want the read failure on top_hit_error only",
			r.Error,
			r.TopHitError,
		)
	}
	if out.Truncated {
		t.Error("a truncated top_hit must not mark the call truncated")
	}
}

func TestShareLines(t *testing.T) {
	tests := []struct {
		name   string
		needs  []int
		budget int
		want   []int
	}{
		{"everything fits", []int{10, 20}, 300, []int{10, 20}},
		{"even split", []int{300, 300, 300}, 300, []int{100, 100, 100}},
		{"small need keeps its own", []int{10, 500, 500}, 300, []int{10, 145, 145}},
		{"leftover goes to the earliest", []int{200, 200, 200}, 100, []int{34, 33, 33}},
		{"zero needs get nothing", []int{0, 50, 0}, 300, []int{0, 50, 0}},
		{"no budget", []int{5}, 0, []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shareLines(tt.needs, tt.budget)
			if !slices.Equal(got, tt.want) {
				t.Errorf("shareLines(%v, %d) = %v, want %v", tt.needs, tt.budget, got, tt.want)
			}
		})
	}
}

func TestFitLineBudget_CutsAndMarks(t *testing.T) {
	page := func(n int) *searchOutput {
		out := buildSearchOutput(contentOutputMode, 50, 0, makeMatches(n, "org/repo", "a.go"))
		return &out
	}
	results := []searchMultiResult{
		{Query: "small", searchOutput: page(20)},
		{Query: "big", searchOutput: page(250)},
		{Query: "capped", searchOutput: page(400)},
		{Query: "broken", Error: "boom"},
	}

	out := fitLineBudget(results, multiLineBudget)

	small, big, capped := out.Results[0], out.Results[1], out.Results[2]
	if small.Total != 20 || small.Truncated {
		t.Errorf(
			"small: total=%d truncated=%v, want 20 lines untouched",
			small.Total,
			small.Truncated,
		)
	}
	if big.Total != 140 || len(big.Lines) != 140 || !big.Truncated || big.TotalAvailable != 250 {
		t.Errorf("big: total=%d lines=%d truncated=%v total_available=%d, want 140 of 250",
			big.Total, len(big.Lines), big.Truncated, big.TotalAvailable)
	}
	if capped.Total != 140 || capped.TotalAvailable != 400 {
		t.Errorf("capped: total=%d total_available=%d, want 140 and the pre-cap 400",
			capped.Total, capped.TotalAvailable)
	}
	if !out.Truncated || !strings.Contains(out.Note, "300-line budget") {
		t.Errorf(
			"truncated=%v note=%q, want the call marked and the budget named",
			out.Truncated,
			out.Note,
		)
	}
}

func TestFitLineBudget_TopHitYieldsToContent(t *testing.T) {
	content := buildSearchOutput(contentOutputMode, 50, 0, makeMatches(10, "org/repo", "a.go"))
	hit := &readOutput{
		Repo:       "org/repo",
		Path:       "a.go",
		Lines:      make([]readLine, topHitLines),
		TotalLines: 200,
	}
	results := []searchMultiResult{{Query: "q", searchOutput: &content, TopHit: hit}}

	out := fitLineBudget(results, 25)

	r := out.Results[0]
	if len(r.Lines) != 10 || r.Truncated {
		t.Errorf("content: %d lines truncated=%v, want all 10 kept", len(r.Lines), r.Truncated)
	}
	if len(r.TopHit.Lines) != 15 || !r.TopHit.Truncated {
		t.Errorf("top_hit: %d lines truncated=%v, want the remaining 15 and truncated",
			len(r.TopHit.Lines), r.TopHit.Truncated)
	}
}

// TestSearchMultiResult_JSON pins the entry shape: a failed query carries
// only its query and error, a successful one csl_search's fields inline.
func TestSearchMultiResult_JSON(t *testing.T) {
	ok := buildSearchOutput(filesOutputMode, 50, 0, makeFileMatches(1, "org/repo"))
	tests := []struct {
		name string
		in   searchMultiResult
		want string
	}{
		{
			"failed",
			searchMultiResult{Query: "x", Error: "boom"},
			`{"query":"x","error":"boom"}`,
		},
		{
			"ok",
			searchMultiResult{Query: "y", searchOutput: &ok},
			`{"query":"y","output_mode":"files_with_matches","files":[{"repo":"org/repo","path":"pkg0/file0.go"}],"total":1,"truncated":false}`,
		},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.in)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tt.name, err)
		}
		if string(got) != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", tt.name, got, tt.want)
		}
	}
}

func TestRenderSearchMultiText(t *testing.T) {
	files := buildSearchOutput(filesOutputMode, 50, 0, makeFileMatches(1, "org/repo"))
	out := searchMultiOutput{
		Results: []searchMultiResult{
			{
				Query:        "alpha",
				searchOutput: &files,
				TopHit: &readOutput{
					Repo:       "org/repo",
					Path:       "pkg0/file0.go",
					Lines:      []readLine{{Number: 1, Text: "package pkg0"}},
					TotalLines: 1,
				},
			},
			{Query: "beta", Error: "query is empty; put the text to search for in query"},
		},
		Note: "the 300-line budget is shared",
	}

	want := strings.Join([]string{
		"=== query 1/2: alpha",
		"org/repo/pkg0/file0.go",
		"",
		"1 file",
		"",
		"top hit: org/repo/pkg0/file0.go lines 1-1 of 1",
		"1:package pkg0",
		"",
		"=== query 2/2: beta",
		"error: query is empty; put the text to search for in query",
		"",
		"note: the 300-line budget is shared",
	}, "\n")
	if got := renderSearchMultiText(out); got != want {
		t.Errorf("renderSearchMultiText:\n%s\n--- want ---\n%s", got, want)
	}
}
