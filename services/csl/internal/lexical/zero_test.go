package lexical

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/lexical/lexicaltest"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// fakeBackend answers per-term counts from a table keyed by the term and
// searches from a table keyed by the exact pattern, recording every request
// so tests can assert what the diagnosis asked for.
type fakeBackend struct {
	files    map[string]int
	results  map[string][]search.Match
	countErr error
	searches []search.SearchOptions
	counts   []string
}

func (f *fakeBackend) Search(_ context.Context, opts search.SearchOptions) ([]search.Match, error) {
	f.searches = append(f.searches, opts)
	return f.results[opts.Pattern], nil
}

func (f *fakeBackend) Count(
	_ context.Context,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	f.counts = append(f.counts, opts.Pattern)
	if f.countErr != nil {
		return nil, 0, f.countErr
	}
	term := strings.Fields(opts.Pattern)[1] // after the type:filename atom
	return nil, f.files[term], nil
}

var oneRepo = []finder.Repo{{Name: "org/repo", Path: "/tmp/repo"}}

// testSurface names commands no front end uses, so a test can see which
// note picked them up.
var testSurface = Surface{RepoLookup: "lookup-cmd", Reindex: "reindex-cmd"}

// explain runs ExplainZero for a files-mode query on the first page.
func explain(t *testing.T, b Backend, query, file string) Result {
	t.Helper()
	req := Request{
		Options: search.SearchOptions{
			Pattern: query, FileFilter: file, Limit: 50, OutputMode: "files_with_matches",
		},
		Repos:    oneRepo,
		IndexDir: t.TempDir(),
	}
	return ExplainZero(context.Background(), b, testSurface, req)
}

func TestExplainZero_DropsAbsentTermAndReruns(t *testing.T) {
	fb := &fakeBackend{
		files:   map[string]int{"retry": 37, "backoff": 12, "jitter": 0},
		results: map[string][]search.Match{"retry backoff": {{Repo: "org/repo", File: "a.go"}}},
	}

	res := explain(t, fb, "retry jitter backoff", `\.go$`)

	if res.RelaxedQuery != "retry backoff" {
		t.Errorf("relaxed_query = %q, want %q", res.RelaxedQuery, "retry backoff")
	}
	if !reflect.DeepEqual(res.DroppedTerms, []string{"jitter"}) {
		t.Errorf("dropped_terms = %q, want [jitter]", res.DroppedTerms)
	}
	if len(res.Matches) != 1 || res.Hint != nil {
		t.Errorf("relaxed result matches=%d hint=%v, want 1 match and no hint",
			len(res.Matches), res.Hint)
	}
	if len(fb.searches) != 1 || fb.searches[0].Offset != 0 || fb.searches[0].FileFilter != `\.go$` {
		t.Errorf("rerun requests = %+v, want one at offset 0 with the file filter kept",
			fb.searches)
	}
	want := `type:filename retry file:\.go$`
	if fb.counts[0] != want {
		t.Errorf("first count pattern = %q, want %q (filters folded, file-count wrapper)",
			fb.counts[0], want)
	}
}

func TestExplainZero_AllTermsPresentNeverTogether(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"IndexRepo": 4, "Blocks": 2}}

	res := explain(t, fb, "IndexRepo Blocks", "")

	if len(res.Matches) != 0 || res.RelaxedQuery != "" || res.Hint == nil {
		t.Fatalf("want a hint and no relaxation, got %+v", res)
	}
	if len(fb.searches) != 0 {
		t.Errorf("reran %d searches, want none when every term exists", len(fb.searches))
	}
	wantCounts := []TermCount{{Term: "IndexRepo", Files: 4}, {Term: "Blocks", Files: 2}}
	if !reflect.DeepEqual(res.Hint.TermCounts, wantCounts) {
		t.Errorf("term_counts = %+v, want %+v", res.Hint.TermCounts, wantCounts)
	}
	if !hasNote(res.Hint.Notes, "IndexRepo|Blocks") {
		t.Errorf("notes = %q, want the a|b suggestion", res.Hint.Notes)
	}
}

func TestExplainZero_NoRelaxationPastFirstPage(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"retry": 37, "jitter": 0}}
	req := Request{
		Options: search.SearchOptions{
			Pattern: "retry jitter", Offset: 50, Limit: 50, OutputMode: "files_with_matches",
		},
		Repos:    oneRepo,
		IndexDir: t.TempDir(),
	}

	res := ExplainZero(context.Background(), fb, testSurface, req)

	if res.RelaxedQuery != "" || len(fb.searches) != 0 {
		t.Errorf("relaxed at offset 50: %+v, searches=%d", res, len(fb.searches))
	}
	if !hasNote(res.Hint.Notes, "offset > 0") {
		t.Errorf("notes = %q, want the offset note", res.Hint.Notes)
	}
}

func TestExplainZero_SingleTermSkipsCounts(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"retry": 0}}

	res := explain(t, fb, "retry", "")

	if len(fb.counts) != 0 || res.Hint.TermCounts != nil {
		t.Errorf("one-term query was counted: counts=%q hint=%+v", fb.counts, res.Hint)
	}
}

func TestExplainZero_RelaxedRerunStillEmptyCarriesNote(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"a": 1, "b": 1, "zzz": 0}}

	res := explain(t, fb, "a zzz b", "")

	if res.RelaxedQuery != "" || len(res.Matches) != 0 {
		t.Fatalf("want a hint without relaxed_query, got %+v", res)
	}
	if !hasNote(res.Hint.Notes, "reran as `a b`") || !hasNote(res.Hint.Notes, "a|b") {
		t.Errorf("notes = %q, want the failed-rerun note with the a|b suggestion",
			res.Hint.Notes)
	}
}

func TestExplainZero_CountErrorSurfacesAsNote(t *testing.T) {
	fb := &fakeBackend{countErr: errors.New("daemon gone")}

	res := explain(t, fb, "retry jitter", "")

	if res.Hint.TermCounts != nil || res.RelaxedQuery != "" {
		t.Errorf("want no counts and no relaxation on a count error, got %+v", res)
	}
	if !hasNote(res.Hint.Notes, "daemon gone") {
		t.Errorf("notes = %q, want the count error surfaced", res.Hint.Notes)
	}
}

func TestExplainZero_TooManyTermsCountsFirstSixOnly(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{
		"a": 1, "b": 1, "c": 1, "d": 1, "e": 1, "f": 1, "g": 0,
	}}

	res := explain(t, fb, "a b c d e f g", "")

	if len(res.Hint.TermCounts) != maxDiagnosedTerms {
		t.Errorf("counted %d terms, want %d", len(res.Hint.TermCounts), maxDiagnosedTerms)
	}
	if res.RelaxedQuery != "" || len(fb.searches) != 0 {
		t.Errorf("relaxed a query with uncounted terms: %+v", res)
	}
	if !hasNote(res.Hint.Notes, "only the first 6 were counted") {
		t.Errorf("notes = %q, want the cap note", res.Hint.Notes)
	}
}

// TestExplainZero_ParamNoteFromSurface: the parameter-name note comes from
// the front end, sits after the syntax traps and before the count notes, and
// is skipped when the front end has none.
func TestExplainZero_ParamNoteFromSurface(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"output_mode": 0, "context_lines": 0}}
	s := testSurface
	s.ParamNote = func(q string) string { return "param note for " + q }
	req := Request{
		Options: search.SearchOptions{
			Pattern: "output_mode | context_lines", Limit: 50, OutputMode: "files_with_matches",
		},
		Repos:    oneRepo,
		IndexDir: t.TempDir(),
	}

	notes := ExplainZero(context.Background(), fb, s, req).Hint.Notes
	if len(notes) < 2 || !strings.Contains(notes[0], "'a | b'") ||
		notes[1] != "param note for output_mode | context_lines" {
		t.Errorf("notes = %q, want the trap note, then the surface's param note", notes)
	}

	notes = ExplainZero(context.Background(), fb, testSurface, req).Hint.Notes
	if hasNote(notes, "param note") {
		t.Errorf("notes = %q, want no param note without a ParamNote hook", notes)
	}
}

func TestZeroNotes_AndTermsNoteYieldsToCounts(t *testing.T) {
	opts := search.SearchOptions{Pattern: "a b c"}
	if notes := zeroNotes(testSurface, opts, termDiagnosis{}); !hasNote(notes, "3 AND terms") {
		t.Errorf("without counts, notes = %q, want the AND-terms reminder", notes)
	}
	diag := termDiagnosis{counts: []TermCount{{Term: "a", Files: 1}}}
	if notes := zeroNotes(testSurface, opts, diag); hasNote(notes, "3 AND terms") {
		t.Errorf("with counts, notes = %q, want the reminder suppressed", notes)
	}
}

// fixtureIndex indexes the shared two-file test/repo (lexicaltest.Index).
func fixtureIndex(t *testing.T) (Local, []finder.Repo) {
	t.Helper()
	indexDir, repo := lexicaltest.Index(t)
	return Local{IndexDir: indexDir, RepoNames: map[string]string{repo.Name: repo.Path}},
		[]finder.Repo{repo}
}

// fixtureExplain runs ExplainZero against the fixture index.
func fixtureExplain(t *testing.T, query, file string) Result {
	t.Helper()
	b, repos := fixtureIndex(t)
	req := Request{
		Options: search.SearchOptions{
			Pattern: query, FileFilter: file, Limit: 50, OutputMode: "files_with_matches",
		},
		Repos:    repos,
		IndexDir: b.IndexDir,
	}
	return ExplainZero(context.Background(), b, testSurface, req)
}

func TestExplainZero_FixtureRelaxesAbsentTerm(t *testing.T) {
	res := fixtureExplain(t, "IndexRepo symbolSections zzznotaterm", "")

	if res.RelaxedQuery != "IndexRepo symbolSections" {
		t.Errorf("relaxed_query = %q, want %q", res.RelaxedQuery, "IndexRepo symbolSections")
	}
	if !reflect.DeepEqual(res.DroppedTerms, []string{"zzznotaterm"}) {
		t.Errorf("dropped_terms = %q", res.DroppedTerms)
	}
	if len(res.Matches) == 0 || res.Matches[0].File != "a.go" {
		t.Errorf("matches = %+v, want a.go", res.Matches)
	}
}

func TestExplainZero_FixtureCountsFilesNotMatches(t *testing.T) {
	res := fixtureExplain(t, "IndexRepo Blocks", "")

	if res.RelaxedQuery != "" || res.Hint == nil {
		t.Fatalf("want a hint, got %+v", res)
	}
	// IndexRepo appears twice in a.go: a file count says 1, a match count 2.
	want := []TermCount{{Term: "IndexRepo", Files: 1}, {Term: "Blocks", Files: 1}}
	if !reflect.DeepEqual(res.Hint.TermCounts, want) {
		t.Errorf("term_counts = %+v, want %+v", res.Hint.TermCounts, want)
	}
}

func TestExplainZero_FixtureFileFilterFoldsIntoCounts(t *testing.T) {
	res := fixtureExplain(t, "IndexRepo Blocks", `b\.go$`)

	// Under the b.go filter IndexRepo matches nothing, so it is the dropped term.
	if res.RelaxedQuery != "Blocks" || !reflect.DeepEqual(res.DroppedTerms, []string{"IndexRepo"}) {
		t.Errorf("relaxed=%q dropped=%q, want Blocks / [IndexRepo]",
			res.RelaxedQuery, res.DroppedTerms)
	}
	if len(res.Matches) == 0 || res.Matches[0].File != "b.go" {
		t.Errorf("matches = %+v, want b.go", res.Matches)
	}
}

func TestQueryTrapNotes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "clean query", query: "foo|bar", want: 0},
		{name: "spaced pipe", query: "foo | bar", want: 1},
		{name: "uppercase OR", query: "foo OR bar", want: 1},
		{name: "both traps", query: "foo | bar OR baz", want: 2},
		{name: "quoted OR", query: `"dead OR alive"`, want: 0},
		{name: "quoted pipe", query: `"a | b"`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryTrapNotes(tt.query); len(got) != tt.want {
				t.Errorf("queryTrapNotes(%q) = %d notes %v, want %d",
					tt.query, len(got), got, tt.want)
			}
		})
	}
}

func TestOverConstraintNotes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		file  string
		want  int
	}{
		{name: "one term, no filters", query: "cluster_name", want: 0},
		{name: "three AND terms alone are andTermsNote's job", query: "a b c", want: 0},
		{name: "file filter set", query: "x", file: `.*\.tf$`, want: 1},
		{name: "multi-word quoted phrase", query: `"backend service"`, want: 1},
		{name: "single-word quote is silent", query: `"backend"`, want: 0},
		{
			name:  "phrase and file filter stack",
			query: `"backend service" health`,
			file:  "x",
			want:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overConstraintNotes(tt.query, tt.file); len(got) != tt.want {
				t.Errorf("overConstraintNotes(%q, %q) = %d notes %v, want %d",
					tt.query, tt.file, len(got), got, tt.want)
			}
		})
	}
}

func TestAndTermsNote(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{query: "cluster name", want: 0},
		{query: "backend service health", want: 1},
		{query: "backend service health check endpoint", want: 1},
		{query: `"foo bar" baz -test lang:go f:x repo:y`, want: 0},
		{query: "foo|bar baz qux", want: 1},
		{query: "foo or bar baz", want: 0}, // top-level or is not an AND chain
	}
	for _, tt := range tests {
		if got := andTermsNote(tt.query); len(got) != tt.want {
			t.Errorf("andTermsNote(%q) = %v, want %d notes", tt.query, got, tt.want)
		}
	}
}

// zeroHint runs buildZeroHint for query and repo filter with no diagnosis.
func zeroHint(t *testing.T, query, repo string, repos []finder.Repo, indexDir string) *ZeroHint {
	t.Helper()
	req := Request{
		Options:  search.SearchOptions{Pattern: query, RepoFilter: RepoFilter(repo)},
		Repo:     repo,
		Repos:    repos,
		IndexDir: indexDir,
	}
	return buildZeroHint(testSurface, req, termDiagnosis{})
}

// saveState writes a state.json holding the given repo paths and index times.
func saveState(t *testing.T, repos map[string]time.Time) string {
	t.Helper()
	indexDir := t.TempDir()
	state := search.EmptyState()
	for path, at := range repos {
		state.Repos[path] = search.RepoState{IndexedAt: at}
	}
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("save state: %v", err)
	}
	return indexDir
}

var twoRepos = []finder.Repo{
	{Name: "org/alpha", Path: "/tmp/alpha"},
	{Name: "org/beta", Path: "/tmp/beta"},
}

func TestBuildZeroHint_RepoFilterMatchesNone(t *testing.T) {
	hint := zeroHint(t, "foo", "nosuchrepo", twoRepos, t.TempDir())

	if hint.ReposDiscovered != 2 || hint.ReposSearched != 0 {
		t.Errorf("discovered=%d searched=%d, want 2 and 0",
			hint.ReposDiscovered, hint.ReposSearched)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "check the name with lookup-cmd") {
		t.Fatalf("notes = %v, want one repo-filter note naming the surface's lookup", hint.Notes)
	}
}

func TestBuildZeroHint_RepoFilterCaseInsensitive(t *testing.T) {
	indexDir := saveState(t, map[string]time.Time{
		"/tmp/alpha": time.Now(), "/tmp/beta": time.Now(),
	})
	repos := []finder.Repo{
		{Name: "org/Alpha", Path: "/tmp/alpha"},
		{Name: "org/beta", Path: "/tmp/beta"},
	}

	hint := zeroHint(t, "foo", "alpha", repos, indexDir)

	if hint.ReposSearched != 1 {
		t.Errorf("expected repos_searched=1 (case-insensitive match), got %d", hint.ReposSearched)
	}
	if len(hint.Notes) != 0 {
		t.Errorf("expected no notes when the filter matches, got %v", hint.Notes)
	}
}

func TestBuildZeroHint_MatchedButUnindexedRepoCarriesNote(t *testing.T) {
	indexDir := saveState(t, map[string]time.Time{"/tmp/beta": time.Now()})

	hint := zeroHint(t, "foo", "alpha", twoRepos, indexDir)

	if hint.ReposSearched != 0 {
		t.Errorf("expected repos_searched=0 for a matched-but-unindexed repo, got %d",
			hint.ReposSearched)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "run reindex-cmd on them") {
		t.Errorf(
			"notes = %v, want the unindexed-repo note naming the surface's reindex",
			hint.Notes,
		)
	}
}

func TestBuildZeroHint_WhitespaceRepoFilterCarriesNote(t *testing.T) {
	hint := zeroHint(t, "foo", "my repo", twoRepos[:1], t.TempDir())

	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "whitespace") {
		t.Errorf("notes = %v, want the whitespace-filter note", hint.Notes)
	}
}

func TestBuildZeroHint_ZeroIndexedAtEntriesSkipped(t *testing.T) {
	stamp := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	// A zero IndexedAt, as in legacy state, is skipped.
	indexDir := saveState(t, map[string]time.Time{"/tmp/alpha": stamp, "/tmp/beta": {}})

	hint := zeroHint(t, "foo", "", twoRepos[:1], indexDir)

	want := stamp.Format(time.RFC3339)
	if hint.NewestIndexedAt != want || hint.OldestIndexedAt != want {
		t.Errorf("index age = %q / %q, want both %q (zero entries skipped)",
			hint.NewestIndexedAt, hint.OldestIndexedAt, want)
	}
}

func TestBuildZeroHint_ParsedQueryAndIndexAge(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	indexDir := saveState(t, map[string]time.Time{"/tmp/alpha": older, "/tmp/beta": newer})

	hint := zeroHint(t, "foo bar", "", twoRepos[:1], indexDir)

	if hint.ParsedQuery == "" {
		t.Error("expected parsed_query to be set for a valid query")
	}
	if hint.ReposIndexed != 2 || hint.ReposSearched != 1 {
		t.Errorf("indexed=%d searched=%d, want 2 and 1", hint.ReposIndexed, hint.ReposSearched)
	}
	if hint.NewestIndexedAt != newer.Format(time.RFC3339) ||
		hint.OldestIndexedAt != older.Format(time.RFC3339) {
		t.Errorf("index age = %q / %q, want %s / %s", hint.NewestIndexedAt,
			hint.OldestIndexedAt, newer.Format(time.RFC3339), older.Format(time.RFC3339))
	}
}

func TestBuildZeroHint_MissingStateOmitsIndexFields(t *testing.T) {
	hint := zeroHint(t, "foo", "", twoRepos[:1], t.TempDir())

	if hint.ReposIndexed != 0 || hint.NewestIndexedAt != "" || hint.OldestIndexedAt != "" {
		t.Errorf("indexed=%d age=%q/%q, want all empty with no state file",
			hint.ReposIndexed, hint.NewestIndexedAt, hint.OldestIndexedAt)
	}
}

func hasNote(notes []string, substr string) bool {
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}
