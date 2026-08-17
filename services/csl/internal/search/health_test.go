package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

func TestDeriveGitAction(t *testing.T) {
	tests := []struct {
		name string
		h    GitHealth
		want string
	}{
		{"clean and synced", GitHealth{Branch: "main", HasUpstream: true}, GitActionReady},
		{"error wins", GitHealth{Error: "boom", Dirty: true}, GitActionError},
		{"detached", GitHealth{Branch: "HEAD", HasUpstream: true}, GitActionDetached},
		{"dirty", GitHealth{Branch: "main", Dirty: true, HasUpstream: true}, GitActionCommitOrStash},
		{"dirty before upstream", GitHealth{Branch: "main", Dirty: true}, GitActionCommitOrStash},
		{"no upstream", GitHealth{Branch: "feature"}, GitActionNoUpstream},
		{"diverged", GitHealth{Branch: "main", HasUpstream: true, Ahead: 1, Behind: 2}, GitActionDiverged},
		{"ahead", GitHealth{Branch: "main", HasUpstream: true, Ahead: 3}, GitActionPush},
		{"behind", GitHealth{Branch: "main", HasUpstream: true, Behind: 1}, GitActionPull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveGitAction(tt.h); got != tt.want {
				t.Errorf("deriveGitAction(%+v) = %q, want %q", tt.h, got, tt.want)
			}
		})
	}
}

func TestGitHealthNeedsAttention(t *testing.T) {
	if (GitHealth{Action: GitActionReady}).NeedsAttention() {
		t.Errorf("ready repo should not need attention")
	}
	if !(GitHealth{Action: GitActionPush}).NeedsAttention() {
		t.Errorf("push_recommended repo should need attention")
	}
}

// gitRun runs git in dir, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// commitFile writes name in dir and commits it.
func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-m", "add "+name)
}

// initClone builds a bare origin with one commit and clones it into a work
// dir, so the clone has a real upstream to compare against.
func initClone(t *testing.T) (bare, clone string) {
	t.Helper()
	seed := t.TempDir()
	gitRun(t, seed, "init", "-b", "main")
	gitRun(t, seed, "config", "user.email", "test@example.com")
	gitRun(t, seed, "config", "user.name", "test")
	commitFile(t, seed, "a.txt")

	bare = filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, seed, "clone", "--bare", seed, bare)

	clone = filepath.Join(t.TempDir(), "clone")
	gitRun(t, filepath.Dir(clone), "clone", bare, clone)
	gitRun(t, clone, "config", "user.email", "test@example.com")
	gitRun(t, clone, "config", "user.name", "test")
	return bare, clone
}

func TestAheadBehind(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	bare, clone := initClone(t)

	t.Run("fresh clone is synced", func(t *testing.T) {
		ahead, behind, hasUpstream, err := AheadBehind(clone)
		if err != nil {
			t.Fatal(err)
		}
		if !hasUpstream || ahead != 0 || behind != 0 {
			t.Errorf("got ahead=%d behind=%d upstream=%v, want 0/0/true", ahead, behind, hasUpstream)
		}
	})

	t.Run("local commit counts as ahead", func(t *testing.T) {
		commitFile(t, clone, "b.txt")
		ahead, behind, _, err := AheadBehind(clone)
		if err != nil {
			t.Fatal(err)
		}
		if ahead != 1 || behind != 0 {
			t.Errorf("got ahead=%d behind=%d, want 1/0", ahead, behind)
		}
	})

	t.Run("upstream commit counts as behind after fetch", func(t *testing.T) {
		other := filepath.Join(t.TempDir(), "other")
		gitRun(t, filepath.Dir(other), "clone", bare, other)
		gitRun(t, other, "config", "user.email", "test@example.com")
		gitRun(t, other, "config", "user.name", "test")
		commitFile(t, other, "c.txt")
		gitRun(t, other, "push", "origin", "main")
		gitRun(t, clone, "fetch")

		ahead, behind, _, err := AheadBehind(clone)
		if err != nil {
			t.Fatal(err)
		}
		if ahead != 1 || behind != 1 {
			t.Errorf("got ahead=%d behind=%d, want 1/1 (diverged)", ahead, behind)
		}
	})

	t.Run("no upstream is not an error", func(t *testing.T) {
		local := t.TempDir()
		gitRun(t, local, "init", "-b", "main")
		gitRun(t, local, "config", "user.email", "test@example.com")
		gitRun(t, local, "config", "user.name", "test")
		commitFile(t, local, "a.txt")

		_, _, hasUpstream, err := AheadBehind(local)
		if err != nil {
			t.Fatal(err)
		}
		if hasUpstream {
			t.Errorf("hasUpstream = true, want false for a local-only branch")
		}
	})

	t.Run("non-repo dir reports no upstream", func(t *testing.T) {
		_, _, hasUpstream, err := AheadBehind(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if hasUpstream {
			t.Errorf("hasUpstream = true, want false outside a git repo")
		}
	})
}

func TestGitHealthSweep(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	_, clone := initClone(t)
	commitFile(t, clone, "unpushed.txt")

	dirty := filepath.Join(t.TempDir(), "dirty")
	gitRun(t, filepath.Dir(dirty), "init", "-b", "main", dirty)
	gitRun(t, dirty, "config", "user.email", "test@example.com")
	gitRun(t, dirty, "config", "user.name", "test")
	commitFile(t, dirty, "a.txt")
	if err := os.WriteFile(filepath.Join(dirty, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos := []finder.Repo{
		{Name: "o/clone", Path: clone, Host: "example.com"},
		{Name: "o/dirty", Path: dirty},
		{Name: "o/broken", Path: t.TempDir()},
	}
	got := GitHealthSweep(context.Background(), repos)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[0].Name != "o/clone" || got[0].Action != GitActionPush || got[0].Ahead != 1 {
		t.Errorf("clone entry = %+v, want push_recommended with ahead=1", got[0])
	}
	if got[1].Action != GitActionCommitOrStash || got[1].UntrackedFiles != 1 {
		t.Errorf("dirty entry = %+v, want commit_or_stash with 1 untracked", got[1])
	}
	if got[2].Action != GitActionError || got[2].Error == "" {
		t.Errorf("broken entry = %+v, want action=error with a message", got[2])
	}
}
