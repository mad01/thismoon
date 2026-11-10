package finder

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/worktree/worktreetest"
)

// TestIsLinkedWorktree: only a .git pointer whose git dir carries a
// commondir file marks a linked worktree; a main clone and a submodule-style
// pointer do not.
func TestIsLinkedWorktree(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat")

	sub := t.TempDir()
	modules := filepath.Join(sub, "modgit")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: modgit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"linked worktree", f.Worktree, true},
		{"main clone", f.Main, false},
		{"submodule pointer", sub, false},
		{"no checkout", t.TempDir(), false},
	}
	for _, tt := range tests {
		if got := IsLinkedWorktree(tt.dir); got != tt.want {
			t.Errorf("%s: IsLinkedWorktree = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestMainCheckout: a linked worktree leads back to its main clone; a main
// clone itself is not a worktree and has none.
func TestMainCheckout(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat")
	if got, ok := MainCheckout(f.Worktree); !ok || got != f.Main {
		t.Errorf("MainCheckout(worktree) = %q, %v; want %q", got, ok, f.Main)
	}
	if got, ok := MainCheckout(f.Main); ok {
		t.Errorf("MainCheckout(main clone) = %q, want none", got)
	}
}

// TestLinkedWorktrees: a main clone lists its live worktrees and skips an
// entry whose directory is gone; a worktree itself lists none.
func TestLinkedWorktrees(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat")
	gone := filepath.Join(filepath.Dir(f.Worktree), "gone")
	worktreetest.Git(t, f.Main, "worktree", "add", "-q", "-b", "gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	if got := LinkedWorktrees(f.Main); !slices.Equal(got, []string{f.Worktree}) {
		t.Errorf("LinkedWorktrees(main) = %v, want [%s]", got, f.Worktree)
	}
	if got := LinkedWorktrees(f.Worktree); len(got) != 0 {
		t.Errorf("LinkedWorktrees(worktree) = %v, want none", got)
	}
}
