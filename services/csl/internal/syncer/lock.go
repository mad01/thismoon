package syncer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const syncLockFile = ".csl-sync.lock"

// ErrLocked reports that another sync run — a `csl sync` in a terminal or the
// background refresher in `csl web` — already holds the lock. Callers match it
// with errors.Is and retry later instead of proceeding unsynchronized.
var ErrLocked = errors.New("syncer: another sync is running")

// Locked reports whether a sync lock file currently exists in indexDir. It is
// the cheap pre-check used by ad-hoc single-repo indexing to step aside while
// a sync runs; Lock remains the authoritative gate.
func Locked(indexDir string) bool {
	_, err := os.Stat(filepath.Join(indexDir, syncLockFile))
	return err == nil
}

// Lock acquires the cross-process sync lock in indexDir and returns the func
// that releases it. Every writer of shards or state.json must hold it: Run
// takes it for the whole pull + index phase, and the ad-hoc index builds
// (search-time reindex, the web fallback's first build) take it around their
// writes. A live holder makes Lock fail with an error matching ErrLocked; a
// lock left by a dead process is removed and re-acquired.
func Lock(indexDir string) (unlock func(), err error) {
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(indexDir, syncLockFile)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// Lock file exists — check if the owning process is still alive.
		if os.IsExist(err) {
			if removeStaleLock(lockPath) {
				// Retry after removing stale lock.
				f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					return nil, fmt.Errorf("acquire sync lock after stale removal: %w", err)
				}
			} else {
				return nil, fmt.Errorf("%w: lock held by a live process (see %s)", ErrLocked, lockPath)
			}
		} else {
			return nil, err
		}
	}
	// Write our PID so others can detect staleness.
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

func removeStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		// No PID written (old format) — assume stale, remove.
		_ = os.Remove(lockPath)
		return true
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		_ = os.Remove(lockPath)
		return true
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(lockPath)
		return true
	}
	// Signal 0 checks if the process exists without actually signaling it.
	if proc.Signal(syscall.Signal(0)) != nil {
		_ = os.Remove(lockPath)
		return true
	}
	return false
}
