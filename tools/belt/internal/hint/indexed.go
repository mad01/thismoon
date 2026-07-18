package hint

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// indexDir is where csl keeps one zoekt shard per indexed repo. Reading the
// directory listing is how belt answers "is this repo indexed" without
// spawning csl: a hook runs on every matching tool call and cannot afford a
// process launch.
func indexDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "csl", "search-index")
}

// indexedRepos returns the set of org/repo names csl has a shard for. Shards
// are named `<org>%2F<repo>_v<N>.<NNNNN>.zoekt`, so the repo name is the
// URL-unescaped portion before the version suffix. A missing or unreadable
// index directory yields an empty set, which makes every caller fall through
// to "not indexed" and stay quiet.
func indexedRepos() map[string]bool {
	dir := indexDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
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
		escaped := name[:idx]
		repo, err := url.QueryUnescape(escaped)
		if err != nil {
			continue
		}
		repos[strings.ToLower(repo)] = true
	}
	return repos
}

// repoForPath walks up from p looking for a git working tree and names it
// `<parent-dir>/<repo-dir>`, matching how csl names its shards. It returns ""
// when p is not inside a repo, which is the correct answer for paths like
// ~/.claude or /tmp where a filesystem grep is the right tool.
func repoForPath(p string) string {
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
			org := filepath.Base(filepath.Dir(dir))
			return org + "/" + filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isIndexed reports whether the repo containing p has a csl shard.
func isIndexed(p string, repos map[string]bool) (string, bool) {
	name := repoForPath(p)
	if name == "" {
		return "", false
	}
	return name, repos[strings.ToLower(name)]
}
