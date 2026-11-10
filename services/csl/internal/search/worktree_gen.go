package search

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// A worktree index directory keeps each build in a generation of its own, so
// a build never writes where a search reads:
//
//	<root>/<base>-<hash>/
//	  .csl-in-use   lease on the whole index (LeaseWorktreeIndex)
//	  .csl-build    build lock, one builder at a time (LockWorktreeBuild)
//	  current       names the published generation
//	  g-<random>/   one complete build: shards, state.json and a
//	                .csl-in-use each caller searching it holds
//
// A build fills a fresh generation and publishes it by replacing current in
// one rename. A caller resolves current once and searches that generation
// for the whole call. A generation that is no longer current goes once no
// caller holds its lease.
const (
	worktreeBuildFile   = ".csl-build"
	worktreeCurrentFile = "current"
	worktreeGenPrefix   = "g-"
)

// generationAttempts bounds how often OpenWorktreeGeneration re-reads
// current when the generation it named was swept before its lease landed.
// Each retry means a newer build was published in between.
const generationAttempts = 5

// ErrWorktreeBuilding is the error LockWorktreeBuild returns while another
// caller holds the build lock.
var ErrWorktreeBuilding = errors.New("another csl call is building this worktree index")

// LockWorktreeBuild takes the build lock of the worktree index directory
// dir without waiting, or fails with ErrWorktreeBuilding. Only the holder
// makes, publishes or sweeps generations. The caller holds a lease on dir
// (LeaseWorktreeIndex), which also created it. The returned func releases
// the lock; the kernel releases it when the process exits.
func LockWorktreeBuild(dir string) (func(), error) {
	path := filepath.Join(dir, worktreeBuildFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open worktree build lock %s: %w", path, err)
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrWorktreeBuilding
		}
		return nil, fmt.Errorf("lock worktree build %s: %w", path, err)
	}
	return func() { _ = f.Close() }, nil
}

// WorktreeGeneration is one build of a worktree index, leased by the caller
// that opened or made it, so no sweep removes it while that caller searches
// it. The zero value means there is no build yet.
type WorktreeGeneration struct {
	// Dir holds the shards and state.json; search it with lexical.Local.
	Dir string

	release func()
}

// Close ends the lease. It is safe on the zero value.
func (g WorktreeGeneration) Close() {
	if g.release != nil {
		g.release()
	}
}

// OpenWorktreeGeneration leases the generation the worktree index directory
// dir publishes. It returns the zero value when none is published yet.
func OpenWorktreeGeneration(dir string) (WorktreeGeneration, error) {
	for range generationAttempts {
		name, err := currentGeneration(dir)
		if err != nil || name == "" {
			return WorktreeGeneration{}, err
		}
		gen := filepath.Join(dir, name)
		f, err := lockShared(filepath.Join(gen, worktreeLeaseFile), false)
		if err != nil {
			return WorktreeGeneration{}, err
		}
		if f != nil {
			return WorktreeGeneration{Dir: gen, release: func() { _ = f.Close() }}, nil
		}
	}
	return WorktreeGeneration{}, fmt.Errorf(
		"open worktree index %s: a newer build replaced it %d times while opening it",
		dir,
		generationAttempts,
	)
}

// NewWorktreeGeneration makes an empty generation in the worktree index
// directory dir for a build to fill, leased by the caller, after sweeping
// the generations nobody uses (a failed build's among them). Call it under
// the build lock, and PublishWorktreeGeneration once the build is complete.
func NewWorktreeGeneration(dir string) (WorktreeGeneration, error) {
	// Housekeeping only: what the sweep leaves goes on a later one.
	_ = sweepGenerations(dir)
	gen, err := os.MkdirTemp(dir, worktreeGenPrefix)
	if err != nil {
		return WorktreeGeneration{}, fmt.Errorf(
			"make worktree index generation in %s: %w",
			dir,
			err,
		)
	}
	f, err := lockShared(filepath.Join(gen, worktreeLeaseFile), true)
	if err != nil {
		return WorktreeGeneration{}, err
	}
	if f == nil {
		return WorktreeGeneration{}, fmt.Errorf(
			"make worktree index generation %s: removed while leasing it",
			gen,
		)
	}
	return WorktreeGeneration{Dir: gen, release: func() { _ = f.Close() }}, nil
}

// PublishWorktreeGeneration makes gen, a complete build, the generation the
// worktree index directory dir answers from: it writes the new current
// beside the old one and renames it over, so a reader sees one or the other
// whole. It then sweeps the generations no caller holds. Call it under the
// build lock.
func PublishWorktreeGeneration(dir string, gen WorktreeGeneration) error {
	tmp, err := os.CreateTemp(dir, worktreeCurrentFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("publish worktree index %s: %w", gen.Dir, err)
	}
	_, err = tmp.WriteString(filepath.Base(gen.Dir) + "\n")
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, worktreeCurrentFile))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("publish worktree index %s: %w", gen.Dir, err)
	}
	// Housekeeping only: a generation still searched goes on a later sweep.
	_ = sweepGenerations(dir)
	return nil
}

// SweepWorktreeGenerations removes the generations of the worktree index
// directory dir that are neither current nor leased. While a build holds
// the lock it does nothing: that build sweeps once it publishes.
func SweepWorktreeGenerations(dir string) error {
	unlock, err := LockWorktreeBuild(dir)
	if errors.Is(err, ErrWorktreeBuilding) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unlock()
	return sweepGenerations(dir)
}

// sweepGenerations removes every entry of the worktree index directory dir
// but its lease, its build lock, current, the current generation and the
// generations a caller holds: superseded and unpublished generations, and
// the tombstones and temp files a crash left. The caller holds the build
// lock, so no generation is being filled.
func sweepGenerations(dir string) error {
	current, err := currentGeneration(dir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read worktree index %s: %w", dir, err)
	}
	root := filepath.Dir(dir)
	var errs []error
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		switch {
		case name == worktreeLeaseFile, name == worktreeBuildFile,
			name == worktreeCurrentFile, name == current:
		case e.IsDir() && !strings.HasPrefix(name, tombstonePrefix):
			_, err := removeUnleased(root, path)
			errs = append(errs, err)
		default:
			errs = append(errs, removeUnder(root, path))
		}
	}
	return errors.Join(errs...)
}

// currentGeneration reads the generation name current holds: "" when there
// is none, or when it holds anything but a generation name, which the next
// build then replaces.
func currentGeneration(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, worktreeCurrentFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read worktree index %s: %w", dir, err)
	}
	name := strings.TrimSpace(string(data))
	if !strings.HasPrefix(name, worktreeGenPrefix) || filepath.Base(name) != name {
		return "", nil
	}
	return name, nil
}
