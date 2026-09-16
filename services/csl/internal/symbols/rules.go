package symbols

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// zoekt's kind vocabulary (internal/ctags/symbol_kind.go in zoekt). A kind
// outside it still indexes but earns no ranking boost, so every rule below
// uses one of these.
const (
	kindFunction   = "function"
	kindMethod     = "method"
	kindMethodSpec = "methodSpec"
	kindStruct     = "struct"
	kindInterface  = "interface"
	kindType       = "type"
	kindTypeAlias  = "typealias"
	kindConst      = "const"
	kindVar        = "var"
	kindField      = "field"
	kindClass      = "class"
	kindEnum       = "enum"
	kindEnumerator = "enumerator"
	kindNamespace  = "namespace"
	kindSection    = "section"
)

// rule maps one tree-sitter node type to the symbol it defines.
type rule struct {
	// kind is the zoekt kind, or empty when kindOf decides per node.
	kind string
	// kindOf picks the kind from the node itself and the enclosing scope:
	// a Go type_spec is a struct, an interface, or a plain type; a Python
	// def inside a class is a method.
	kindOf func(n *sitter.Node, src []byte, parent *scope) string
	// scopeOf overrides the lexical parent, for declarations whose parent is
	// written on the node rather than around it (a Go method's receiver).
	scopeOf func(n *sitter.Node, src []byte) *scope
	// nameField is the field holding the name node(s); several children may
	// carry it (`const A, B = 1, 2`).
	nameField string
	// nameType is the child node type holding the name, for grammars without
	// a name field (protobuf's message_name).
	nameType string
	// self marks the node itself as the name (TypeScript enum members are
	// bare property_identifier children of enum_body).
	self bool
	// under restricts the rule to nodes whose direct parent has one of these
	// types, for node types that also appear in non-definition positions.
	under []string
	// container walks the node's children with this symbol as their parent.
	container bool
}

// appliesUnder reports whether the rule fires for a node whose parent is of
// type parentType.
func (r rule) appliesUnder(parentType string) bool {
	if len(r.under) == 0 {
		return true
	}
	for _, u := range r.under {
		if u == parentType {
			return true
		}
	}
	return false
}

// spec is one language's rules keyed by node type.
type spec map[string]rule

// specs holds the rules per language tag. A tag mapped to an empty spec is
// linked but not yet extracted from; a tag missing here has no grammar.
var specs = map[string]spec{
	"go": {
		"function_declaration": {kind: kindFunction, nameField: "name"},
		"method_declaration":   {kind: kindMethod, nameField: "name", scopeOf: goReceiver},
		"type_spec":            {kindOf: goTypeKind, nameField: "name", container: true},
		"type_alias":           {kind: kindTypeAlias, nameField: "name"},
		"const_spec":           {kind: kindConst, nameField: "name"},
		"var_spec":             {kind: kindVar, nameField: "name"},
		"field_declaration":    {kind: kindField, nameField: "name"},
		"method_elem":          {kind: kindMethodSpec, nameField: "name"},
	},
	"typescript": {
		"function_declaration":           {kind: kindFunction, nameField: "name"},
		"generator_function_declaration": {kind: kindFunction, nameField: "name"},
		"class_declaration":              {kind: kindClass, nameField: "name", container: true},
		"abstract_class_declaration":     {kind: kindClass, nameField: "name", container: true},
		"method_definition":              {kind: kindMethod, nameField: "name"},
		"abstract_method_signature":      {kind: kindMethod, nameField: "name"},
		"method_signature":               {kind: kindMethod, nameField: "name"},
		"public_field_definition":        {kind: kindField, nameField: "name"},
		"property_signature":             {kind: kindField, nameField: "name"},
		"interface_declaration":          {kind: kindInterface, nameField: "name", container: true},
		"type_alias_declaration":         {kind: kindTypeAlias, nameField: "name"},
		"enum_declaration":               {kind: kindEnum, nameField: "name", container: true},
		"enum_assignment": {
			kind:      kindEnumerator,
			nameField: "name",
			under:     []string{"enum_body"},
		},
		"property_identifier": {
			kind:  kindEnumerator,
			self:  true,
			under: []string{"enum_body"},
		},
		"variable_declarator": {kindOf: tsDeclaratorKind, nameField: "name"},
		"internal_module":     {kind: kindNamespace, nameField: "name", container: true},
	},
	"python": {
		"function_definition": {kindOf: pyFunctionKind, nameField: "name", container: true},
		"class_definition":    {kind: kindClass, nameField: "name", container: true},
	},
	"java": {
		"class_declaration": {kind: kindClass, nameField: "name", container: true},
		"interface_declaration": {
			kind:      kindInterface,
			nameField: "name",
			container: true,
		},
		"annotation_type_declaration": {
			kind:      kindInterface,
			nameField: "name",
			container: true,
		},
		"enum_declaration":                {kind: kindEnum, nameField: "name", container: true},
		"record_declaration":              {kind: kindClass, nameField: "name", container: true},
		"method_declaration":              {kind: kindMethod, nameField: "name"},
		"constructor_declaration":         {kind: kindMethod, nameField: "name"},
		"compact_constructor_declaration": {kind: kindMethod, nameField: "name"},
		"enum_constant":                   {kind: kindEnumerator, nameField: "name"},
		"variable_declarator": {
			kind:      kindField,
			nameField: "name",
			under:     []string{"field_declaration", "constant_declaration"},
		},
	},
	"protobuf": {
		"message":    {kind: kindStruct, nameType: "message_name", container: true},
		"enum":       {kind: kindEnum, nameType: "enum_name", container: true},
		"enum_field": {kind: kindEnumerator, nameType: "identifier"},
		"field":      {kind: kindField, nameType: "identifier"},
		"service":    {kind: kindInterface, nameType: "service_name", container: true},
		"rpc":        {kind: kindMethod, nameType: "rpc_name"},
	},
	"markdown": {
		"atx_heading":    {kind: kindSection, nameField: "heading_content"},
		"setext_heading": {kind: kindSection, nameField: "heading_content"},
	},
	"bash": {
		"function_definition": {kind: kindFunction, nameField: "name"},
	},
	// Linked grammars with no definition rules yet: files parse to nothing
	// and index without symbols.
	"hcl":        {},
	"sql":        {},
	"yaml":       {},
	"dockerfile": {},
}

// goTypeKind classifies a type_spec by its type node.
func goTypeKind(n *sitter.Node, _ []byte, _ *scope) string {
	t := n.ChildByFieldName("type")
	if t == nil {
		return kindType
	}
	switch t.Type() {
	case "struct_type":
		return kindStruct
	case "interface_type":
		return kindInterface
	default:
		return kindType
	}
}

// goReceiver resolves a method's parent from its receiver, unwrapping
// pointer and generic types down to the type identifier. The receiver's own
// declaration is not resolved, so its kind is the generic "type".
func goReceiver(n *sitter.Node, src []byte) *scope {
	recv := n.ChildByFieldName("receiver")
	if recv == nil || recv.NamedChildCount() == 0 {
		return nil
	}
	t := recv.NamedChild(0).ChildByFieldName("type")
	for t != nil {
		switch t.Type() {
		case "pointer_type":
			t = t.NamedChild(0)
		case "generic_type":
			t = t.ChildByFieldName("type")
		default:
			return &scope{name: string(src[t.StartByte():t.EndByte()]), kind: kindType}
		}
	}
	return nil
}

// tsDeclaratorKind classifies a variable_declarator: a function when its
// value is a function expression (the dominant TypeScript style), otherwise
// const or var by the declaration keyword.
func tsDeclaratorKind(n *sitter.Node, src []byte, _ *scope) string {
	if v := n.ChildByFieldName("value"); v != nil {
		switch v.Type() {
		case "arrow_function", "function_expression", "generator_function":
			return kindFunction
		}
	}
	if decl := n.Parent(); decl != nil {
		if k := decl.ChildByFieldName("kind"); k != nil &&
			string(src[k.StartByte():k.EndByte()]) == "const" {
			return kindConst
		}
	}
	return kindVar
}

// pyFunctionKind makes a def inside a class a method.
func pyFunctionKind(_ *sitter.Node, _ []byte, parent *scope) string {
	if parent != nil && parent.kind == kindClass {
		return kindMethod
	}
	return kindFunction
}
