package mcpformat

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// encodeXML renders the value under a <result> root: one element per field
// named by its key, arrays as one <item> per element, objects nested, scalars
// as escaped text, null and empty containers as an empty element. One element
// per line, no indentation, no XML declaration. Keys are taken as valid
// element names, which the lowercase_underscore json tags here are.
func encodeXML(root *node) (string, error) {
	if root.kind != kindObject {
		return "", errors.New("mcpformat: xml needs an object at the top level")
	}
	var b strings.Builder
	if err := writeXMLElement(&b, "result", root); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeXMLElement(b *strings.Builder, name string, n *node) error {
	if n.isScalar() {
		return writeXMLScalar(b, name, n)
	}
	children := xmlChildren(n)
	if len(children) == 0 {
		b.WriteString("<" + name + "></" + name + ">\n")
		return nil
	}
	b.WriteString("<" + name + ">\n")
	for _, c := range children {
		if err := writeXMLElement(b, c.key, c.value); err != nil {
			return err
		}
	}
	b.WriteString("</" + name + ">\n")
	return nil
}

// xmlChildren lists a container's child elements: object fields under their
// own keys, array elements each as <item>.
func xmlChildren(n *node) []field {
	if n.kind == kindObject {
		return n.fields
	}
	children := make([]field, len(n.items))
	for i, it := range n.items {
		children[i] = field{key: "item", value: it}
	}
	return children
}

// writeXMLScalar writes a leaf element; EscapeText turns newlines and tabs
// into character references, so a multi-line string still fits on one line.
func writeXMLScalar(b *strings.Builder, name string, n *node) error {
	b.WriteString("<" + name + ">")
	if err := xml.EscapeText(b, []byte(n.scalarText())); err != nil {
		return fmt.Errorf("mcpformat: escape xml text: %w", err)
	}
	b.WriteString("</" + name + ">\n")
	return nil
}
