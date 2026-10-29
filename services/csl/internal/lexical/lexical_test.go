package lexical

import (
	"reflect"
	"testing"
)

func TestRepoFilter(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"thismoon", "(?i)thismoon"},
		{"Mad01/.*", "(?i)Mad01/.*"},
	}
	for _, tt := range tests {
		if got := RepoFilter(tt.in); got != tt.want {
			t.Errorf("RepoFilter(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestZeroHintLines(t *testing.T) {
	if lines := (*ZeroHint)(nil).Lines(); lines != nil {
		t.Errorf("nil hint renders %q, want no lines", lines)
	}
	hint := &ZeroHint{
		ParsedQuery:     "substr:\"x\"",
		ReposSearched:   1,
		ReposIndexed:    2,
		ReposDiscovered: 3,
		NewestIndexedAt: "2026-08-01T00:00:00Z",
		OldestIndexedAt: "2026-01-01T00:00:00Z",
		TermCounts:      []TermCount{{Term: "retry", Files: 37}, {Term: "jitter", Files: 0}},
		Notes:           []string{"first", "second"},
	}
	want := []string{
		"parsed query: substr:\"x\"",
		"files per term: retry=37, jitter=0",
		"repos searched: 1 (2 indexed, 3 discovered)",
		"index age: newest 2026-08-01T00:00:00Z, oldest 2026-01-01T00:00:00Z",
		"- first",
		"- second",
	}
	if got := hint.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %q\nwant %q", got, want)
	}
}

func TestRelaxedLine(t *testing.T) {
	if got, want := RelaxedLine([]string{"jitter"}, "retry backoff"),
		"relaxed: dropped 'jitter' (0 files); showing results for `retry backoff`"; got != want {
		t.Errorf("one term: %q, want %q", got, want)
	}
	if got, want := RelaxedLine([]string{"a", "b"}, "c"),
		"relaxed: dropped 'a', 'b' (0 files each); showing results for `c`"; got != want {
		t.Errorf("two terms: %q, want %q", got, want)
	}
}
