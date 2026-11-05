package vault

import (
	"fmt"
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
	if full != v.Root && !strings.HasPrefix(full, v.Root+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes vault: %s", ErrInvalidPath, rel)
	}
	return full, nil
}

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
func skipDir(name string) bool {
	switch name {
	case ".git", ".obsidian", ".trash", "attachments":
		return true
	}
	return false
}
