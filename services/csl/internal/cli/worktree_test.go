package cli

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/worktree/worktreetest"
)

// TestMain points the working-directory rule at a directory in no checkout,
// so a test that runs a command never picks up the worktree this package
// may be checked out in. Tests of the rule set workingDir themselves.
func TestMain(m *testing.M) {
	workingDir = func() (string, error) { return os.TempDir(), nil }
	os.Exit(m.Run())
}

// setupCLIWorktree builds a clone org/repo with a linked worktree on branch
// feat, plus a clone org/repo-two beside it, configures csl to discover
// both clones, and returns the fixture. The worktree holds an uncommitted
// feat.go defining BranchOnly.
func setupCLIWorktree(t *testing.T) worktreetest.Fixture {
	t.Helper()
	root := t.TempDir()
	f := worktreetest.New(t, root, "feat")
	worktreetest.Write(t, f.Worktree, "feat.go", "package main\n\nfunc BranchOnly() {}\n")
	two := filepath.Join(filepath.Dir(f.Main), "repo-two")
	if err := os.MkdirAll(two, 0o755); err != nil {
		t.Fatal(err)
	}
	worktreetest.Git(t, two, "init", "-q", "-b", "main")
	worktreetest.Git(t, two, "remote", "add", "origin", "https://github.com/org/repo-two.git")
	setupTestConfig(t, "dirs:\n  - "+filepath.Dir(filepath.Dir(f.Main))+"\n")
	return f
}

// inDir sets the working directory the worktree rule sees for one test.
func inDir(t *testing.T, dir string) {
	t.Helper()
	prev := workingDir
	workingDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { workingDir = prev })
}

// withWorktreeFlags sets a command's worktree flags for one test.
func withWorktreeFlags(t *testing.T, flags *worktreeFlags, set worktreeFlags) {
	t.Helper()
	prev := *flags
	*flags = set
	t.Cleanup(func() { *flags = prev })
}

// TestWorktreeChoose: the working directory never picks a worktree on its
// own. --worktree names one (. for the one you are in); otherwise a --repo
// that selects only the worktree's repo picks it, unless --no-worktree is
// set.
func TestWorktreeChoose(t *testing.T) {
	f := setupCLIWorktree(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		flags  worktreeFlags
		cwd    string
		repo   string
		want   string
		byRepo bool
	}{
		{
			name:  "named",
			flags: worktreeFlags{arg: f.Worktree},
			cwd:   os.TempDir(),
			want:  "org/repo@feat",
		},
		{name: "dot", flags: worktreeFlags{arg: "."}, cwd: f.Worktree, want: "org/repo@feat"},
		{
			name:  "dot from a subdirectory",
			flags: worktreeFlags{arg: "."},
			cwd:   filepath.Join(f.Worktree, "pkg"),
			want:  "org/repo@feat",
		},
		{name: "cwd alone stays on the main index", cwd: f.Worktree},
		{
			name:   "repo selects only the worktree's",
			cwd:    f.Worktree,
			repo:   "org/repo$",
			want:   "org/repo@feat",
			byRepo: true,
		},
		{
			name:   "repo names the branch",
			cwd:    f.Worktree,
			repo:   "@feat",
			want:   "org/repo@feat",
			byRepo: true,
		},
		{name: "repo matches repo-two too", cwd: f.Worktree, repo: "repo"},
		{name: "other repo", cwd: f.Worktree, repo: "^dotfiles$"},
		{name: "opted out", flags: worktreeFlags{off: true}, cwd: f.Worktree, repo: "org/repo$"},
		{name: "main clone cwd", cwd: f.Main, repo: "org/repo$"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inDir(t, tt.cwd)
			choice, err := tt.flags.choose(cfg, tt.repo)
			if err != nil {
				t.Fatalf("choose: %v", err)
			}
			got, byRepo := "", false
			if choice != nil {
				got, byRepo = choice.repo.Name, choice.byRepo
			}
			if got != tt.want || byRepo != tt.byRepo {
				t.Errorf(
					"choose = %q (byRepo %v), want %q (byRepo %v)",
					got,
					byRepo,
					tt.want,
					tt.byRepo,
				)
			}
		})
	}
}

// TestRunSearch_WorktreeDot: --worktree . inside a linked worktree searches
// the worktree's own index, uncommitted files included, prints
// org/repo@feat paths, and adds no note, since the flag named it.
func TestRunSearch_WorktreeDot(t *testing.T) {
	keepSearchFlags(t)
	f := setupCLIWorktree(t)
	inDir(t, f.Worktree)
	withWorktreeFlags(t, &searchWorktreeFlags, worktreeFlags{arg: "."})
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := runSearch(cmd, []string{"BranchOnly"}); err != nil {
		t.Fatalf("csl search: %v", err)
	}
	if stdout.String() != "org/repo@feat/feat.go\n" {
		t.Errorf("stdout = %q, want the worktree's feat.go", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing for a named worktree", stderr.String())
	}
}

// TestRunSearch_RepoSelectsWorktree: inside a worktree, a --repo that
// selects only its repo searches the worktree and says so on stderr.
func TestRunSearch_RepoSelectsWorktree(t *testing.T) {
	keepSearchFlags(t)
	f := setupCLIWorktree(t)
	inDir(t, f.Worktree)
	searchRepoFlag = "org/repo$"
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := runSearch(cmd, []string{"BranchOnly"}); err != nil {
		t.Fatalf("csl search: %v", err)
	}
	if stdout.String() != "org/repo@feat/feat.go\n" {
		t.Errorf("stdout = %q, want the worktree's feat.go", stdout.String())
	}
	for _, want := range []string{"matches only org/repo", "org/repo@feat", "--no-worktree"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
}

// TestReadTarget_Worktree: inside a worktree csl read still needs --repo or
// --worktree; --worktree . reads the worktree, and a --repo that selects
// only its repo reads it too and says so.
func TestReadTarget_Worktree(t *testing.T) {
	f := setupCLIWorktree(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	prevRepo := readfileRepoFlag
	t.Cleanup(func() { readfileRepoFlag = prevRepo })
	inDir(t, f.Worktree)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	readfileRepoFlag = ""
	if _, err := readTarget(cfg, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "--repo is required") {
		t.Errorf("no --repo, no --worktree: err = %v, want the --repo refusal", err)
	}

	withWorktreeFlags(t, &readfileWorktree, worktreeFlags{arg: "."})
	var quiet bytes.Buffer
	if repo, err := readTarget(cfg, &quiet); err != nil || repo.Name != "org/repo@feat" {
		t.Errorf("--worktree .: %v, %v; want org/repo@feat", repo, err)
	}
	if quiet.Len() != 0 {
		t.Errorf("--worktree . printed %q, want nothing", quiet.String())
	}

	readfileWorktree = worktreeFlags{}
	readfileRepoFlag = "org/repo$"
	var note bytes.Buffer
	if repo, err := readTarget(cfg, &note); err != nil || repo.Name != "org/repo@feat" {
		t.Errorf("--repo org/repo$: %v, %v; want org/repo@feat", repo, err)
	}
	if !strings.Contains(note.String(), "matches only org/repo") {
		t.Errorf("stderr = %q, want the worktree note", note.String())
	}
}

// TestOutlineTarget_AnnouncesWorktree: a repo argument that selects only
// the worktree's repo outlines the worktree and says so.
func TestOutlineTarget_AnnouncesWorktree(t *testing.T) {
	f := setupCLIWorktree(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	inDir(t, f.Worktree)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	var note bytes.Buffer
	repo, path, err := outlineTarget(cfg, []string{"org/repo$", "pkg"}, &note)
	if err != nil || repo.Name != "org/repo@feat" || path != "pkg" {
		t.Errorf("outlineTarget = %s, %q, %v; want org/repo@feat, pkg", repo.Name, path, err)
	}
	if !strings.Contains(note.String(), "matches only org/repo") {
		t.Errorf("stderr = %q, want the worktree note", note.String())
	}
}
