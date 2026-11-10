package search

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorktreeIndexDir: the directory name starts with the worktree's base
// name, and two worktrees sharing a base name get different directories.
func TestWorktreeIndexDir(t *testing.T) {
	root := t.TempDir()
	a := WorktreeIndexDir(root, "/w/one/feat")
	b := WorktreeIndexDir(root, "/w/two/feat")
	if filepath.Dir(a) != root || !strings.HasPrefix(filepath.Base(a), "feat-") {
		t.Errorf("dir %s: want <root>/feat-<hash>", a)
	}
	if a == b {
		t.Errorf("two worktrees named feat share %s", a)
	}
	if again := WorktreeIndexDir(root, "/w/one/feat/"); again != a {
		t.Errorf("trailing slash moved the dir: %s vs %s", again, a)
	}
	if hidden := filepath.Base(WorktreeIndexDir(root, "/w/.tomb-x")); strings.HasPrefix(
		hidden,
		".",
	) {
		t.Errorf("a dot-named worktree got %s, which reads as a tombstone", hidden)
	}
}

// TestPruneWorktreeIndexes: an index whose worktree has a .git entry stays;
// one whose worktree is gone goes; a directory that publishes no build stays
// while young (a first build may be running) and goes once abandoned; a
// tombstone a crash left goes.
func TestPruneWorktreeIndexes(t *testing.T) {
	root := testRoot(t)
	live := t.TempDir()
	if err := os.WriteFile(filepath.Join(live, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	liveDir := publishFor(t, root, live)
	goneDir := publishFor(t, root, filepath.Join(t.TempDir(), "removed"))
	building := filepath.Join(root, "building-0000")
	abandoned := filepath.Join(root, "abandoned-0000")
	tomb := filepath.Join(root, tombstonePrefix+"crashed-0000-x")
	for _, d := range []string{building, abandoned, filepath.Join(tomb, "g-1")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * abandonedBuildAge)
	if err := os.Chtimes(abandoned, old, old); err != nil {
		t.Fatal(err)
	}

	removed, err := PruneWorktreeIndexes(root)
	if err != nil {
		t.Fatalf("PruneWorktreeIndexes: %v", err)
	}
	slices.Sort(removed)
	want := []string{abandoned, goneDir}
	slices.Sort(want)
	if !slices.Equal(removed, want) {
		t.Errorf("removed %v, want %v", removed, want)
	}
	for _, d := range []string{liveDir, building} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s was pruned: %v", d, err)
		}
	}
	if left := entries(t, root); slices.ContainsFunc(left, isTombstone) {
		t.Errorf("tombstones left in the root: %v", left)
	}
	if got, err := PruneWorktreeIndexes(filepath.Join(root, "missing")); err != nil || got != nil {
		t.Errorf("missing root: removed %v, err %v; want nothing", got, err)
	}
}

// TestPruneWorktreeIndexes_SkipsLeased: an index a caller holds a lease on
// stays even when its worktree is gone, and goes once the lease ends.
func TestPruneWorktreeIndexes_SkipsLeased(t *testing.T) {
	root := testRoot(t)
	goneDir := publishFor(t, root, filepath.Join(t.TempDir(), "removed"))
	release, err := LeaseWorktreeIndex(goneDir)
	if err != nil {
		t.Fatal(err)
	}
	if removed, err := PruneWorktreeIndexes(root); err != nil || len(removed) != 0 {
		t.Fatalf("with a lease held: removed %v, err %v; want nothing", removed, err)
	}
	release()
	removed, err := PruneWorktreeIndexes(root)
	if err != nil || !slices.Equal(removed, []string{goneDir}) {
		t.Errorf("after release: removed %v, err %v; want [%s]", removed, err, goneDir)
	}
}

// TestEvictLeastRecentlyUsed: past the limit the least recently leased
// indexes go first, and a leased one is skipped for the next oldest.
func TestEvictLeastRecentlyUsed(t *testing.T) {
	root := testRoot(t)
	base := time.Now().Add(-time.Hour)
	var uses []indexUse
	for i, name := range []string{"oldest", "older", "newer", "newest"} {
		dir := filepath.Join(root, name)
		release, err := LeaseWorktreeIndex(dir)
		if err != nil {
			t.Fatal(err)
		}
		release()
		at := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(dir, worktreeLeaseFile), at, at); err != nil {
			t.Fatal(err)
		}
		uses = append(uses, indexUse{dir: dir, lastUse: lastUse(dir)})
	}
	release, err := LeaseWorktreeIndex(filepath.Join(root, "oldest"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	removed, err := evictLeastRecentlyUsed(root, uses, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "older"), filepath.Join(root, "newer")}
	if !slices.Equal(removed, want) {
		t.Errorf("evicted %v, want %v (oldest is leased)", removed, want)
	}
	if _, err := os.Stat(filepath.Join(root, "oldest")); err != nil {
		t.Errorf("the leased index was evicted: %v", err)
	}
}

// TestLeaseWorktreeIndex_AfterRemoval: a lease taken after its directory
// was removed recreates the directory and holds there.
func TestLeaseWorktreeIndex_AfterRemoval(t *testing.T) {
	root := testRoot(t)
	dir := filepath.Join(root, "idx")
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if ok, err := removeUnleased(root, dir); err != nil || !ok {
		t.Fatalf("removeUnleased = %v, %v; want removed", ok, err)
	}
	release, err = LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if ok, err := removeUnleased(root, dir); err != nil || ok {
		t.Errorf("removeUnleased under a lease = %v, %v; want kept", ok, err)
	}
	if left := entries(t, root); !slices.Equal(left, []string{"idx"}) {
		t.Errorf("root holds %v, want only idx", left)
	}
}

// TestRemoveUnleased_RacingLeases: removers running against callers that
// take and drop leases on one index never delete anything while a caller
// holds its lease. Each caller writes a file into the index under its lease
// and checks it is still there before letting go. Removing the directory in
// place, without renaming it aside first, fails this: a caller that opens
// the lease while the delete is under way gets a fresh lease file the
// delete then takes with it.
func TestRemoveUnleased_RacingLeases(t *testing.T) {
	const wantRemovals = 100
	root := testRoot(t)
	dir := filepath.Join(root, "idx")
	stop := make(chan struct{})
	var removals atomic.Int64
	var removers, callers sync.WaitGroup
	for range 2 {
		removers.Go(func() {
			for !closed(stop) {
				ok, err := removeUnleased(root, dir)
				if err != nil {
					t.Errorf("removeUnleased: %v", err)
					return
				}
				if ok {
					removals.Add(1)
				}
				jitter()
			}
		})
	}
	deadline := time.Now().Add(10 * time.Second)
	for c := range 3 {
		callers.Go(func() {
			for i := 0; removals.Load() < wantRemovals && time.Now().Before(deadline); i++ {
				holdAndCheck(t, dir, fmt.Sprintf("caller-%d-%d", c, i))
				jitter() // a gap in which no caller may hold a lease
			}
		})
	}
	callers.Wait()
	close(stop)
	removers.Wait()
	if n := removals.Load(); n < wantRemovals {
		t.Errorf("%d removals ran between the leases in 10s, want %d", n, wantRemovals)
	}
	for _, name := range entries(t, root) {
		if isTombstone(name) {
			t.Errorf("tombstone %s left behind", name)
		}
	}
}

// holdAndCheck leases dir, writes a marker into it, and fails the test when
// the marker or the lease file is gone before the lease ends. A lease the
// removers took away on every attempt is not a failure: this test checks
// what a held lease protects, not how soon one lands.
func holdAndCheck(t *testing.T, dir, marker string) {
	t.Helper()
	release, err := LeaseWorktreeIndex(dir)
	if errors.Is(err, errLeaseRemoved) {
		return
	}
	if err != nil {
		t.Errorf("lease: %v", err)
		return
	}
	defer release()
	path := filepath.Join(dir, marker)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Errorf("write under a lease: %v", err)
		return
	}
	jitter()
	for _, p := range []string{path, filepath.Join(dir, worktreeLeaseFile)} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed while its lease was held: %v", p, err)
		}
	}
	_ = os.Remove(path)
}

// jitter sleeps up to 100 microseconds, so racing goroutines interleave in
// a different order each round.
func jitter() {
	time.Sleep(time.Duration(rand.IntN(100)) * time.Microsecond)
}

// closed reports whether ch is closed.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestRemoveRefusesOutsideRoot: no delete happens unless the root is a
// worktree index root and the path sits strictly inside it, whatever the
// caller passes.
func TestRemoveRefusesOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, worktreeIndexDirName)
	mainIndex := filepath.Join(parent, "index")
	victim := filepath.Join(mainIndex, "victim-0000")
	for _, d := range []string{root, victim} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct{ name, root, path string }{
		{"root not a worktree index root", mainIndex, victim},
		{"path climbs out of the root", root, filepath.Join(root, "..", "index", "victim-0000")},
		{"path is the root", root, root},
		{"path beside the root", root, mainIndex},
	}
	for _, c := range cases {
		if err := removeUnder(c.root, c.path); err == nil {
			t.Errorf("%s: removeUnder(%s, %s) = nil, want a refusal", c.name, c.root, c.path)
		}
		if ok, err := removeUnleased(c.root, c.path); err == nil || ok {
			t.Errorf("%s: removeUnleased = %v, %v; want a refusal", c.name, ok, err)
		}
	}
	// A stale-looking index under the wrong root is refused, not removed.
	old := time.Now().Add(-2 * abandonedBuildAge)
	if err := os.Chtimes(victim, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneWorktreeIndexes(mainIndex); err == nil {
		t.Error("PruneWorktreeIndexes on the main index: no error, want a refusal")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("%s outside the worktree index root was removed: %v", victim, err)
	}
}

// testRoot is an empty worktree index root, named as the real one is.
func testRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), worktreeIndexDirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// publishFor publishes a build under root whose state.json records path,
// with every lease and lock already let go, and returns its index dir.
func publishFor(t *testing.T, root, path string) string {
	t.Helper()
	dir := WorktreeIndexDir(root, path)
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	gen := publish(t, dir, path)
	gen.Close()
	return dir
}

// publish builds an empty generation in dir whose state.json records path,
// publishes it, and returns it still leased.
func publish(t *testing.T, dir, path string) WorktreeGeneration {
	t.Helper()
	gen, err := publishGen(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

// entries lists the names in dir.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.Name()
	}
	return names
}

// isTombstone reports whether name is a tombstone.
func isTombstone(name string) bool {
	return strings.HasPrefix(name, tombstonePrefix)
}
