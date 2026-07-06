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
// Callers use the returned regex against Repo.Name (e.g. "mad01/brain").
func CompileMatcher(q string) (*regexp.Regexp, error) {
	q = NormalizeQuery(q)
	if q == "" {
		return nil, fmt.Errorf("name is required")
	}
	re, err := regexp.Compile("(?i)" + q)
	if err != nil {
		return nil, fmt.Errorf("invalid name pattern %q: %w", q, err)
	}
	return re, nil
}
