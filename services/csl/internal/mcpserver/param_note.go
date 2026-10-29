package mcpserver

import (
	"fmt"
	"strings"
)

// searchParamWords are the csl_search parameter names and output_mode values
// an agent sometimes sends as the query text itself. The test beside this
// keeps it in step with searchInput's json tags.
var searchParamWords = map[string]bool{
	"query":              true,
	"repo":               true,
	"lang":               true,
	"file":               true,
	"output_mode":        true,
	"context_lines":      true,
	"limit":              true,
	"offset":             true,
	"case_sensitive":     true,
	"response_format":    true,
	"files_with_matches": true,
	"content":            true,
}

// paramNameNote flags a query made only of csl_search parameter names: the
// agent put its arguments in the query text. It is a note rather than an
// error because content and repo are plausible search terms too, so it
// fires only once the search has come back empty.
func paramNameNote(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	for _, f := range fields {
		if !searchParamWords[f] {
			return ""
		}
	}
	return fmt.Sprintf(
		"%q is made of csl_search parameter names, not search terms; "+
			"pass them as parameters (e.g. output_mode=\"content\") "+
			"and put the text to find in query",
		q,
	)
}
