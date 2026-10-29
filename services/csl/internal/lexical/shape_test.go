package lexical

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckShape(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string // substring of Problem; "" means well-formed
	}{
		{name: "empty", query: "", want: "empty"},
		{name: "whitespace", query: "  \t", want: "empty"},
		{name: "one quote", query: `"`, want: "only quote characters"},
		{name: "two quotes", query: `""`, want: "only quote characters"},
		{name: "three quotes", query: `"""`, want: "only quote characters"},
		{name: "unbalanced quote", query: `foo "bar`, want: "unbalanced double quote"},
		{name: "unbalanced quote at end", query: `foo bar"`, want: "unbalanced double quote"},
		{name: "empty phrase among terms", query: `foo "" bar`, want: "empty quoted phrase"},
		{name: "trailing backslash", query: `foo\`, want: "lone backslash"},
		{name: "escaped quote is balanced", query: `foo\"bar`, want: ""},
		{name: "escaped quote inside phrase", query: `"say \"hi\""`, want: ""},
		{name: "phrase", query: `"foo bar" baz`, want: ""},
		{name: "param names are not malformed", query: "output_mode context_lines", want: ""},
		{name: "regex", query: `func \(Walk`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckShape(tt.query, testSurface)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("CheckShape(%q) = %v, want nil", tt.query, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("CheckShape(%q) = nil, want problem containing %q", tt.query, tt.want)
			}
			if !strings.Contains(got.Problem, tt.want) {
				t.Errorf("problem = %q, want it to contain %q", got.Problem, tt.want)
			}
			if got.Fix == "" {
				t.Error("malformed query carries no fix")
			}
			if !strings.Contains(got.Error(), got.Fix) {
				t.Errorf("Error() = %q does not carry the fix", got.Error())
			}
		})
	}
}

// TestCheckShape_FixNamesQueryArg: a fix that says where the search text
// goes names the front end's own argument, so each front end gets its own
// wording.
func TestCheckShape_FixNamesQueryArg(t *testing.T) {
	for _, q := range []string{"", `"`} {
		m := CheckShape(q, testSurface)
		if m == nil || !strings.Contains(m.Fix, " in query-arg") {
			t.Errorf("CheckShape(%q).Fix = %v, want it to name query-arg", q, m)
		}
	}
}

func TestSplitQuery(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{query: "foo bar", want: []string{"foo", "bar"}},
		{query: "  foo\tbar\n", want: []string{"foo", "bar"}},
		{query: `"foo bar" baz`, want: []string{`"foo bar"`, "baz"}},
		{query: `(a b) c`, want: []string{"(a b)", "c"}},
		{query: `(a (b c)) d`, want: []string{"(a (b c))", "d"}},
		{query: `foo\ bar c`, want: []string{`foo\ bar`, "c"}},
		{query: `f:"a b" x`, want: []string{`f:"a b"`, "x"}},
		{query: `-x foo`, want: []string{"-x", "foo"}},
	}
	for _, tt := range tests {
		got, m := splitQuery(tt.query)
		if m != nil {
			t.Errorf("splitQuery(%q) malformed: %v", tt.query, m)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitQuery(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestSplitTerms(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		terms   []string
		filters []string
	}{
		{name: "one term", query: "foo", terms: []string{"foo"}},
		{name: "plain AND", query: "foo bar baz", terms: []string{"foo", "bar", "baz"}},
		{
			name:  "quoted phrase is one term",
			query: `"foo bar" baz`,
			terms: []string{`"foo bar"`, "baz"},
		},
		{name: "a|b group is one term", query: "foo|bar baz", terms: []string{"foo|bar", "baz"}},
		{name: "regex is a term", query: `foo.*bar baz`, terms: []string{`foo.*bar`, "baz"}},
		{
			name:    "paren group is one term",
			query:   "(a b) c",
			terms:   []string{"(a b)", "c"},
			filters: nil,
		},
		{
			name:    "filter atoms are fixed",
			query:   `foo repo:x f:\.go$ lang:go case:yes bar`,
			terms:   []string{"foo", "bar"},
			filters: []string{"repo:x", `f:\.go$`, "lang:go", "case:yes"},
		},
		{
			name:    "sym atom and negation are fixed",
			query:   "sym:Foo bar -baz",
			terms:   []string{"bar"},
			filters: []string{"sym:Foo", "-baz"},
		},
		{
			name:    "content atom is a term",
			query:   "c:foo bar",
			terms:   []string{"c:foo", "bar"},
			filters: nil,
		},
		{name: "top-level or is not an AND chain", query: "foo or bar baz"},
		{name: "lone negation is not modelled", query: "- foo"},
		{name: "malformed yields nothing", query: `foo "bar`},
		{name: "empty yields nothing", query: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitTerms(tt.query)
			if !reflect.DeepEqual(got.Terms, tt.terms) {
				t.Errorf("terms = %q, want %q", got.Terms, tt.terms)
			}
			if !reflect.DeepEqual(got.Fixed, tt.filters) {
				t.Errorf("filters = %q, want %q", got.Fixed, tt.filters)
			}
		})
	}
}

func TestParts_Without(t *testing.T) {
	parts := SplitTerms(`retry f:\.go$ backoff -test jitter`)
	if got, want := parts.without([]string{"jitter"}), `retry f:\.go$ backoff -test`; got != want {
		t.Errorf("without(jitter) = %q, want %q", got, want)
	}
	if got, want := parts.without([]string{"retry", "jitter"}), `f:\.go$ backoff -test`; got != want {
		t.Errorf("without(retry, jitter) = %q, want %q", got, want)
	}
	// Dropping a name that is a fixed atom, not a term, changes nothing.
	if got, want := parts.without([]string{"-test"}), `retry f:\.go$ backoff -test jitter`; got != want {
		t.Errorf("without(-test) = %q, want %q", got, want)
	}
}
