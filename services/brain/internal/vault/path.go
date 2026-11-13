package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Resolve joins a vault-relative path with the vault root. It rejects an
// empty path, an absolute path, and any path that escapes the vault with
// ErrInvalidPath. The note need not exist.
func (v *Vault) Resolve(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf(
			"%w: path must be vault-relative, got absolute: %s",
			ErrInvalidPath,
			rel,
		)
	}
	full := filepath.Join(v.Root, filepath.Clean(rel))
	if !within(v.Root, full) {
		return "", fmt.Errorf("%w: path escapes vault: %s", ErrInvalidPath, rel)
	}
	if err := v.contained(full); err != nil {
		return "", err
	}
	return full, nil
}

// contained refuses, with ErrInvalidPath, a note path that is a symbolic
// link, or whose real path leaves the vault root through a linked
// directory. A committed link in a shared vault could otherwise read any
// file of the machine into a note. The path need not exist: its nearest
// existing parent is checked.
func (v *Vault) contained(abs string) error {
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return fmt.Errorf("resolve vault root %s: %w", v.Root, err)
	}
	for p := abs; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		switch {
		case err == nil && info.Mode()&fs.ModeSymlink != 0 && p == abs:
			return fmt.Errorf(
				"%w: %s is a symbolic link; brain reads and writes no note through a link",
				ErrInvalidPath,
				v.Rel(abs),
			)
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return err
		case err == nil:
			real, err := filepath.EvalSymlinks(p)
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: %s goes through a broken link", ErrInvalidPath, v.Rel(abs))
			}
			if err != nil {
				return err
			}
			if !within(root, real) {
				return fmt.Errorf(
					"%w: %s resolves outside the vault, to %s",
					ErrInvalidPath,
					v.Rel(abs),
					real,
				)
			}
			return nil
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}

// within reports whether path is root or lies under it, lexically.
func within(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// isLink reports whether a walk entry is a symbolic link. Walks skip them:
// a note is never read through a link.
func isLink(d fs.DirEntry) bool { return d.Type()&fs.ModeSymlink != 0 }

// Rel returns abs relative to the vault root with forward slashes, or abs
// unchanged when it is not under the root.
func (v *Vault) Rel(abs string) string {
	rel, err := filepath.Rel(v.Root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return filepath.ToSlash(rel)
}

// skipDir reports whether a vault walk should skip a directory by name.
// .brain holds the vault's policy, not notes.
func skipDir(name string) bool {
	switch name {
	case ".git", ".obsidian", ".trash", ".brain", "attachments":
		return true
	}
	return false
}
