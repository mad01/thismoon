package mcpserver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

func makeMatches(n int, repo, file string) []search.Match {
	matches := make([]search.Match, n)
	for i := range n {
		matches[i] = search.Match{
			Repo:   repo,
			File:   file,
			Line:   i + 1,
			Column: 1,
			Text:   "match line",
		}
	}
	return matches
}

func makeFileMatches(n int, repo string) []search.Match {
	matches := make([]search.Match, n)
	for i := range n {
		matches[i] = search.Match{
			Repo:   repo,
			File:   fmt.Sprintf("pkg%d/file%d.go", i/10, i),
			Line:   1,
			Column: 1,
			Text:   "match",
		}
	}
	return matches
}

func TestBuildSearchOutput_FilesNotTruncated(t *testing.T) {
	matches := makeFileMatches(10, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, 0, matches)

	if out.Truncated {
		t.Error("expected truncated=false for 10 files with limit 50")
	}
	if out.TotalAvailable != 0 {
		t.Errorf("expected total_available=0 when not truncated, got %d", out.TotalAvailable)
	}
	if out.Total != 10 {
		t.Errorf("expected total=10, got %d", out.Total)
	}
}

func TestBuildSearchOutput_FilesTruncated(t *testing.T) {
	matches := makeFileMatches(60, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, 0, matches)

	if !out.Truncated {
		t.Error("expected truncated=true when files >= limit")
	}
	if out.TotalAvailable != 60 {
		t.Errorf("expected total_available=60, got %d", out.TotalAvailable)
	}
	if out.Total != 50 {
		t.Errorf("expected total=50 (capped at limit), got %d", out.Total)
	}
	if len(out.Files) != 50 {
		t.Errorf("expected 50 file entries, got %d", len(out.Files))
	}
}

func TestBuildSearchOutput_FilesExactlyAtLimit(t *testing.T) {
	matches := makeFileMatches(50, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, 0, matches)

	if !out.Truncated {
		t.Error("expected truncated=true when files == limit (more may exist upstream)")
	}
	if out.Total != 50 {
		t.Errorf("expected total=50, got %d", out.Total)
	}
}

func TestBuildSearchOutput_ContentNotTruncated(t *testing.T) {
	matches := makeMatches(100, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, 0, matches)

	if out.Truncated {
		t.Error("expected truncated=false for 100 lines (under maxContentLines)")
	}
	if out.Total != 100 {
		t.Errorf("expected total=100, got %d", out.Total)
	}
}

func TestBuildSearchOutput_ContentTruncatedAtMaxLines(t *testing.T) {
	matches := makeMatches(500, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, 0, matches)

	if !out.Truncated {
		t.Error("expected truncated=true for 500 lines")
	}
	if out.TotalAvailable != 500 {
		t.Errorf("expected total_available=500, got %d", out.TotalAvailable)
	}
	if out.Total != maxContentLines {
		t.Errorf("expected total=%d (maxContentLines), got %d", maxContentLines, out.Total)
	}
	if len(out.Lines) != maxContentLines {
		t.Errorf("expected %d line entries, got %d", maxContentLines, len(out.Lines))
	}
}

func TestBuildSearchOutput_ContentExactlyAtMax(t *testing.T) {
	matches := makeMatches(maxContentLines, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, 0, matches)

	if out.Truncated {
		t.Error("expected truncated=false when lines == maxContentLines exactly")
	}
	if out.Total != maxContentLines {
		t.Errorf("expected total=%d, got %d", maxContentLines, out.Total)
	}
}

func TestBuildSearchOutput_EmptyResults(t *testing.T) {
	out := buildSearchOutput(filesOutputMode, 50, 0, nil)
	if out.Truncated {
		t.Error("expected truncated=false for empty results")
	}
	if out.Total != 0 {
		t.Errorf("expected total=0, got %d", out.Total)
	}

	out = buildSearchOutput(contentOutputMode, 50, 0, nil)
	if out.Truncated {
		t.Error("expected truncated=false for empty content results")
	}
}

func TestBuildSearchOutput_FilesDeduplicated(t *testing.T) {
	// Multiple matches in the same file should collapse to one entry.
	matches := makeMatches(20, "org/repo", "main.go")
	out := buildSearchOutput(filesOutputMode, 50, 0, matches)

	if out.Total != 1 {
		t.Errorf("expected 1 unique file, got %d", out.Total)
	}
	if out.Truncated {
		t.Error("expected truncated=false for 1 unique file")
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryTrapNotes(tt.query)
			if len(got) != tt.want {
				t.Errorf(
					"queryTrapNotes(%q) = %d notes %v, want %d",
					tt.query,
					len(got),
					got,
					tt.want,
				)
			}
		})
	}
}

func TestOverConstraintNotes(t *testing.T) {
	tests := []struct {
		name string
		in   searchInput
		want int
	}{
		{name: "one term, no filters", in: searchInput{Query: "cluster_name"}, want: 0},
		{name: "two terms", in: searchInput{Query: "cluster name"}, want: 0},
		{name: "three AND terms", in: searchInput{Query: "backend service health"}, want: 1},
		{
			name: "five AND terms",
			in:   searchInput{Query: "backend service health check endpoint"},
			want: 1,
		},
		{name: "file filter set", in: searchInput{Query: "x", File: `.*\.tf$`}, want: 1},
		{name: "multi-word quoted phrase", in: searchInput{Query: `"backend service"`}, want: 1},
		{name: "single-word quote is silent", in: searchInput{Query: `"backend"`}, want: 0},
		{
			name: "filters, negations, OR groups don't count",
			in:   searchInput{Query: "foo|bar -test lang:go f:x repo:y"},
			want: 0,
		},
		{
			name: "terms and file filter stack",
			in:   searchInput{Query: "backend service health", File: "x"},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := overConstraintNotes(tt.in)
			if len(got) != tt.want {
				t.Errorf(
					"overConstraintNotes(%+v) = %d notes %v, want %d",
					tt.in,
					len(got),
					got,
					tt.want,
				)
			}
		})
	}
}

func TestAndTermCount(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{query: "foo", want: 1},
		{query: "foo bar baz", want: 3},
		{query: `"foo bar" baz`, want: 2},
		{query: "foo|bar baz", want: 1},
		{query: "foo -bar lang:go", want: 1},
		{query: "foo or bar", want: 2},
	}
	for _, tt := range tests {
		if got := andTermCount(tt.query); got != tt.want {
			t.Errorf("andTermCount(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestBuildZeroHint_RepoFilterMatchesNone(t *testing.T) {
	repos := []finder.Repo{
		{Name: "org/alpha", Path: "/tmp/alpha"},
		{Name: "org/beta", Path: "/tmp/beta"},
	}
	in := searchInput{Query: "foo", Repo: "nosuchrepo"}
	opts := search.SearchOptions{Pattern: in.Query, RepoFilter: insensitiveRepoFilter(in.Repo)}

	hint := buildZeroHint(in, opts, repos, t.TempDir())

	if hint.ReposDiscovered != 2 {
		t.Errorf("expected repos_discovered=2, got %d", hint.ReposDiscovered)
	}
	if hint.ReposSearched != 0 {
		t.Errorf("expected repos_searched=0 for non-matching filter, got %d", hint.ReposSearched)
	}
	if len(hint.Notes) != 1 {
		t.Fatalf("expected 1 note about the repo filter, got %v", hint.Notes)
	}
}

func TestBuildZeroHint_RepoFilterCaseInsensitive(t *testing.T) {
	indexDir := t.TempDir()
	state := search.EmptyState()
	state.Repos["/tmp/alpha"] = search.RepoState{IndexedAt: time.Now()}
	state.Repos["/tmp/beta"] = search.RepoState{IndexedAt: time.Now()}
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("save state: %v", err)
	}
	repos := []finder.Repo{
		{Name: "org/Alpha", Path: "/tmp/alpha"},
		{Name: "org/beta", Path: "/tmp/beta"},
	}
	in := searchInput{Query: "foo", Repo: "alpha"}
	opts := search.SearchOptions{Pattern: in.Query, RepoFilter: insensitiveRepoFilter(in.Repo)}

	hint := buildZeroHint(in, opts, repos, indexDir)

	if hint.ReposSearched != 1 {
		t.Errorf("expected repos_searched=1 (case-insensitive match), got %d", hint.ReposSearched)
	}
	if len(hint.Notes) != 0 {
		t.Errorf("expected no notes when the filter matches, got %v", hint.Notes)
	}
}

func TestBuildZeroHint_MatchedButUnindexedRepoCarriesNote(t *testing.T) {
	indexDir := t.TempDir()
	state := search.EmptyState()
	state.Repos["/tmp/beta"] = search.RepoState{IndexedAt: time.Now()}
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("save state: %v", err)
	}
	repos := []finder.Repo{
		{Name: "org/alpha", Path: "/tmp/alpha"},
		{Name: "org/beta", Path: "/tmp/beta"},
	}
	in := searchInput{Query: "foo", Repo: "alpha"}
	opts := search.SearchOptions{Pattern: in.Query, RepoFilter: insensitiveRepoFilter(in.Repo)}

	hint := buildZeroHint(in, opts, repos, indexDir)

	if hint.ReposSearched != 0 {
		t.Errorf("expected repos_searched=0 for a matched-but-unindexed repo, got %d",
			hint.ReposSearched)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "not in the search index") {
		t.Errorf("notes = %v, want the unindexed-repo note", hint.Notes)
	}
}

func TestBuildZeroHint_WhitespaceRepoFilterCarriesNote(t *testing.T) {
	repos := []finder.Repo{{Name: "org/alpha", Path: "/tmp/alpha"}}
	in := searchInput{Query: "foo", Repo: "my repo"}
	opts := search.SearchOptions{Pattern: in.Query, RepoFilter: insensitiveRepoFilter(in.Repo)}

	hint := buildZeroHint(in, opts, repos, t.TempDir())

	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "whitespace") {
		t.Errorf("notes = %v, want the whitespace-filter note", hint.Notes)
	}
}

func TestBuildZeroHint_ZeroIndexedAtEntriesSkipped(t *testing.T) {
	indexDir := t.TempDir()
	state := search.EmptyState()
	stamp := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	state.Repos["/tmp/alpha"] = search.RepoState{IndexedAt: stamp}
	state.Repos["/tmp/beta"] = search.RepoState{} // zero IndexedAt, e.g. legacy state
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("save state: %v", err)
	}
	repos := []finder.Repo{{Name: "org/alpha", Path: "/tmp/alpha"}}
	in := searchInput{Query: "foo"}
	opts := search.SearchOptions{Pattern: in.Query}

	hint := buildZeroHint(in, opts, repos, indexDir)

	want := stamp.Format(time.RFC3339)
	if hint.NewestIndexedAt != want || hint.OldestIndexedAt != want {
		t.Errorf("index age = %q / %q, want both %q (zero entries skipped)",
			hint.NewestIndexedAt, hint.OldestIndexedAt, want)
	}
}

func TestQueryTrapNotes_QuotedOperatorsIgnored(t *testing.T) {
	if notes := queryTrapNotes(`"dead OR alive"`); len(notes) != 0 {
		t.Errorf("quoted phrase produced trap notes: %v", notes)
	}
	if notes := queryTrapNotes(`"a | b"`); len(notes) != 0 {
		t.Errorf("quoted pipe produced trap notes: %v", notes)
	}
}

func TestBuildZeroHint_ParsedQueryAndIndexAge(t *testing.T) {
	indexDir := t.TempDir()
	state := search.EmptyState()
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	state.Repos["/tmp/alpha"] = search.RepoState{IndexedAt: older}
	state.Repos["/tmp/beta"] = search.RepoState{IndexedAt: newer}
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("save state: %v", err)
	}

	repos := []finder.Repo{{Name: "org/alpha", Path: "/tmp/alpha"}}
	in := searchInput{Query: "foo bar"}
	opts := search.SearchOptions{Pattern: in.Query}

	hint := buildZeroHint(in, opts, repos, indexDir)

	if hint.ParsedQuery == "" {
		t.Error("expected parsed_query to be set for a valid query")
	}
	if hint.ReposIndexed != 2 {
		t.Errorf("expected repos_indexed=2, got %d", hint.ReposIndexed)
	}
	if hint.NewestIndexedAt != newer.Format(time.RFC3339) {
		t.Errorf(
			"expected newest_indexed_at=%s, got %s",
			newer.Format(time.RFC3339),
			hint.NewestIndexedAt,
		)
	}
	if hint.OldestIndexedAt != older.Format(time.RFC3339) {
		t.Errorf(
			"expected oldest_indexed_at=%s, got %s",
			older.Format(time.RFC3339),
			hint.OldestIndexedAt,
		)
	}
	if hint.ReposSearched != 1 {
		t.Errorf("expected repos_searched=1 with no filter, got %d", hint.ReposSearched)
	}
}

func TestBuildZeroHint_MissingStateOmitsIndexFields(t *testing.T) {
	repos := []finder.Repo{{Name: "org/alpha", Path: "/tmp/alpha"}}
	in := searchInput{Query: "foo"}
	opts := search.SearchOptions{Pattern: in.Query}

	hint := buildZeroHint(in, opts, repos, t.TempDir())

	if hint.ReposIndexed != 0 {
		t.Errorf("expected repos_indexed=0 with no state file, got %d", hint.ReposIndexed)
	}
	if hint.NewestIndexedAt != "" || hint.OldestIndexedAt != "" {
		t.Errorf(
			"expected empty index-age fields, got %q / %q",
			hint.NewestIndexedAt,
			hint.OldestIndexedAt,
		)
	}
}
