// Package config loads the optional deps discovery config — which repos to skip
// entirely and which sub-paths to skip while walking. It mirrors how csl is
// configured: a small TOML file, symlinked into place by the recipe, that the
// tool reads at scan time. A missing file is not an error; discovery just runs
// with no extra exclusions (git worktrees and nested checkouts are skipped
// regardless, in the walker).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultPath is where the recipe symlinks the config.
const DefaultPath = "~/.config/deps/config.toml"

// Config is the parsed discovery config. Both lists are glob patterns
// (filepath.Match syntax).
type Config struct {
	// ExcludeRepos skips a whole repo. Each pattern is matched against the
	// repo's absolute path and against its basename, so "some-repo" and
	// "*/archive/*" both work.
	ExcludeRepos []string `toml:"exclude_repos"`
	// ExcludePaths skips a directory while walking a repo. Each pattern is
	// matched against the repo-relative path and against the directory's
	// basename, so "third_party" and "internal/gen/*" both work.
	ExcludePaths []string `toml:"exclude_paths"`
}

// Load reads the config at path (tilde-expanded). A missing file yields an empty
// Config and no error.
func Load(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(expandTilde(path))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := toml.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, nil
}

// RepoExcluded reports whether repoPath is excluded by ExcludeRepos.
func (c Config) RepoExcluded(repoPath string) bool {
	return matchesAny(c.ExcludeRepos, repoPath, filepath.Base(repoPath))
}

// PathExcluded reports whether a repo-relative directory path is excluded by
// ExcludePaths.
func (c Config) PathExcluded(relPath string) bool {
	return matchesAny(c.ExcludePaths, relPath, filepath.Base(relPath))
}

// matchesAny returns true if any pattern matches the full string or the base.
func matchesAny(patterns []string, full, base string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, full); ok {
			return true
		}
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
	}
	return false
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
