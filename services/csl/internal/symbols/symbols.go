// Package symbols extracts definition sites from source files with the
// tree-sitter grammars csl links, in the byte-range shape zoekt's index
// builder takes. Feeding them in at index time is what makes sym: queries
// return definitions and lets zoekt rank a definition file above its call
// sites, without a ctags binary on the machine.
package symbols

import (
	"fmt"
	"sort"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/mad01/thismoon/services/csl/internal/grammar"
)

// Symbol is one definition: the byte range of its name in the file, its kind
// in zoekt's ctags vocabulary, and the enclosing declaration when nested.
type Symbol struct {
	// Start is the byte offset of the name's first byte.
	Start uint32
	// End is the byte offset one past the name's last byte.
	End uint32
	// Kind is one of zoekt's recognised kind strings (function, method,
	// struct, interface, type, const, var, field, class, enum, ...).
	Kind string
	// Parent names the enclosing declaration: a Go receiver type, a class, a
	// protobuf message or service. Empty for top-level symbols.
	Parent string
	// ParentKind is Parent's kind, empty when Parent is.
	ParentKind string
}

// Extract returns the definitions in content, sorted by Start and holding the
// invariants zoekt's shard builder checks: every range lies on rune
// boundaries, inside content, and never overlaps another. Files without a
// grammar, or whose grammar has no rules yet, yield nil. tree-sitter recovers
// from syntax errors, so a malformed file still yields what parsed.
func Extract(path string, content []byte) ([]Symbol, error) {
	lang := grammar.LangForPath(path)
	rules, ok := specs[lang]
	if !ok || len(rules) == 0 {
		return nil, nil
	}
	tree, err := grammar.Parse(lang, content)
	if err != nil {
		return nil, fmt.Errorf("symbols: %s: %w", path, err)
	}
	defer tree.Close()

	w := walker{src: content, rules: rules}
	w.walk(tree.RootNode(), nil)
	return normalize(content, w.out), nil
}

// scope is the declaration the walk is currently inside, handed to nested
// symbols as their parent.
type scope struct {
	name string
	kind string
}

// walker collects symbols from one parse tree under one language's rules.
type walker struct {
	src   []byte
	rules spec
	out   []Symbol
}

// walk visits the named children of n. A child with a rule emits one symbol
// per name it declares; containers are then walked with themselves as the
// parent, other matched nodes are not entered (a function body's locals are
// not definitions). Children without a rule are walked through unchanged, so
// wrappers like Go's type_declaration or TypeScript's export_statement need
// no rule of their own.
func (w *walker) walk(n *sitter.Node, parent *scope) {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(i)
		r, ok := w.rules[child.Type()]
		if !ok || !r.appliesUnder(n.Type()) {
			w.walk(child, parent)
			continue
		}
		names := w.names(child, r)
		kind := r.kind
		if r.kindOf != nil {
			kind = r.kindOf(child, w.src, parent)
		}
		symParent := parent
		if r.scopeOf != nil {
			symParent = r.scopeOf(child, w.src)
		}
		for _, name := range names {
			w.emit(name, kind, symParent)
		}
		if r.container {
			inner := parent
			if len(names) > 0 {
				inner = &scope{name: w.text(names[0]), kind: kind}
			}
			w.walk(child, inner)
		}
	}
}

// emit records one symbol for the name node, trimmed of surrounding
// whitespace (a setext heading's paragraph carries its newline).
func (w *walker) emit(name *sitter.Node, kind string, parent *scope) {
	start, end := trimRange(w.src, name.StartByte(), name.EndByte())
	s := Symbol{Start: start, End: end, Kind: kind}
	if parent != nil {
		s.Parent, s.ParentKind = parent.name, parent.kind
	}
	w.out = append(w.out, s)
}

// names returns the nodes holding the names n declares: every child in the
// rule's name field (Go's `X, Y int` declares two), the first child of the
// rule's name type, or n itself. Destructuring patterns declare no single
// name and are skipped.
func (w *walker) names(n *sitter.Node, r rule) []*sitter.Node {
	if r.self {
		return []*sitter.Node{n}
	}
	var out []*sitter.Node
	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(i)
		if !child.IsNamed() || isPattern(child.Type()) {
			continue
		}
		switch {
		case r.nameField != "" && n.FieldNameForChild(i) == r.nameField:
			out = append(out, child)
		case r.nameType != "" && child.Type() == r.nameType:
			return []*sitter.Node{child}
		}
	}
	return out
}

// text returns the trimmed source text of a node.
func (w *walker) text(n *sitter.Node) string {
	start, end := trimRange(w.src, n.StartByte(), n.EndByte())
	return string(w.src[start:end])
}

// isPattern reports a destructuring pattern node (object_pattern,
// array_pattern), which names several bindings at once.
func isPattern(nodeType string) bool {
	return len(nodeType) > len("_pattern") &&
		nodeType[len(nodeType)-len("_pattern"):] == "_pattern"
}

// trimRange shrinks [start, end) past leading and trailing whitespace.
func trimRange(src []byte, start, end uint32) (uint32, uint32) {
	for start < end && isSpace(src[start]) {
		start++
	}
	for end > start && isSpace(src[end-1]) {
		end--
	}
	return start, end
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// normalize is the one place the zoekt invariants are enforced: symbols are
// sorted by Start, and any that is empty, runs past the content, splits a
// rune, or overlaps the symbol before it is dropped. The builder rejects a
// whole document otherwise, and one odd declaration must not cost a file its
// place in the index.
func normalize(content []byte, syms []Symbol) []Symbol {
	sort.SliceStable(syms, func(i, j int) bool { return syms[i].Start < syms[j].Start })
	out := syms[:0]
	var last uint32
	for _, s := range syms {
		if s.Start >= s.End || int(s.End) > len(content) {
			continue
		}
		if !utf8.RuneStart(content[s.Start]) ||
			(int(s.End) < len(content) && !utf8.RuneStart(content[s.End])) {
			continue
		}
		if len(out) > 0 && s.Start < last {
			continue
		}
		out = append(out, s)
		last = s.End
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
