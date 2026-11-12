package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Allow configures one allow (docs/adr/0032): a PreToolUse check that may
// answer allow for one narrow call shape, run only after every guard
// passed cleanly. Unlike a guard or a hint, an allow is off until a config
// sets enabled: it lets a call run without a prompt, which a machine should
// choose rather than inherit from a hook entry.
//
// Like the ledger and watch values, the roots are resolved, never
// rejected. A config error denies every guarded call, far too much for a
// typo in a feature that may be off, so an unusable root is dropped and
// Adjustments names it for `belt doctor`.
type Allow struct {
	Enabled bool `toml:"enabled" yaml:"enabled"`
	// Roots names the directories the allow may approve a call in, beside
	// the session's own repo. It is purpose-named under the allow's key and
	// read by that allow alone, so editing it cannot reach a guard
	// (docs/adr/0013).
	Roots []string `toml:"roots"   yaml:"roots,omitempty"`
}

// AllowEnabled reports whether an allow is on. Allows default to off, the
// opposite of guards and hints: a missing entry must never approve a call.
func (c Config) AllowEnabled(id string) bool { return c.Allows[id].Enabled }

// RootDirs is the roots in effect, each resolved physically (see rootDir).
// An entry rootDir cannot use is dropped.
func (a Allow) RootDirs() []string {
	var out []string
	for _, entry := range a.Roots {
		if dir, problem := rootDir(entry); problem == "" {
			out = append(out, dir)
		}
	}
	return out
}

// Adjustments names every root entry the allow drops, and why.
func (a Allow) Adjustments(id string) []string {
	var out []string
	for _, entry := range a.Roots {
		if _, problem := rootDir(entry); problem != "" {
			out = append(out, fmt.Sprintf(
				"allows.%s.roots entry %q %s, so it is ignored", id, entry, problem,
			))
		}
	}
	return out
}

// rootDir resolves one roots entry the way git -C resolves a directory, or
// says why it cannot be used. A leading ~ or ~/ is replaced by home, and
// the path is never cleaned before EvalSymlinks walks it: ~/link/.. names
// the parent of the link's target, as it does for git, where a lexical
// clean would name home.
func rootDir(entry string) (dir, problem string) {
	p := entry
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "starts with ~ but the home directory is unknown"
		}
		p = home + p[1:]
	}
	if !filepath.IsAbs(p) {
		return "", "is not an absolute or ~/ path"
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", "does not resolve to an existing directory"
	}
	if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
		return "", "is not a directory"
	}
	if CoversHome(real) {
		return "", "covers the home directory (it is /, home, or a directory above home)"
	}
	return real, ""
}

// CoversHome reports whether dir, a path with its symlinks resolved, is
// home or a directory above it. An allow that approved calls there would
// approve them in every repo under home, so its callers treat such a dir as
// unusable. When home itself cannot be resolved, the answer is true: there
// is then no way to say the dir is safe.
func CoversHome(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	real, err := filepath.EvalSymlinks(home)
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(dir, real)
	return err != nil || rel != ".." && !strings.HasPrefix(rel, "../")
}
