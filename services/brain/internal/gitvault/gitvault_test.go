package gitvault_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/gitvault"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsRepoNeedsTheTopLevel(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	if gitvault.New(root).IsRepo() {
		t.Fatal("a plain directory is a repo")
	}
	testgit.Init(t, root)
	if !gitvault.New(root).IsRepo() {
		t.Fatal("the top of a work tree is not a repo")
	}
	sub := filepath.Join(root, "vault")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if gitvault.New(sub).IsRepo() {
		t.Fatal("a directory inside another repo counts as a vault repo")
	}
}

func TestCommitTakesOnlyItsPaths(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", map[string]string{
		"notes/a.md": "a\n", "notes/b.md": "b\n", "inbox/old.md": "old\n",
	})
	// A person's own edits: one staged, one not, one untracked.
	write(t, root, "notes/b.md", "b edited by hand\n")
	write(t, root, "notes/staged.md", "staged by hand\n")
	testgit.Run(t, root, "add", "notes/staged.md")
	write(t, root, "notes/untracked.md", "untracked\n")

	// brain's change: edit a, delete old, create c, and a path that never
	// existed in git or on disk.
	write(t, root, "notes/a.md", "a by brain\n")
	write(t, root, "notes/c.md", "c by brain\n")
	if err := os.Remove(filepath.Join(root, "inbox", "old.md")); err != nil {
		t.Fatal(err)
	}
	repo := gitvault.New(root)
	paths := []string{"notes/a.md", "notes/c.md", "inbox/old.md", "inbox/never.md"}
	if err := repo.Commit("brain: edit notes/a.md", paths); err != nil {
		t.Fatal(err)
	}

	if got := testgit.Subjects(t, root); got[0] != "brain: edit notes/a.md" {
		t.Fatalf("subjects = %v", got)
	}
	files := strings.Fields(testgit.Run(t, root, "show", "--name-only", "--format=", "HEAD"))
	slices.Sort(files)
	if want := []string{"inbox/old.md", "notes/a.md", "notes/c.md"}; !slices.Equal(files, want) {
		t.Errorf("commit holds %v, want %v", files, want)
	}
	status := testgit.Status(t, root)
	for _, want := range []string{"M notes/b.md", "A  notes/staged.md", "?? notes/untracked.md"} {
		if !strings.Contains(status, want) {
			t.Errorf("status %q lost the person's change %q", status, want)
		}
	}

	dirty, err := repo.Dirty()
	if err != nil || !slices.Equal(dirty, []string{"notes/b.md", "notes/staged.md"}) {
		t.Errorf("Dirty() = %q, %v; want the hand edits, untracked files left out", dirty, err)
	}

	// Nothing changed: no empty commit.
	if err := repo.Commit("brain: edit notes/a.md", []string{"notes/a.md"}); err != nil {
		t.Fatal(err)
	}
	if n := len(testgit.Subjects(t, root)); n != 2 {
		t.Errorf("an unchanged path made a commit: %d commits", n)
	}
}

func TestCommitOutsideARepoDoesNothing(t *testing.T) {
	testgit.Isolate(t)
	// git's answer is read in the C locale, so a translated one is still
	// a clean "not a git repository".
	t.Setenv("LC_ALL", "sv_SE.UTF-8")
	t.Setenv("LANGUAGE", "sv")
	root := t.TempDir()
	write(t, root, "notes/a.md", "a\n")
	if err := gitvault.New(root).Commit("brain: write notes/a.md", []string{"notes/a.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("commit made a repo: %v", err)
	}
}

func TestCommitWithoutGitOnPathDoesNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := gitvault.New(root).Commit("brain: add one", []string{"one.md"}); err != nil {
		t.Fatalf("Commit with no git on PATH: %v", err)
	}
}

func TestFirstCommitInAnEmptyRepo(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	testgit.Init(t, root)
	write(t, root, "notes/a.md", "a\n")
	if err := gitvault.New(root).Commit("brain: write notes/a.md", []string{"notes/a.md"}); err != nil {
		t.Fatal(err)
	}
	if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"brain: write notes/a.md"}) {
		t.Errorf("subjects = %q", got)
	}
}

// TestCommitReportsAGitThatCannotRun pins that only git's own "not a git
// repository" means no commit: a git that cannot read its config fails the
// commit, in a repo and in a plain directory alike.
func TestCommitReportsAGitThatCannotRun(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", map[string]string{"notes/a.md": "a\n"})
	write(t, root, "notes/a.md", "a by brain\n")
	plain := t.TempDir()
	write(t, plain, "notes/a.md", "a\n")
	testgit.UnreadableGlobalConfig(t)

	for _, dir := range []string{root, plain} {
		err := gitvault.New(dir).Commit("brain: edit notes/a.md", []string{"notes/a.md"})
		if err == nil || !strings.Contains(err.Error(), "unable to access") {
			t.Errorf("Commit() in %s = %v, want git's failure to read its config", dir, err)
		}
	}
	if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"seed"}) {
		t.Errorf("subjects = %q", got)
	}
	if staged := testgit.Staged(t, root); len(staged) != 0 {
		t.Errorf("staged = %q, want nothing", staged)
	}
}

// TestFailedCommitUnstagesItsPaths puts the index of brain's paths back as
// it was before the commit, a person's staged version of one of them
// included, and leaves every file on disk as brain wrote it.
func TestFailedCommitUnstagesItsPaths(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", map[string]string{
		"notes/a.md": "a\n", "notes/b.md": "b\n", "inbox/old.md": "old\n",
	})
	// A person's staged edits: one to a note brain leaves alone, one to a
	// note brain then changes.
	write(t, root, "notes/hand.md", "staged by hand\n")
	write(t, root, "notes/b.md", "b staged by hand\n")
	testgit.Run(t, root, "add", "notes/hand.md", "notes/b.md")
	testgit.Hook(t, root, "pre-commit", "echo 'hook says no' >&2\nexit 1\n")

	write(t, root, "notes/a.md", "a by brain\n")
	write(t, root, "notes/b.md", "b by brain\n")
	write(t, root, "notes/c.md", "c by brain\n")
	if err := os.Remove(filepath.Join(root, "inbox", "old.md")); err != nil {
		t.Fatal(err)
	}
	paths := []string{"notes/a.md", "notes/b.md", "notes/c.md", "inbox/old.md"}
	err := gitvault.New(root).Commit("brain: edit notes/a.md", paths)
	if err == nil || !strings.Contains(err.Error(), "hook says no") ||
		strings.Contains(err.Error(), "still staged") {
		t.Fatalf("Commit() = %v, want the hook's refusal alone", err)
	}

	if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"seed"}) {
		t.Errorf("subjects = %q", got)
	}
	if staged := testgit.Staged(t, root); !slices.Equal(
		staged,
		[]string{"notes/b.md", "notes/hand.md"},
	) {
		t.Errorf("staged = %q, want the person's two staged notes alone", staged)
	}
	if b := testgit.Run(t, root, "show", ":notes/b.md"); b != "b staged by hand" {
		t.Errorf("index holds notes/b.md as %q, want the person's staged version", b)
	}
	unstaged := strings.Fields(testgit.Run(t, root, "diff", "--name-only"))
	if want := []string{"inbox/old.md", "notes/a.md", "notes/b.md"}; !slices.Equal(unstaged, want) {
		t.Errorf("unstaged changes = %q, want %q", unstaged, want)
	}
	if untracked := testgit.Run(t, root, "ls-files", "--others"); untracked != "notes/c.md" {
		t.Errorf("untracked = %q, want notes/c.md", untracked)
	}
	for rel, want := range map[string]string{
		"notes/a.md": "a by brain\n", "notes/b.md": "b by brain\n", "notes/c.md": "c by brain\n",
	} {
		if data, err := os.ReadFile(filepath.Join(root, rel)); err != nil || string(data) != want {
			t.Errorf("%s on disk = %q, %v; want %q", rel, data, err, want)
		}
	}
}

// TestFailedUnstageSaysThePathsAreStillStaged runs a hook that refuses the
// commit and breaks the repo's config, so the unstage after it fails too.
func TestFailedUnstageSaysThePathsAreStillStaged(t *testing.T) {
	testgit.Isolate(t)
	root := t.TempDir()
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", map[string]string{"notes/a.md": "a\n"})
	testgit.Hook(t, root, "pre-commit", "printf '[broken' >> .git/config\nexit 1\n")
	write(t, root, "notes/a.md", "a by brain\n")
	err := gitvault.New(root).Commit("brain: edit notes/a.md", []string{"notes/a.md"})
	if err == nil || !strings.Contains(err.Error(), "notes/a.md still staged") {
		t.Fatalf("Commit() = %v, want it to say notes/a.md is still staged", err)
	}
}

func TestRemoteHelpers(t *testing.T) {
	testgit.Isolate(t)
	bare := testgit.Bare(t)
	seed := filepath.Join(t.TempDir(), "seed")
	testgit.Clone(t, bare, seed)
	testgit.Commit(t, seed, "seed", map[string]string{"notes/a.md": "a\n"})
	testgit.Run(t, seed, "push", "-q", "origin", "HEAD:main")

	local := filepath.Join(t.TempDir(), "local")
	testgit.Clone(t, bare, local)
	repo := gitvault.New(local)
	remote, branch, err := repo.Upstream()
	if err != nil || remote != "origin" || branch != "main" {
		t.Fatalf("Upstream() = %q, %q, %v", remote, branch, err)
	}

	testgit.Commit(t, seed, "upstream change", map[string]string{"notes/b.md": "b\n"})
	testgit.Run(t, seed, "push", "-q", "origin", "HEAD:main")
	if err := repo.Fetch("origin"); err != nil {
		t.Fatal(err)
	}
	ahead, behind, err := repo.Counts("origin/main")
	if err != nil || ahead != 0 || behind != 1 {
		t.Fatalf("Counts() = %d, %d, %v", ahead, behind, err)
	}
	if err := repo.PullFFOnly(); err != nil {
		t.Fatal(err)
	}
	write(t, local, "notes/c.md", "c\n")
	if err := repo.Commit("brain: write notes/c.md", []string{"notes/c.md"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(remote, branch); err != nil {
		t.Fatal(err)
	}
	if got := testgit.Run(t, bare, "log", "-1", "--format=%s", "main"); got != "brain: write notes/c.md" {
		t.Errorf("remote head = %q", got)
	}

	wtDir := filepath.Join(t.TempDir(), "wt")
	wt, err := repo.AddWorktree(wtDir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	write(t, wtDir, "notes/d.md", "d\n")
	if err := wt.Commit("brain: promote notes/d.md", []string{"notes/d.md"}); err != nil {
		t.Fatal(err)
	}
	if err := wt.Push("origin", "brain/promote/d"); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.RemoteBranchExists("origin", "brain/promote/d"); err != nil || !ok {
		t.Fatalf("RemoteBranchExists() = %v, %v", ok, err)
	}
	if err := repo.RemoveWorktree(wtDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("worktree dir left behind: %v", err)
	}
	if testgit.Status(t, local) != "" {
		t.Errorf("the worktree changed the checkout: %q", testgit.Status(t, local))
	}

	testgit.Run(t, local, "checkout", "-q", "--detach")
	if _, _, err := repo.Upstream(); !errors.Is(err, gitvault.ErrNoUpstream) {
		t.Errorf("detached Upstream() err = %v", err)
	}
}
