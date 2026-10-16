package guard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseLocation(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want Location
	}{
		{
			name: "main checkout",
			out:  "/src/repo\n/src/repo/.git\n/src/repo/.git\nmain",
			want: Location{Toplevel: "/src/repo", Canonical: "/src/repo", Branch: "main"},
		},
		{
			name: "linked worktree",
			out: "/home/.worktrees/repo/feat-x\n/src/repo/.git/worktrees/feat-x\n" +
				"/src/repo/.git\nfeat/x",
			want: Location{
				Toplevel:  "/home/.worktrees/repo/feat-x",
				Canonical: "/src/repo",
				Branch:    "feat/x",
				Linked:    true,
			},
		},
		{
			name: "worktree of a bare repo has no main checkout",
			out:  "/src/wt\n/src/hub.git/worktrees/wt\n/src/hub.git\nb2",
			want: Location{Toplevel: "/src/wt", Branch: "b2", Linked: true},
		},
		{
			name: "submodule is not a linked worktree",
			out:  "/src/super/sub\n/src/super/.git/modules/sub\n/src/super/.git/modules/sub\nmain",
			want: Location{Toplevel: "/src/super/sub", Canonical: "/src/super/sub", Branch: "main"},
		},
		{
			name: "detached HEAD keeps git's answer",
			out:  "/src/repo\n/src/repo/.git\n/src/repo/.git\nHEAD",
			want: Location{Toplevel: "/src/repo", Canonical: "/src/repo", Branch: "HEAD"},
		},
		{
			name: "old git echoes the unknown flag",
			out:  "--path-format=absolute\n/src/repo\n/src/repo/.git\n.git\nmain",
			want: Location{},
		},
		{
			name: "relative path is rejected",
			out:  "/src/repo\n.git\n.git\nmain",
			want: Location{},
		},
		{name: "no output", out: "", want: Location{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLocation(tt.out); got != tt.want {
				t.Errorf("parseLocation() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestLocationAt runs the real git call against the loom layout: a main
// checkout on main and a linked worktree on a feature branch beside it,
// each asked from a subdirectory so the toplevel has to be found, not
// assumed. Paths are compared physically because the temp dir may sit
// behind a symlink (/var on macOS) and git reports resolved paths.
func TestLocationAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := physical(t, t.TempDir())
	t.Setenv("HOME", home)
	// Keep the user's and the system's gitconfig out of the fixture: a
	// commit.gpgsign or hooksPath there would break the setup commit.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	canonical := initRepo(t, filepath.Join(home, "canonical"), "git@github.com:mad01/thismoon.git")
	worktree := filepath.Join(home, ".worktrees", "thismoon", "feat-x")
	if out, err := gitCmd(canonical, "worktree", "add", "-q", "-b", "feat/x", worktree).
		CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	notRepo := filepath.Join(home, "plain")
	unborn := filepath.Join(home, "unborn")
	for _, dir := range []string{
		filepath.Join(canonical, "sub"), filepath.Join(worktree, "sub"), notRepo, unborn,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := gitCmd(unborn, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	const repo = "github.com/mad01/thismoon"
	tests := []struct {
		name string
		dir  string
		want Location
	}{
		{
			name: "main checkout",
			dir:  filepath.Join(canonical, "sub"),
			want: Location{Toplevel: canonical, Canonical: canonical, Branch: "main", Repo: repo},
		},
		{
			name: "linked worktree",
			dir:  filepath.Join(worktree, "sub"),
			want: Location{
				Toplevel: worktree, Canonical: canonical, Branch: "feat/x", Repo: repo, Linked: true,
			},
		},
		{name: "outside any repo", dir: notRepo, want: Location{}},
		// HEAD does not resolve before the first commit, so the rev-parse
		// call fails as a whole; with no origin either, nothing is known.
		{name: "repo with no commit yet", dir: unborn, want: Location{}},
		{name: "unknown directory never runs git", dir: "", want: Location{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocationAt(context.Background(), tt.dir); got != tt.want {
				t.Errorf("LocationAt(%q) = %+v, want %+v", tt.dir, got, tt.want)
			}
		})
	}

	// A context that is already done kills both git processes before they
	// answer, so even the main checkout degrades to the zero Location.
	t.Run("expired context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := LocationAt(ctx, canonical); got != (Location{}) {
			t.Errorf("LocationAt(cancelled) = %+v, want the zero Location", got)
		}
	})
}

// physical resolves symlinks in a test directory so it compares equal to
// the paths git reports.
func physical(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
