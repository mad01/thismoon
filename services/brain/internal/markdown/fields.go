package markdown

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// typedFields maps each YAML key a typed Frontmatter field owns to that
// field's index, read from the struct tags so the tags stay the one list.
var typedFields = fieldIndexByKey(reflect.TypeFor[Frontmatter]())

func fieldIndexByKey(t reflect.Type) map[string]int {
	keys := make(map[string]int, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			keys[name] = i
		}
	}
	return keys
}

// fieldsOnly is Frontmatter without its YAML methods, so the typed fields
// encode and decode with the default struct rules.
type fieldsOnly Frontmatter

// UnmarshalYAML decodes the typed keys into their fields and keeps every
// other key in Extra, recording the source key order for Render.
func (f *Frontmatter) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("frontmatter must be a YAML mapping, got %s", n.ShortTag())
	}
	typed := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	var parsed Frontmatter
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		parsed.order = append(parsed.order, key.Value)
		if _, ok := typedFields[key.Value]; ok {
			typed.Content = append(typed.Content, key, value)
			continue
		}
		parsed.setExtra(key.Value, value)
	}
	fields := fieldsOnly(parsed)
	if err := typed.Decode(&fields); err != nil {
		return err
	}
	*f = Frontmatter(fields)
	return nil
}

// MarshalYAML writes the keys in source order: typed fields from their
// current values (empty ones are dropped) and extras from Extra. Keys added
// since parsing follow, typed fields in struct order, then extras in the
// order they were set.
func (f Frontmatter) MarshalYAML() (any, error) {
	fields, err := f.fields()
	if err != nil {
		return nil, err
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, field := range fields {
		out.Content = append(out.Content, keyNode(field.Key), field.Value)
	}
	return out, nil
}

// fields returns the keys MarshalYAML writes, in its order, each with its
// value node.
func (f Frontmatter) fields() ([]Field, error) {
	var typed yaml.Node
	if err := typed.Encode(fieldsOnly(f)); err != nil {
		return nil, err
	}
	typedValues := make(map[string]*yaml.Node, len(typed.Content)/2)
	for i := 0; i+1 < len(typed.Content); i += 2 {
		typedValues[typed.Content[i].Value] = typed.Content[i+1]
	}

	out := make([]Field, 0, len(typedValues)+len(f.Extra))
	emitted := make(map[string]bool, len(typedValues)+len(f.Extra))
	emit := func(key string, value *yaml.Node) {
		if value == nil || emitted[key] {
			return
		}
		emitted[key] = true
		out = append(out, Field{Key: key, Value: value})
	}
	for _, key := range f.order {
		if _, ok := typedFields[key]; ok {
			emit(key, typedValues[key])
		} else {
			emit(key, f.extraValue(key))
		}
	}
	for i := 0; i+1 < len(typed.Content); i += 2 {
		emit(typed.Content[i].Value, typed.Content[i+1])
	}
	for _, e := range f.Extra {
		emit(e.Key, e.Value)
	}
	return out, nil
}

// Keys returns the keys fm holds a value for, in the order Render writes
// them.
func (f Frontmatter) Keys() []string {
	fields, err := f.fields()
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		keys = append(keys, field.Key)
	}
	return keys
}

// Set assigns a YAML value to key. A typed key decodes into its field and
// fails when the value has the wrong shape (a list for title, say); any
// other key goes to Extra, replacing an earlier value. A null value unsets.
func (f *Frontmatter) Set(key string, value *yaml.Node) error {
	if key == "" {
		return errors.New("frontmatter key is empty")
	}
	if value == nil || value.ShortTag() == "!!null" {
		f.Unset(key)
		return nil
	}
	i, ok := typedFields[key]
	if !ok {
		f.setExtra(key, value)
		return nil
	}
	field := reflect.ValueOf(f).Elem().Field(i)
	decoded := reflect.New(field.Type())
	if err := value.Decode(decoded.Interface()); err != nil {
		var typeErr *yaml.TypeError
		if errors.As(err, &typeErr) {
			return fmt.Errorf(
				"frontmatter %s takes %s, got %s",
				key,
				shapeOf(field.Kind()),
				shapeOfNode(value),
			)
		}
		return fmt.Errorf("frontmatter %s: %w", key, err)
	}
	field.Set(decoded.Elem())
	return nil
}

// shapeOf names the YAML shape a typed field accepts, for error messages.
func shapeOf(k reflect.Kind) string {
	if k == reflect.Slice {
		return "a list of strings"
	}
	return "a string"
}

func shapeOfNode(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a mapping"
	}
	return "a scalar"
}

// Unset removes key: a typed field returns to its zero value and an extra
// key is dropped. Unsetting an absent key is a no-op.
func (f *Frontmatter) Unset(key string) {
	if i, ok := typedFields[key]; ok {
		reflect.ValueOf(f).Elem().Field(i).SetZero()
		return
	}
	kept := make([]Field, 0, len(f.Extra))
	for _, e := range f.Extra {
		if e.Key != key {
			kept = append(kept, e)
		}
	}
	f.Extra = kept
}

// ExtraValues returns Extra as plain Go values keyed by name, ready for JSON
// encoding. Timestamps keep their source text.
func (f Frontmatter) ExtraValues() (map[string]any, error) {
	if len(f.Extra) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(f.Extra))
	for _, e := range f.Extra {
		v, err := plainValue(e.Value)
		if err != nil {
			return nil, fmt.Errorf("frontmatter %s: %w", e.Key, err)
		}
		out[e.Key] = v
	}
	return out, nil
}

// setExtra stores value under key in Extra. It copies Extra first, so a
// Frontmatter copied by value never sees the change.
func (f *Frontmatter) setExtra(key string, value *yaml.Node) {
	f.Extra = slices.Clone(f.Extra)
	for i := range f.Extra {
		if f.Extra[i].Key == key {
			f.Extra[i].Value = value
			return
		}
	}
	f.Extra = append(f.Extra, Field{Key: key, Value: value})
}

func (f Frontmatter) extraValue(key string) *yaml.Node {
	for _, e := range f.Extra {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func keyNode(key string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
}

// plainValue converts a YAML node to the maps, slices and scalars
// encoding/json accepts. Mapping keys become strings whatever their YAML type.
func plainValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return plainValue(n.Content[0])
	case yaml.AliasNode:
		return plainValue(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := plainValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[n.Content[i].Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			v, err := plainValue(item)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	}
	if n.ShortTag() == "!!timestamp" {
		return n.Value, nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// ParseValue parses s as a single YAML value, the form a CLI --set flag
// takes: "a" is a string, "[a, b]" a list, "{url: x}" a mapping.
func ParseValue(s string) (*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(s))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse YAML value %q: %w", s, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 ||
		!errors.Is(dec.Decode(&next), io.EOF) {
		return nil, fmt.Errorf("parse YAML value %q: want exactly one value", s)
	}
	return doc.Content[0], nil
}
