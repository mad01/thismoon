// Package config finds and reads the agent-sync config file and hands each
// section's YAML to the package that owns it.
//
// The file has one top-level key per section: settings, mods and mcp. This
// package rejects anything else at the top level and checks nothing inside a
// section. Each section package decodes its own Section, with the helpers
// here (Fields, String, Bool, Path, PathOr, Errorf), so every error names
// the file, the line and the key at fault in the same form.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
)

// Section names, in the order they appear in docs and doctor output.
const (
	SectionSettings = "settings"
	SectionMods     = "mods"
	SectionMCP      = "mcp"
)

var sectionNames = []string{SectionSettings, SectionMods, SectionMCP}

// ErrNotFound reports a config file that does not exist. Every section
// names its data in the file, so no command can run without one.
var ErrNotFound = errors.New("config: file not found")

// File is a parsed config file: its path and its three sections.
type File struct {
	Path     string
	Settings Section
	Mods     Section
	MCP      Section
}

// Section is one top-level section, kept as YAML so the package that owns it
// decodes it into its own type. A section the file does not mention is not
// Present.
type Section struct {
	Name string
	file string
	node *yaml.Node
}

// Path returns the config file location: override when it is non-empty (the
// --config flag, which defaults to $AGENT_SYNC_CONFIG), otherwise
// config.yaml in agent-sync's directory under $XDG_CONFIG_HOME or
// ~/.config.
func Path(override string) (string, error) {
	if override != "" {
		return confdir.Expand(override)
	}
	return confdir.Path(agentsync.Name, agentsync.ConfigFileName)
}

// Load reads and parses the config file at path. A missing file is
// ErrNotFound; an unreadable or malformed one, or one with a top-level key
// that names no section, is an error naming the file and line.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Parse(path, data)
}

// Parse parses data as the config file at path; path only labels errors.
func Parse(path string, data []byte) (*File, error) {
	f := &File{
		Path:     path,
		Settings: Section{Name: SectionSettings, file: path},
		Mods:     Section{Name: SectionMods, file: path},
		MCP:      Section{Name: SectionMCP, file: path},
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return f, nil // an empty file: every section absent
		}
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config %s: want one YAML document, found more", path)
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return f, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config %s:%d: want a mapping of sections (%s)",
			path, root.Line, strings.Join(sectionNames, ", "))
	}
	sections := map[string]*Section{
		SectionSettings: &f.Settings,
		SectionMods:     &f.Mods,
		SectionMCP:      &f.MCP,
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		s, ok := sections[key.Value]
		if !ok {
			return nil, fmt.Errorf("config %s:%d: unknown section %q (want %s)",
				path, key.Line, key.Value, strings.Join(sectionNames, ", "))
		}
		if s.node != nil {
			return nil, fmt.Errorf("config %s:%d: section %q appears twice",
				path, key.Line, key.Value)
		}
		s.node = val
	}
	return f, nil
}

// Present reports whether the file has this section.
func (s Section) Present() bool { return s.node != nil }

// Node returns the section's YAML, nil when it is absent.
func (s Section) Node() *yaml.Node { return s.node }

// Errorf returns an error that points at node's line in the config file and
// names the key at fault, e.g. `config ~/x.yaml:4: settings.template: ...`.
// key is relative to the section; empty names the section itself. format
// follows fmt.Errorf, so %w wraps an error.
func (s Section) Errorf(node *yaml.Node, key, format string, args ...any) error {
	where := s.file
	if node != nil {
		where = fmt.Sprintf("%s:%d", s.file, node.Line)
	}
	field := s.Name
	if key != "" {
		field += "." + key
	}
	return fmt.Errorf("config %s: %s: "+format, append([]any{where, field}, args...)...)
}

// Fields returns the section's keys and values, rejecting a key outside
// allowed. An absent or empty (null) section has no fields.
func (s Section) Fields(allowed ...string) (map[string]*yaml.Node, error) {
	fields := map[string]*yaml.Node{}
	if s.node == nil || (s.node.Kind == yaml.ScalarNode && s.node.Tag == "!!null") {
		return fields, nil
	}
	if s.node.Kind != yaml.MappingNode {
		return nil, s.Errorf(s.node, "", "want a mapping, got %s", Describe(s.node))
	}
	for i := 0; i+1 < len(s.node.Content); i += 2 {
		key, val := s.node.Content[i], s.node.Content[i+1]
		if !slices.Contains(allowed, key.Value) {
			return nil, s.Errorf(key, key.Value, "unknown key (want %s)",
				strings.Join(allowed, ", "))
		}
		if _, dup := fields[key.Value]; dup {
			return nil, s.Errorf(key, key.Value, "key appears twice")
		}
		fields[key.Value] = val
	}
	return fields, nil
}

// String returns node as a string, rejecting anything but a string scalar:
// an unquoted number or bool is an error, not a silent conversion.
func (s Section) String(node *yaml.Node, key string) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", s.Errorf(node, key, "want a string, got %s", Describe(node))
	}
	return node.Value, nil
}

// Bool returns node as a bool, rejecting anything but a bare true or false:
// a quoted "true" or a yes is an error, not a silent conversion.
func (s Section) Bool(node *yaml.Node, key string) (bool, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
		return false, s.Errorf(node, key, "want true or false, got %s", Describe(node))
	}
	var b bool
	if err := node.Decode(&b); err != nil {
		return false, s.Errorf(node, key, "%w", err)
	}
	return b, nil
}

// Path returns node as an absolute path with a leading ~ expanded. A
// relative path is an error: it would resolve against whatever directory
// ralph happens to run in.
func (s Section) Path(node *yaml.Node, key string) (string, error) {
	raw, err := s.String(node, key)
	if err != nil {
		return "", err
	}
	path, err := confdir.Expand(raw)
	if err != nil {
		return "", s.Errorf(node, key, "%w", err)
	}
	if !filepath.IsAbs(path) {
		return "", s.Errorf(node, key, "want an absolute or ~/ path, got %q", raw)
	}
	return filepath.Clean(path), nil
}

// PathOr is Path for a key the section sets, and def with a leading ~
// expanded for one it leaves out (node is nil). def is a section's
// compiled default, absolute or ~/.
func (s Section) PathOr(node *yaml.Node, key, def string) (string, error) {
	if node != nil {
		return s.Path(node, key)
	}
	path, err := confdir.Expand(def)
	if err != nil {
		return "", s.Errorf(nil, key, "default %s: %w", def, err)
	}
	return path, nil
}

// Describe names a node's YAML type and value for error messages, e.g.
// `the string "x"`, `int 1` or `a list`.
func Describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	case yaml.AliasNode:
		return "an alias"
	}
	switch n.Tag {
	case "!!str":
		return fmt.Sprintf("the string %q", n.Value)
	case "!!null":
		return "null"
	default:
		return fmt.Sprintf("%s %s", strings.TrimPrefix(n.Tag, "!!"), n.Value)
	}
}
