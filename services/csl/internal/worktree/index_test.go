package worktree

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/worktree/worktreetest"
)

// shortLockWait makes a wait for another build fail or retry fast.
func shortLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	prevWait, prevPoll := lockWait, lockPoll
	lockWait, lockPoll = wait, 10*time.Millisecond
	t.Cleanup(func() { lockWait, lockPoll = prevWait, prevPoll })
}

// fixtureWorktree is a resolved worktree in an isolated state dir, its
// files settled.
func fixtureWorktree(t *testing.T) (worktreetest.Fixture, finder.Repo) {
	t.Helper()
	isolateState(t)
	f := worktreetest.New(t, t.TempDir(), "feat")
	settle(t, f.Worktree)
	wt, err := Resolve(configFor(f), f.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	return f, wt
}

// settle dates every file under dir an hour back, as in a worktree whose
// checkout and last edit are a while behind it, so a build does not read
// them as written while it ran (changeSlack) and leaves its fingerprint
// for the next call to match.
func settle(t *testing.T, dir string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// holdBuild takes the build lock of the worktree's index the way another
// csl call's build does, and returns the func that ends it.
func holdBuild(t *testing.T, wt finder.Repo) func() {
	t.Helper()
	root, err := search.WorktreeIndexRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := search.WorktreeIndexDir(root, wt.Path)
	release, err := search.LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := search.LockWorktreeBuild(dir)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		unlock()
		release()
	}
}

// edit rewrites main.go with a function named name, so each edit changes
// the file's size and the fingerprint. It writes beside the worktree and
// renames the file in, so a build running at the same time reads the old
// content or the new, never a half-written file: these tests check what a
// search sees of the index, not of a file mid-write. It reports failures
// with Errorf, since the rebuild test calls it from a goroutine.
func edit(t *testing.T, f worktreetest.Fixture, name string) {
	t.Helper()
	content := "package main\n\nfunc " + name + "() {}\n"
	tmp, err := os.CreateTemp(filepath.Dir(f.Worktree), "edit-*.go")
	if err != nil {
		t.Errorf("edit: %v", err)
		return
	}
	_, err = tmp.WriteString(content)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(f.Worktree, "main.go"))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		t.Errorf("edit: %v", err)
	}
}

// hits searches the index for pattern and returns the files that match.
func hits(t *testing.T, idx Index, pattern string) []string {
	t.Helper()
	matches, err := search.Search(
		context.Background(),
		idx.Dir,
		search.SearchOptions{Pattern: pattern},
		map[string]string{idx.Repo.Name: idx.Repo.Path},
	)
	if err != nil {
		t.Errorf("search %q: %v", pattern, err)
		return nil
	}
	files := make([]string, 0, len(matches))
	for _, m := range matches {
		files = append(files, m.File)
	}
	return files
}

// TestEnsure_ConcurrentCallsBuildOnce: two calls for one worktree at once
// both get the index, and only one of them builds it; the other waits for
// that build, since there is no earlier one to answer from.
func TestEnsure_ConcurrentCallsBuildOnce(t *testing.T) {
	_, wt := fixtureWorktree(t)
	shortLockWait(t, time.Minute)

	const callers = 2
	results := make([]Index, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Go(func() {
			<-start
			results[i], errs[i] = Ensure(wt, nil)
		})
	}
	close(start)
	wg.Wait()

	built := 0
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		defer results[i].Close()
		if results[i].Stale {
			t.Errorf("caller %d answered stale with no earlier build", i)
		}
		if results[i].Built {
			built++
		}
		if got := hits(t, results[i], "MainOnly"); len(got) != 1 {
			t.Errorf("caller %d: MainOnly hits %v, want main.go", i, got)
		}
	}
	if built != 1 {
		t.Errorf("%d callers built the index, want 1", built)
	}
}

// TestEnsure_StaleWhileAnotherBuilds: while another call holds the build
// lock, a caller with an earlier complete build answers from it at once,
// marked stale, and the next call after the lock frees rebuilds.
func TestEnsure_StaleWhileAnotherBuilds(t *testing.T) {
	f, wt := fixtureWorktree(t)
	edit(t, f, "FirstEdit")
	first := mustEnsure(t, wt, true)

	edit(t, f, "SecondEditLonger")
	unlock := holdBuild(t, wt)    // another call's build
	shortLockWait(t, time.Minute) // a wait would show as a slow test, not a pass
	began := time.Now()
	idx, err := Ensure(wt, nil)
	if err != nil {
		t.Fatalf("Ensure during another build: %v", err)
	}
	if !idx.Stale || idx.Built || time.Since(began) > 10*time.Second {
		t.Errorf(
			"stale %v, built %v after %s; want a stale answer at once",
			idx.Stale,
			idx.Built,
			time.Since(began),
		)
	}
	if note := idx.StaleNote(); !strings.Contains(note, "previous build") {
		t.Errorf("stale note = %q, want it to say the results come from the previous build", note)
	}
	if got := hits(t, idx, "FirstEdit"); len(got) != 1 || idx.Dir != first.Dir {
		t.Errorf(
			"stale index %s: FirstEdit hits %v, want main.go from the previous build %s",
			idx.Dir,
			got,
			first.Dir,
		)
	}
	idx.Close()
	unlock()

	fresh := mustEnsure(t, wt, true)
	if fresh.Dir == first.Dir {
		t.Errorf("the rebuild wrote into the previous build's directory %s", first.Dir)
	}
	if fresh.StaleNote() != "" {
		t.Errorf("a rebuilt index carries a stale note: %q", fresh.StaleNote())
	}
	if got := hits(t, fresh, "SecondEditLonger"); len(got) != 1 {
		t.Errorf("rebuilt index: SecondEditLonger hits %v, want main.go", got)
	}
}

// TestEnsure_WaitTimeoutNamesTheBuild: with no earlier build to answer from,
// a caller waits for the other build and, past the wait, says so in its own
// words rather than the sync lock's.
func TestEnsure_WaitTimeoutNamesTheBuild(t *testing.T) {
	_, wt := fixtureWorktree(t)
	defer holdBuild(t, wt)()
	shortLockWait(t, 100*time.Millisecond)

	_, err := Ensure(wt, nil)
	if err == nil {
		t.Fatal("Ensure with the build lock held and no earlier build: no error")
	}
	msg := err.Error()
	for _, want := range []string{wt.Name, "building its index", "leave the worktree out"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "another sync is running") {
		t.Errorf("error %q names the sync lock instead of the worktree build", msg)
	}
}

// TestEnsure_SearchDuringRebuild: searches that run while the worktree is
// edited and its index rebuilt always find the file, from the build before
// or after, and never fail. Run it with -race.
func TestEnsure_SearchDuringRebuild(t *testing.T) {
	f, wt := fixtureWorktree(t)
	shortLockWait(t, time.Minute)
	edit(t, f, "Edit0")
	mustEnsure(t, wt, true)

	const rounds = 8
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		defer close(done)
		for i := 1; i <= rounds; i++ {
			edit(t, f, fmt.Sprintf("Edit%d%s", i, strings.Repeat("x", i)))
			idx, err := Ensure(wt, nil)
			if err != nil {
				t.Errorf("rebuild %d: %v", i, err)
				return
			}
			idx.Close()
		}
	})
	searches := 0
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			idx, err := Ensure(wt, nil)
			if err != nil {
				t.Errorf("search %d: Ensure: %v", searches, err)
				return
			}
			if got := hits(t, idx, `func Edit\d`); len(got) != 1 {
				t.Errorf("search %d (stale %v): hits %v, want main.go", searches, idx.Stale, got)
			}
			idx.Close()
			searches++
		}
	})
	wg.Wait()
	if searches == 0 {
		t.Error("no search ran during the rebuilds")
	}
}

// TestEnsure_FileChangedDuringBuild: an editor truncates main.go in a clean
// worktree, the build reads it empty, and the editor writes the same bytes
// back. The worktree is clean again and its fingerprint as it was, so only
// the build's own check can tell; the next call must rebuild and find the
// content the first build missed.
func TestEnsure_FileChangedDuringBuild(t *testing.T) {
	f, wt := fixtureWorktree(t)
	path := filepath.Join(f.Worktree, "main.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := fingerprint(wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	index := indexWorktree
	t.Cleanup(func() { indexWorktree = index })
	indexWorktree = func(genDir string, repo finder.Repo, hiddenDirs []string) ([]search.WalkFile, error) {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return nil, err
		}
		files, err := index(genDir, repo, hiddenDirs)
		if restoreErr := os.WriteFile(path, content, 0o644); err == nil {
			err = restoreErr
		}
		return files, err
	}

	first, err := Ensure(wt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hits(t, first, "MainOnly"); len(got) != 0 {
		t.Fatalf("the first build found MainOnly in %v; it should have read main.go empty", got)
	}
	first.Close()
	indexWorktree = index
	if after, err := fingerprint(wt.Path); err != nil || after.Fingerprint != before.Fingerprint {
		t.Fatalf("fingerprint moved (err %v): the worktree is not as it was before the build", err)
	}

	again, err := Ensure(wt, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !again.Built {
		t.Error("the next call reused the build that read main.go mid-edit")
	}
	if got := hits(t, again, "MainOnly"); len(got) != 1 {
		t.Errorf("rebuilt index: MainOnly hits %v, want main.go", got)
	}
}
