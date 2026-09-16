package mcpserver

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sourcegraph/zoekt/query"
)

// Query-shape analysis for the search tools: the malformed shapes that can
// never search, the split of a query into its top-level AND terms and the
// fixed atoms around them, and the parameter-name trap. Everything here is
// text work over zoekt's own lexer rules and parser; nothing touches the
// index.

// malformedQuery names a query shape zoekt would reject, or one that can only
// have been sent by mistake, together with its fix. csl_search returns it as
// the error instead of running the search; csl_query_validate reports it as a
// valid=false diagnosis.
type malformedQuery struct {
	Problem string
	Fix     string
}

func (m *malformedQuery) Error() string { return m.Problem + "; " + m.Fix }

// checkQueryShape returns the malformed shape of a query, or nil when the
// query is worth sending to zoekt. The shapes: empty, made only of quote
// characters, an unbalanced double quote, a trailing backslash, and an empty
// "" phrase.
func checkQueryShape(q string) *malformedQuery {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return &malformedQuery{
			Problem: "query is empty",
			Fix:     "put the text to search for in query",
		}
	}
	if strings.Trim(trimmed, `"`) == "" {
		return &malformedQuery{
			Problem: fmt.Sprintf("query %q is only quote characters", q),
			Fix: `put the search term itself in query; ` +
				`quotes are only needed around a multi-word phrase, e.g. "retry backoff"`,
		}
	}
	tokens, m := splitQuery(q)
	if m != nil {
		return m
	}
	for _, tok := range tokens {
		if strings.Trim(tok, `"`) == "" {
			return &malformedQuery{
				Problem: `query contains an empty quoted phrase ""`,
				Fix:     "put the phrase text inside the quotes or drop them",
			}
		}
	}
	return nil
}

// splitQuery splits a query into its top-level tokens by zoekt's lexer rules:
// whitespace separates tokens except inside double quotes or parentheses, and
// a backslash escapes the next character. Quotes, parens, and escapes stay in
// the token text, so tokens re-join into a query verbatim. The two shapes the
// lexer rejects, an unterminated quote and a trailing backslash, come back as
// the malformed shape instead of tokens.
func splitQuery(q string) ([]string, *malformedQuery) {
	var (
		tokens  []string
		cur     strings.Builder
		inQuote bool
		depth   int
	)
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(q)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			if i+1 == len(runes) {
				return nil, &malformedQuery{
					Problem: "query ends with a lone backslash",
					Fix:     `double it (\\) to match a literal backslash, or remove it`,
				}
			}
			i++
			cur.WriteRune(r)
			cur.WriteRune(runes[i])
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case inQuote:
			cur.WriteRune(r)
		case r == '(':
			depth++
			cur.WriteRune(r)
		case r == ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case unicode.IsSpace(r) && depth == 0:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, &malformedQuery{
			Problem: "query has an unbalanced double quote",
			Fix:     `close the phrase ("exact phrase") or remove the stray quote`,
		}
	}
	flush()
	return tokens, nil
}

// queryParts is a query split into the top-level AND terms zoekt requires in
// one file and the fixed atoms around them: filters (repo:, f:, lang:, sym:,
// case:, ...) and negations, which narrow rather than require and so are
// never counted or dropped.
type queryParts struct {
	Terms  []string
	Fixed  []string
	tokens []string
	isTerm []bool
}

// splitTerms classifies each top-level token by parsing it alone with zoekt's
// parser: a content substring or regexp (including a "quoted phrase", an a|b
// alternation, and a parenthesised group) is a term; everything else is
// fixed. A query the AND model does not describe, one with the or keyword at
// the top level, a token zoekt rejects, or a malformed shape, yields empty
// parts, so callers skip the diagnosis rather than misreport it.
func splitTerms(q string) queryParts {
	tokens, m := splitQuery(q)
	if m != nil {
		return queryParts{}
	}
	parts := queryParts{tokens: tokens, isTerm: make([]bool, len(tokens))}
	for i, tok := range tokens {
		term, ok := classifyToken(tok)
		if !ok {
			return queryParts{}
		}
		parts.isTerm[i] = term
		if term {
			parts.Terms = append(parts.Terms, tok)
		} else {
			parts.Fixed = append(parts.Fixed, tok)
		}
	}
	return parts
}

// classifyToken reports whether a token is an AND term or a fixed atom; ok is
// false for a token the AND model cannot place.
func classifyToken(tok string) (term, ok bool) {
	if tok == "or" {
		return false, false
	}
	q, err := query.Parse(tok)
	if err != nil {
		return false, false
	}
	switch n := q.(type) {
	case *query.Substring:
		return !n.FileName, true
	case *query.Regexp:
		return !n.FileName, true
	case *query.And, *query.Or:
		return true, true
	default:
		return false, true
	}
}

// without returns the query with the given terms removed: the remaining
// tokens in their original order, joined by single spaces.
func (p queryParts) without(drop []string) string {
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}
	kept := make([]string, 0, len(p.tokens))
	for i, tok := range p.tokens {
		if p.isTerm[i] && skip[tok] {
			continue
		}
		kept = append(kept, tok)
	}
	return strings.Join(kept, " ")
}

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

// quoteList renders terms as 'a', 'b' for notes and the relaxed line.
func quoteList(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = "'" + t + "'"
	}
	return strings.Join(quoted, ", ")
}
