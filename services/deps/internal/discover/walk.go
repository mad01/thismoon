package discover

import (
	"io/fs"
	"os"
	"path/filepath"
)

// findManifests walks root and returns every file named filename, skipping what
// findFiles skips.
func findManifests(root, filename string, opts Options) ([]string, error) {
	return findFiles(root, opts, func(rel string) bool { return filepath.Base(rel) == filename })
}

// findFiles walks root and returns every file whose root-relative path match
// accepts. It skips the usual non-source trees, any directory the Options
// exclude, and — crucially — any nested git checkout (a submodule or a linked
// worktree), so a worktree sitting under a registry repo isn't scanned twice.
// The repo root itself is never skipped on the nested-repo test (it
// legitimately holds .git).
func findFiles(root string, opts Options, match func(rel string) bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree — skip, don't abort the repo
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDir(d.Name()) || isNestedCheckout(path) || opts.excluded(root, path) {
				return fs.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(root, path); err == nil && match(rel) {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// skipDir names directories never worth descending into during discovery.
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "testdata", ".idea", ".vscode":
		return true
	}
	return false
}

// isNestedCheckout reports whether dir is the root of a nested git checkout — a
// submodule or a linked worktree. A worktree marks its root with a `.git` *file*
// (a gitdir pointer); a submodule has a `.git` file or dir. Either way, a `.git`
// entry inside a subdirectory means a separate checkout we should not re-scan.
func isNestedCheckout(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}
