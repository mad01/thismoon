package gitloc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/firstlaunchtest"
)

const sha = "1a612e4313cddd60ef596c66f2fccaacea2e8410"

func TestParseLocation(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want Location
	}{
		{
			name: "main checkout",
			out:  "/src/repo\n/src/repo/.git\n/src/repo/.git\n" + sha + "\nmain",
			want: Location{
				Toplevel:  "/src/repo",
				Canonical: "/src/repo",
				Branch:    "main",
				Head:      sha,
			},
		},
		{
			name: "linked worktree",
			out: "/home/.worktrees/repo/feat-x\n/src/repo/.git/worktrees/feat-x\n" +
				"/src/repo/.git\n" + sha + "\nfeat/x",
			want: Location{
				Toplevel:  "/home/.worktrees/repo/feat-x",
				Canonical: "/src/repo",
				Branch:    "feat/x",
				Head:      sha,
				Linked:    true,
			},
		},
		{
			name: "worktree of a bare repo has no main checkout",
			out:  "/src/wt\n/src/hub.git/worktrees/wt\n/src/hub.git\n" + sha + "\nb2",
			want: Location{Toplevel: "/src/wt", Branch: "b2", Head: sha, Linked: true},
		},
		{
			name: "submodule is not a linked worktree",
			out: "/src/super/sub\n/src/super/.git/modules/sub\n/src/super/.git/modules/sub\n" +
				sha + "\nmain",
			want: Location{
				Toplevel: "/src/super/sub", Canonical: "/src/super/sub", Branch: "main", Head: sha,
			},
		},
		{
			name: "detached HEAD keeps git's answer",
			out:  "/src/repo\n/src/repo/.git\n/src/repo/.git\n" + sha + "\nHEAD",
			want: Location{
				Toplevel:  "/src/repo",
				Canonical: "/src/repo",
				Branch:    "HEAD",
				Head:      sha,
			},
		},
		{
			name: "old git echoes the unknown flag",
			out:  "--path-format=absolute\n/src/repo\n/src/repo/.git\n.git\n" + sha + "\nmain",
			want: Location{},
		},
		{
			name: "relative path is rejected",
			out:  "/src/repo\n.git\n.git\n" + sha + "\nmain",
			want: Location{},
		},
		{
			name: "output without the commit line is rejected",
			out:  "/src/repo\n/src/repo/.git\n/src/repo/.git\nmain",
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
	head, err := gitCmd(canonical, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	commit := strings.TrimSpace(string(head))
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
			want: Location{
				Toplevel: canonical, Canonical: canonical, Branch: "main", Head: commit, Repo: repo,
			},
		},
		{
			name: "linked worktree",
			dir:  filepath.Join(worktree, "sub"),
			want: Location{
				Toplevel: worktree, Canonical: canonical, Branch: "feat/x", Head: commit,
				Repo: repo, Linked: true,
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

func TestCanonicalRepo(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"git@github.com:mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"https://github.com/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"https://github.com/mad01/dotfiles", "github.com/mad01/dotfiles"},
		{"ssh://git@github.com/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"ssh://git@github.com:22/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"/local/path/repo", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := CanonicalRepo(tt.in); got != tt.want {
			t.Errorf("CanonicalRepo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestOutputDoesNotWaitOnALingeringChild pins the pipe grace: a git that
// leaves a background child holding stdout must not hold the caller until
// that child exits. A git that exits 0 first keeps what it printed; one
// that outlives ctx ends in "" like any other failure. The child sleeps far
// past the ceiling, so a missing grace fails on the ceiling, not on a tight
// timing bound.
func TestOutputDoesNotWaitOnALingeringChild(t *testing.T) {
	tests := []struct {
		name    string
		rest    string // what the fake git runs after it forks the child
		timeout time.Duration
		want    string
	}{
		{"git exits first and keeps its output", "echo main\n", 30 * time.Second, "main"},
		{"git outlives the deadline", "exec sleep 60\n", 200 * time.Millisecond, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			pidFile := filepath.Join(bin, "child.pid")
			git := writeScript(t, bin, "git", "#!/bin/sh\n"+
				"[ \"$1\" = warm ] && exit 0\n"+
				"sleep 60 &\necho $! > '"+pidFile+"'\n"+tt.rest)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Cleanup(func() { killRecorded(pidFile) })
			// A fresh script's first launch can be slow under load; launch it
			// once untimed so the deadline measures the pipe, not the launch.
			firstlaunchtest.Warm(t, git, []string{"warm"}, nil)
			dir := t.TempDir()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()
			done := make(chan string, 1)
			go func() { done <- Output(ctx, dir, "rev-parse") }()
			select {
			case got := <-done:
				if got != tt.want {
					t.Errorf("Output = %q, want %q", got, tt.want)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("Output waited on the child git left holding stdout")
			}
		})
	}
}

func TestOutputWithoutDirectoryNeverRunsGit(t *testing.T) {
	if got := Output(context.Background(), "", "rev-parse", "HEAD"); got != "" {
		t.Errorf("Output(\"\") = %q, want empty", got)
	}
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

func gitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

// initRepo creates a git repository at dir on branch main, with one commit
// and the given origin URL, and returns dir.
func initRepo(t *testing.T, dir, origin string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
		{"remote", "add", "origin", origin},
	} {
		cmd := gitCmd(dir, args...)
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// killRecorded ends the process whose pid a fake git wrote to pidFile, if
// it got that far.
func killRecorded(pidFile string) {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
