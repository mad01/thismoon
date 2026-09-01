package goscan

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// ScanSource extracts prose from in-memory Go source. filename is used for
// the reported location and does not have to exist on disk, which is what
// makes the extractor usable from an MCP tool handed a text buffer.
// Blocks come back in source order.
func ScanSource(filename, src string, opts Options) ([]Block, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(
		fset,
		filename,
		src,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		return nil, fmt.Errorf("goscan: parse %s: %w", filename, err)
	}
	e := &extractor{fset: fset, file: filename, opts: opts, seen: map[token.Pos]bool{}}
	e.run(f)
	return e.blocks, nil
}

// extractor accumulates the blocks of one parsed file.
type extractor struct {
	fset   *token.FileSet
	file   string
	opts   Options
	seen   map[token.Pos]bool
	blocks []Block
}

func (e *extractor) run(f *ast.File) {
	e.doc(f.Doc, "package "+f.Name.Name)
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GenDecl:
			e.doc(node.Doc, genDeclLabel(node))
		case *ast.FuncDecl:
			e.doc(node.Doc, "func "+node.Name.Name)
		case *ast.TypeSpec:
			e.doc(node.Doc, "type "+node.Name.Name)
		case *ast.ValueSpec:
			e.doc(node.Doc, valueSpecLabel(node))
		case *ast.Field:
			e.doc(node.Doc, fieldName(node))
			e.tag(node)
		case *ast.CompositeLit:
			e.composite(node)
		case *ast.CallExpr:
			e.call(node)
		}
		return true
	})
	slices.SortStableFunc(e.blocks, func(a, b Block) int { return cmp.Compare(a.Line, b.Line) })
}

// doc records a documentation comment. The same comment group reaches the
// visitor through more than one node (a declaration and its single spec),
// so groups are emitted once.
func (e *extractor) doc(cg *ast.CommentGroup, label string) {
	if cg == nil || e.seen[cg.Pos()] {
		return
	}
	e.seen[cg.Pos()] = true
	e.add(KindDoc, cg.Pos(), label, cg.Text())
}

// composite records the string fields of a struct literal that hold prose:
// the cobra command fields and the Description an MCP tool definition
// carries.
func (e *extractor) composite(lit *ast.CompositeLit) {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		kind, ok := fieldKind(key.Name)
		if !ok {
			continue
		}
		text, ok := stringValue(kv.Value)
		if !ok {
			continue
		}
		e.add(kind, kv.Value.Pos(), key.Name, text)
	}
}

// call records the message argument of the error constructors.
func (e *extractor) call(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	arg, ok := errorArg(pkg.Name, sel.Sel.Name)
	if !ok || arg >= len(call.Args) {
		return
	}
	text, ok := stringValue(call.Args[arg])
	if !ok {
		return
	}
	e.add(KindError, call.Args[arg].Pos(), pkg.Name+"."+sel.Sel.Name, text)
}

// tag records the jsonschema tag of a struct field: the description an MCP
// client shows for that input.
func (e *extractor) tag(f *ast.Field) {
	if f.Tag == nil {
		return
	}
	raw, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return
	}
	text, ok := reflect.StructTag(raw).Lookup("jsonschema")
	if !ok {
		return
	}
	e.add(KindSchema, f.Tag.Pos(), fieldName(f), text)
}

// add appends a block, dropping the kinds the caller filtered out and any
// text that is blank once trimmed.
func (e *extractor) add(kind Kind, pos token.Pos, label, text string) {
	if !e.opts.wants(kind) {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	e.blocks = append(e.blocks, Block{
		File:  e.file,
		Line:  e.fset.Position(pos).Line,
		Kind:  kind,
		Label: label,
		Text:  text,
	})
}

// fieldKind maps a struct-literal field name to the kind of prose it holds.
func fieldKind(name string) (Kind, bool) {
	switch name {
	case "Use", "Short", "Long", "Example":
		return KindCobra, true
	case "Description":
		return KindMCP, true
	}
	return "", false
}

// errorArg returns the index of the message argument of an error
// constructor, reporting whether pkg.fn is one.
func errorArg(pkg, fn string) (int, bool) {
	switch {
	case pkg == "fmt" && fn == "Errorf", pkg == "errors" && fn == "New":
		return 0, true
	case pkg == "http" && fn == "Error":
		return 1, true
	}
	return 0, false
}

// stringValue resolves a string-literal expression, following the "+"
// concatenation long descriptions are written with. Anything that is not a
// literal (a variable, a function call) reports false.
func stringValue(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, ok := stringValue(v.X)
		if !ok {
			return "", false
		}
		right, ok := stringValue(v.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	case *ast.ParenExpr:
		return stringValue(v.X)
	}
	return "", false
}

// genDeclLabel names a declaration group by its keyword and first name.
func genDeclLabel(d *ast.GenDecl) string {
	if len(d.Specs) == 0 {
		return d.Tok.String()
	}
	switch s := d.Specs[0].(type) {
	case *ast.TypeSpec:
		return "type " + s.Name.Name
	case *ast.ValueSpec:
		if len(s.Names) > 0 {
			return d.Tok.String() + " " + s.Names[0].Name
		}
	}
	return d.Tok.String()
}

// valueSpecLabel names a var or const spec by its first name.
func valueSpecLabel(s *ast.ValueSpec) string {
	if len(s.Names) == 0 {
		return ""
	}
	return s.Names[0].Name
}

// fieldName returns a struct field's name, empty for an embedded field.
func fieldName(f *ast.Field) string {
	if len(f.Names) == 0 {
		return ""
	}
	return f.Names[0].Name
}
