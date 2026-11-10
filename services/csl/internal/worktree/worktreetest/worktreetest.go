// Package worktreetest builds a git clone with one linked worktree for the
// worktree index tests in the worktree, mcpserver and cli packages. Git runs
// with the user's global and system config shut out, so a signing or hook
// setting on the developer's machine cannot change what a test sees.
package worktreetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Remote is the origin every fixture clone carries, so its repos are named
// org/repo and its worktree org/repo@<branch>.
const Remote = "https://github.com/org/repo.git"

// Fixture is a main clone and a linked worktree of it.
type Fixture struct {
	// Main is the main clone, at <root>/src/org/repo on branch main. Its one
	// commit holds main.go, which defines MainOnly.
	Main string
	// Worktree is the linked worktree at <root>/wt/<Branch>.
	Worktree string
	// Branch is the worktree's branch.
	Branch string
}

// New creates the fixture under root, with the worktree on branch. root is
// resolved through symlinks first (macOS temp dirs sit behind /var), since
// git records the worktree by its real path.
func New(t testing.TB, root, branch string) Fixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	f := Fixture{
		Main:     filepath.Join(root, "src", "org", "repo"),
		Worktree: filepath.Join(root, "wt", branch),
		Branch:   branch,
	}
	if err := os.MkdirAll(f.Main, 0o755); err != nil {
		t.Fatal(err)
	}
	Git(t, f.Main, "init", "-q", "-b", "main")
	Git(t, f.Main, "remote", "add", "origin", Remote)
	Write(t, f.Main, "main.go", "package main\n\nfunc MainOnly() {}\n")
	Git(t, f.Main, "add", "main.go")
	Git(t, f.Main, "commit", "-q", "-m", "initial")
	Git(t, f.Main, "worktree", "add", "-q", "-b", branch, f.Worktree)
	return f
}

// Git runs git in dir and fails the test on an error.
func Git(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// Write writes content to rel under dir, creating parent directories.
func Write(t testing.TB, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
