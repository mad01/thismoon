package finder

import (
	"os"
	"path/filepath"
	"strings"
)

// IsLinkedWorktree reports whether dir is the root of a linked git worktree:
// its .git is a pointer file whose git dir carries a "commondir" file. A
// submodule's .git is a pointer file too, but its git dir has no commondir,
// and a main clone's .git is a directory.
func IsLinkedWorktree(dir string) bool {
	gitFile := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitFile)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	gitDir := readGitdirPointer(gitFile)
	if gitDir == "" {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	_, err = os.Stat(filepath.Join(gitDir, "commondir"))
	return err == nil
}

// MainCheckout returns the main clone a linked worktree at root belongs to:
// the directory holding the common git dir its .git pointer leads to. ok is
// false when root is not a linked worktree.
func MainCheckout(root string) (string, bool) {
	if !IsLinkedWorktree(root) {
		return "", false
	}
	gitDir := readGitdirPointer(filepath.Join(root, ".git"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	common := resolveCommonDir(gitDir)
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), true
	}
	return common, true // a bare clone is its own git dir
}

// LinkedWorktrees returns the root of every linked worktree the clone at
// repoPath records under .git/worktrees, read from each entry's "gitdir"
// file (no subprocess). An entry whose worktree directory is gone, the kind
// `git worktree prune` would drop, is skipped. A path that is not a main
// clone has none.
func LinkedWorktrees(repoPath string) []string {
	base := filepath.Join(repoPath, ".git", "worktrees")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var roots []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root, ok := worktreeRoot(filepath.Join(base, e.Name()))
		if ok {
			roots = append(roots, root)
		}
	}
	return roots
}

// worktreeRoot reads a worktree entry's "gitdir" file, which names the
// worktree's .git pointer file, and returns the directory holding it when
// that directory is still a linked worktree.
func worktreeRoot(entryDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(entryDir, "gitdir"))
	if err != nil {
		return "", false
	}
	gitFile := strings.TrimSpace(string(data))
	if gitFile == "" {
		return "", false
	}
	if !filepath.IsAbs(gitFile) {
		gitFile = filepath.Join(entryDir, gitFile)
	}
	root := filepath.Dir(filepath.Clean(gitFile))
	if !IsLinkedWorktree(root) {
		return "", false
	}
	return root, true
}
