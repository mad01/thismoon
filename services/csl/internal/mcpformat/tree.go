package mcpformat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// kind tells which of a node's fields carries its value.
type kind int

const (
	kindNull kind = iota
	kindBool
	kindNumber
	kindString
	kindArray
	kindObject
)

// node is one value in the ordered tree Encode builds from a marshaled value.
// Object fields keep the order json.Marshal emitted them in, which is struct
// field order with omitempty already applied, so every encoder rendering
// from the tree follows the struct rather than the alphabet.
type node struct {
	kind    kind
	boolean bool
	number  json.Number
	str     string
	items   []*node // kindArray
	fields  []field // kindObject
}

// field is one key/value pair of an object node.
type field struct {
	key   string
	value *node
}

// parseTree decodes data, a single JSON value, into an ordered tree.
func parseTree(data []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	root, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("mcpformat: trailing data after the JSON value")
	}
	return root, nil
}

func parseValue(dec *json.Decoder) (*node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("mcpformat: parse json: %w", err)
	}
	switch t := tok.(type) {
	case nil:
		return &node{kind: kindNull}, nil
	case bool:
		return &node{kind: kindBool, boolean: t}, nil
	case json.Number:
		return &node{kind: kindNumber, number: t}, nil
	case string:
		return &node{kind: kindString, str: t}, nil
	case json.Delim:
		switch t {
		case '{':
			return parseObject(dec)
		case '[':
			return parseArray(dec)
		}
	}
	return nil, fmt.Errorf("mcpformat: unexpected JSON token %v", tok)
}

func parseObject(dec *json.Decoder) (*node, error) {
	obj := &node{kind: kindObject}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("mcpformat: parse json: %w", err)
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("mcpformat: object key is %T, want string", tok)
		}
		value, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		obj.fields = append(obj.fields, field{key: key, value: value})
	}
	if err := consumeClose(dec); err != nil {
		return nil, err
	}
	return obj, nil
}

func parseArray(dec *json.Decoder) (*node, error) {
	arr := &node{kind: kindArray}
	for dec.More() {
		item, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		arr.items = append(arr.items, item)
	}
	if err := consumeClose(dec); err != nil {
		return nil, err
	}
	return arr, nil
}

// consumeClose reads the closing bracket of the container just parsed.
func consumeClose(dec *json.Decoder) error {
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("mcpformat: parse json: %w", err)
	}
	return nil
}

// isScalar reports whether n is a leaf: null, bool, number, or string.
func (n *node) isScalar() bool {
	return n.kind != kindArray && n.kind != kindObject
}

// isObjectArray reports whether n is a non-empty array whose items are all
// objects, the shape the row-oriented formats lay out as records.
func (n *node) isObjectArray() bool {
	if n.kind != kindArray || len(n.items) == 0 {
		return false
	}
	for _, it := range n.items {
		if it.kind != kindObject {
			return false
		}
	}
	return true
}

// lookup returns the value of the named field of an object node.
func (n *node) lookup(key string) (*node, bool) {
	for _, f := range n.fields {
		if f.key == key {
			return f.value, true
		}
	}
	return nil, false
}

// without returns a copy of an object node minus the field at index i.
func (n *node) without(i int) *node {
	out := &node{kind: kindObject, fields: make([]field, 0, len(n.fields)-1)}
	out.fields = append(out.fields, n.fields[:i]...)
	out.fields = append(out.fields, n.fields[i+1:]...)
	return out
}

// primaryArray finds the first top-level field holding a non-empty array of
// objects. Every other top-level field is meta.
func primaryArray(root *node) (int, bool) {
	if root.kind != kindObject {
		return 0, false
	}
	for i, f := range root.fields {
		if f.value.isObjectArray() {
			return i, true
		}
	}
	return 0, false
}

// scalarText is the bare text of a leaf: bools and numbers as written,
// strings unquoted, null empty.
func (n *node) scalarText() string {
	switch n.kind {
	case kindBool:
		return strconv.FormatBool(n.boolean)
	case kindNumber:
		return n.number.String()
	case kindString:
		return n.str
	}
	return ""
}

// compactJSON re-serializes n as compact JSON, keeping field order.
func (n *node) compactJSON() string {
	var b strings.Builder
	n.writeJSON(&b)
	return b.String()
}

func (n *node) writeJSON(b *strings.Builder) {
	switch n.kind {
	case kindNull:
		b.WriteString("null")
	case kindBool:
		b.WriteString(strconv.FormatBool(n.boolean))
	case kindNumber:
		b.WriteString(n.number.String())
	case kindString:
		b.WriteString(jsonString(n.str))
	case kindArray:
		b.WriteByte('[')
		for i, it := range n.items {
			if i > 0 {
				b.WriteByte(',')
			}
			it.writeJSON(b)
		}
		b.WriteByte(']')
	case kindObject:
		b.WriteByte('{')
		for i, f := range n.fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(jsonString(f.key))
			b.WriteByte(':')
			f.value.writeJSON(b)
		}
		b.WriteByte('}')
	}
}

// jsonString quotes s the way json.Marshal does, so re-serialized strings
// match the JSON format byte for byte.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// Marshaling a string cannot fail: invalid UTF-8 is replaced, not rejected.
		panic("mcpformat: marshal string: " + err.Error())
	}
	return string(b)
}
