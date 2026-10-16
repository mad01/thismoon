package checkout

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runGit runs git in dir with a fixed identity, so the test needs no global
// git config (CI runners have none).
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// sourceRepo makes a repo with one commit holding svc/README.md and
// returns its path and the commit sha.
func sourceRepo(t *testing.T) (string, string) {
	t.Helper()
	src := t.TempDir()
	runGit(t, src, "init", "--quiet")
	writeFile(t, filepath.Join(src, "svc", "README.md"), "# svc\n")
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "--quiet", "-m", "init")
	return src, runGit(t, src, "rev-parse", "HEAD")
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDetachesAtCommitWithoutOrigin(t *testing.T) {
	src, sha := sourceRepo(t)
	c, err := Create(context.Background(), src, sha, t.TempDir())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := runGit(t, c.Repo, "rev-parse", "HEAD"); got != sha {
		t.Errorf("HEAD = %s, want the pinned %s", got, sha)
	}
	if got := runGit(t, c.Repo, "remote"); got != "" {
		t.Errorf("remotes = %q, want none so nothing can be pushed back", got)
	}
	if info, err := os.Stat(c.Worktrees); err != nil || !info.IsDir() {
		t.Errorf("worktrees dir missing: %v", err)
	}
	if err := c.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(c.Root); !os.IsNotExist(err) {
		t.Errorf("Root still exists after Remove: %v", err)
	}
}

func TestCreateRemovesPartialDirOnBadCommit(t *testing.T) {
	src, _ := sourceRepo(t)
	parent := t.TempDir()
	if _, err := Create(context.Background(), src,
		"0123456789abcdef0123456789abcdef01234567", parent); err == nil {
		t.Fatal("Create() with an unknown commit = nil error")
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("parent holds %d entries after a failed Create, want 0", len(entries))
	}
}

func TestBranchReadsSeeOnlyTheBranch(t *testing.T) {
	src, sha := sourceRepo(t)
	c, err := Create(context.Background(), src, sha, t.TempDir())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	ctx := context.Background()
	if c.BranchExists(ctx, "eval/svc") {
		t.Fatal("BranchExists before the branch was made = true")
	}
	runGit(t, c.Repo, "switch", "--quiet", "-c", "eval/svc")
	writeFile(t, filepath.Join(c.Repo, "svc", "README.md"), "# svc\n\nmore\n")
	writeFile(t, filepath.Join(c.Repo, "other.txt"), "x\n")
	runGit(t, c.Repo, "add", ".")
	runGit(t, c.Repo, "commit", "--quiet", "-m", "edit")
	runGit(t, c.Repo, "switch", "--quiet", "--detach", sha)

	if !c.BranchExists(ctx, "eval/svc") {
		t.Error("BranchExists after commit = false")
	}
	files, err := c.ChangedFiles(ctx, "eval/svc")
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if want := []string{"other.txt", "svc/README.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("ChangedFiles() = %v, want %v", files, want)
	}
	if got := c.Original(ctx, "svc/README.md"); got != "# svc\n" {
		t.Errorf("Original() = %q, want the pinned content", got)
	}
	if got := c.AtBranch(ctx, "eval/svc", "svc/README.md"); got != "# svc\n\nmore\n" {
		t.Errorf("AtBranch() = %q, want the branch content", got)
	}
	if got := c.AtBranch(ctx, "eval/none", "svc/README.md"); got != "" {
		t.Errorf("AtBranch(missing branch) = %q, want empty", got)
	}
	if got := c.Original(ctx, "other.txt"); got != "" {
		t.Errorf("Original(new file) = %q, want empty", got)
	}
	dir, remove, err := c.Worktree(ctx, "eval/svc")
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "svc", "README.md"))
	if err != nil || string(raw) != "# svc\n\nmore\n" {
		t.Errorf("worktree README = %q, %v; want the branch content", raw, err)
	}
	remove()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove: %v", err)
	}
}

func TestCreateLeavesSourceUntouched(t *testing.T) {
	src, sha := sourceRepo(t)
	before := runGit(t, src, "for-each-ref")
	c, err := Create(context.Background(), src, sha, t.TempDir())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	runGit(t, c.Repo, "branch", "eval/x")
	if _, _, err := c.Worktree(context.Background(), "eval/x"); err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}
	if after := runGit(t, src, "for-each-ref"); after != before {
		t.Errorf("source refs changed:\nbefore %s\nafter  %s", before, after)
	}
	if got := runGit(t, src, "worktree", "list"); strings.Count(got, "\n") != 0 {
		t.Errorf("source gained worktrees:\n%s", got)
	}
}
