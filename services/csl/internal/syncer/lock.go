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

// ErrLocked reports that another index writer already holds the sync lock: a
// `csl sync` in a terminal, the background refresher in `csl web`, a `csl
// index` run, or a csl_repo_reindex call. Callers match it with errors.Is and
// fail fast or retry later instead of proceeding unsynchronized.
var ErrLocked = errors.New("syncer: another sync is running")

// Locked reports whether a sync lock file currently exists in indexDir. It is
// a cheap probe for tests and diagnostics only; Lock remains the authoritative
// gate, and every writer goes through it.
func Locked(indexDir string) bool {
	_, err := os.Stat(filepath.Join(indexDir, syncLockFile))
	return err == nil
}

// Lock acquires the cross-process sync lock in indexDir and returns the func
// that releases it. Every writer of shards or state.json must hold it: Run
// takes it for the whole pull + index phase, and the ad-hoc index writers
// (`csl index` with and without --repo or --drain, the search-time reindex,
// the web fallback's first build, the csl_repo_reindex tool) take it around
// their writes. A live holder makes Lock fail with an error matching ErrLocked
// that names the holder's PID; a lock left by a dead process is removed and
// re-acquired.
func Lock(indexDir string) (unlock func(), err error) {
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(indexDir, syncLockFile)
	f, err := createLockFile(lockPath)
	if os.IsExist(err) {
		if pid, live := liveHolder(lockPath); live {
			return nil, fmt.Errorf("%w: held by PID %d (see %s)", ErrLocked, pid, lockPath)
		}
		if f, err = createLockFile(lockPath); err != nil {
			return nil, fmt.Errorf("acquire sync lock after stale removal: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	// Write our PID so others can detect staleness.
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

func createLockFile(lockPath string) (*os.File, error) {
	return os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

// liveHolder reads the PID recorded in lockPath and reports whether that
// process still runs. A lock with no parseable PID (the pre-PID format) or a
// dead owner is stale: it is removed so the caller can re-acquire, and live is
// false. A lock that vanished since the caller saw it is not live either. An
// unreadable lock counts as live with PID 0, since removing a lock nobody can
// inspect is riskier than failing fast.
func liveHolder(lockPath string) (pid int, live bool) {
	data, err := os.ReadFile(lockPath)
	if os.IsNotExist(err) {
		return 0, false
	}
	if err != nil {
		return 0, true
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && processAlive(pid) {
		return pid, true
	}
	_ = os.Remove(lockPath)
	return 0, false
}

// processAlive reports whether pid names a running process. Signal 0 checks
// existence without delivering anything.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
