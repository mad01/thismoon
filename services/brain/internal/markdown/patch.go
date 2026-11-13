package markdown

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"
)

// source is the frontmatter text a note was parsed from. Render patches it:
// the lines of a key whose value is unchanged stay byte for byte, a changed
// key's lines are replaced, an unset key's lines go, and a new key is added
// before the closing fence. Comments, quoting, indentation and line endings
// of every untouched key survive an edit that way.
type source struct {
	// open and close are the fence lines, byte order mark and line endings
	// included.
	open, close []byte
	// eol is the note's line ending, "\n" or "\r\n".
	eol string
	// lines is the YAML text between the fences, one line per entry, each
	// with its line ending.
	lines [][]byte
	// keys are the top-level keys in source order. patchable is false when
	// the text cannot be patched by line (a flow mapping, two keys on one
	// line, a key repeated); Render then writes the frontmatter fresh.
	keys      []sourceKey
	patchable bool
	// typed holds the typed fields as parsed, extras the plain value of
	// every other key, for telling which keys changed.
	typed  fieldsOnly
	extras map[string]any
}

// sourceKey is one top-level key and the lines its entry spans.
type sourceKey struct {
	name string
	// start and end are the entry's line span [start, end) in lines. The
	// span runs to the next key's line, less the blank and comment lines
	// before it, which belong to the next key or to the block's end.
	start, end int
	// value is the parsed value node, whose style a rewritten typed field
	// keeps.
	value *yaml.Node
}

// newSource records the text of f's frontmatter, its top-level mapping (nil
// for an empty block) and the frontmatter parsed from it.
func newSource(f fences, mapping *yaml.Node, fm Frontmatter) *source {
	s := &source{
		open:      f.open,
		close:     f.close,
		eol:       lineEnding(f.open),
		lines:     splitLines(f.yaml),
		typed:     snapshotTyped(fm),
		extras:    map[string]any{},
		patchable: true,
	}
	for _, e := range fm.Extra {
		if v, err := plainValue(e.Value); err == nil {
			s.extras[e.Key] = v
		}
	}
	if mapping == nil {
		return s
	}
	if mapping.Kind != yaml.MappingNode || mapping.Style&yaml.FlowStyle != 0 {
		s.patchable = false
		return s
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		start := key.Line - 1
		if seen[key.Value] || key.Column != 1 || start < 0 || start >= len(s.lines) ||
			(len(s.keys) > 0 && start <= s.keys[len(s.keys)-1].start) {
			s.patchable = false
			return s
		}
		seen[key.Value] = true
		s.keys = append(s.keys, sourceKey{
			name: key.Value, start: start, value: mapping.Content[i+1],
		})
	}
	for i := range s.keys {
		end := len(s.lines)
		if i+1 < len(s.keys) {
			end = s.keys[i+1].start
		}
		for end > s.keys[i].start+1 && isTrivia(s.lines[end-1]) {
			end--
		}
		s.keys[i].end = end
	}
	return s
}

// render returns fm as the frontmatter text: the source patched where it
// can be, else fm written fresh with the source's line endings.
func (s *source) render(fm Frontmatter) ([]byte, error) {
	if s.patchable {
		out, err := s.patch(fm)
		if err != nil {
			return nil, err
		}
		if s.holds(out, fm) {
			return out, nil
		}
	}
	out, err := encodeFresh(fm)
	if err != nil {
		return nil, err
	}
	return withLineEnding(out, s.eol), nil
}

// patch rewrites the source's lines for fm: each key in source order kept,
// replaced or dropped, then the keys fm adds.
func (s *source) patch(fm Frontmatter) ([]byte, error) {
	fields, err := fm.fields()
	if err != nil {
		return nil, err
	}
	current := make(map[string]*yaml.Node, len(fields))
	for _, f := range fields {
		current[f.Key] = f.Value
	}
	var out bytes.Buffer
	inSource := make(map[string]bool, len(s.keys))
	pos := 0
	for _, k := range s.keys {
		inSource[k.name] = true
		writeLines(&out, s.lines[pos:k.start])
		pos = k.end
		value, present := current[k.name]
		switch {
		case s.unchanged(fm, k.name):
			writeLines(&out, s.lines[k.start:k.end])
		case present:
			if err := s.writeEntry(&out, k.name, styled(k.name, k.value, value)); err != nil {
				return nil, err
			}
		}
	}
	writeLines(&out, s.lines[pos:])
	if out.Len() > 0 && !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
		out.WriteString(s.eol)
	}
	for _, f := range fields {
		if !inSource[f.Key] {
			if err := s.writeEntry(&out, f.Key, f.Value); err != nil {
				return nil, err
			}
		}
	}
	return out.Bytes(), nil
}

// unchanged reports whether key holds the value it was parsed with. A typed
// field compares its Go value, any other key its plain value, so a key set
// again to the same value keeps its text.
func (s *source) unchanged(fm Frontmatter, key string) bool {
	if i, ok := typedFields[key]; ok {
		return sameField(
			reflect.ValueOf(s.typed).Field(i),
			reflect.ValueOf(fieldsOnly(fm)).Field(i),
		)
	}
	value := fm.extraValue(key)
	if value == nil {
		return false
	}
	was, ok := s.extras[key]
	if !ok {
		return false
	}
	now, err := plainValue(value)
	return err == nil && reflect.DeepEqual(was, now)
}

// writeEntry writes key: value as 2-space YAML in the source's line endings.
func (s *source) writeEntry(out *bytes.Buffer, key string, value *yaml.Node) error {
	entry := &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: []*yaml.Node{keyNode(key), value},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(entry); err != nil {
		return fmt.Errorf("marshal frontmatter %s: %w", key, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("marshal frontmatter %s: %w", key, err)
	}
	out.Write(withLineEnding(buf.Bytes(), s.eol))
	return nil
}

// holds reports whether the patched text out parses back to fm. A patch
// that would not (an anchor in a dropped key, a kept block scalar's
// trailing lines) falls back to a fresh encoding.
func (s *source) holds(out []byte, fm Frontmatter) bool {
	got, _, err := ParseFrontmatter(slices.Concat([]byte("---\n"), out, []byte("---\n")))
	return err == nil && sameContent(got, fm)
}

// styled returns the value a rewritten key gets. A typed field is encoded
// anew, so it takes the style its parsed value had: quoting, a flow list, a
// plain date. Any other key keeps the node it was given.
func styled(key string, was, value *yaml.Node) *yaml.Node {
	if _, ok := typedFields[key]; !ok || was == nil || was.Kind != value.Kind {
		return value
	}
	out := *value
	switch value.Kind {
	case yaml.ScalarNode:
		restyleScalar(&out, was)
	case yaml.SequenceNode:
		out.Style = was.Style & yaml.FlowStyle
		if len(was.Content) > 0 && was.Content[0].Kind == yaml.ScalarNode {
			out.Content = make([]*yaml.Node, len(value.Content))
			for i, item := range value.Content {
				c := *item
				restyleScalar(&c, was.Content[0])
				out.Content[i] = &c
			}
		}
	}
	return &out
}

// restyleScalar gives n the quoting of was. A plain value that read as
// something other than a string, such as a date, stays plain when the new
// value reads the same way.
func restyleScalar(n, was *yaml.Node) {
	if was.Kind != yaml.ScalarNode {
		return
	}
	if was.Style != 0 {
		n.Style = was.Style
		return
	}
	plain := yaml.Node{Kind: yaml.ScalarNode, Value: n.Value}
	if was.ShortTag() != "!!str" && plain.ShortTag() == was.ShortTag() {
		n.Tag, n.Style = "", 0
	}
}

// sameContent reports whether a and b hold the same keys and values.
func sameContent(a, b Frontmatter) bool {
	ta, tb := reflect.ValueOf(fieldsOnly(a)), reflect.ValueOf(fieldsOnly(b))
	for _, i := range typedFields {
		if !sameField(ta.Field(i), tb.Field(i)) {
			return false
		}
	}
	ea, errA := a.ExtraValues()
	eb, errB := b.ExtraValues()
	return errA == nil && errB == nil && reflect.DeepEqual(ea, eb)
}

// sameField compares two typed field values; an empty list equals a nil one.
func sameField(a, b reflect.Value) bool {
	if a.Kind() == reflect.Slice && a.Len() == 0 && b.Len() == 0 {
		return true
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// snapshotTyped copies fm's typed fields, lists included, so a later change
// to fm cannot reach the copy.
func snapshotTyped(fm Frontmatter) fieldsOnly {
	return fieldsOnly{
		Title: fm.Title, Aliases: slices.Clone(fm.Aliases), Tags: slices.Clone(fm.Tags),
		Type: fm.Type, Status: fm.Status, Source: fm.Source, Evidence: fm.Evidence,
		Created: fm.Created, Updated: fm.Updated, ID: fm.ID,
	}
}

// lineEnding returns the line ending of line: "\r\n" or "\n".
func lineEnding(line []byte) string {
	if bytes.HasSuffix(line, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// withLineEnding returns yml with each "\n" written as eol.
func withLineEnding(yml []byte, eol string) []byte {
	if eol == "\n" {
		return yml
	}
	return bytes.ReplaceAll(yml, []byte("\n"), []byte(eol))
}

// splitLines splits text after each line break, keeping the breaks.
func splitLines(text []byte) [][]byte {
	var lines [][]byte
	for len(text) > 0 {
		var line []byte
		line, text = nextLine(text)
		lines = append(lines, line)
	}
	return lines
}

func writeLines(out *bytes.Buffer, lines [][]byte) {
	for _, l := range lines {
		out.Write(l)
	}
}

// isTrivia reports whether a frontmatter line is blank or a comment in the
// first column, which never belongs to the entry above it.
func isTrivia(line []byte) bool {
	trimmed := bytes.TrimRight(line, " \t\r\n")
	return len(trimmed) == 0 || trimmed[0] == '#'
}
