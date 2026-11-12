package hint

import (
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

// repoForPath names the git working tree p sits in as `<parent-dir>/<repo-dir>`,
// matching how csl names its shards. It returns "" when p is not inside a
// repo, which is the correct answer for paths like ~/.claude or /tmp where a
// filesystem grep is the right tool.
func repoForPath(p string) string {
	root := cslindex.RepoRoot(p)
	if root == "" {
		return ""
	}
	return cslindex.RepoName(root)
}

// isIndexed reports whether the repo containing p has a csl shard.
func isIndexed(p string, repos map[string]bool) (string, bool) {
	name := repoForPath(p)
	if name == "" {
		return "", false
	}
	return name, repos[strings.ToLower(name)]
}
