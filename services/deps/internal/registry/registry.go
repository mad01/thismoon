// Package registry resolves the list of repos to scan from the catalog registry
// file (recipes/catalog/registry.yaml, symlinked to ~/.config/catalog/
// registry.yaml). Reusing it means a new tool repo enrolls in dependency
// scanning the moment it is catalogued — one source of truth, no second list.
package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the catalog registry symlink the catalog recipe maintains.
const DefaultPath = "~/.config/catalog/registry.yaml"

// file mirrors registry.yaml: a list of repo-root source paths.
type file struct {
	Sources []struct {
		Path string `yaml:"path"`
	} `yaml:"sources"`
}

// Repos reads path and returns the existing repo roots to scan, tilde-expanded.
// Listed paths that are not present on this machine are skipped (the registry is
// shared across hosts; not every repo is checked out everywhere).
func Repos(path string) ([]string, error) {
	raw, err := os.ReadFile(expandTilde(path))
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", path, err)
	}
	var repos []string
	for _, s := range f.Sources {
		dir := expandTilde(s.Path)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			repos = append(repos, dir)
		}
	}
	return repos, nil
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. Registry
// paths are stored with ~ so the same file works across machines.
func expandTilde(path string) string {
	if path == "~" || len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
