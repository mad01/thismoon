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

	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// multiFake is a lexical.Backend the concurrent workers can share: results
// and errors keyed by pattern, patterns that panic, per-term file counts for
// the zero-result diagnosis, the searches it ran, and the most it saw in
// flight at once.
type multiFake struct {
	mu       sync.Mutex
	results  map[string][]search.Match
	errs     map[string]error
	panics   map[string]bool
	files    map[string]int
	searches int
	inFlight int
	peak     int
}

func (f *multiFake) Search(_ context.Context, opts search.SearchOptions) ([]search.Match, error) {
	if f.panics[opts.Pattern] {
		panic("fake backend: " + opts.Pattern)
	}
	f.mu.Lock()
	f.searches++
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

func (f *multiFake) Count(
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
func multiEnv(t *testing.T, b lexical.Backend, root string, names ...string) searchEnv {
	t.Helper()
	env := searchEnv{backend: b, repoPaths: map[string]string{}, indexDir: t.TempDir()}
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
	wantCounts := []lexical.TermCount{{Term: "retry", Files: 3}, {Term: "backoff", Files: 2}}
	if got := zero.ZeroHint.TermCounts; !slices.Equal(got, wantCounts) {
		t.Errorf("zero-hit term counts = %v, want %v", got, wantCounts)
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
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
		"escape":  {{Repo: "org/repo", File: "../secret.txt", Line: 1}},
	}}
	env := multiEnv(t, fb, root, "org/repo")

	out := runSearchMulti(context.Background(), env, []searchMultiQuery{
		{Query: "needle", ReadTopHit: true},
		{Query: "plain"},
		{Query: "nothing", ReadTopHit: true},
		{Query: "missing", ReadTopHit: true},
		{Query: "escape", ReadTopHit: true},
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
	if r := out.Results[4]; r.TopHit != nil ||
		!strings.Contains(r.TopHitError, "file must stay inside the repo") {
		t.Errorf(
			"top hit outside the repo: top_hit %v, top_hit_error %q; want it refused like csl_read",
			r.TopHit,
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

func TestSearchMulti_PanicStaysInItsEntry(t *testing.T) {
	fb := &multiFake{
		results: map[string][]search.Match{"fine": makeFileMatches(1, "org/repo")},
		panics:  map[string]bool{"kaboom": true},
	}
	env := multiEnv(t, fb, t.TempDir(), "org/repo")

	out := runSearchMulti(context.Background(), env, []searchMultiQuery{
		{Query: "kaboom"},
		{Query: "fine"},
	})

	if r := out.Results[0]; r.Query != "kaboom" || !strings.Contains(r.Error, "internal error") {
		t.Errorf(
			"panicking query: query %q error %q, want an internal error entry",
			r.Query,
			r.Error,
		)
	}
	if r := out.Results[1]; r.Error != "" || len(r.Files) != 1 {
		t.Errorf("fine: error %q with %d files, want 1 file", r.Error, len(r.Files))
	}
}

func TestSearchMulti_CancelledCallStartsNoQuery(t *testing.T) {
	fb := &multiFake{results: map[string][]search.Match{"a": makeFileMatches(1, "org/repo")}}
	env := multiEnv(t, fb, t.TempDir(), "org/repo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := runSearchMulti(ctx, env, []searchMultiQuery{{Query: "a"}, {Query: "b"}})

	for _, r := range out.Results {
		if !strings.Contains(r.Error, context.Canceled.Error()) {
			t.Errorf("%s: error %q, want the cancellation", r.Query, r.Error)
		}
	}
	if fb.searches != 0 {
		t.Errorf("%d searches ran after the call was cancelled, want 0", fb.searches)
	}
}

func TestFitLineBudget_TopHitWithNoRoomIsDropped(t *testing.T) {
	content := buildSearchOutput(contentOutputMode, 50, 0, makeMatches(10, "org/repo", "a.go"))
	hit := &readOutput{Repo: "org/repo", Path: "a.go", Lines: make([]readLine, topHitLines)}
	results := []searchMultiResult{{Query: "q", searchOutput: &content, TopHit: hit}}

	out := fitLineBudget(results, 10)

	r := out.Results[0]
	if len(r.Lines) != 10 || r.TopHit != nil || r.TopHitError != topHitCutNote {
		t.Errorf("lines=%d top_hit=%v top_hit_error=%q, want 10 lines and the cut note",
			len(r.Lines), r.TopHit, r.TopHitError)
	}
	if text := renderSearchMultiText(out); !strings.Contains(text, "top hit: "+topHitCutNote) {
		t.Errorf("rendered text lacks the cut note:\n%s", text)
	}
}

// stubDaemonStart replaces the up-front daemon start for one test with one
// that returns err, and counts the calls.
func stubDaemonStart(t *testing.T, err error) *int {
	t.Helper()
	calls := 0
	prev := ensureDaemon
	ensureDaemon = func(string, string) error {
		calls++
		return err
	}
	t.Cleanup(func() { ensureDaemon = prev })
	return &calls
}

// setupMultiRepo writes a discoverable repo org/multirepo holding a.go,
// points csl's config at it under an isolated home, and indexes it into the
// index dir csl resolves there.
func setupMultiRepo(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	reposRoot := filepath.Join(home, "workspace")
	repoDir := filepath.Join(reposRoot, "org", "multirepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(repoDir, ".git", "config"):             "[remote \"origin\"]\n\turl = git@github.com:org/multirepo.git\n",
		filepath.Join(repoDir, "a.go"):                       "package a\n\nfunc Alpha() {}\n\nfunc Beta() {}\n",
		filepath.Join(home, ".config", "csl", "config.yaml"): "dirs:\n  - " + reposRoot + "\n",
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolateConfigEnv(t, home)

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		t.Fatal(err)
	}
	repo := finder.Repo{Name: "org/multirepo", Path: repoDir}
	if err := search.IndexRepo(indexDir, repo, nil); err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}
}

func TestNewMultiSearchEnv_FailedDaemonStartSearchesInProcess(t *testing.T) {
	setupMultiRepo(t)
	tests := []struct {
		name     string
		startErr error
		wantType string
	}{
		{"daemon started", nil, "lexical.Daemon"},
		{"start failed", errors.New("no daemon"), "lexical.Local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := stubDaemonStart(t, tt.startErr)
			env, err := newMultiSearchEnv()
			if err != nil {
				t.Fatalf("newMultiSearchEnv: %v", err)
			}
			if got := fmt.Sprintf("%T", env.backend); got != tt.wantType || *calls != 1 {
				t.Errorf("backend %s after %d starts, want %s after 1", got, *calls, tt.wantType)
			}
		})
	}
}

// TestSearchMulti_MixedBatchOverMCP sends one call through a live server
// against a real index: a plain query, a malformed one, one relaxed by a
// dead term, and one with read_top_hit. The daemon start fails, so the call
// searches in-process.
func TestSearchMulti_MixedBatchOverMCP(t *testing.T) {
	setupMultiRepo(t)
	stubDaemonStart(t, errors.New("no daemon in tests"))
	ctx, session := connect(t, New("test", Options{}))

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "csl_search_multi",
		Arguments: map[string]any{
			"response_format": "json",
			"queries": []map[string]any{
				{"query": "Alpha"},
				{"query": `"`},
				{"query": "Alpha zzznotaterm"},
				{"query": "Beta", "read_top_hit": true},
			},
		},
	})
	if err != nil {
		t.Fatalf("call csl_search_multi: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", textOf(t, res))
	}
	var got struct {
		Results []struct {
			Query        string            `json:"query"`
			Error        string            `json:"error"`
			Files        []searchMatchFile `json:"files"`
			RelaxedQuery string            `json:"relaxed_query"`
			DroppedTerms []string          `json:"dropped_terms"`
			TopHit       *readOutput       `json:"top_hit"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 4 {
		t.Fatalf("got %d entries, want 4", len(got.Results))
	}
	wantFile := []searchMatchFile{{Repo: "org/multirepo", Path: "a.go"}}

	if r := got.Results[0]; r.Error != "" || !slices.Equal(r.Files, wantFile) {
		t.Errorf("plain: error %q files %v, want %v", r.Error, r.Files, wantFile)
	}
	if r := got.Results[1]; !strings.Contains(r.Error, "only quote characters") || r.Files != nil {
		t.Errorf("malformed: error %q files %v, want only the shape refusal", r.Error, r.Files)
	}
	if r := got.Results[2]; r.RelaxedQuery != "Alpha" ||
		!slices.Equal(r.DroppedTerms, []string{"zzznotaterm"}) || !slices.Equal(r.Files, wantFile) {
		t.Errorf("relaxed: relaxed_query %q dropped %v files %v, want Alpha without zzznotaterm",
			r.RelaxedQuery, r.DroppedTerms, r.Files)
	}
	r := got.Results[3]
	if r.TopHit == nil || r.TopHit.Path != "a.go" || len(r.TopHit.Lines) != 5 ||
		r.TopHit.Lines[4].Text != "func Beta() {}" {
		t.Errorf("read_top_hit: top_hit %+v, want all 5 lines of a.go", r.TopHit)
	}
}
