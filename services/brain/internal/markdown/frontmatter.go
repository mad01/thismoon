// Package markdown parses Obsidian-style markdown: YAML frontmatter between
// --- fences, [[wikilinks]], and standard [text](url) markdown links.
//
// Pure — no I/O. Callers pass the file content as a byte slice.
package markdown

import (
	"bytes"
	"fmt"
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

// Split returns the (frontmatter raw bytes, body bytes). Frontmatter is empty
// if the file has no YAML fence. The body excludes the trailing --- fence.
func Split(content []byte) (frontmatterRaw, body []byte) {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, content
	}

	// Find the closing fence. It must be on its own line.
	start := bytes.IndexByte(content, '\n') + 1
	rest := content[start:]
	end := -1
	for i := 0; i < len(rest); {
		// line starts at i
		nl := bytes.IndexByte(rest[i:], '\n')
		var line []byte
		if nl < 0 {
			line = rest[i:]
		} else {
			line = rest[i : i+nl]
		}
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(trimmed, []byte("---")) {
			end = i + len(line)
			if nl >= 0 {
				end = i + nl
			}
			break
		}
		if nl < 0 {
			break
		}
		i += nl + 1
	}
	if end < 0 {
		// Unterminated fence — treat the whole file as body.
		return nil, content
	}
	fm := rest[:end]
	// Advance past the closing fence line (and its newline if present).
	tail := rest[end:]
	if len(tail) > 0 && tail[0] == '\n' {
		tail = tail[1:]
	} else if len(tail) > 1 && tail[0] == '\r' && tail[1] == '\n' {
		tail = tail[2:]
	} else if len(tail) >= 4 {
		// Skip the closing "---" + newline if we didn't already.
		if bytes.HasPrefix(tail, []byte("---\n")) {
			tail = tail[4:]
		} else if bytes.HasPrefix(tail, []byte("---\r\n")) {
			tail = tail[5:]
		}
	}
	return fm, tail
}

// ParseFrontmatter returns the typed frontmatter plus the body bytes. When the
// file has no frontmatter, returns a zero Frontmatter and the full content.
func ParseFrontmatter(content []byte) (Frontmatter, []byte, error) {
	raw, body := Split(content)
	if len(raw) == 0 {
		return Frontmatter{}, body, nil
	}
	var fm Frontmatter
	if err := yaml.Unmarshal(raw, &fm); err != nil {
		return Frontmatter{}, body, fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, body, nil
}

// yamlIndent is the indentation Encode writes. Two spaces is the width
// hand-written frontmatter uses, so an edit to one key leaves the lines of
// every other key as they were; yaml.v3 would otherwise write four.
const yamlIndent = 2

// Encode returns fm as the YAML that goes between the --- fences.
func Encode(fm Frontmatter) ([]byte, error) {
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

// Render returns bytes in the form "---\n<yaml>\n---\n<body>". If body is
// empty, only the frontmatter fence block is emitted.
func Render(fm Frontmatter, body []byte) ([]byte, error) {
	yml, err := Encode(fm)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(yml)
	out.WriteString("---\n")
	if len(body) > 0 {
		if !bytes.HasPrefix(body, []byte("\n")) {
			out.WriteString("")
		}
		out.Write(body)
	}
	return out.Bytes(), nil
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
