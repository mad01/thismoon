package store

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newBare creates a bare git repo to act as origin in tests — real git, no
// network, matching how the store tests already treat git as a dependency.
func newBare(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "origin.git")
	out, err := exec.Command("git", "init", "--bare", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	return dir
}

// remoteStore returns a store whose root does not exist yet, wired to origin.
func remoteStore(t *testing.T, origin string) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), "worklog"))
	s.Remote = Remote{URL: origin, Push: true}
	tick := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	s.Now = func() time.Time {
		now := tick
		tick = tick.Add(time.Hour)
		return now
	}
	return s
}

func bareLog(t *testing.T, bare string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", bare, "log", "--format=%s", "--all").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func TestCheckpointPushesToOrigin(t *testing.T) {
	bare := newBare(t)
	s := remoteStore(t, bare)

	if _, err := s.Checkpoint("MAD-1", CheckpointInput{Where: "started"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	if got := bareLog(t, bare); !strings.Contains(got, "worklog: checkpoint MAD-1") {
		t.Errorf("origin log = %q, want checkpoint commit pushed", got)
	}
}

func TestEnsureClonedBootstrapsFreshMachine(t *testing.T) {
	bare := newBare(t)
	first := remoteStore(t, bare)
	if _, err := first.Checkpoint("MAD-2", CheckpointInput{Where: "seeded"}); err != nil {
		t.Fatalf("seed Checkpoint: %v", err)
	}

	second := remoteStore(t, bare)
	if err := second.EnsureCloned(); err != nil {
		t.Fatalf("EnsureCloned: %v", err)
	}
	items, err := second.List("", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].FM.Key != "MAD-2" {
		t.Errorf("cloned store items = %v, want [MAD-2]", items)
	}
}

func TestExistingLocalStoreAdoptsRemote(t *testing.T) {
	s := remoteStore(t, "")
	s.Remote = Remote{}
	if _, err := s.Checkpoint("MAD-3", CheckpointInput{Where: "local only"}); err != nil {
		t.Fatalf("local Checkpoint: %v", err)
	}

	bare := newBare(t)
	s.Remote = Remote{URL: bare, Push: true}
	if _, err := s.Checkpoint("MAD-3", CheckpointInput{Note: "now remote"}); err != nil {
		t.Fatalf("remote Checkpoint: %v", err)
	}

	got := bareLog(t, bare)
	if !strings.Contains(got, "worklog: checkpoint MAD-3") {
		t.Errorf("origin log = %q, want history pushed after adopting remote", got)
	}
}

func TestChangedUpstreamRepointsOrigin(t *testing.T) {
	bareA := newBare(t)
	s := remoteStore(t, bareA)
	if _, err := s.Checkpoint("MAD-4", CheckpointInput{Where: "on A"}); err != nil {
		t.Fatalf("Checkpoint A: %v", err)
	}

	bareB := newBare(t)
	s.Remote.URL = bareB
	if _, err := s.Checkpoint("MAD-4", CheckpointInput{Note: "moved to B"}); err != nil {
		t.Fatalf("Checkpoint B: %v", err)
	}

	url, err := s.gitOut("remote", "get-url", "origin")
	if err != nil {
		t.Fatalf("get-url: %v", err)
	}
	if url != bareB {
		t.Errorf("origin url = %q, want %q", url, bareB)
	}
	if got := bareLog(t, bareB); !strings.Contains(got, "worklog: checkpoint MAD-4") {
		t.Errorf("new origin log = %q, want history pushed", got)
	}
}

func TestPushFailureIsErrPushAndKeepsItem(t *testing.T) {
	s := remoteStore(t, filepath.Join(t.TempDir(), "missing.git"))

	it, err := s.Checkpoint("MAD-5", CheckpointInput{Where: "written anyway"})
	if !errors.Is(err, ErrPush) {
		t.Fatalf("Checkpoint err = %v, want ErrPush", err)
	}
	if it == nil || it.FM.Key != "MAD-5" {
		t.Errorf("item = %v, want MAD-5 despite failed push", it)
	}
	if !s.Exists("MAD-5") {
		t.Error("item not on disk after failed push")
	}
}

func TestSyncPullsRemoteWork(t *testing.T) {
	bare := newBare(t)
	machineA := remoteStore(t, bare)
	if _, err := machineA.Checkpoint("MAD-6", CheckpointInput{Where: "from A"}); err != nil {
		t.Fatalf("A Checkpoint: %v", err)
	}

	machineB := remoteStore(t, bare)
	if err := machineB.EnsureCloned(); err != nil {
		t.Fatalf("B EnsureCloned: %v", err)
	}
	if _, err := machineA.Checkpoint("MAD-7", CheckpointInput{Where: "A again"}); err != nil {
		t.Fatalf("A second Checkpoint: %v", err)
	}

	if err := machineB.Sync(); err != nil {
		t.Fatalf("B Sync: %v", err)
	}
	items, err := machineB.List("", "")
	if err != nil {
		t.Fatalf("B List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("B items after sync = %d, want 2", len(items))
	}
}

func TestSyncToleratesEmptyRemote(t *testing.T) {
	bare := newBare(t)
	s := remoteStore(t, bare)
	if _, err := s.Checkpoint("MAD-8", CheckpointInput{Where: "x"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	// Repoint at a second, still-empty remote: pull has nothing to find and
	// must not fail the sync.
	s.Remote.URL = newBare(t)
	if err := s.Sync(); err != nil {
		t.Fatalf("Sync against empty remote: %v", err)
	}
	if got := bareLog(t, s.Remote.URL); !strings.Contains(got, "worklog: checkpoint MAD-8") {
		t.Errorf("empty remote log after sync = %q, want history pushed", got)
	}
}

func TestNoRemoteKeepsLocalOnlyBehavior(t *testing.T) {
	s := remoteStore(t, "")
	if _, err := s.Checkpoint("MAD-9", CheckpointInput{Where: "local"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := s.Sync(); err == nil {
		t.Error("Sync with no remote should error")
	}
	if _, err := s.gitOut("remote", "get-url", "origin"); err == nil {
		t.Error("no origin should be configured without a remote")
	}
}
