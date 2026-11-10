package worktree

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Lock waits are variables so a test can shorten them.
var (
	// lockWait bounds how long Ensure waits for another csl call that is
	// building the same worktree index when there is no earlier build to
	// answer from. A build reads every file of the worktree once, so a
	// large repo takes seconds, not minutes.
	lockWait = 2 * time.Minute
	// lockPoll is how often Ensure retries the lock while it waits.
	lockPoll = 200 * time.Millisecond
)

// indexWorktree indexes a worktree into a generation directory and returns
// the files it read. A test swaps it to change a file under the build.
var indexWorktree = search.IndexRepoFiles

// changeSlack widens the span a build ran over when it checks whether a
// file it read was written meanwhile. A filesystem stores mtimes at its own
// granularity (whole seconds on HFS+, two on FAT), so a write just after the
// build started can carry an earlier time. Erring wide costs one extra
// rebuild after a call that came within the slack of an edit.
const changeSlack = 2 * time.Second

// changedFingerprint is what a build records instead of its fingerprint
// when a file it read may have changed under it. A computed fingerprint is
// a hex digest, never this, so the next call rebuilds.
const changedFingerprint = "changed-during-build"

// Index is a worktree's own lexical index, current as of the Ensure call
// that returned it unless Stale is set. Close it once the last search of it
// is done.
type Index struct {
	// Dir holds the shards and state.json of one complete build; search it
	// with lexical.Local. No build writes to it.
	Dir string
	// Repo is the worktree, named org/repo@branch.
	Repo finder.Repo
	// Built is set when the call built the index.
	Built bool
	// Stale is set when another csl call is building a newer index, so this
	// one answers from the last complete build instead of waiting.
	Stale bool

	release func()
}

// Close ends the leases Ensure took, which keep pruning from removing the
// index while it is searched. It is safe on the zero Index.
func (i Index) Close() {
	if i.release != nil {
		i.release()
	}
}

// StaleNote is the line a result carries when Stale is set, "" otherwise.
func (i Index) StaleNote() string {
	if !i.Stale {
		return ""
	}
	return fmt.Sprintf(
		"another csl call is rebuilding the %s index, so these results come from its previous build and may miss the latest edits",
		i.Repo.Name,
	)
}

// Ensure returns the worktree's index, building it first when it is missing
// or the worktree changed since it was built. It leases the worktree's
// index directory before anything else, then prunes the others: the indexes
// of worktrees that are gone, and the least recently used past the cap. A
// worktree index thus outlives its worktree only until the next call or the
// next full sync. The caller must Close the returned Index.
func Ensure(wt finder.Repo, hiddenDirs []string) (Index, error) {
	root, err := search.WorktreeIndexRoot()
	if err != nil {
		return Index{}, fmt.Errorf("resolve worktree index root: %w", err)
	}
	dir := search.WorktreeIndexDir(root, wt.Path)
	release, err := search.LeaseWorktreeIndex(dir)
	if err != nil {
		return Index{}, err
	}
	// Housekeeping only: a failed prune leaves the dirs for the next run.
	_, _ = search.PruneWorktreeIndexes(root)
	idx, err := ensureIn(dir, wt, hiddenDirs)
	if err != nil {
		release()
		return Index{}, err
	}
	closeGen := idx.release
	idx.release = func() {
		closeGen()
		release()
	}
	return idx, nil
}

// ensureIn is Ensure against a given index directory, which the caller has
// leased. When the published build is out of date and another call holds
// the build lock, it answers from that build, marked Stale, and waits for
// the lock only when there is none.
func ensureIn(dir string, wt finder.Repo, hiddenDirs []string) (Index, error) {
	// Taken before the fingerprint, so any write the fingerprint may have
	// missed comes after it.
	start := time.Now()
	fp, err := fingerprint(wt.Path)
	if err != nil {
		return Index{}, fmt.Errorf("fingerprint worktree %s: %w", wt.Path, err)
	}
	gen, err := search.OpenWorktreeGeneration(dir)
	if err != nil {
		return Index{}, err
	}
	if builtFrom(gen, wt, fp) {
		// Housekeeping only: drops the builds earlier searches let go of.
		_ = search.SweepWorktreeGenerations(dir)
		return indexOf(gen, wt), nil
	}
	unlock, err := search.LockWorktreeBuild(dir)
	if errors.Is(err, search.ErrWorktreeBuilding) && gen.Dir != "" {
		idx := indexOf(gen, wt)
		idx.Stale = true
		return idx, nil
	}
	gen.Close()
	if errors.Is(err, search.ErrWorktreeBuilding) {
		unlock, err = waitForBuild(dir, wt)
	}
	if err != nil {
		return Index{}, err
	}
	defer unlock()
	return buildLocked(dir, wt, hiddenDirs, fp, start)
}

// buildLocked builds the worktree into a fresh generation and publishes it,
// unless another call published one from fp while this one waited for the
// build lock, which it holds. start is when fp began to be taken.
func buildLocked(
	dir string,
	wt finder.Repo,
	hiddenDirs []string,
	fp search.RepoState,
	start time.Time,
) (Index, error) {
	gen, err := search.OpenWorktreeGeneration(dir)
	if err != nil {
		return Index{}, err
	}
	if builtFrom(gen, wt, fp) {
		return indexOf(gen, wt), nil
	}
	gen.Close()
	gen, err = search.NewWorktreeGeneration(dir)
	if err != nil {
		return Index{}, err
	}
	if err := build(gen.Dir, wt, hiddenDirs, fp, start); err != nil {
		gen.Close()
		return Index{}, err
	}
	if err := search.PublishWorktreeGeneration(dir, gen); err != nil {
		gen.Close()
		return Index{}, err
	}
	idx := indexOf(gen, wt)
	idx.Built = true
	return idx, nil
}

// indexOf is the Index over a leased generation; closing it ends the lease.
func indexOf(gen search.WorktreeGeneration, wt finder.Repo) Index {
	return Index{Dir: gen.Dir, Repo: wt, release: gen.Close}
}

// build indexes the worktree into genDir, an empty generation, and records
// fp, the fingerprint taken from start on. When a file it read may have
// been written since start it records changedFingerprint instead, so the
// next call rebuilds: a file changed and changed back while the build read
// it leaves a clean worktree's fingerprint as it was, and the index would
// keep what the build read until some other change.
func build(
	genDir string,
	wt finder.Repo,
	hiddenDirs []string,
	fp search.RepoState,
	start time.Time,
) error {
	files, err := indexWorktree(genDir, wt, hiddenDirs)
	if err != nil {
		return fmt.Errorf("index worktree %s: %w", wt.Path, err)
	}
	if changedSince(wt.Path, files, start) {
		fp.Fingerprint = changedFingerprint
	}
	fp.IndexedAt = time.Now()
	state := search.EmptyState()
	state.SetRepo(wt.Path, fp)
	if err := state.Save(genDir); err != nil {
		return fmt.Errorf("save worktree index state: %w", err)
	}
	return nil
}

// changedSince reports whether one of files, those a build of the worktree
// at root read, may have been written since start: it is gone, or git
// tracks it and its mtime falls between start and now, each widened by
// changeSlack. An untracked file that is still there needs no check. When
// git status lists it, its size and mtime are in the fingerprint already;
// when git ignores it, no edit of it ever rebuilds the index, and checking
// it would let a process that keeps writing one (a log) force a rebuild on
// every call. An mtime past now is left out, or a file dated ahead of the
// clock would do the same. When git cannot list what it tracks, any file
// in the span counts.
func changedSince(root string, files []search.WalkFile, start time.Time) bool {
	from, to := start.Add(-changeSlack), time.Now().Add(changeSlack)
	var tracked map[string]bool
	for _, f := range files {
		info, err := os.Lstat(f.Abs)
		if err != nil {
			return true
		}
		if mtime := info.ModTime(); mtime.Before(from) || mtime.After(to) {
			continue
		}
		if tracked == nil {
			if tracked, err = search.TrackedFiles(root); err != nil {
				return true
			}
		}
		if tracked[f.Rel] {
			return true
		}
	}
	return false
}

// builtFrom reports whether gen is a build of the worktree from the
// fingerprint fp. The fingerprint covers the branch, so a build under the
// worktree's earlier name never matches.
func builtFrom(gen search.WorktreeGeneration, wt finder.Repo, fp search.RepoState) bool {
	if gen.Dir == "" {
		return false
	}
	state, err := search.LoadState(gen.Dir)
	if err != nil {
		return false
	}
	stored, ok := state.GetRepo(wt.Path)
	return ok && stored.Fingerprint == fp.Fingerprint
}

// waitForBuild waits up to lockWait for the build lock another csl call
// holds on dir, for a caller with no earlier build to answer from.
func waitForBuild(dir string, wt finder.Repo) (func(), error) {
	deadline := time.Now().Add(lockWait)
	for {
		time.Sleep(lockPoll)
		unlock, err := search.LockWorktreeBuild(dir)
		if !errors.Is(err, search.ErrWorktreeBuilding) {
			return unlock, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"worktree %s: another csl call has been building its index for over %s and no earlier build exists to answer from; "+
					"retry once it finishes, or leave the worktree out to search the main index",
				wt.Name,
				lockWait,
			)
		}
	}
}

// fingerprint is the main index's fingerprint with the size and mtime of
// every changed file mixed in. git status alone stays the same while an
// already modified file is edited again, which in a worktree under active
// edit is the common case.
func fingerprint(path string) (search.RepoState, error) {
	fp, err := search.Fingerprint(path)
	if err != nil {
		return search.RepoState{}, err
	}
	if !fp.Dirty {
		return fp, nil
	}
	sig, err := changedFilesSignature(path)
	if err != nil {
		return search.RepoState{}, err
	}
	sum := sha256.Sum256([]byte(fp.Fingerprint + "\n" + sig))
	fp.Fingerprint = fmt.Sprintf("%x", sum)
	return fp, nil
}

// changedFilesSignature lists every changed or untracked file git status
// reports, each with its size and modification time, one per line. A
// deleted file shows as "-".
func changedFilesSignature(root string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status in %s: %w", root, err)
	}
	var b bytes.Buffer
	for _, p := range statusPaths(out) {
		info, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			fmt.Fprintf(&b, "%s -\n", p)
			continue
		}
		fmt.Fprintf(&b, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano())
	}
	return b.String(), nil
}

// statusPaths returns the paths of `git status --porcelain=v1 -z` output.
// Each entry is "XY path"; a rename or copy carries its source path as the
// next NUL-separated field, which is skipped.
func statusPaths(out []byte) []string {
	fields := bytes.Split(out, []byte{0})
	var paths []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		paths = append(paths, string(f[3:]))
		if f[0] == 'R' || f[0] == 'C' {
			i++
		}
	}
	return paths
}
