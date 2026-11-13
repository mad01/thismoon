// Package cslindex answers "does csl index this file, and under which repo
// name" from csl's shard listing on disk, follows a linked worktree to its
// main checkout (worktree.go), tells a file a csl_ask call cannot answer
// from and counts its lines (skip.go), and builds the csl_ask call scoped
// to one file, sized by kit/cslask.
// git's small bookkeeping files are read through SmallFile, which the
// guard's git discovery shares, so no read blocks or runs on. A hook
// runs on every matching tool call and cannot afford to launch csl, so
// nothing here runs a process. The read-size hint and the large-read guard
// both use it, so the guard's refusal hands over the same call the hint
// does, and the prefer-csl hint shares its worktree lookup.
package cslindex

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// dir is where csl keeps one zoekt shard per indexed repo. Reading the
// directory listing is how belt answers "is this repo indexed" without
// spawning csl.
func dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "csl", "search-index")
}

// Repos returns the set of org/repo names csl has a shard for, lowercased.
// Shards are named `<org>%2F<repo>_v<N>.<NNNNN>.zoekt`, so the repo name is
// the URL-unescaped portion before the version suffix. A missing or
// unreadable index directory yields an empty set, which makes every caller
// fall through to "not indexed" and stay quiet.
func Repos() map[string]bool {
	d := dir()
	if d == "" {
		return nil
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil
	}
	repos := make(map[string]bool, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".zoekt") {
			continue
		}
		// Trim the `_v16.00000.zoekt` tail: the repo name never contains an
		// underscore-v-digit sequence, so the last such separator wins.
		idx := strings.LastIndex(name, "_v")
		if idx <= 0 {
			continue
		}
		repo, err := url.QueryUnescape(name[:idx])
		if err != nil {
			continue
		}
		repos[strings.ToLower(repo)] = true
	}
	return repos
}

// RepoRoot walks up from p to the nearest directory holding a .git entry
// and returns it, "" when p is not inside a git working tree. A linked
// worktree's .git is a pointer file, so its own root is found.
func RepoRoot(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	// A file path needs its directory; a directory is already the start.
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	for dir := abs; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// RepoName names a working tree root the way csl names its shard:
// `<parent-dir>/<repo-dir>`.
func RepoName(root string) string {
	return filepath.Base(filepath.Dir(root)) + "/" + filepath.Base(root)
}

// locateIn returns repo and file's slash path below root, ok only when csl
// has a shard for repo.
func locateIn(root, repo, file string) (string, string, bool) {
	if !Repos()[strings.ToLower(repo)] {
		return "", "", false
	}
	r, err := filepath.Rel(root, file)
	if err != nil {
		return "", "", false
	}
	return repo, filepath.ToSlash(r), true
}

// ZoektTerm renders an RE2 pattern as one zoekt term. Unquoted, a pattern
// with a space reads as an AND of several terms, a double quote opens a
// phrase, and a leading - negates; such a pattern is quoted as a phrase.
// zoekt unescapes one level of backslashes inside quotes, so they double.
func ZoektTerm(re string) string {
	if !strings.ContainsAny(re, " \t\"") && !strings.HasPrefix(re, "-") {
		return re
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(re) + `"`
}
