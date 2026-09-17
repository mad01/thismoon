package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func (f *fakeBackend) search(_ context.Context, opts search.SearchOptions) ([]search.Match, error) {
	f.searches = append(f.searches, opts)
	return f.results[opts.Pattern], nil
}

func (f *fakeBackend) count(
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

func TestExplainZero_DropsAbsentTermAndReruns(t *testing.T) {
	fb := &fakeBackend{
		files:   map[string]int{"retry": 37, "backoff": 12, "jitter": 0},
		results: map[string][]search.Match{"retry backoff": makeFileMatches(2, "org/repo")},
	}
	in := searchInput{Query: "retry jitter backoff", File: `\.go$`}
	opts := search.SearchOptions{
		Pattern: in.Query, FileFilter: in.File, Limit: 50, OutputMode: filesOutputMode,
	}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if out.RelaxedQuery != "retry backoff" {
		t.Errorf("relaxed_query = %q, want %q", out.RelaxedQuery, "retry backoff")
	}
	if !reflect.DeepEqual(out.DroppedTerms, []string{"jitter"}) {
		t.Errorf("dropped_terms = %q, want [jitter]", out.DroppedTerms)
	}
	if out.Total != 2 || out.ZeroHint != nil {
		t.Errorf(
			"relaxed result total=%d hint=%v, want 2 files and no hint",
			out.Total,
			out.ZeroHint,
		)
	}
	if len(fb.searches) != 1 || fb.searches[0].Offset != 0 || fb.searches[0].FileFilter != in.File {
		t.Errorf(
			"rerun requests = %+v, want one at offset 0 with the file filter kept",
			fb.searches,
		)
	}
	want := `type:filename retry file:\.go$`
	if fb.counts[0] != want {
		t.Errorf(
			"first count pattern = %q, want %q (filters folded, file-count wrapper)",
			fb.counts[0],
			want,
		)
	}
}

func TestExplainZero_AllTermsPresentNeverTogether(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"IndexRepo": 4, "Blocks": 2}}
	in := searchInput{Query: "IndexRepo Blocks"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if out.Total != 0 || out.RelaxedQuery != "" || out.ZeroHint == nil {
		t.Fatalf("want a zero result with a hint and no relaxation, got %+v", out)
	}
	if len(fb.searches) != 0 {
		t.Errorf("reran %d searches, want none when every term exists", len(fb.searches))
	}
	wantCounts := []termCount{{Term: "IndexRepo", Files: 4}, {Term: "Blocks", Files: 2}}
	if !reflect.DeepEqual(out.ZeroHint.TermCounts, wantCounts) {
		t.Errorf("term_counts = %+v, want %+v", out.ZeroHint.TermCounts, wantCounts)
	}
	if !hasNote(out.ZeroHint.Notes, "IndexRepo|Blocks") {
		t.Errorf("notes = %q, want the a|b suggestion", out.ZeroHint.Notes)
	}
}

func TestExplainZero_NoRelaxationPastFirstPage(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"retry": 37, "jitter": 0}}
	in := searchInput{Query: "retry jitter", Offset: 50}
	opts := search.SearchOptions{
		Pattern: in.Query, Offset: 50, Limit: 50, OutputMode: filesOutputMode,
	}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if out.RelaxedQuery != "" || len(fb.searches) != 0 {
		t.Errorf("relaxed at offset 50: %+v, searches=%d", out, len(fb.searches))
	}
	if out.Offset != 50 {
		t.Errorf("offset = %d, want the requested 50 echoed", out.Offset)
	}
	if !hasNote(out.ZeroHint.Notes, "offset > 0") {
		t.Errorf("notes = %q, want the offset note", out.ZeroHint.Notes)
	}
}

func TestExplainZero_SingleTermSkipsCounts(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"retry": 0}}
	in := searchInput{Query: "retry"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if len(fb.counts) != 0 || out.ZeroHint.TermCounts != nil {
		t.Errorf("one-term query was counted: counts=%q hint=%+v", fb.counts, out.ZeroHint)
	}
}

func TestExplainZero_RelaxedRerunStillEmptyCarriesNote(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"a": 1, "b": 1, "zzz": 0}}
	in := searchInput{Query: "a zzz b"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if out.RelaxedQuery != "" || out.Total != 0 {
		t.Fatalf("want zero without relaxed_query, got %+v", out)
	}
	if !hasNote(out.ZeroHint.Notes, "reran as `a b`") || !hasNote(out.ZeroHint.Notes, "a|b") {
		t.Errorf(
			"notes = %q, want the failed-rerun note with the a|b suggestion",
			out.ZeroHint.Notes,
		)
	}
}

func TestExplainZero_CountErrorSurfacesAsNote(t *testing.T) {
	fb := &fakeBackend{countErr: errors.New("daemon gone")}
	in := searchInput{Query: "retry jitter"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if out.ZeroHint.TermCounts != nil || out.RelaxedQuery != "" {
		t.Errorf("want no counts and no relaxation on a count error, got %+v", out)
	}
	if !hasNote(out.ZeroHint.Notes, "daemon gone") {
		t.Errorf("notes = %q, want the count error surfaced", out.ZeroHint.Notes)
	}
}

func TestExplainZero_TooManyTermsCountsFirstSixOnly(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{
		"a": 1, "b": 1, "c": 1, "d": 1, "e": 1, "f": 1, "g": 0,
	}}
	in := searchInput{Query: "a b c d e f g"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if len(out.ZeroHint.TermCounts) != maxDiagnosedTerms {
		t.Errorf("counted %d terms, want %d", len(out.ZeroHint.TermCounts), maxDiagnosedTerms)
	}
	if out.RelaxedQuery != "" || len(fb.searches) != 0 {
		t.Errorf("relaxed a query with uncounted terms: %+v", out)
	}
	if !hasNote(out.ZeroHint.Notes, "only the first 6 were counted") {
		t.Errorf("notes = %q, want the cap note", out.ZeroHint.Notes)
	}
}

func TestExplainZero_ParamNamesAreNoted(t *testing.T) {
	fb := &fakeBackend{files: map[string]int{"output_mode": 0, "context_lines": 0}}
	in := searchInput{Query: "output_mode context_lines"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), fb, in, opts, oneRepo, t.TempDir())

	if !hasNote(out.ZeroHint.Notes, "parameter names") {
		t.Errorf("notes = %q, want the parameter-name trap", out.ZeroHint.Notes)
	}
	if out.RelaxedQuery != "" {
		t.Errorf("relaxed a query whose every term is absent: %+v", out)
	}
}

func TestZeroNotes_AndTermsNoteYieldsToCounts(t *testing.T) {
	in := searchInput{Query: "a b c"}
	if notes := zeroNotes(in, termDiagnosis{}); !hasNote(notes, "3 AND terms") {
		t.Errorf("without counts, notes = %q, want the AND-terms reminder", notes)
	}
	diag := termDiagnosis{counts: []termCount{{Term: "a", Files: 1}}}
	if notes := zeroNotes(in, diag); hasNote(notes, "3 AND terms") {
		t.Errorf("with counts, notes = %q, want the reminder suppressed", notes)
	}
}

// indexBackend runs the diagnosis in-process against a fixture index, the
// same search and count functions the daemon calls, so the file-count
// semantics of the per-term counts are checked against real zoekt.
type indexBackend struct {
	indexDir  string
	repoNames map[string]string
}

func (b indexBackend) search(
	ctx context.Context,
	opts search.SearchOptions,
) ([]search.Match, error) {
	return search.Search(ctx, b.indexDir, opts, b.repoNames)
}

func (b indexBackend) count(
	ctx context.Context,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	return search.Count(ctx, b.indexDir, opts)
}

// fixtureIndex indexes a two-file repo: a.go holds IndexRepo twice and
// symbolSections once, b.go holds Blocks. No file holds both IndexRepo and
// Blocks.
func fixtureIndex(t *testing.T) (indexBackend, []finder.Repo) {
	t.Helper()
	repoDir := t.TempDir()
	files := map[string]string{
		"a.go": "package a\n\nfunc IndexRepo() {}\n\n// IndexRepo is called by symbolSections.\nfunc symbolSections() {}\n",
		"b.go": "package b\n\nfunc Blocks() {}\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(repoDir, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	repo := finder.Repo{Name: "test/repo", Path: repoDir}
	indexDir := t.TempDir()
	if err := search.IndexRepo(indexDir, repo, nil); err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}
	return indexBackend{indexDir: indexDir, repoNames: map[string]string{repo.Name: repo.Path}},
		[]finder.Repo{repo}
}

func TestExplainZero_FixtureRelaxesAbsentTerm(t *testing.T) {
	b, repos := fixtureIndex(t)
	in := searchInput{Query: "IndexRepo symbolSections zzznotaterm"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), b, in, opts, repos, b.indexDir)

	if out.RelaxedQuery != "IndexRepo symbolSections" {
		t.Errorf("relaxed_query = %q, want %q", out.RelaxedQuery, "IndexRepo symbolSections")
	}
	if !reflect.DeepEqual(out.DroppedTerms, []string{"zzznotaterm"}) {
		t.Errorf("dropped_terms = %q", out.DroppedTerms)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "a.go" {
		t.Errorf("files = %+v, want a.go only", out.Files)
	}
	text := renderSearchText(out)
	wantLine := "relaxed: dropped 'zzznotaterm' (0 files); showing results for `IndexRepo symbolSections`"
	if !strings.HasPrefix(text, wantLine+"\n\n") {
		t.Errorf("text rendering:\n%s\nwant it to open with %q", text, wantLine)
	}
}

func TestExplainZero_FixtureCountsFilesNotMatches(t *testing.T) {
	b, repos := fixtureIndex(t)
	in := searchInput{Query: "IndexRepo Blocks"}
	opts := search.SearchOptions{Pattern: in.Query, Limit: 50, OutputMode: filesOutputMode}

	out := explainZero(context.Background(), b, in, opts, repos, b.indexDir)

	if out.RelaxedQuery != "" || out.Total != 0 || out.ZeroHint == nil {
		t.Fatalf("want a zero result with a hint, got %+v", out)
	}
	// IndexRepo appears twice in a.go: a file count says 1, a match count 2.
	want := []termCount{{Term: "IndexRepo", Files: 1}, {Term: "Blocks", Files: 1}}
	if !reflect.DeepEqual(out.ZeroHint.TermCounts, want) {
		t.Errorf("term_counts = %+v, want %+v", out.ZeroHint.TermCounts, want)
	}
	if !hasNote(out.ZeroHint.Notes, "IndexRepo|Blocks") {
		t.Errorf("notes = %q, want the a|b suggestion", out.ZeroHint.Notes)
	}
	if !strings.Contains(renderSearchText(out), "files per term: IndexRepo=1, Blocks=1") {
		t.Errorf("text rendering lacks the per-term line:\n%s", renderSearchText(out))
	}
}

func TestExplainZero_FixtureFileFilterFoldsIntoCounts(t *testing.T) {
	b, repos := fixtureIndex(t)
	in := searchInput{Query: "IndexRepo Blocks", File: `b\.go$`}
	opts := search.SearchOptions{
		Pattern: in.Query, FileFilter: in.File, Limit: 50, OutputMode: filesOutputMode,
	}

	out := explainZero(context.Background(), b, in, opts, repos, b.indexDir)

	// Under the b.go filter IndexRepo matches nothing, so it is the dropped term.
	if out.RelaxedQuery != "Blocks" || !reflect.DeepEqual(out.DroppedTerms, []string{"IndexRepo"}) {
		t.Errorf(
			"relaxed=%q dropped=%q, want Blocks / [IndexRepo]",
			out.RelaxedQuery,
			out.DroppedTerms,
		)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "b.go" {
		t.Errorf("files = %+v, want b.go", out.Files)
	}
}

func TestHandleQueryValidate_MalformedAndSplit(t *testing.T) {
	_, out, err := handleQueryValidate(context.Background(), nil, queryValidateInput{Query: `"`})
	if err != nil {
		t.Fatalf("malformed query returned an error, want valid=false: %v", err)
	}
	if out.Valid || !strings.Contains(out.Error, "only quote characters") || out.Hint == "" {
		t.Errorf("validate(%q) = %+v, want valid=false with problem and fix", `"`, out)
	}

	_, out, err = handleQueryValidate(context.Background(), nil,
		queryValidateInput{Query: `retry f:\.go$ "back off" -test`})
	if err != nil || !out.Valid {
		t.Fatalf("valid query: err=%v out=%+v", err, out)
	}
	if !reflect.DeepEqual(out.Terms, []string{"retry", `"back off"`}) {
		t.Errorf("terms = %q", out.Terms)
	}
	if !reflect.DeepEqual(out.Filters, []string{`f:\.go$`, "-test"}) {
		t.Errorf("filters = %q", out.Filters)
	}

	_, out, _ = handleQueryValidate(
		context.Background(),
		nil,
		queryValidateInput{Query: "output_mode"},
	)
	if !strings.Contains(out.Hint, "parameter names") {
		t.Errorf("validate(output_mode).hint = %q, want the parameter-name trap", out.Hint)
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
