// Package match is the filter matcher: does a card fit what the user typed.
// A query is split on white space into terms, and every term must match
// (AND). A term matches when its characters appear in the card's match text
// in order, not necessarily next to each other (a subsequence), so "lgn"
// finds "login". Case is smart: the match ignores case unless the query
// holds an uppercase letter.
//
// A card may carry a scope, the name of a set such as "ignored". The query's
// first word selects scopes when it holds a colon and the part before the
// first colon is a prefix of a scope that some card has: then only cards in
// the scopes it is a prefix of show, and the rest of the query, the rest of
// that word included, filters within them. Otherwise the word is plain
// filter text, so "docs:" matches titles while no scope starts with "docs".
// With no scope selected, the default set shows: the cards with no scope.
package match

import (
	"iter"
	"slices"
	"strings"
	"unicode"
)

// Query is a parsed filter query. The zero Query shows every card in the
// default set.
type Query struct {
	terms         [][]rune
	caseSensitive bool
	// scopes holds the scopes the query selects, sorted; nil selects the
	// default set.
	scopes []string
}

// Parse turns the typed query q into a Query. present yields the scopes the
// cards have, all of them and not only the shown ones, since only those can
// be selected; a scope may come more than once, and nil means none.
func Parse(q string, present iter.Seq[string]) Query {
	scopes, rest := selectScopes(q, present)
	sensitive := strings.ContainsFunc(rest, unicode.IsUpper)
	if !sensitive {
		rest = strings.ToLower(rest)
	}
	fields := strings.Fields(rest)
	terms := make([][]rune, len(fields))
	for i, f := range fields {
		terms[i] = []rune(f)
	}
	return Query{terms: terms, caseSensitive: sensitive, scopes: scopes}
}

// selectScopes returns the present scopes that the prefix before the first
// colon in q's first word starts, sorted and each once, and q with that
// prefix and its colon cut off. When the prefix is empty or starts no
// present scope, it returns nil and q as it is.
func selectScopes(q string, present iter.Seq[string]) (scopes []string, rest string) {
	if present == nil {
		return nil, q
	}
	words := strings.TrimLeftFunc(q, unicode.IsSpace)
	first := words
	if end := strings.IndexFunc(words, unicode.IsSpace); end >= 0 {
		first = words[:end]
	}
	prefix, _, found := strings.Cut(first, ":")
	if !found || prefix == "" {
		return nil, q
	}
	for s := range present {
		if strings.HasPrefix(s, prefix) && !slices.Contains(scopes, s) {
			scopes = append(scopes, s)
		}
	}
	if scopes == nil {
		return nil, q
	}
	slices.Sort(scopes)
	return scopes, words[len(prefix)+1:]
}

// Scopes returns the scopes q selects, sorted, or nil when it selects the
// default set.
func (q Query) Scopes() []string { return slices.Clone(q.scopes) }

// Selects reports whether q shows cards in scope at all: scope is one q
// selects, or q selects none and scope is "", the default set.
func (q Query) Selects(scope string) bool {
	if q.scopes == nil {
		return scope == ""
	}
	_, ok := slices.BinarySearch(q.scopes, scope)
	return ok
}

// Match reports whether a card in scope with match text text shows: q
// selects scope, and every term of q is a subsequence of text.
func (q Query) Match(scope, text string) bool {
	if !q.Selects(scope) {
		return false
	}
	if len(q.terms) == 0 {
		return true
	}
	if !q.caseSensitive {
		text = strings.ToLower(text)
	}
	return q.matchTerms(text)
}

// Text is a card's scope and match text prepared once, so a store that runs
// many queries against the same card does not lowercase its text for each
// one. Construct with Prepare.
type Text struct {
	scope, text, lower string
}

// Prepare returns the match text of a card in scope, "" for the default
// set, ready for Query.MatchText.
func Prepare(scope, text string) Text {
	return Text{scope: scope, text: text, lower: strings.ToLower(text)}
}

// MatchText reports whether the card t was prepared from shows, the same as
// Match on its scope and text.
func (q Query) MatchText(t Text) bool {
	switch {
	case !q.Selects(t.scope):
		return false
	case q.caseSensitive:
		return q.matchTerms(t.text)
	}
	return q.matchTerms(t.lower)
}

// matchTerms reports whether every term is a subsequence of text, which is
// already lowercased when q ignores case.
func (q Query) matchTerms(text string) bool {
	for _, term := range q.terms {
		if !subsequence(term, text) {
			return false
		}
	}
	return true
}

// subsequence reports whether term's runes appear in text in order.
func subsequence(term []rune, text string) bool {
	next := 0
	for _, r := range text {
		if next == len(term) {
			break
		}
		if r == term[next] {
			next++
		}
	}
	return next == len(term)
}
