package search

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateQuery_SplitsSymbolAlternation: a sym: term whose regexp
// alternates at the top level parses to the same tree as one sym: term per
// alternative joined with or, so zoekt never sees the alternation its symbol
// matcher rejects.
func TestValidateQuery_SplitsSymbolAlternation(t *testing.T) {
	tests := []struct {
		name  string
		query string
		same  string // the hand-written form the query must parse identically to
	}{
		{
			name:  "bare alternation",
			query: "sym:StatePath|DaemonLogFile",
			same:  "sym:StatePath or sym:DaemonLogFile",
		},
		{
			name:  "grouped alternation",
			query: "sym:(StatePath|DaemonLogFile)",
			same:  "sym:StatePath or sym:DaemonLogFile",
		},
		{
			name:  "followed by another term",
			query: "sym:StatePath|DaemonLogFile =",
			same:  "(sym:StatePath or sym:DaemonLogFile) =",
		},
		{
			name:  "regexp alternative",
			query: "sym:Foo|Bar.*",
			same:  "sym:Foo or sym:Bar.*",
		},
		{
			name:  "case-insensitive",
			query: "sym:foo|bar",
			same:  "sym:foo or sym:bar",
		},
		{
			name:  "with filters",
			query: `sym:Foo|Bar f:\.go$ repo:x`,
			same:  `(sym:Foo or sym:Bar) f:\.go$ repo:x`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateQuery(tc.query)
			want := ValidateQuery(tc.same)
			if !got.Valid || !want.Valid {
				t.Fatalf("valid: got %+v, want %+v", got, want)
			}
			if got.Parsed != want.Parsed {
				t.Errorf("parsed %q = %s, want the tree of %q = %s",
					tc.query, got.Parsed, tc.same, want.Parsed)
			}
		})
	}
}

// TestValidateQuery_SymbolShapesKept pins the sym: shapes the rewrite leaves
// alone (an alternation nested under a concatenation or an optional keeps a
// regexp node zoekt can run) and the smart case a split carries over: one
// uppercase letter makes the whole term case-sensitive, so both halves of
// sym:Foo|bar stay case-sensitive, while an explicit (?i) keeps its literals
// case-insensitive.
func TestValidateQuery_SymbolShapesKept(t *testing.T) {
	tests := []struct{ query, parsed string }{
		{"sym:Foo|bar", `(or sym:case_substr:"Foo" sym:case_substr:"bar")`},
		{"sym:(?i)foo|bar", `(or sym:substr:"FOO" sym:substr:"BAR")`},
		{"sym:State(Path|File)", `sym:case_regex:"State(?:Path|File)"`},
		{"sym:(Foo|Bar)?", `sym:case_regex:"(?:Foo|Bar)?"`},
		{"sym:Hello", `sym:case_substr:"Hello"`},
		{"Hello|Point", `(or case_file_regex:"Hello|Point" case_regex:"Hello|Point")`},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			info := ValidateQuery(tc.query)
			if !info.Valid {
				t.Fatalf("valid = false: %+v", info)
			}
			if info.Parsed != tc.parsed {
				t.Errorf("parsed = %s, want %s", info.Parsed, tc.parsed)
			}
		})
	}
}

// TestValidateQuery_RefusesSymbolShapes: the two sym: shapes csl cannot
// rewrite come back invalid with the form to write instead, and never reach
// zoekt: an alternation under a repetition, which hits zoekt's internal
// error with no top-level alternation to split, and a second sym: prefix
// inside the regexp, which can only be a mistake.
func TestValidateQuery_RefusesSymbolShapes(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantError string
		wantHint  string
	}{
		{
			name:      "repeated alternation",
			query:     "sym:(Hello|Point)+",
			wantError: "repeats an alternation",
			wantHint:  "(sym:Foo or sym:Bar)",
		},
		{
			name:      "repeated alternation in one alternative",
			query:     "sym:Foo|(Hello|Point)+",
			wantError: "repeats an alternation",
			wantHint:  "(sym:Foo or sym:Bar)",
		},
		{
			name:      "prefix repeated inside",
			query:     "sym:Foo|sym:Bar",
			wantError: "has sym: inside the name",
			wantHint:  "sym:Foo|Bar",
		},
		{
			name:      "prefix inside a regexp alternative",
			query:     "sym:Foo|sym:Bar.*",
			wantError: "has sym: inside the name",
			wantHint:  "sym:Foo|Bar",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ValidateQuery(tc.query)
			if info.Valid || info.Parsed != "" {
				t.Fatalf("validate(%q) = %+v, want valid=false", tc.query, info)
			}
			if !strings.Contains(info.Error, tc.wantError) {
				t.Errorf("error = %q, want substring %q", info.Error, tc.wantError)
			}
			if !strings.Contains(info.Hint, tc.wantHint) {
				t.Errorf("hint = %q, want substring %q", info.Hint, tc.wantHint)
			}
		})
	}
}

// TestParseFailure: a refused sym: term reaches the search caller as is,
// since it carries its own fix, while any other parse error gains the
// pointer at csl query.
func TestParseFailure(t *testing.T) {
	refused := &SymbolShapeError{Term: "sym:x", Why: "why", Fix: "fix"}
	if got := parseFailure(refused, "p"); got != refused {
		t.Errorf("parseFailure(refused) = %v, want the error unchanged", got)
	}
	if want := "sym:x why; fix"; refused.Error() != want {
		t.Errorf("Error() = %q, want %q", refused.Error(), want)
	}

	parse := errors.New("missing closing )")
	got := parseFailure(parse, "func (Walk")
	if !errors.Is(got, parse) {
		t.Errorf("parseFailure(parse) = %v, want it to wrap the parse error", got)
	}
	if !strings.Contains(got.Error(), `csl query "func (Walk"`) {
		t.Errorf("parseFailure(parse) = %q, want the csl query hint", got)
	}
}
