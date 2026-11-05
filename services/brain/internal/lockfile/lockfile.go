// Package lockfile takes cross-process advisory locks (flock) on files in
// brain's lock directory, so the CLI, the MCP server and the coming brain
// serve never build one vault's index, or rewrite its notes, at the same
// time. The locks are advisory: an editor writing the vault ignores them.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// Hold blocks until it holds an exclusive lock on path, creating the file
// and its directory when missing, and returns the function that releases
// it. The lock dies with the process, so a crashed holder blocks nobody.
func Hold(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lockfile: create %s: %w", filepath.Dir(path), err)
	}
	lock := flock.New(path)
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lockfile: lock %s: %w", path, err)
	}
	return lock.Unlock, nil
}
