package mcpformat

import (
	"errors"
	"fmt"
	"strings"
)

// encodeMarkdownKV renders the top-level fields as "key: value" lines in
// order: nested objects flattened with dots, primitive arrays joined with
// ", ", empty arrays as "(none)", null spelled out, and a multi-line string
// as "key: |" followed by its lines indented two spaces. Each array-of-objects
// field then follows as a "## field (N)" section whose records use the same
// rules and are separated by one blank line.
func encodeMarkdownKV(root *node) (string, error) {
	if root.kind != kindObject {
		return "", errors.New("mcpformat: markdown-kv needs an object at the top level")
	}
	var b strings.Builder
	for _, f := range root.fields {
		if !f.value.isObjectArray() {
			writeKVField(&b, f.key, f.value)
		}
	}
	for _, f := range root.fields {
		if f.value.isObjectArray() {
			writeKVSection(&b, f)
		}
	}
	return b.String(), nil
}

// writeKVSection writes a "## field (N)" heading and its records, preceded by
// a blank line when anything came before it.
func writeKVSection(b *strings.Builder, f field) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "## %s (%d)\n", f.key, len(f.value.items))
	for i, rec := range f.value.items {
		if i > 0 {
			b.WriteByte('\n')
		}
		for _, rf := range rec.fields {
			writeKVField(b, rf.key, rf.value)
		}
	}
}

// writeKVField writes one field, recursing into non-empty objects with a
// dotted key prefix.
func writeKVField(b *strings.Builder, key string, n *node) {
	switch {
	case n.kind == kindObject && len(n.fields) > 0:
		for _, f := range n.fields {
			writeKVField(b, key+"."+f.key, f.value)
		}
	case n.kind == kindString && strings.Contains(n.str, "\n"):
		b.WriteString(key + ": |\n")
		for _, line := range strings.Split(n.str, "\n") {
			b.WriteString("  " + line + "\n")
		}
	default:
		writeKVLine(b, key, kvText(n))
	}
}

// kvText is a value's inline text: null spelled out, scalars bare, empty
// arrays "(none)", primitive arrays joined with ", ", anything else compact
// JSON.
func kvText(n *node) string {
	switch {
	case n.kind == kindNull:
		return "null"
	case n.isScalar():
		return n.scalarText()
	case n.kind == kindArray && len(n.items) == 0:
		return "(none)"
	case n.kind == kindArray && allScalars(n.items):
		parts := make([]string, len(n.items))
		for i, it := range n.items {
			parts[i] = it.scalarText()
		}
		return strings.Join(parts, ", ")
	}
	return n.compactJSON()
}

func writeKVLine(b *strings.Builder, key, text string) {
	if text == "" {
		b.WriteString(key + ":\n")
		return
	}
	b.WriteString(key + ": " + text + "\n")
}

func allScalars(items []*node) bool {
	for _, it := range items {
		if !it.isScalar() {
			return false
		}
	}
	return true
}
