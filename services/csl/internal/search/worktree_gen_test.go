package search

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWorktreeGenerations_PublishAndSweep: a publish switches what callers
// open; the generation it replaced stays while a caller holds it and goes
// on the next sweep after that, with any temp file or tombstone a crash
// left.
func TestWorktreeGenerations_PublishAndSweep(t *testing.T) {
	root := testRoot(t)
	dir := filepath.Join(root, "idx")
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if gen, err := OpenWorktreeGeneration(dir); err != nil || gen.Dir != "" {
		t.Fatalf("before any build: %+v, %v; want none", gen, err)
	}

	first := publish(t, dir, "/w")
	second := publish(t, dir, "/w")
	defer second.Close()
	if got := openDir(t, dir); got != second.Dir {
		t.Errorf("open = %s, want the newer build %s", got, second.Dir)
	}
	if _, err := os.Stat(first.Dir); err != nil {
		t.Fatalf("the replaced build went while still held: %v", err)
	}

	first.Close()
	for _, stray := range []string{"current.tmp-1", tombstonePrefix + "g-1-x"} {
		if err := os.WriteFile(filepath.Join(dir, stray), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := SweepWorktreeGenerations(dir); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	want := []string{
		worktreeBuildFile,
		worktreeLeaseFile,
		worktreeCurrentFile,
		filepath.Base(second.Dir),
	}
	slices.Sort(want)
	if got := entries(t, dir); !slices.Equal(got, want) {
		t.Errorf("after the sweep the index holds %v, want %v", got, want)
	}
}

// TestSweepWorktreeGenerations_LeavesABuildAlone: while a build holds the
// lock a sweep removes nothing, so it never races the build's own files.
func TestSweepWorktreeGenerations_LeavesABuildAlone(t *testing.T) {
	root := testRoot(t)
	dir := filepath.Join(root, "idx")
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	publish(t, dir, "/w").Close()
	unlock, err := LockWorktreeBuild(dir)
	if err != nil {
		t.Fatal(err)
	}
	unpublished, err := NewWorktreeGeneration(dir)
	if err != nil {
		t.Fatal(err)
	}
	unpublished.Close()
	if _, err := LockWorktreeBuild(dir); err != ErrWorktreeBuilding {
		t.Errorf("a second build lock: err %v, want ErrWorktreeBuilding", err)
	}
	if err := SweepWorktreeGenerations(dir); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := os.Stat(unpublished.Dir); err != nil {
		t.Errorf("a sweep during a build removed %s: %v", unpublished.Dir, err)
	}
	unlock()
	if err := SweepWorktreeGenerations(dir); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := os.Stat(unpublished.Dir); !os.IsNotExist(err) {
		t.Errorf("an unpublished build outlived the sweep after its lock (stat err %v)", err)
	}
}

// TestOpenWorktreeGeneration_DuringPublishes: callers opening the index
// while builds publish one after another always get a whole generation,
// which stays in place until they let it go. Run it with -race.
func TestOpenWorktreeGeneration_DuringPublishes(t *testing.T) {
	root := testRoot(t)
	dir := filepath.Join(root, "idx")
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	publish(t, dir, "/w").Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(done)
		for range 100 {
			gen, err := publishGen(dir, "/w")
			if err != nil {
				t.Errorf("publish: %v", err)
				return
			}
			gen.Close()
		}
	})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				if !openAndCheck(t, dir) {
					return
				}
			}
		})
	}
	wg.Wait()
	checkOneGeneration(t, dir)
}

// checkOneGeneration sweeps dir with no lease held and fails the test unless
// only the current generation is left.
func checkOneGeneration(t *testing.T, dir string) {
	t.Helper()
	if err := SweepWorktreeGenerations(dir); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	gens := slices.DeleteFunc(entries(t, dir), func(n string) bool {
		return !strings.HasPrefix(n, worktreeGenPrefix)
	})
	if len(gens) != 1 {
		t.Errorf(
			"after the last sweep %d generations remain (%v), want the current one",
			len(gens),
			gens,
		)
	}
}

// openAndCheck opens the published generation and fails the test when it
// is missing or loses its state.json while held. It reports whether to go
// on.
func openAndCheck(t *testing.T, dir string) bool {
	t.Helper()
	gen, err := OpenWorktreeGeneration(dir)
	if err != nil || gen.Dir == "" {
		t.Errorf("open during publishes: %+v, %v; want a generation", gen, err)
		return false
	}
	defer gen.Close()
	time.Sleep(20 * time.Microsecond)
	if _, err := os.Stat(filepath.Join(gen.Dir, stateFileName)); err != nil {
		t.Errorf("generation %s lost its state while held: %v", gen.Dir, err)
		return false
	}
	return true
}

// TestCurrentGeneration_OnlyAName: current naming anything but a generation
// in the same directory reads as no build, never as a path to follow.
func TestCurrentGeneration_OnlyAName(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{"../index", "g-1/../../index", "/etc", "state.json", ""} {
		if err := os.WriteFile(filepath.Join(dir, worktreeCurrentFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if name, err := currentGeneration(dir); err != nil || name != "" {
			t.Errorf("current %q = %q, %v; want none", content, name, err)
		}
	}
}

// leaseHelperDir names, in the environment, the worktree index the helper
// process leases.
const leaseHelperDir = "CSL_TEST_WORKTREE_LEASE_DIR"

// TestLeaseHelperProcess is not a test of its own: the cross-process test
// runs the test binary again with only it selected, as a second csl process
// that leases a worktree index and its published generation, says so on
// stdout, and holds both until it is killed or its stdin closes.
func TestLeaseHelperProcess(t *testing.T) {
	dir := os.Getenv(leaseHelperDir)
	if dir == "" {
		t.Skip("run by TestWorktreeLeases_AcrossProcesses")
	}
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	gen, err := OpenWorktreeGeneration(dir)
	if err != nil || gen.Dir == "" {
		t.Fatalf("open: %+v, %v", gen, err)
	}
	defer gen.Close()
	fmt.Println("held", gen.Dir)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// TestWorktreeLeases_AcrossProcesses: leases held by another process keep
// its generation through a newer publish and sweep and its index through a
// prune, and once that process dies, without letting go, the kernel's
// release of its locks lets both go.
func TestWorktreeLeases_AcrossProcesses(t *testing.T) {
	root := testRoot(t)
	gone := filepath.Join(t.TempDir(), "removed") // no .git: a gone worktree
	dir := publishFor(t, root, gone)
	held := startLeaseHelper(t, dir)

	newer := func() {
		release, err := LeaseWorktreeIndex(dir)
		if err != nil {
			t.Fatal(err)
		}
		publish(t, dir, gone).Close()
		release()
	}
	newer()
	if _, err := os.Stat(held.dir); err != nil {
		t.Fatalf("the other process's generation went while it held it: %v", err)
	}
	if removed, err := PruneWorktreeIndexes(root); err != nil || len(removed) != 0 {
		t.Fatalf(
			"prune with the other process holding the index: %v, %v; want nothing",
			removed,
			err,
		)
	}

	held.kill(t)
	release, err := LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := SweepWorktreeGenerations(dir); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	release()
	if _, err := os.Stat(held.dir); !os.IsNotExist(err) {
		t.Errorf("the dead process's generation outlived the sweep (stat err %v)", err)
	}
	if removed, err := PruneWorktreeIndexes(root); err != nil ||
		!slices.Equal(removed, []string{dir}) {
		t.Errorf("prune after the other process died: %v, %v; want [%s]", removed, err, dir)
	}
}

// leaseHelper is a running TestLeaseHelperProcess and the generation it
// holds. stdin stays open until the helper is killed.
type leaseHelper struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	dir   string
}

// startLeaseHelper starts the helper process on dir and waits until it
// holds its leases.
func startLeaseHelper(t *testing.T, dir string) leaseHelper {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLeaseHelperProcess$")
	cmd.Env = append(os.Environ(), leaseHelperDir+"="+dir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := leaseHelper{cmd: cmd, stdin: stdin}
	t.Cleanup(func() { h.kill(t) })
	lines := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if after, ok := strings.CutPrefix(sc.Text(), "held "); ok {
				lines <- after
				break
			}
		}
		close(lines)
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case h.dir = <-lines:
	case <-time.After(30 * time.Second):
		t.Fatal("the helper process never reported its leases")
	}
	if h.dir == "" {
		t.Fatal("the helper process exited without taking its leases")
	}
	return h
}

// kill ends the helper the way a crash would, with no chance to let go of
// its leases, and waits for it to exit. A second call does nothing.
func (h leaseHelper) kill(t *testing.T) {
	t.Helper()
	if h.cmd.ProcessState != nil {
		return
	}
	_ = h.cmd.Process.Kill()
	_ = h.cmd.Wait()
}

// publishGen is publish without the test: it builds an empty generation in
// dir whose state.json records path and publishes it, still leased.
func publishGen(dir, path string) (WorktreeGeneration, error) {
	unlock, err := LockWorktreeBuild(dir)
	if err != nil {
		return WorktreeGeneration{}, err
	}
	defer unlock()
	gen, err := NewWorktreeGeneration(dir)
	if err != nil {
		return WorktreeGeneration{}, err
	}
	state := EmptyState()
	state.SetRepo(path, RepoState{Fingerprint: "x"})
	if err := state.Save(gen.Dir); err != nil {
		gen.Close()
		return WorktreeGeneration{}, err
	}
	if err := PublishWorktreeGeneration(dir, gen); err != nil {
		gen.Close()
		return WorktreeGeneration{}, err
	}
	return gen, nil
}

// openDir opens the published generation and returns its directory, with
// the lease let go.
func openDir(t *testing.T, dir string) string {
	t.Helper()
	gen, err := OpenWorktreeGeneration(dir)
	if err != nil {
		t.Fatal(err)
	}
	gen.Close()
	return gen.Dir
}
