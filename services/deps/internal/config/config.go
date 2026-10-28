// Package config loads the optional deps discovery config — which repos to skip
// entirely, which sub-paths to skip while walking, and where mise keeps the
// tool trees scanned beside the repos. It mirrors how csl is configured: a
// small TOML file, symlinked into place by the recipe, that the tool reads at
// scan time. A missing file is not an error; discovery just runs with no extra
// exclusions and mise at its default paths (git worktrees and nested checkouts
// are skipped regardless, in the walker). A file that is present but holds a
// pattern that does not compile IS an error: an exclusion that silently
// disables itself scans a repo the operator asked to skip.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gobwas/glob"

	"github.com/mad01/thismoon/kit/confdir"
)

// FileName is the discovery config's name inside the deps config directory.
const FileName = "config.toml"

// DefaultPath is the compiled fallback location, used when the config
// directory cannot be resolved. Normal resolution goes through
// confdir.Path("deps", FileName), which honors XDG_CONFIG_HOME.
const DefaultPath = "~/.config/deps/" + FileName

// Default locations of the mise files discovery reads: the global lock beside
// mise's config, and the installs directory under mise's data directory.
const (
	DefaultMiseLock     = "~/.config/mise/mise.lock"
	DefaultMiseInstalls = "~/.local/share/mise/installs"
)

// Config is the parsed, validated discovery config. Both lists are glob
// patterns (the gobwas/glob syntax prs's excludes use) matched a path segment
// at a time: `*` never crosses a "/", `**` does, and `{a,b}` alternates.
//
// Load is the constructor — it compiles the patterns and fills the mise
// defaults. A Config built as a literal carries no compiled patterns and no
// mise paths, so it excludes nothing and scans no mise tools.
type Config struct {
	// ExcludeRepos skips a whole repo. See RepoExcluded for what each pattern
	// is matched against.
	ExcludeRepos []string `toml:"exclude_repos"`
	// ExcludePaths skips a directory while walking a repo. See PathExcluded.
	ExcludePaths []string `toml:"exclude_paths"`
	// Mise locates the installed mise tools scanned beside the repos.
	Mise Mise `toml:"mise"`

	repoGlobs []glob.Glob
	pathGlobs []glob.Glob
}

// Mise holds the paths of the mise files discovery reads, each with a leading
// ~ allowed. An unset path takes its default, so a config names only the path
// that moved. A machine without mise needs no setting: a missing lock or
// installs directory just means no tools to scan.
type Mise struct {
	// Lock is the mise.lock that pins each tool's version.
	Lock string `toml:"lock"`
	// Installs is the directory mise installs tools into, one per tool.
	Installs string `toml:"installs"`
}

// fileConfig mirrors the TOML on disk, kept apart from Config so the compiled
// patterns have somewhere to live that the decoder never touches.
type fileConfig struct {
	ExcludeRepos []string `toml:"exclude_repos"`
	ExcludePaths []string `toml:"exclude_paths"`
	Mise         Mise     `toml:"mise"`
}

// Default is the config deps scans with when no file exists: no exclusions,
// and mise at its default paths.
func Default() Config {
	return Config{}.withMiseDefaults()
}

// withMiseDefaults fills each unset mise path with its default.
func (c Config) withMiseDefaults() Config {
	c.Mise.Lock = cmp.Or(c.Mise.Lock, DefaultMiseLock)
	c.Mise.Installs = cmp.Or(c.Mise.Installs, DefaultMiseInstalls)
	return c
}

// Load reads the config at path (tilde-expanded). A missing file yields the
// Default config and no error. A file that is present but malformed — bad TOML,
// or a pattern that does not compile — is an error naming what failed.
func Load(path string) (Config, error) {
	expanded, err := confdir.Expand(path)
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(expanded)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var fc fileConfig
	if err := toml.Unmarshal(raw, &fc); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	c := Config{
		ExcludeRepos: fc.ExcludeRepos,
		ExcludePaths: fc.ExcludePaths,
		Mise:         fc.Mise,
	}.withMiseDefaults()
	if c.repoGlobs, err = compile(path, "exclude_repos", fc.ExcludeRepos); err != nil {
		return Config{}, err
	}
	if c.pathGlobs, err = compile(path, "exclude_paths", fc.ExcludePaths); err != nil {
		return Config{}, err
	}
	return c, nil
}

// RepoExcluded reports whether repoPath is excluded by ExcludeRepos. Each
// pattern is matched against every trailing run of whole path segments —
// "thismoon", then "mad01/thismoon", then "github.com/mad01/thismoon", up to
// the absolute path itself — so a pattern can name a repo by basename
// ("archive-old"), by org/repo identity ("mad01/*"), or by a deeper suffix
// ("*/archive/*") without knowing where this machine keeps its checkouts.
func (c Config) RepoExcluded(repoPath string) bool {
	return matchSuffixes(c.repoGlobs, repoPath)
}

// PathExcluded reports whether a repo-relative directory path is excluded by
// ExcludePaths. Matching works the same way as RepoExcluded, so "third_party"
// skips that directory wherever it sits in the tree and "internal/gen/*" skips
// each child of any internal/gen.
func (c Config) PathExcluded(relPath string) bool {
	return matchSuffixes(c.pathGlobs, relPath)
}

// matchSuffixes reports whether any pattern matches path or one of its
// trailing segment runs. Patterns never match across a "/", so a suffix has to
// line up segment for segment — which is what keeps "mad01/*" from reaching
// past the org directory it names.
func matchSuffixes(globs []glob.Glob, path string) bool {
	if len(globs) == 0 {
		return false
	}
	for s := filepath.Clean(path); s != ""; s = dropLeadingSegment(s) {
		for _, g := range globs {
			if g.Match(s) {
				return true
			}
		}
	}
	return false
}

// dropLeadingSegment removes everything up to and including the first "/",
// returning "" once no whole segment is left to strip.
func dropLeadingSegment(path string) string {
	i := strings.IndexByte(path, '/')
	if i < 0 || i+1 == len(path) {
		return ""
	}
	return path[i+1:]
}

// compile turns one key's patterns into matchers, failing on the first that
// does not compile and naming it. An unclosed bracket used to disable its own
// exclusion in silence, which scans a repo the operator asked to skip.
func compile(path, key string, patterns []string) ([]glob.Glob, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	globs := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, fmt.Errorf("parse config %s: %s pattern %q: %w", path, key, p, err)
		}
		globs = append(globs, g)
	}
	return globs, nil
}
