package store

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// withPageLock runs fn while holding an exclusive flock on the page's
// directory. Every writer of a page takes it: the MCP server, serve, and the
// CLI are separate processes over one workdir. The lock belongs to the open
// file, not the process, so it orders goroutines of one process too. A page
// whose directory does not exist is ErrNotFound and fn never runs, so a
// writer that lost a race with Delete cannot bring the directory back.
func (s *FS) withPageLock(id string, fn func() error) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	dir, err := os.Open(s.pageDir(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("open page dir: %w", err)
	}
	// Closing the only descriptor of the open directory releases the lock.
	defer dir.Close()
	if err := lockExclusive(dir); err != nil {
		return fmt.Errorf("lock page dir: %w", err)
	}
	return fn()
}

// lockExclusive blocks until f carries an exclusive flock, retrying a call
// a signal interrupted: the Go runtime preempts goroutines with signals,
// and a blocked flock returns EINTR when one lands.
func lockExclusive(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
