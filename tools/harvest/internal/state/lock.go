package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrRunning means another process holds the session's lock: a harvest of
// that session is running.
var ErrRunning = errors.New("state: a harvest of this session is running")

// Lock takes the lock of session without waiting, or fails with ErrRunning.
// A run holds it from before it resets the session's job dir until after it
// removes it, so two runs of one session never share a job dir. The
// returned func releases it. The lock is a flock, which the kernel releases
// when the process exits, so a run that was killed blocks no later one.
//
// The lock file stays when the lock is released: removing it would let a
// run that opened the old file lock it while a later run locks a new one.
func (d *Dir) Lock(session string) (func(), error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	dir := filepath.Join(d.path, locksDir)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, session+lockSuffix), os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, fmt.Errorf("state: open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunning
		}
		return nil, fmt.Errorf("state: lock %s: %w", session, err)
	}
	// Closing the file releases the lock; there is nothing to report.
	return func() { _ = f.Close() }, nil
}

// SweepJobs removes the job dir of every session whose lock no process
// holds: what a run that was killed left behind, a session's text among it.
// It takes each lock only while it removes that dir, and leaves alone a
// name that is not a session id, which no run can lock.
func (d *Dir) SweepJobs() error {
	jobs := filepath.Join(d.path, jobsDir)
	entries, err := os.ReadDir(jobs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if CheckSession(e.Name()) != nil {
			continue
		}
		release, err := d.Lock(e.Name())
		if errors.Is(err, ErrRunning) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.RemoveAll(filepath.Join(jobs, e.Name())); err != nil {
			errs = append(errs, fmt.Errorf("state: sweep job %s: %w", e.Name(), err))
		}
		release()
	}
	return errors.Join(errs...)
}
