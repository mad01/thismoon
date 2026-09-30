package search

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"

	"github.com/sourcegraph/zoekt/query"
)

// The query pipeline every csl search runs, shared by SearchWith, CountWith
// and ValidateQuery: parse the zoekt string, split the sym: terms zoekt
// cannot run as written, refuse the sym: shapes that cannot be split, then
// zoekt's own file/content expansion and simplification.

// SymbolShapeError reports a sym: term csl refuses to send to zoekt, with
// the form to write instead. Term is the term as zoekt parsed it.
type SymbolShapeError struct {
	Term string
	Why  string
	Fix  string
}

// problem is the diagnosis without the fix, the text ValidateQuery reports
// as Error while Fix goes to Hint.
func (e *SymbolShapeError) problem() string { return e.Term + " " + e.Why }

func (e *SymbolShapeError) Error() string { return e.problem() + "; " + e.Fix }

// The two shapes refused, each with its fix.
const (
	repeatedAlternationWhy = "repeats an alternation, which zoekt's symbol matcher " +
		"cannot run and csl cannot split"
	repeatedAlternationFix = "write one sym: term per alternative joined with lowercase or: " +
		"(sym:Foo or sym:Bar)"
	nestedPrefixWhy = "has sym: inside the name, so that alternative matches only " +
		"text that literally contains sym:"
	nestedPrefixFix = "write the prefix once, sym:Foo|Bar, " +
		"or one sym: term per name: (sym:Foo or sym:Bar)"
)

// parseQuery parses a zoekt query string into the tree every csl search
// runs. A sym: term whose regexp alternates at the top level becomes one
// sym: term per alternative; a sym: term csl cannot rewrite is refused with
// a *SymbolShapeError.
func parseQuery(pattern string) (query.Q, error) {
	q, err := query.Parse(pattern)
	if err != nil {
		return nil, err
	}
	q = query.Map(q, splitSymbolAlternation)
	if err := refusedSymbol(q); err != nil {
		return nil, err
	}
	q = query.Map(q, query.ExpandFileContent)
	return query.Simplify(q), nil
}

// parseFailure turns a parseQuery error into the error a search caller
// sees: a refused sym: term already carries its fix, any other parse error
// points at csl query.
func parseFailure(err error, pattern string) error {
	var shape *SymbolShapeError
	if errors.As(err, &shape) {
		return err
	}
	return fmt.Errorf(
		"query parse error: %w\n\nHint: run 'csl query %q' to validate your query",
		err,
		pattern,
	)
}

// splitSymbolAlternation is a query.Map step that rewrites a sym: term whose
// regexp alternates at the top level (sym:Foo|Bar, sym:(Foo|Bar)) into one
// sym: term per alternative under an Or, the tree zoekt parses for
// sym:Foo or sym:Bar. zoekt's symbol matcher cannot run the alternation
// itself: it reduces an alternation of literals to an Or of substrings,
// then finds no regexp to match symbol sections with and fails the search
// with "found *index.orMatchTree inside query.Symbol".
func splitSymbolAlternation(q query.Q) query.Q {
	sym, ok := q.(*query.Symbol)
	if !ok {
		return q
	}
	re, ok := sym.Expr.(*query.Regexp)
	if !ok || re.Regexp.Op != syntax.OpAlternate {
		return q
	}
	terms := make([]query.Q, 0, len(re.Regexp.Sub))
	for _, alt := range re.Regexp.Sub {
		terms = append(terms, &query.Symbol{Expr: alternativeTerm(alt, re)})
	}
	return query.NewOr(terms...)
}

// alternativeTerm builds one alternative the way zoekt's parser builds a
// term from that text alone: a literal is a substring, anything else keeps
// its regexp. Case sensitivity carries over from the whole term, except that
// a literal under an explicit (?i) flag stays case-insensitive, the rule
// zoekt applies when it reduces a regexp to substrings.
func alternativeTerm(alt *syntax.Regexp, whole *query.Regexp) query.Q {
	if alt.Op != syntax.OpLiteral {
		return &query.Regexp{
			Regexp:        alt,
			FileName:      whole.FileName,
			Content:       whole.Content,
			CaseSensitive: whole.CaseSensitive,
		}
	}
	return &query.Substring{
		Pattern:       string(alt.Rune),
		FileName:      whole.FileName,
		Content:       whole.Content,
		CaseSensitive: whole.CaseSensitive && alt.Flags&syntax.FoldCase == 0,
	}
}

// refusedSymbol returns the error for the first sym: term csl cannot send
// to zoekt after splitSymbolAlternation ran, or nil when every sym: term
// can run.
func refusedSymbol(q query.Q) error {
	var refused *SymbolShapeError
	query.VisitAtoms(q, func(atom query.Q) {
		sym, ok := atom.(*query.Symbol)
		if !ok || refused != nil {
			return
		}
		if why, fix := symbolProblem(sym); why != "" {
			refused = &SymbolShapeError{Term: sym.String(), Why: why, Fix: fix}
		}
	})
	if refused == nil {
		return nil
	}
	return refused
}

// symbolProblem says why csl refuses a sym: term, or "" when zoekt can run
// it. A sym: prefix inside the name (sym:Foo|sym:Bar is one regexp) can only
// be a mistake. An alternation under the wrappers zoekt looks through when
// it reduces a regexp to substrings (a plus, a capture, a {1,n} repeat)
// hits the same internal error as sym:Foo|Bar but has no top-level
// alternation to split, so sym:(Foo|Bar)+ is refused. Alternations under
// other operators, as in sym:State(Path|File) or sym:(Foo|Bar)?, keep a
// regexp node and run.
func symbolProblem(sym *query.Symbol) (why, fix string) {
	switch expr := sym.Expr.(type) {
	case *query.Substring:
		if strings.Contains(expr.Pattern, "sym:") {
			return nestedPrefixWhy, nestedPrefixFix
		}
	case *query.Regexp:
		if strings.Contains(expr.Regexp.String(), "sym:") {
			return nestedPrefixWhy, nestedPrefixFix
		}
		if wrapsAlternation(expr.Regexp) {
			return repeatedAlternationWhy, repeatedAlternationFix
		}
	}
	return "", ""
}

// wrapsAlternation reports whether r is an alternation under zero or more
// of the wrappers zoekt looks through: a capture, a plus, or a repeat with
// a minimum of one.
func wrapsAlternation(r *syntax.Regexp) bool {
	for r.Op == syntax.OpCapture || r.Op == syntax.OpPlus ||
		(r.Op == syntax.OpRepeat && r.Min == 1) {
		r = r.Sub[0]
	}
	return r.Op == syntax.OpAlternate
}
