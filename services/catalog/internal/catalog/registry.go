package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry lists the source repositories the catalog scans for entities.
type Registry struct {
	Sources []Source `yaml:"sources"`
}

// Source is one scanned location — typically a repo root (a monorepo with many
// service-info.yaml files, or a single-tool repo with one at its root).
type Source struct {
	Path string `yaml:"path"`
}

// ParseRegistry decodes registry YAML.
func ParseRegistry(data []byte) (*Registry, error) {
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return &r, nil
}

// LoadRegistry reads and parses a registry file from disk.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}
	return ParseRegistry(data)
}

// Paths returns the source paths with '~' expanded to the home directory.
func (r *Registry) Paths() []string {
	out := make([]string, 0, len(r.Sources))
	for _, s := range r.Sources {
		out = append(out, ExpandPath(s.Path))
	}
	return out
}

// ExpandPath expands a leading '~' (or '~/...') to the user's home directory.
// Paths without a leading tilde are returned unchanged.
func ExpandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
