// Package registry resolves the list of repos to scan from the catalog registry
// file (recipes/catalog/registry.yaml, symlinked to ~/.config/catalog/
// registry.yaml). Reusing it means a new tool repo enrolls in dependency
// scanning the moment it is catalogued — one source of truth, no second list.
package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
)

// FileName is the registry's name inside the catalog config directory. deps
// reads the file catalog owns, so both resolve it the same way.
const FileName = "registry.yaml"

// DefaultPath is the compiled fallback location of the catalog registry
// symlink the catalog recipe maintains, used when the config directory cannot
// be resolved. Normal resolution goes through confdir.Path("catalog",
// FileName), which honors XDG_CONFIG_HOME.
const DefaultPath = "~/.config/catalog/" + FileName

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
	expanded, err := confdir.Expand(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", path, err)
	}
	var repos []string
	for _, s := range f.Sources {
		// Registry paths are stored with a leading ~ so the same file works
		// across machines; a home directory that cannot be resolved fails the
		// scan rather than silently producing a cwd-relative root.
		dir, err := confdir.Expand(s.Path)
		if err != nil {
			return nil, fmt.Errorf("registry %s: source %q: %w", path, s.Path, err)
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			repos = append(repos, dir)
		}
	}
	return repos, nil
}
