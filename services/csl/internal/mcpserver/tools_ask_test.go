package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/services/csl/internal/ask"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

func TestCheckAskRequest(t *testing.T) {
	queries := func(n int) []string {
		qs := make([]string, n)
		for i := range qs {
			qs[i] = "foo"
		}
		return qs
	}
	tests := []struct {
		name    string
		req     AskRequest
		wantErr string
	}{
		{name: "ok", req: AskRequest{Question: "q", Queries: []string{"a", "b"}}},
		{name: "file list ok", req: AskRequest{Question: "q", FileList: []string{"o/r/a"}}},
		{
			name:    "empty question",
			req:     AskRequest{Question: " ", Queries: []string{"a"}},
			wantErr: "question is empty",
		},
		{name: "no queries", req: AskRequest{Question: "q"}, wantErr: "queries is empty"},
		{
			name:    "too many",
			req:     AskRequest{Question: "q", Queries: queries(11)},
			wantErr: "at most 10",
		},
		{
			name:    "malformed",
			req:     AskRequest{Question: "q", Queries: []string{"ok", `"open`}},
			wantErr: "query 2/2",
		},
		{
			name:    "both",
			req:     AskRequest{Question: "q", Queries: []string{"a"}, FileList: []string{"o/r/a"}},
			wantErr: "not both",
		},
		{
			name:    "file list with a filter",
			req:     AskRequest{Question: "q", FileList: []string{"o/r/a"}, Lang: "go"},
			wantErr: "narrow queries",
		},
		{
			name: "over a ceiling",
			req: AskRequest{
				Question: "q",
				Queries:  []string{"a"},
				Limits:   ask.Limits{MaxFiles: 101},
			},
			wantErr: "max_files",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Surface = mcpSurface
			lim, err := checkAskRequest(tc.req)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("err = %v, want nil", err)
			case tc.wantErr == "" && lim.MaxFiles != ask.DefaultMaxFiles:
				t.Errorf("limits = %+v, want the defaults", lim)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestSearchAskQueriesBuildsTheCorpusInTurns(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("line\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	match := func(file string) search.Match { return search.Match{Repo: "o/r", File: file, Line: 1} }
	fb := &multiFake{results: map[string][]search.Match{
		"first":  {match("a.go"), match("b.go")},
		"second": {match("c.go"), match("a.go")},
	}}
	env := multiEnv(t, fb, root, "o/r")

	sel, err := searchAskQueries(
		context.Background(), env,
		AskRequest{Queries: []string{"first", "second"}}, askSearchLimit,
	)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := buildAskCorpus(
		ask.Union(sel.lists),
		env.repoPaths,
		ask.Limits{MaxFiles: 2, MaxLines: 100},
	)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, f := range corpus.Files {
		kept = append(kept, f.Path)
	}
	if want := []string{"a.go", "c.go"}; !slices.Equal(kept, want) {
		t.Errorf("corpus = %v, want each query's top file first: %v", kept, want)
	}
	if corpus.Matched != 3 || len(corpus.Skipped) != 0 || !corpus.Truncated {
		t.Errorf(
			"matched=%d skipped=%+v truncated=%v, want 3 matched, b.go never considered",
			corpus.Matched, corpus.Skipped, corpus.Truncated,
		)
	}
}

func TestSearchAskQueriesBackfillsFromPastTheFileCap(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"bin.dat": "\x00", "a.go": "a\n", "b.go": "b\n"}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	match := func(file string) search.Match { return search.Match{Repo: "o/r", File: file, Line: 1} }
	fb := &multiFake{results: map[string][]search.Match{
		"q": {match("bin.dat"), match("gone.go"), match("a.go"), match("b.go")},
	}}
	env := multiEnv(t, fb, root, "o/r")

	sel, err := searchAskQueries(
		context.Background(), env, AskRequest{Queries: []string{"q"}}, askSearchLimit,
	)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := buildAskCorpus(
		ask.Union(sel.lists), env.repoPaths, ask.Limits{MaxFiles: 2, MaxLines: 100},
	)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, f := range corpus.Files {
		kept = append(kept, f.Path)
	}
	if want := []string{"a.go", "b.go"}; !slices.Equal(kept, want) {
		t.Errorf("corpus = %v, want the two skipped files replaced by %v", kept, want)
	}
	if corpus.Matched != 4 || len(corpus.Skipped) != 2 || corpus.Truncated {
		t.Errorf(
			"matched=%d skipped=%d truncated=%v, want 4, 2, false",
			corpus.Matched, len(corpus.Skipped), corpus.Truncated,
		)
	}
}

func TestSearchAskQueriesMarksASearchCutAtItsLimit(t *testing.T) {
	fb := &multiFake{results: map[string][]search.Match{"q": makeFileMatches(3, "o/r")}}
	env := multiEnv(t, fb, t.TempDir(), "o/r")
	sel, err := searchAskQueries(
		context.Background(), env, AskRequest{Queries: []string{"q"}}, 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !sel.truncated || len(sel.lists[0]) != 2 {
		t.Errorf(
			"truncated=%v files=%d, want the search cut at 2 and marked",
			sel.truncated, len(sel.lists[0]),
		)
	}
}

func TestSearchAskQueriesNamesARelaxedQuery(t *testing.T) {
	fb := &multiFake{
		results: map[string][]search.Match{"retry": {{Repo: "o/r", File: "a.go", Line: 1}}},
		files:   map[string]int{"retry": 1, "jitter": 0},
	}
	env := multiEnv(t, fb, t.TempDir(), "o/r")
	sel, err := searchAskQueries(
		context.Background(), env,
		AskRequest{Queries: []string{"retry jitter"}}, askSearchLimit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.lists[0]) != 1 {
		t.Errorf("files = %v, want the relaxed rerun's file kept", sel.lists[0])
	}
	want := []ask.Relaxed{
		{Query: "retry jitter", RelaxedQuery: "retry", DroppedTerms: []string{"jitter"}},
	}
	if len(sel.relaxed) != 1 || sel.relaxed[0].Query != want[0].Query ||
		sel.relaxed[0].RelaxedQuery != want[0].RelaxedQuery ||
		!slices.Equal(sel.relaxed[0].DroppedTerms, want[0].DroppedTerms) {
		t.Errorf("relaxed = %+v, want %+v", sel.relaxed, want)
	}
}

func TestSearchAskQueriesZeroFilesCarriesEachHint(t *testing.T) {
	fb := &multiFake{files: map[string]int{}}
	env := multiEnv(t, fb, t.TempDir(), "o/r")
	_, err := searchAskQueries(
		context.Background(), env,
		AskRequest{Queries: []string{"nothing", "neither"}}, askSearchLimit,
	)
	if err == nil {
		t.Fatal("zero files searched fine, want an error carrying the hints")
	}
	for _, want := range []string{
		"nothing was sent to the worker",
		"query 1/2: nothing",
		"query 2/2: neither",
		"repos searched",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%s", want, err)
		}
	}
}

func TestSearchAskQueriesFailedQueryFailsTheQuestion(t *testing.T) {
	fb := &multiFake{
		results: map[string][]search.Match{"good": {{Repo: "o/r", File: "a.go"}}},
		errs:    map[string]error{"bad": errors.New("zoekt: broken regexp")},
	}
	env := multiEnv(t, fb, t.TempDir(), "o/r")
	_, err := searchAskQueries(
		context.Background(), env,
		AskRequest{Queries: []string{"good", "bad"}}, askSearchLimit,
	)
	if err == nil || !strings.Contains(err.Error(), `query 2/2 "bad"`) {
		t.Errorf("err = %v, want the failed query named", err)
	}
}

func TestBuildAskCorpusRefusesAllSkipped(t *testing.T) {
	_, err := buildAskCorpus(
		[]ask.Hit{{Repo: "o/r", Path: "gone.go"}},
		map[string]string{"o/r": t.TempDir()},
		ask.Limits{MaxFiles: 1, MaxLines: 10},
	)
	if err == nil || !strings.Contains(err.Error(), "o/r/gone.go: not in the checkout") {
		t.Errorf("err = %v, want the skipped file and its reason", err)
	}
}

// workerCounts is what the fake worker saw: backends built and the calls
// that would cost money.
type workerCounts struct {
	built atomic.Int32
	calls atomic.Int32
}

// countingWorker swaps newAskWorker for a fake that counts the backends it
// builds and the calls that would cost money, failing resolution with
// resolveErr when it is set.
func countingWorker(t *testing.T, resolveErr error) *workerCounts {
	t.Helper()
	var n workerCounts
	orig := newAskWorker
	newAskWorker = func(s ask.Settings, observe func(llm.CallInfo)) (llm.Backend, error) {
		n.built.Add(1)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return countingBackend{calls: &n.calls}, nil
	}
	t.Cleanup(func() { newAskWorker = orig })
	return &n
}

type countingBackend struct{ calls *atomic.Int32 }

func (b countingBackend) Name() string  { return "fake" }
func (b countingBackend) Model() string { return "claude-haiku-5-5" }
func (b countingBackend) Complete(context.Context, string, string) (string, error) {
	b.calls.Add(1)
	return "- answer", nil
}

// noConfigHome points every csl path at an empty temp home, so Ask runs on
// defaults with no repos configured.
func noConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CSL_CONFIG", "")
}

func TestAskWithoutBackendFailsBeforeSearching(t *testing.T) {
	noConfigHome(t)
	countingWorker(t, llm.ErrNoBackend)
	_, err := Ask(context.Background(), AskRequest{Question: "q", Queries: []string{"x"}})
	if !errors.Is(err, llm.ErrNoBackend) {
		t.Errorf("err = %v, want ErrNoBackend", err)
	}
}

func TestAskWithNoFilesNeverCallsTheWorker(t *testing.T) {
	noConfigHome(t)
	counts := countingWorker(t, nil)
	_, err := Ask(context.Background(), AskRequest{Question: "q", Queries: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), noReposNote) {
		t.Errorf("err = %v, want the zero-file error carrying %q", err, noReposNote)
	}
	if n := counts.calls.Load(); n != 0 {
		t.Errorf("%d worker calls, want none", n)
	}
	if n := counts.built.Load(); n != 1 {
		t.Errorf("worker resolved %d times before the search, want once", n)
	}
}
