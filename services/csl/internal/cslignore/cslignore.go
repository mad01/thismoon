// Package cslignore loads a repo's .cslignore file: per-repo path exclusion
// rules honored by every csl index (lexical and semantic).
package cslignore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
)

// Matcher holds the compiled patterns of a repo's .cslignore file.
// Patterns are globs matched against repo-relative slash paths: `*` stops at
// separators, `**` crosses them, a trailing `/` covers the whole tree under a
// directory, and a leading `/` anchors the pattern to the repo root
// (unanchored patterns match at any depth).
type Matcher struct {
	globs []glob.Glob
}

// Load reads root/.cslignore and compiles its patterns. Returns nil when the
// file is absent or holds no valid patterns; unparsable lines are skipped.
// One glob per line, `#` starts a comment, blank lines are ignored.
func Load(root string) *Matcher {
	data, err := os.ReadFile(filepath.Join(root, ".cslignore"))
	if err != nil {
		return nil
	}
	var m Matcher
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		anchored := strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		if strings.HasSuffix(line, "/") {
			line += "**"
		}
		patterns := []string{line}
		if !anchored {
			patterns = append(patterns, "**/"+line)
		}
		for _, p := range patterns {
			if g, err := glob.Compile(p, '/'); err == nil {
				m.globs = append(m.globs, g)
			}
		}
	}
	if len(m.globs) == 0 {
		return nil
	}
	return &m
}

// Match reports whether a repo-relative path is covered by the ignore file.
// A nil receiver (no .cslignore) matches nothing.
func (m *Matcher) Match(rel string) bool {
	if m == nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, g := range m.globs {
		if g.Match(rel) {
			return true
		}
	}
	return false
}
