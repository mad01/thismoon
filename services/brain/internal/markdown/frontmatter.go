// Package markdown parses Obsidian-style markdown: YAML frontmatter between
// --- fences, [[wikilinks]], and standard [text](url) markdown links.
//
// Pure — no I/O. Callers pass the file content as a byte slice.
package markdown

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the canonical brain-note frontmatter shape. The typed fields
// cover the keys brain reads; every other key is kept in Extra so a
// ParseFrontmatter then Render round trip writes it back, nested values
// included. Render keeps the source key order.
//
// Evidence is the trust/correctness tier for an empirical note (typically
// "A" | "B" | "C" | "D" | "mixed"). Empty for non-empirical notes (personal
// profiles, MOCs, tool references). For back-compat, callers should use
// EvidenceOf() to read — it falls back to parsing "evidence/<tier>" tags
// when the explicit field is absent.
type Frontmatter struct {
	Title    string   `yaml:"title,omitempty"`
	Aliases  []string `yaml:"aliases,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	Type     string   `yaml:"type,omitempty"`
	Status   string   `yaml:"status,omitempty"`
	Source   string   `yaml:"source,omitempty"`
	Evidence string   `yaml:"evidence,omitempty"`
	Created  string   `yaml:"created,omitempty"`
	Updated  string   `yaml:"updated,omitempty"`
	ID       string   `yaml:"id,omitempty"`
	// Extra holds the keys no typed field models, in source order, each with
	// its YAML value node.
	Extra []Field `yaml:"-"`

	// order is the key order of the parsed source; Render follows it.
	order []string
	// src is the text the frontmatter was parsed from, nil for a new one.
	src *source
}

// Field is one frontmatter key with its YAML value.
type Field struct {
	Key   string
	Value *yaml.Node
}

// EvidenceOf returns the note's evidence tier. Prefers the explicit
// frontmatter.evidence field; falls back to parsing an "evidence/<tier>"
// tag. Returns empty string when neither is set.
func EvidenceOf(fm Frontmatter) string {
	if fm.Evidence != "" {
		return fm.Evidence
	}
	for _, t := range fm.Tags {
		if s, ok := strings.CutPrefix(t, "evidence/"); ok {
			return s
		}
	}
	return ""
}

// bom is the UTF-8 byte order mark some editors write at the start of a
// file. A note may open with one before its frontmatter fence.
var bom = []byte("\xEF\xBB\xBF")

// ErrUnclosed means a note opens a frontmatter fence that no later line
// closes. ParseFrontmatter reads such a note as body alone; a change to it
// would stack a second frontmatter block on top, so writers refuse it.
var ErrUnclosed = errors.New("frontmatter is not closed: the note opens with --- and no line closes it")

// fences is a note cut at its frontmatter fences. open holds the byte order
// mark, if any, and the opening fence line; close the closing fence line.
// Both keep their line endings, so the note renders back byte for byte.
type fences struct {
	open, yaml, close, body []byte
}

// cut finds the frontmatter of content. A fence is a line of "---" (the
// closing one may also be "...") followed by nothing but whitespace, and
// the opening one may follow a byte order mark. ok is false when content
// has no frontmatter; unclosed reports an opening fence with no closing one.
func cut(content []byte) (f fences, ok, unclosed bool) {
	first, rest := nextLine(bytes.TrimPrefix(content, bom))
	if !isFence(first, "---") {
		return fences{}, false, false
	}
	open := content[:len(content)-len(rest)]
	for i := 0; i < len(rest); {
		line, next := nextLine(rest[i:])
		if isFence(line, "---") || isFence(line, "...") {
			return fences{open: open, yaml: rest[:i], close: line, body: next}, true, false
		}
		i = len(rest) - len(next)
	}
	return fences{}, false, true
}

// nextLine splits data after its first line break: the line, its line
// ending included, and the rest.
func nextLine(data []byte) (line, rest []byte) {
	if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
		return data[:nl+1], data[nl+1:]
	}
	return data, nil
}

// isFence reports whether line is mark followed only by whitespace.
func isFence(line []byte, mark string) bool {
	return string(bytes.TrimRight(line, " \t\r\n")) == mark
}

// Split returns the (frontmatter raw bytes, body bytes). Frontmatter is nil
// if the file has no closed YAML fence. The body excludes the closing fence.
func Split(content []byte) (frontmatterRaw, body []byte) {
	f, ok, _ := cut(content)
	if !ok {
		return nil, content
	}
	return f.yaml, f.body
}

// Unclosed reports whether content opens a frontmatter fence that no line
// closes (see ErrUnclosed).
func Unclosed(content []byte) bool {
	_, _, unclosed := cut(content)
	return unclosed
}

// ParseFrontmatter returns the typed frontmatter plus the body bytes. When the
// file has no frontmatter, returns a zero Frontmatter and the full content.
// The frontmatter remembers its source text, so Render rewrites only the
// keys that changed since.
func ParseFrontmatter(content []byte) (Frontmatter, []byte, error) {
	f, ok, _ := cut(content)
	if !ok {
		return Frontmatter{}, content, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(f.yaml, &doc); err != nil {
		return Frontmatter{}, f.body, fmt.Errorf("parse frontmatter: %w", err)
	}
	var fm Frontmatter
	var mapping *yaml.Node
	if len(doc.Content) > 0 {
		mapping = doc.Content[0]
		if err := fm.UnmarshalYAML(mapping); err != nil {
			return Frontmatter{}, f.body, fmt.Errorf("parse frontmatter: %w", err)
		}
	}
	fm.src = newSource(f, mapping, fm)
	return fm, f.body, nil
}

// yamlIndent is the indentation a new note's frontmatter gets. Two spaces is
// the width hand-written frontmatter uses; yaml.v3 would otherwise write four.
const yamlIndent = 2

// Encode returns fm as the YAML that goes between the --- fences. A
// frontmatter parsed from a note keeps its source text: only the lines of
// keys whose value changed are rewritten, new keys go last, and the note's
// line endings stay. Any other frontmatter is written fresh.
func Encode(fm Frontmatter) ([]byte, error) {
	if fm.src != nil {
		return fm.src.render(fm)
	}
	return encodeFresh(fm)
}

// encodeFresh writes fm as new 2-space YAML.
func encodeFresh(fm Frontmatter) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&fm); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	return buf.Bytes(), nil
}

// Render returns bytes in the form "---\n<yaml>---\n<body>". A parsed
// frontmatter keeps the fence lines it was read with.
func Render(fm Frontmatter, body []byte) ([]byte, error) {
	yml, err := Encode(fm)
	if err != nil {
		return nil, err
	}
	open, closing := []byte("---\n"), []byte("---\n")
	if fm.src != nil {
		open, closing = fm.src.open, fm.src.close
		if len(body) > 0 && !bytes.HasSuffix(closing, []byte("\n")) {
			closing = append(slices.Clone(closing), fm.src.eol...)
		}
	}
	return slices.Concat(open, yml, closing, body), nil
}

// Touch updates the Updated timestamp to the current UTC date (YYYY-MM-DD).
func (f *Frontmatter) Touch(now time.Time) {
	f.Updated = now.UTC().Format("2006-01-02")
}

// Tag returns true when the frontmatter contains the given tag.
func (f *Frontmatter) HasTag(tag string) bool {
	for _, t := range f.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
