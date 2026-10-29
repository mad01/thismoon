package lexical

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sourcegraph/zoekt/query"
)

// Query-shape analysis for the search front ends: the malformed shapes that
// can never search, and the split of a query into its top-level AND terms and
// the fixed atoms around them. Everything here is text work over zoekt's own
// lexer rules and parser; nothing touches the index.

// Malformed names a query shape zoekt would reject, or one that can only
// have been sent by mistake, together with its fix. A search front end
// returns it as the error instead of running the search; a validator reports
// it as a diagnosis.
type Malformed struct {
	Problem string
	Fix     string
}

func (m *Malformed) Error() string { return m.Problem + "; " + m.Fix }

// CheckShape returns the malformed shape of a query, or nil when the query is
// worth sending to zoekt. The shapes: empty, made only of quote characters,
// an unbalanced double quote, a trailing backslash, and an empty "" phrase.
func CheckShape(q string) *Malformed {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return &Malformed{
			Problem: "query is empty",
			Fix:     "put the text to search for in query",
		}
	}
	if strings.Trim(trimmed, `"`) == "" {
		return &Malformed{
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
			return &Malformed{
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
func splitQuery(q string) ([]string, *Malformed) {
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
				return nil, &Malformed{
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
		return nil, &Malformed{
			Problem: "query has an unbalanced double quote",
			Fix:     `close the phrase ("exact phrase") or remove the stray quote`,
		}
	}
	flush()
	return tokens, nil
}

// Parts is a query split into the top-level AND terms zoekt requires in one
// file and the fixed atoms around them: filters (repo:, f:, lang:, sym:,
// case:, ...) and negations, which narrow rather than require and so are
// never counted or dropped.
type Parts struct {
	Terms  []string
	Fixed  []string
	tokens []string
	isTerm []bool
}

// SplitTerms classifies each top-level token by parsing it alone with zoekt's
// parser: a content substring or regexp (including a "quoted phrase", an a|b
// alternation, and a parenthesised group) is a term; everything else is
// fixed. A query the AND model does not describe, one with the or keyword at
// the top level, a token zoekt rejects, or a malformed shape, yields empty
// parts, so callers skip the diagnosis rather than misreport it.
func SplitTerms(q string) Parts {
	tokens, m := splitQuery(q)
	if m != nil {
		return Parts{}
	}
	parts := Parts{tokens: tokens, isTerm: make([]bool, len(tokens))}
	for i, tok := range tokens {
		term, ok := classifyToken(tok)
		if !ok {
			return Parts{}
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
func (p Parts) without(drop []string) string {
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

// quoteList renders terms as 'a', 'b' for notes and the relaxed line.
func quoteList(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = "'" + t + "'"
	}
	return strings.Join(quoted, ", ")
}
