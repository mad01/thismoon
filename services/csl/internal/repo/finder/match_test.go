package finder

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/catalogspec"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "clean", in: "octo", want: "octo"},
		{name: "trailing ascii space", in: "octo ", want: "octo"},
		{name: "leading ascii space", in: " octo", want: "octo"},
		{name: "both sides ascii space", in: "  octo  ", want: "octo"},
		{name: "tab and newline", in: "\tocto\n", want: "octo"},
		{name: "non-breaking space", in: "octo ", want: "octo"},
		{name: "narrow no-break space", in: " octo", want: "octo"},
		{name: "ideographic space", in: "octo　", want: "octo"},
		{name: "empty", in: "", want: ""},
		{name: "only whitespace", in: " \t\n ", want: ""},
		{name: "internal space preserved", in: "foo bar", want: "foo bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeQuery(tc.in); got != tc.want {
				t.Errorf("NormalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompileMatcher(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantErr    bool
		matches    []string
		nonMatches []string
	}{
		{
			name:       "substring",
			query:      "octo",
			matches:    []string{"mad01/octo", "someone/octopus", "MAD01/OCTO"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "trailing space still matches cleanly",
			query:      "octo ",
			matches:    []string{"mad01/octo", "someone/octopus"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "non-breaking space trimmed",
			query:      "octo ",
			matches:    []string{"mad01/octo"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "regex anchors work",
			query:      "^mad01/octo$",
			matches:    []string{"mad01/octo"},
			nonMatches: []string{"mad01/octopus", "someone/mad01/octo"},
		},
		{name: "empty is rejected", query: "", wantErr: true},
		{name: "whitespace-only is rejected", query: " \t ", wantErr: true},
		{name: "invalid regex is rejected", query: "[", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re, err := CompileMatcher(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CompileMatcher(%q) expected error, got nil", tc.query)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompileMatcher(%q) error: %v", tc.query, err)
			}
			for _, s := range tc.matches {
				if !re.MatchString(s) {
					t.Errorf("expected %q to match %q", tc.query, s)
				}
			}
			for _, s := range tc.nonMatches {
				if re.MatchString(s) {
					t.Errorf("expected %q NOT to match %q", tc.query, s)
				}
			}
		})
	}
}

func TestQueryMatchers(t *testing.T) {
	withCatalog := Repo{
		Name: "mad01/code-search-local",
		Catalog: &catalogspec.Component{
			Name: "csl", Owner: "group:default/platform", System: "thismoon",
		},
	}
	noCatalog := Repo{Name: "mad01/dotfiles"}
	noSystem := Repo{
		Name:    "mad01/ralph",
		Catalog: &catalogspec.Component{Name: "ralph", Owner: "mad01"},
	}

	tests := []struct {
		name  string
		q     Query
		want  map[string]bool // repo name -> substring match expected
		regex map[string]bool // overrides for the regex matcher, when it differs
	}{
		{
			name: "name only",
			q:    Query{Name: "MAD01/"},
			want: map[string]bool{
				withCatalog.Name: true,
				noCatalog.Name:   true,
				noSystem.Name:    true,
			},
		},
		{
			name: "owner substring, case-insensitive",
			q:    Query{Owner: "Platform"},
			want: map[string]bool{
				withCatalog.Name: true,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
		},
		{
			name: "system with surrounding whitespace",
			q:    Query{System: "  thismoon \t"},
			want: map[string]bool{
				withCatalog.Name: true,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
		},
		{
			name: "component name differs from repo name",
			q:    Query{Component: "csl"},
			want: map[string]bool{
				withCatalog.Name: true,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
		},
		{
			name: "all set fields must match",
			q:    Query{Name: "mad01", Owner: "platform", System: "shop"},
			want: map[string]bool{
				withCatalog.Name: false,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
		},
		{
			name: "a permissive pattern never matches a repo without a descriptor",
			q:    Query{Owner: "."},
			// As a substring "." is a literal dot, which no owner here has; as
			// a regex it is any character, which every declared owner has.
			want: map[string]bool{
				withCatalog.Name: false,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
			regex: map[string]bool{
				withCatalog.Name: true,
				noCatalog.Name:   false,
				noSystem.Name:    true,
			},
		},
		{
			name: "empty system field does not match a repo that declares none",
			q:    Query{System: "a"},
			want: map[string]bool{
				withCatalog.Name: false,
				noCatalog.Name:   false,
				noSystem.Name:    false,
			},
		},
	}
	repos := []Repo{withCatalog, noCatalog, noSystem}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := tc.q.SubstringMatcher()
			if err != nil {
				t.Fatalf("SubstringMatcher: %v", err)
			}
			re, err := tc.q.RegexMatcher()
			if err != nil {
				t.Fatalf("RegexMatcher: %v", err)
			}
			for _, r := range repos {
				if got := sub(r); got != tc.want[r.Name] {
					t.Errorf("substring %v on %s = %v, want %v", tc.q, r.Name, got, tc.want[r.Name])
				}
				wantRe := tc.want[r.Name]
				if tc.regex != nil {
					wantRe = tc.regex[r.Name]
				}
				if got := re(r); got != wantRe {
					t.Errorf("regex %v on %s = %v, want %v", tc.q, r.Name, got, wantRe)
				}
			}
		})
	}
}

func TestQueryRejectsEmptyAndInvalid(t *testing.T) {
	for _, q := range []Query{{}, {Name: "  "}, {Owner: "\t\n"}} {
		if !q.IsZero() {
			t.Errorf("%+v.IsZero() = false, want true", q)
		}
		if _, err := q.SubstringMatcher(); err == nil {
			t.Errorf("%+v.SubstringMatcher() error = nil, want one", q)
		}
		if _, err := q.RegexMatcher(); err == nil {
			t.Errorf("%+v.RegexMatcher() error = nil, want one", q)
		}
	}
	_, err := (Query{Owner: "("}).RegexMatcher()
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Errorf("RegexMatcher with a bad owner pattern = %v, want an error naming owner", err)
	}
	if _, err := (Query{Owner: "("}).SubstringMatcher(); err != nil {
		t.Errorf("SubstringMatcher treats a paren literally, got %v", err)
	}
}

func TestQueryString(t *testing.T) {
	tests := []struct {
		q    Query
		want string
	}{
		{Query{Name: " x ", System: "y"}, "name=x system=y"},
		{Query{Owner: "o"}, "owner=o"},
		// A bare name stays bare: the `csl repo <query>` error text predates
		// catalog fields and scripts may match on it.
		{Query{Name: " nosuchrepo "}, "nosuchrepo"},
		{Query{}, ""},
	}
	for _, tc := range tests {
		if got := tc.q.String(); got != tc.want {
			t.Errorf("%+v.String() = %q, want %q", tc.q, got, tc.want)
		}
	}
}
