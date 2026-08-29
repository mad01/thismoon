package catalog

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
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
// Sources are written with a leading ~ so one registry works across machines;
// a home directory that cannot be resolved is an error rather than a
// cwd-relative path, which would scan a tree nobody registered.
func (r *Registry) Paths() ([]string, error) {
	out := make([]string, 0, len(r.Sources))
	for _, s := range r.Sources {
		path, err := confdir.Expand(s.Path)
		if err != nil {
			return nil, fmt.Errorf("registry source %q: %w", s.Path, err)
		}
		out = append(out, path)
	}
	return out, nil
}
