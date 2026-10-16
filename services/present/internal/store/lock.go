package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withPageLock runs fn while holding an exclusive flock on the page's
// directory. Every writer of a page takes it: the MCP server, serve, and the
// CLI are separate processes over one workdir. The lock belongs to the open
// file, not the process, so it orders goroutines of one process too. A page
// that is gone once the lock is held is ErrNotFound and fn never runs, so a
// writer that waited out a Delete cannot bring the directory back.
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
	if err := s.stillPresent(id, dir); err != nil {
		return err
	}
	return fn()
}

// stillPresent runs after the lock is granted and reports ErrNotFound when
// the page went away while this writer waited: its meta.json is gone, or a
// directory created since sits at its path that the held lock does not
// cover.
func (s *FS) stillPresent(id string, locked *os.File) error {
	held, err := locked.Stat()
	if err != nil {
		return fmt.Errorf("stat locked page dir: %w", err)
	}
	current, err := os.Stat(s.pageDir(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("stat page dir: %w", err)
	}
	if !os.SameFile(held, current) {
		return ErrNotFound
	}
	_, err = os.Stat(filepath.Join(s.pageDir(id), metaFile))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("stat meta: %w", err)
	}
	return nil
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
