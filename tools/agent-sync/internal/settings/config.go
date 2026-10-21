// Package settings merges a template settings.json into Claude Code's live
// settings file, then merges the machine's local overlay on top. It is the
// Go port of the dotfiles repo's sync_settings.py and keeps that script's
// stdout, exit codes and written bytes.
//
// The rules: the template wins over live (objects merge key by key, arrays
// and scalars replace), keys only live carries are kept, the prune list
// deletes retired keys from live before the merge, and the overlay merges
// last with the permission lists unioned instead of replaced. A symlinked or
// hard-linked live file becomes a regular file.
//
// config.go reads the settings section, merge.go holds the pure merge and
// diff, report.go renders the diff, and sync.go is the file I/O around them.
package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

// Defaults for the live and overlay paths when the config leaves them out.
const (
	// ClaudeConfigDirEnv is the variable Claude Code reads its config
	// directory from; the live file defaults to settings.json inside it.
	ClaudeConfigDirEnv = "CLAUDE_CONFIG_DIR"
	// DefaultClaudeDir is Claude Code's config directory when
	// ClaudeConfigDirEnv is unset.
	DefaultClaudeDir = "~/.claude"
	// LiveFileName is the live settings file inside the config directory.
	LiveFileName = "settings.json"
	// OverlayFileName is the overlay's default name, beside the live file.
	OverlayFileName = "settings.local.json"
)

// The section's keys.
const (
	keyTemplate = "template"
	keyLive     = "live"
	keyOverlay  = "overlay"
	keyPrune    = "prune"
)

// KeyPath is a path of object keys from the document root, e.g.
// [hooks Notification]. Keys may contain dots, so a KeyPath compares key by
// key, never as its dotted String.
type KeyPath []string

// String joins the keys with dots, the way the report prints a path.
func (p KeyPath) String() string { return strings.Join(p, ".") }

// Config is the settings section, resolved: every path absolute with ~
// expanded, the prune list checked.
type Config struct {
	// Template is the settings.json the live file is merged from.
	Template string
	// Live is the settings file Claude Code reads.
	Live string
	// Overlay is the machine-local file merged on top; it need not exist.
	Overlay string
	// Prune lists the retired key paths deleted from live before the merge.
	Prune []KeyPath
}

// ParseConfig reads the settings section into a Config. A missing section,
// a missing template, an unknown key, a relative path or a malformed prune
// entry is an error naming the config line at fault.
func ParseConfig(s config.Section) (Config, error) {
	if !s.Present() {
		return Config{}, s.Errorf(nil, "",
			"section missing; it needs at least %s.%s, the template settings.json",
			config.SectionSettings, keyTemplate)
	}
	fields, err := s.Fields(keyTemplate, keyLive, keyOverlay, keyPrune)
	if err != nil {
		return Config{}, err
	}
	var c Config
	node, ok := fields[keyTemplate]
	if !ok {
		return Config{}, s.Errorf(s.Node(), keyTemplate,
			"required: the path of the template settings.json to merge from")
	}
	if c.Template, err = s.Path(node, keyTemplate); err != nil {
		return Config{}, err
	}
	if c.Live, err = livePath(s, fields[keyLive]); err != nil {
		return Config{}, err
	}
	c.Overlay = filepath.Join(filepath.Dir(c.Live), OverlayFileName)
	if node, ok := fields[keyOverlay]; ok {
		if c.Overlay, err = s.Path(node, keyOverlay); err != nil {
			return Config{}, err
		}
	}
	if c.Prune, err = parsePrune(s, fields[keyPrune]); err != nil {
		return Config{}, err
	}
	return c, nil
}

// livePath returns the configured live path, or settings.json in
// $CLAUDE_CONFIG_DIR, or in ~/.claude.
func livePath(s config.Section, node *yaml.Node) (string, error) {
	if node != nil {
		return s.Path(node, keyLive)
	}
	dir := os.Getenv(ClaudeConfigDirEnv)
	if dir == "" {
		dir = DefaultClaudeDir
	}
	expanded, err := confdir.Expand(dir)
	if err != nil {
		return "", fmt.Errorf("settings: default live path: %w", err)
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("settings: %s=%q is not an absolute path; set %s.%s instead",
			ClaudeConfigDirEnv, dir, config.SectionSettings, keyLive)
	}
	return filepath.Join(expanded, LiveFileName), nil
}

// parsePrune reads the prune list: a list of key paths, each a non-empty
// list of string keys. An unquoted number or bool is not a key: the live
// file's keys are strings, so `[hooks, 1]` could never match.
func parsePrune(s config.Section, node *yaml.Node) ([]KeyPath, error) {
	if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, s.Errorf(node, keyPrune, "want a list of key paths, got %s",
			config.Describe(node))
	}
	paths := make([]KeyPath, 0, len(node.Content))
	for i, entry := range node.Content {
		key := fmt.Sprintf("%s[%d]", keyPrune, i)
		if entry.Kind != yaml.SequenceNode {
			return nil, s.Errorf(entry, key,
				"want a key path as a list of keys, e.g. [hooks, Notification], got %s",
				config.Describe(entry))
		}
		if len(entry.Content) == 0 {
			return nil, s.Errorf(entry, key, "want at least one key, got an empty list")
		}
		path := make(KeyPath, 0, len(entry.Content))
		for j, k := range entry.Content {
			name, err := s.String(k, fmt.Sprintf("%s[%d]", key, j))
			if err != nil {
				return nil, err
			}
			path = append(path, name)
		}
		paths = append(paths, path)
	}
	return paths, nil
}
