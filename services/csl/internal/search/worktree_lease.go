package search

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// worktreeLeaseFile is the file a caller holds a shared flock on while it
// uses a worktree index directory, or one generation inside it. A remover
// takes it exclusive without waiting and skips a directory it cannot take,
// so nothing in use is removed. The worktree directory's lease file mtime
// records the index's last use, the order eviction follows. The kernel
// drops a flock when its process exits, so a caller that dies never pins
// an index.
const worktreeLeaseFile = ".csl-in-use"

// tombstonePrefix starts the name a directory gets just before it is
// deleted. No caller ever opens a path under a tombstone, so a delete can
// take its time; a tombstone a crash left behind goes on the next prune or
// sweep.
const tombstonePrefix = ".tomb-"

// maxWorktreeIndexes caps how many worktree indexes stay on disk. Each is a
// full lexical copy of its repo (about 40 MB for a mid-size repo, far more
// for a large one), and a machine runs a handful of agent worktrees at a
// time. Sixteen keeps every active worktree's index warm while bounding the
// disk to sixteen repo copies; past it the least recently used index goes,
// and its worktree rebuilds on the next call that names it. Indexes in use
// are never evicted, so the count can sit above the cap while they are.
const maxWorktreeIndexes = 16

// leaseAttempts bounds how often LeaseWorktreeIndex starts over when a
// remover renamed the directory aside between its open and its lock.
const leaseAttempts = 3

// errLeaseRemoved is what LeaseWorktreeIndex fails with when removers took
// the directory away on every attempt.
var errLeaseRemoved = fmt.Errorf("removed %d times while taking it", leaseAttempts)

// LeaseWorktreeIndex creates the worktree index directory dir when missing,
// takes a shared lease on it, and marks it used now. Hold the lease from
// before the index is built or opened until the last search of it is done;
// the returned func ends it.
func LeaseWorktreeIndex(dir string) (func(), error) {
	path := filepath.Join(dir, worktreeLeaseFile)
	for range leaseAttempts {
		err := os.MkdirAll(dir, 0o755)
		if errors.Is(err, fs.ErrExist) && !isFile(dir) {
			// Another caller made dir and a remover renamed it aside
			// between MkdirAll's mkdir and its check that dir exists.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create worktree index dir %s: %w", dir, err)
		}
		f, err := lockShared(path, true)
		if err != nil {
			return nil, err
		}
		if f != nil {
			now := time.Now()
			_ = os.Chtimes(path, now, now) // the last-use time; eviction order only
			return func() { _ = f.Close() }, nil
		}
	}
	return nil, fmt.Errorf("lease worktree index %s: %w", dir, errLeaseRemoved)
}

// isFile reports whether path exists as something other than a directory.
func isFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && !info.IsDir()
}

// lockShared opens the lease file at path, creating it when create is set,
// and takes a shared lock on it, waiting out a remover's brief exclusive
// one. A nil file and no error mean the lease is gone: its directory was
// renamed aside before the lock landed, and the caller starts over.
func lockShared(path string, create bool) (*os.File, error) {
	flag := os.O_RDWR
	if create {
		flag |= os.O_CREATE
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open worktree index lease %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock worktree index lease %s: %w", path, err)
	}
	if !stillAt(f, path) {
		_ = f.Close()
		return nil, nil
	}
	return f, nil
}

// stillAt reports whether path still names the file f has open.
func stillAt(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(held, current)
}

// removeUnleased deletes dir, a worktree index directory or a generation in
// one, unless a caller holds its lease. With the lease held exclusive it
// first renames dir to a tombstone, then deletes the tombstone: a caller
// that opened the lease before the rename finds it gone once its own lock
// lands (stillAt) and starts over on a fresh directory, and none can open
// anything inside the tombstone while it is deleted. It reports whether it
// removed dir; one that is gone already counts as not removed and no error.
// root is the worktree index root, the only tree it may delete from.
func removeUnleased(root, dir string) (bool, error) {
	if err := checkUnder(root, dir); err != nil {
		return false, err
	}
	path := filepath.Join(dir, worktreeLeaseFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open worktree index lease %s: %w", path, err)
	}
	tomb, err := renameUnleased(f, path, dir)
	_ = f.Close()
	if err != nil || tomb == "" {
		return false, err
	}
	return true, removeUnder(root, tomb)
}

// renameUnleased renames dir to a fresh tombstone beside it when the
// exclusive lock on f, dir's lease file at path, lands at once and f is
// still that lease. It returns the tombstone, or "" when a caller holds the
// lease or another remover got there first.
func renameUnleased(f *os.File, path, dir string) (string, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lock worktree index lease %s: %w", path, err)
	}
	if !stillAt(f, path) {
		return "", nil
	}
	tomb := filepath.Join(filepath.Dir(dir), tombstonePrefix+filepath.Base(dir)+"-"+rand.Text())
	if err := os.Rename(dir, tomb); err != nil {
		return "", fmt.Errorf("move worktree index %s aside: %w", dir, err)
	}
	return tomb, nil
}

// removeUnder deletes path and everything below it, after checkUnder.
func removeUnder(root, path string) error {
	if err := checkUnder(root, path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// checkUnder refuses any delete but one strictly inside root, which must be
// a directory named worktreeIndexDirName. Every delete of worktree index
// files goes through it: the main index sits beside that root, so a wrong
// root or a path that climbs out of it must fail here, never reach
// os.RemoveAll.
func checkUnder(root, path string) error {
	if filepath.Base(root) != worktreeIndexDirName {
		return fmt.Errorf(
			"refusing to remove %s: %s is not a worktree index root (%s)",
			path,
			root,
			worktreeIndexDirName,
		)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf(
			"refusing to remove %s: not inside the worktree index root %s",
			path,
			root,
		)
	}
	return nil
}

// indexUse is a worktree index directory and when a caller last leased it.
type indexUse struct {
	dir     string
	lastUse time.Time
}

// lastUse reads when dir was last leased: its lease file's mtime, or the
// directory's own for an index no lease has touched.
func lastUse(dir string) time.Time {
	if info, err := os.Stat(filepath.Join(dir, worktreeLeaseFile)); err == nil {
		return info.ModTime()
	}
	if info, err := os.Stat(dir); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// evictLeastRecentlyUsed removes the least recently used indexes under root
// until at most limit remain, skipping any a caller holds a lease on. It
// returns the removed directories.
func evictLeastRecentlyUsed(root string, uses []indexUse, limit int) ([]string, error) {
	excess := len(uses) - limit
	if excess <= 0 {
		return nil, nil
	}
	slices.SortFunc(uses, func(a, b indexUse) int { return a.lastUse.Compare(b.lastUse) })
	var removed []string
	for _, u := range uses {
		if len(removed) == excess {
			break
		}
		ok, err := removeUnleased(root, u.dir)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, u.dir)
		}
	}
	return removed, nil
}
