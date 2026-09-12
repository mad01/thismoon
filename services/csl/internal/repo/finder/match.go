package finder

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// NormalizeQuery strips leading and trailing Unicode whitespace from q.
//
// Whitespace is defined by unicode.IsSpace, so this covers the common
// offenders that slip into copy-pasted or tab-completed repo names:
// ASCII space and tab, newlines, non-breaking space (U+00A0), narrow
// no-break space (U+202F), and ideographic space (U+3000).
func NormalizeQuery(q string) string {
	return strings.TrimFunc(q, unicode.IsSpace)
}

// CompileMatcher builds a case-insensitive regex matcher for a repo-name
// query. The query is normalized with NormalizeQuery first; an empty query
// after normalization is rejected.
//
// Callers use the returned regex against Repo.Name (e.g. "mad01/octo").
func CompileMatcher(q string) (*regexp.Regexp, error) {
	return compileInsensitive("name", q)
}

// compileInsensitive is CompileMatcher for any field, naming the field in
// its errors so a bad owner pattern is not reported as a bad name.
func compileInsensitive(field, q string) (*regexp.Regexp, error) {
	q = NormalizeQuery(q)
	if q == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	re, err := regexp.Compile("(?i)" + q)
	if err != nil {
		return nil, fmt.Errorf("invalid %s pattern %q: %w", field, q, err)
	}
	return re, nil
}

// Query selects repos by name and by the identity fields of their catalog
// descriptor. An empty field is unconstrained; every set field must match,
// so Query{Owner: "platform", System: "shop"} is the platform team's repos
// in the shop system. Name matches the org/repo name from the git remote;
// the other three match the Component the repo's catalog-info.yaml or
// service-info.yaml declares, and a repo with no descriptor never matches
// them, however permissive the pattern.
type Query struct {
	Name      string // org/repo from the remote, e.g. "mad01/thismoon"
	Component string // catalog metadata.name
	Owner     string // catalog spec.owner
	System    string // catalog spec.system
}

// Matcher reports whether a repo satisfies a Query.
type Matcher func(Repo) bool

// IsZero reports whether no field is set once whitespace is trimmed, which
// is the query that would match everything and that both surfaces reject.
func (q Query) IsZero() bool {
	return q.normalized() == Query{}
}

// String renders the set fields as "name=x owner=y", for error messages that
// have to say what was asked for. A query that is only a name renders as
// the bare name, so `csl repo nosuchrepo` keeps saying "nosuchrepo" the way
// it did before catalog fields existed.
func (q Query) String() string {
	q = q.normalized()
	if q == (Query{Name: q.Name}) {
		return q.Name
	}
	var parts []string
	for _, f := range q.fields() {
		if f.pattern != "" {
			parts = append(parts, f.name+"="+f.pattern)
		}
	}
	return strings.Join(parts, " ")
}

// SubstringMatcher matches each set field as a case-insensitive substring:
// the `csl repo` contract, where the query is typed by hand and a dot means
// a dot. It errors on a zero Query.
func (q Query) SubstringMatcher() (Matcher, error) {
	return q.matcher(func(_, pattern string) (func(string) bool, error) {
		want := strings.ToLower(pattern)
		return func(value string) bool {
			return strings.Contains(strings.ToLower(value), want)
		}, nil
	})
}

// RegexMatcher matches each set field as a case-insensitive regex: the MCP
// contract, where a caller can write "mad01/.*" and expects it to mean that.
// It errors on a zero Query and on a pattern that does not compile.
func (q Query) RegexMatcher() (Matcher, error) {
	return q.matcher(func(field, pattern string) (func(string) bool, error) {
		re, err := compileInsensitive(field, pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	})
}

// field is one Query field: its name for messages, the pattern the caller
// set, and how to read the value it constrains from a repo. present is false
// when the repo cannot carry the value at all (a catalog field on a repo
// with no descriptor), which is a non-match rather than an empty string to
// test, so a pattern like ".*" cannot match a repo that declares nothing.
type field struct {
	name    string
	pattern string
	get     func(Repo) (value string, present bool)
}

func (q Query) fields() []field {
	catalog := func(pick func(r Repo) string) func(Repo) (string, bool) {
		return func(r Repo) (string, bool) {
			if r.Catalog == nil {
				return "", false
			}
			return pick(r), true
		}
	}
	return []field{
		{"name", q.Name, func(r Repo) (string, bool) { return r.Name, true }},
		{"component", q.Component, catalog(func(r Repo) string { return r.Catalog.Name })},
		{"owner", q.Owner, catalog(func(r Repo) string { return r.Catalog.Owner })},
		{"system", q.System, catalog(func(r Repo) string { return r.Catalog.System })},
	}
}

func (q Query) normalized() Query {
	return Query{
		Name:      NormalizeQuery(q.Name),
		Component: NormalizeQuery(q.Component),
		Owner:     NormalizeQuery(q.Owner),
		System:    NormalizeQuery(q.System),
	}
}

// matcher compiles every set field with compile and ANDs the results.
func (q Query) matcher(
	compile func(field, pattern string) (func(string) bool, error),
) (Matcher, error) {
	q = q.normalized()
	if q.IsZero() {
		return nil, fmt.Errorf("a name, component, owner, or system query is required")
	}
	type test struct {
		match func(string) bool
		get   func(Repo) (string, bool)
	}
	var tests []test
	for _, f := range q.fields() {
		if f.pattern == "" {
			continue
		}
		match, err := compile(f.name, f.pattern)
		if err != nil {
			return nil, err
		}
		tests = append(tests, test{match: match, get: f.get})
	}
	return func(r Repo) bool {
		for _, t := range tests {
			value, present := t.get(r)
			if !present || !t.match(value) {
				return false
			}
		}
		return true
	}, nil
}
