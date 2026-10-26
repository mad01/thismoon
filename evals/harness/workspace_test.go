package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newRepo makes a git repo named name under a fresh src root with one
// commit, and isolates git from the user's global and system config.
func newRepo(t *testing.T, name string) (srcRoot, repoDir string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = eval\n\temail = eval@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	srcRoot = t.TempDir()
	repoDir = filepath.Join(srcRoot, name)
	mustGit(t, srcRoot, "init", "-q", repoDir)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", "README.md")
	mustGit(t, repoDir, "commit", "-q", "-m", "chore: init")
	top, err := git(context.Background(), repoDir, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatal(err)
	}
	return srcRoot, top
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var stamp = time.Date(2026, 10, 26, 12, 30, 0, 0, time.UTC)

func TestWorktreeWorkspace(t *testing.T) {
	ctx := context.Background()
	srcRoot, repoDir := newRepo(t, "demo")
	roots := workspaceRoots{SrcRoot: srcRoot, WorktreeRoot: t.TempDir()}

	ws, err := prepareWorkspace(
		ctx,
		WorkspaceSpec{Kind: workspaceWorktree, Repo: "demo"},
		roots,
		"commit-task",
		stamp,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(roots.WorktreeRoot, "demo", "eval-commit-task-20261026-123000"); ws.Dir != want {
		t.Errorf("dir = %s, want %s", ws.Dir, want)
	}
	if b := mustGit(t, ws.Dir, "branch", "--show-current"); b != "eval/commit-task-20261026-123000" {
		t.Errorf("branch = %q", b)
	}
	if commits, _ := ws.Commits(ctx); len(commits) != 0 {
		t.Errorf("fresh worktree has commits %v", commits)
	}

	if err := os.WriteFile(filepath.Join(ws.Dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ws.Dir, "add", "a.txt")
	mustGit(t, ws.Dir, "commit", "-q", "-m", "feat: add a")
	mustGit(t, ws.Dir, "commit", "-q", "--allow-empty", "-m", "docs: note")
	commits, err := ws.Commits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(commits, "|") != "feat: add a|docs: note" {
		t.Errorf("commits = %v, want oldest first", commits)
	}

	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Errorf("worktree dir still there: %v", err)
	}
	if out := mustGit(t, repoDir, "branch", "--list", "eval/*"); out != "" {
		t.Errorf("eval branch left behind: %q", out)
	}
	if out := mustGit(t, repoDir, "worktree", "list"); strings.Contains(out, "eval-commit-task") {
		t.Errorf("worktree still registered:\n%s", out)
	}
}

func TestRepoWorkspace(t *testing.T) {
	ctx := context.Background()
	srcRoot, repoDir := newRepo(t, "demo")
	roots := workspaceRoots{SrcRoot: srcRoot}

	for _, ref := range []string{"demo", repoDir} {
		ws, err := prepareWorkspace(
			ctx,
			WorkspaceSpec{Kind: workspaceRepo, Repo: ref},
			roots,
			"q",
			stamp,
		)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		if ws.Dir != repoDir {
			t.Errorf("%s: dir = %s, want %s", ref, ws.Dir, repoDir)
		}
		if err := ws.Cleanup(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(repoDir, "README.md")); err != nil {
			t.Errorf("cleanup touched the canonical checkout: %v", err)
		}
	}
	if _, err := prepareWorkspace(ctx, WorkspaceSpec{Kind: workspaceRepo, Repo: "absent"}, roots, "q", stamp); err == nil {
		t.Error("a missing repo resolved")
	}
}

func TestTmpdirWorkspace(t *testing.T) {
	root := t.TempDir()
	ws, err := prepareWorkspace(context.Background(), WorkspaceSpec{Kind: workspaceTmpdir},
		workspaceRoots{TmpRoot: root}, "scratch", stamp)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(ws.Dir) != root || !strings.Contains(filepath.Base(ws.Dir), "scratch") {
		t.Errorf("dir = %s, want a scratch dir under %s", ws.Dir, root)
	}
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Errorf("tmpdir still there: %v", err)
	}
}
