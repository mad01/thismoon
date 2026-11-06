package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readNote reads the note at the vault-relative path rel under root. Like
// the walks that build the index, it never reads a note through a symbolic
// link: a path whose file, or any directory below root on the way to it, is
// a link fails, so a note swapped for a link after it was indexed reads as
// missing.
func readNote(root, rel string) ([]byte, error) {
	path := root
	for _, part := range strings.Split(filepath.FromSlash(rel), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s is a symbolic link; brain reads no note through a link", path)
		}
	}
	return os.ReadFile(path)
}
