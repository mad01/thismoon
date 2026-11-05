// Package lockfile takes cross-process advisory locks (flock) on files in
// brain's lock directory, so the CLI, the MCP server and the coming brain
// serve never build one vault's index, or rewrite its notes, at the same
// time. The locks are advisory: an editor writing the vault ignores them.
package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// DefaultTimeout is how long brain waits for a lock another brain process
// holds before HoldTimeout gives up.
const DefaultTimeout = 30 * time.Second

// retryDelay is how often HoldTimeout tries a held lock again.
const retryDelay = 50 * time.Millisecond

// ErrTimeout marks a lock HoldTimeout gave up waiting for.
var ErrTimeout = errors.New("lockfile: timed out")

// Hold blocks until it holds an exclusive lock on path, creating the file
// and its directory when missing, and returns the function that releases
// it. The lock dies with the process, so a crashed holder blocks nobody,
// but a live one that never lets go blocks Hold forever; HoldTimeout gives
// up instead.
func Hold(path string) (func() error, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	lock := flock.New(path)
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lockfile: lock %s: %w", path, err)
	}
	return lock.Unlock, nil
}

// HoldTimeout is Hold that waits at most timeout for the lock. When another
// holder keeps it longer, it fails with ErrTimeout and names the lock file.
func HoldTimeout(path string, timeout time.Duration) (func() error, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	lock := flock.New(path)
	locked, err := lock.TryLockContext(ctx, retryDelay)
	switch {
	case locked:
		return lock.Unlock, nil
	case err == nil || errors.Is(err, context.DeadlineExceeded):
		return nil, fmt.Errorf(
			"%w after %s waiting for %s: another brain process holds it "+
				"(wait for it to finish, or stop it)", ErrTimeout, timeout, path)
	default:
		return nil, fmt.Errorf("lockfile: lock %s: %w", path, err)
	}
}

func ensureDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("lockfile: create %s: %w", filepath.Dir(path), err)
	}
	return nil
}
