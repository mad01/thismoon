package match

import (
	"iter"
	"slices"
	"testing"
)

func TestMatch(t *testing.T) {
	const title = "acme/api#12 Fix login redirect alice"
	cases := []struct {
		name  string
		query string
		text  string
		want  bool
	}{
		{"empty query matches all", "", title, true},
		{"blank query matches all", "   ", title, true},
		{"empty query matches empty text", "", "", true},
		{"substring", "login", title, true},
		{"subsequence", "lgnrdr", title, true},
		{"subsequence across words", "apifix", title, true},
		{"order matters", "nigol", title, false},
		{"missing letter", "logout", title, false},
		{"term against empty text", "a", "", false},
		{"lowercase query ignores case", "fix", title, true},
		{"lowercase query matches uppercase text", "acme", "ACME/API#12", true},
		{"uppercase query is case-sensitive", "Fix", title, true},
		{"uppercase query misses other case", "Login", title, false},
		{"one uppercase letter makes every term case-sensitive", "Fix ALICE", title, false},
		{"all terms match", "fix alice", title, true},
		{"terms match in any order", "alice api#12", title, true},
		{"one term misses", "fix bob", title, false},
		{"repeated spaces between terms", "fix   alice", title, true},
		{"repeated letters need repeats in text", "ll", "login", false},
		{"non-ASCII ignores case", "åsa", "Åsa fixed the build", true},
		{"non-ASCII uppercase is case-sensitive", "Åsa", "åsa", false},
		{"invalid UTF-8 text does not panic", "a", "\xffa\xfe", true},
		{"a colon with no scopes is text", "fix:", "fix: login", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := Parse(tc.query, nil)
			if got := q.Match("", tc.text); got != tc.want {
				t.Errorf("Parse(%q).Match(%q) = %t, want %t", tc.query, tc.text, got, tc.want)
			}
			if got := q.MatchText(Prepare("", tc.text)); got != tc.want {
				t.Errorf("Parse(%q).MatchText(Prepare(%q)) = %t, want %t",
					tc.query, tc.text, got, tc.want)
			}
		})
	}
}

func TestZeroQueryShowsTheDefaultSet(t *testing.T) {
	var q Query
	if !q.Match("", "anything") {
		t.Error(`Query{}.Match("", "anything") = false, want true`)
	}
	if q.Match("ignored", "anything") {
		t.Error(`Query{}.Match("ignored", "anything") = true, want false`)
	}
}

// present yields the scopes a store's cards have, as a store passes them.
func present(scopes ...string) iter.Seq[string] { return slices.Values(scopes) }

func TestParseScopes(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		present []string
		// wantScopes is what the query selects; nil is the default set.
		wantScopes []string
		// match and noMatch are scope and text pairs the query shows and
		// hides.
		match, noMatch [][2]string
	}{
		{
			name: "a one-letter prefix selects", query: "i:", present: []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "acme/api#12 fix login"}},
			noMatch:    [][2]string{{"", "acme/api#12 fix login"}, {"", "i: in the title"}},
		},
		{
			name: "a longer prefix selects", query: "ignore:", present: []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "docs typo"}},
		},
		{
			name: "the whole name selects and the rest filters", query: "ignored:abc",
			present:    []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "a b c"}},
			noMatch:    [][2]string{{"ignored", "xyz"}, {"", "a b c"}},
		},
		{
			name: "a prefix of no scope is text", query: "docs:", present: []string{"ignored"},
			match:   [][2]string{{"", "docs: fix typo"}},
			noMatch: [][2]string{{"", "docs fix typo"}, {"ignored", "docs: fix typo"}},
		},
		{
			name: "a colon word is text while no scope is present", query: "x:",
			match: [][2]string{{"", "x: one"}},
		},
		{
			name: "a prefix of two scopes selects both", query: "i:",
			present:    []string{"inbox", "ignored", "muted", "inbox"},
			wantScopes: []string{"ignored", "inbox"},
			match:      [][2]string{{"ignored", "one"}, {"inbox", "two"}},
			noMatch:    [][2]string{{"muted", "three"}, {"", "four"}},
		},
		{
			name: "an empty prefix is text", query: ":abc", present: []string{"ignored"},
			match: [][2]string{{"", "x:abc"}},
		},
		{
			name: "a longer word than the scope is text", query: "ignoredx:", present: []string{"ignored"},
			noMatch: [][2]string{{"ignored", "ignoredx:"}},
			match:   [][2]string{{"", "ignoredx:"}},
		},
		{
			name: "later words filter within the scope", query: "  i: fix alice",
			present:    []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "fix login alice"}},
			noMatch:    [][2]string{{"ignored", "fix login bob"}},
		},
		{
			name: "only the first word selects", query: "fix i:", present: []string{"ignored"},
			match:   [][2]string{{"", "fix i: thing"}},
			noMatch: [][2]string{{"ignored", "fix login"}},
		},
		{
			name: "a capital prefix is text", query: "I:", present: []string{"ignored"},
			match:   [][2]string{{"", "I: thing"}},
			noMatch: [][2]string{{"", "i: thing"}},
		},
		{
			name: "the rest keeps smart case", query: "i:Fix", present: []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "Fix login"}},
			noMatch:    [][2]string{{"ignored", "fix login"}},
		},
		{
			name: "a second colon is text", query: "i:a:b", present: []string{"ignored"},
			wantScopes: []string{"ignored"},
			match:      [][2]string{{"ignored", "a:b"}},
			noMatch:    [][2]string{{"ignored", "ab"}},
		},
		{
			name: "the empty query shows the default set", query: "", present: []string{"ignored"},
			match:   [][2]string{{"", "anything"}},
			noMatch: [][2]string{{"ignored", "anything"}},
		},
		{
			name: "a plain word hides scoped cards", query: "fix", present: []string{"ignored"},
			match:   [][2]string{{"", "fix login"}},
			noMatch: [][2]string{{"ignored", "fix login"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := Parse(tc.query, present(tc.present...))
			if got := q.Scopes(); !slices.Equal(got, tc.wantScopes) {
				t.Errorf("Parse(%q).Scopes() = %q, want %q", tc.query, got, tc.wantScopes)
			}
			for _, want := range []bool{true, false} {
				pairs := tc.match
				if !want {
					pairs = tc.noMatch
				}
				for _, p := range pairs {
					if got := q.Match(p[0], p[1]); got != want {
						t.Errorf("Parse(%q).Match(%q, %q) = %t, want %t",
							tc.query, p[0], p[1], got, want)
					}
					if got := q.MatchText(Prepare(p[0], p[1])); got != want {
						t.Errorf("Parse(%q).MatchText(Prepare(%q, %q)) = %t, want %t",
							tc.query, p[0], p[1], got, want)
					}
				}
			}
		})
	}
}
