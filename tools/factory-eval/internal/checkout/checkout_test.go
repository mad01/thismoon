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

// TestCreateResolvesTheTempRoot hands Create a symlinked parent, the way
// macOS hands every program its temp dir, and wants the root under the
// resolved directory: claude records the session cwd resolved, so the
// paths the prompt and the flags name share its spelling.
func TestCreateResolvesTheTempRoot(t *testing.T) {
	src, sha := sourceRepo(t)
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	c, err := Create(context.Background(), src, sha, link)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = c.Remove() })
	wantParent, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(c.Root) != wantParent {
		t.Errorf("Root = %s, want it directly under the resolved parent %s", c.Root, wantParent)
	}
	if strings.HasPrefix(c.Root, link) {
		t.Errorf("Root = %s still names the symlink %s", c.Root, link)
	}
	for _, p := range []string{c.Repo, c.Worktrees} {
		if !strings.HasPrefix(p, c.Root+string(filepath.Separator)) {
			t.Errorf("%s is not under the resolved Root %s", p, c.Root)
		}
	}
}

// TestCreateKeepsNothingPastThePin gives the source a main that moved on,
// a side branch and a tag, the way the real repo looks after the pin.
func TestCreateKeepsNothingPastThePin(t *testing.T) {
	src, pin := sourceRepo(t)
	runGit(t, src, "tag", "svc/v1.0.0")
	runGit(t, src, "branch", "side")
	writeFile(t, filepath.Join(src, "svc", "cases.yaml"), "answers: here\n")
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "--quiet", "-m", "after the pin")
	newer := runGit(t, src, "rev-parse", "HEAD")

	c, err := Create(context.Background(), src, pin, t.TempDir())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	refs, err := c.LocalRefs(context.Background())
	if err != nil || len(refs) != 0 {
		t.Errorf("LocalRefs() = %v, %v; want no branch and no tag", refs, err)
	}
	if got := runGit(t, c.Repo, "for-each-ref"); got != "" {
		t.Errorf("refs left in the clone:\n%s", got)
	}
	if got := runGit(t, c.Repo, "reflog", "--all"); got != "" {
		t.Errorf("reflog still names commits:\n%s", got)
	}
	cmd := exec.Command("git", "cat-file", "-e", newer)
	cmd.Dir = c.Repo
	if err := cmd.Run(); err == nil {
		t.Errorf("commit %s from after the pin is still in the object store", newer)
	}
	if got := runGit(t, c.Repo, "config", "--local", "user.name"); got != IdentityName {
		t.Errorf("user.name = %q, want the harness identity %q", got, IdentityName)
	}
	if got := runGit(t, c.Repo, "config", "--local", "user.email"); got != IdentityEmail {
		t.Errorf("user.email = %q, want %q", got, IdentityEmail)
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

func TestFingerprintSeesEveryKindOfChange(t *testing.T) {
	src, _ := sourceRepo(t)
	ctx := context.Background()
	before, err := Fingerprint(ctx, src)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	again, err := Fingerprint(ctx, src)
	if err != nil || len(before.Changes(again)) != 0 {
		t.Fatalf("an untouched repo changed: %v, %v", before.Changes(again), err)
	}
	runGit(t, src, "branch", "eval/x")
	writeFile(t, filepath.Join(src, "svc", "README.md"), "# svc\n\nedited\n")
	runGit(t, src, "worktree", "add", "--quiet", "--detach", filepath.Join(t.TempDir(), "wt"))
	after, err := Fingerprint(ctx, src)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	want := []string{
		"git status --porcelain changed",
		"worktrees were 1, now 2",
		"ref added: refs/heads/eval/x",
	}
	if got := before.Changes(after); !reflect.DeepEqual(got, want) {
		t.Errorf("Changes() = %q, want %q", got, want)
	}
	runGit(t, src, "commit", "--quiet", "-am", "edit")
	moved, err := Fingerprint(ctx, src)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	got := strings.Join(after.Changes(moved), "; ")
	for _, w := range []string{"HEAD moved from", "ref moved: refs/heads/main"} {
		if !strings.Contains(got, w) {
			t.Errorf("Changes() after a commit = %q, want it to contain %q", got, w)
		}
	}
}
