package catalog

import (
	"bufio"
	"path"
	"path/filepath"
	"strings"
)

// originURL extracts the origin remote URL from the text of a git config file.
// It scans for the `[remote "origin"]` section and returns its `url` value.
// The bool is false when no origin url is present.
func originURL(gitConfig string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(gitConfig))
	inOrigin := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if inOrigin && strings.HasPrefix(line, "url") {
			if _, val, ok := strings.Cut(line, "="); ok {
				return strings.TrimSpace(val), true
			}
		}
	}
	return "", false
}

// remoteToHostSlug converts an SSH or HTTPS git remote into a host and an
// "owner/repo" slug. It handles the common forms:
//
//	git@github.com:mad01/dotfiles.git      -> github.com, mad01/dotfiles
//	ssh://git@github.com/mad01/dotfiles.git -> github.com, mad01/dotfiles
//	https://github.com/mad01/dotfiles.git  -> github.com, mad01/dotfiles
//
// The bool is false when the remote cannot be parsed into both parts.
func remoteToHostSlug(remote string) (host, slug string, ok bool) {
	remote = strings.TrimSpace(remote)
	switch {
	case strings.HasPrefix(remote, "git@"):
		// git@host:owner/repo(.git)
		rest := strings.TrimPrefix(remote, "git@")
		host, slug, ok = strings.Cut(rest, ":")
	case strings.Contains(remote, "://"):
		// scheme://[user@]host/owner/repo(.git)
		_, rest, _ := strings.Cut(remote, "://")
		if at := strings.LastIndex(rest, "@"); at != -1 {
			rest = rest[at+1:]
		}
		host, slug, ok = strings.Cut(rest, "/")
	default:
		return "", "", false
	}
	if !ok {
		return "", "", false
	}
	slug = strings.TrimSuffix(strings.Trim(slug, "/"), ".git")
	if host == "" || slug == "" {
		return "", "", false
	}
	return host, slug, true
}

// repoWebURL builds a browseable URL for a path inside a repo. subPath is the
// entity's directory relative to the repo root; "" or "." links to the repo
// root. Like code-search-local, it targets the "HEAD" ref, which GitHub and
// GitHub Enterprise resolve to the repo's default branch.
func repoWebURL(host, slug, subPath string) string {
	base := "https://" + host + "/" + slug
	subPath = strings.Trim(path.Clean(subPath), "/")
	if subPath == "" || subPath == "." {
		return base
	}
	return base + "/tree/HEAD/" + subPath
}

// RepoURLFor derives a browseable remote URL for an entity whose source file
// lives at sourcePath, given the text of the enclosing repo's git config and
// the repo root the file was found under. It returns "" when no remote can be
// resolved, so callers can treat a link as optional.
func RepoURLFor(gitConfig, repoRoot, sourcePath string) string {
	remote, ok := originURL(gitConfig)
	if !ok {
		return ""
	}
	host, slug, ok := remoteToHostSlug(remote)
	if !ok {
		return ""
	}
	sub := relSubPath(repoRoot, sourcePath)
	return repoWebURL(host, slug, sub)
}

// relSubPath returns the directory of sourcePath relative to repoRoot, using
// forward slashes for URL composition. It returns "" when the file sits at the
// repo root or the paths are unrelated.
func relSubPath(repoRoot, sourcePath string) string {
	dir := path.Dir(filepath.ToSlash(sourcePath))
	root := strings.TrimRight(filepath.ToSlash(repoRoot), "/")
	if root == "" {
		return ""
	}
	if dir == root {
		return ""
	}
	if rel, ok := strings.CutPrefix(dir, root+"/"); ok {
		return rel
	}
	return ""
}
