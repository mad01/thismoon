// Package repopath confines a caller-supplied path to a repository root.
package repopath

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Resolve joins rel to root and returns the result, refusing any rel that
// leaves root: an absolute path, a ".." escape, or a symlink whose target
// lies outside the real root. The index skips symlinks it would have to
// follow, so refusing them keeps reads to what search can see. A path that
// does not exist passes, so the caller's open still reports not-found. param
// names the input in the error text.
func Resolve(root, param, rel string) (string, error) {
	if !filepath.IsLocal(filepath.Clean(rel)) {
		return "", outside(param, rel)
	}
	abs := filepath.Join(root, rel)
	// The root is resolved too, since a checkout can sit behind a symlink
	// (macOS keeps /var behind /private/var).
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	real, err := realPath(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s %q: %w", param, rel, err)
	}
	if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return "", outside(param, rel)
	}
	return abs, nil
}

func outside(param, rel string) error {
	return fmt.Errorf("%s must stay inside the repo, got %q", param, rel)
}

// realPath resolves the symlinks in p. A missing tail is kept as written
// once its nearest existing ancestor is resolved, so a link to a directory
// outside the root is caught even when the file under it does not exist.
func realPath(p string) (string, error) {
	real, err := filepath.EvalSymlinks(p)
	if !errors.Is(err, fs.ErrNotExist) {
		return real, err
	}
	parent := filepath.Dir(p)
	if parent == p {
		return "", err
	}
	realParent, err := realPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(p)), nil
}
