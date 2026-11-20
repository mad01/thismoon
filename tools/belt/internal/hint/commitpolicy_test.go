package hint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newCommitPolicyHint(branch, repo string, exclude []string) *CommitPolicy {
	h := NewCommitPolicy(config.Config{
		Hints: map[string]config.Toggle{CommitPolicyID: {ExcludeRepos: exclude}},
	})
	h.resolveBranch = func(string) string { return branch }
	h.resolveRepo = func(string) string { return repo }
	h.resolveRoot = func(string) string { return "" } // no overlay unless a test sets one
	h.landed = func(string) bool { return true }
	return h
}

// withOverlay points the hint's overlay lookup at a temp repo root holding
// the given .belt.yaml content, or an empty root when content is empty.
func withOverlay(t *testing.T, h *CommitPolicy, content string) {
	t.Helper()
	root := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(root, OverlayFileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.resolveRoot = func(string) string { return root }
}

func TestCommitPolicy(t *testing.T) {
	const thismoon = "github.com/mad01/thismoon"
	const dotfiles = "github.com/mad01/dotfiles"
	exclude := []string{dotfiles}

	tests := []struct {
		name    string
		command string
		branch  string
		repo    string
		exclude []string
		fires   bool
	}{
		{"commit on main", "git commit -m 'x'", "main", thismoon, exclude, true},
		{"commit on master", "git commit -m 'x'", "master", thismoon, exclude, true},
		{"commit on feature branch", "git commit -m 'x'", "feat/x", thismoon, exclude, false},
		{"detached HEAD", "git commit -m 'x'", "HEAD", thismoon, exclude, false},
		{"excluded repo", "git commit -m 'x'", "main", dotfiles, exclude, false},
		{
			"org wildcard excludes",
			"git commit -m 'x'",
			"main",
			thismoon,
			[]string{"github.com/mad01/*"},
			false,
		},
		{"no exclusions fires", "git commit -m 'x'", "main", thismoon, nil, true},
		{"unresolved repo silent", "git commit -m 'x'", "main", "", exclude, false},
		{"non-commit git", "git status", "main", thismoon, exclude, false},
		{"quoted mention not a commit", `echo "git commit -m x"`, "main", thismoon, exclude, false},
		{"compound command", "git add -A && git commit -m 'x'", "main", thismoon, exclude, true},
		{"commit then push", "git commit -m 'x'; git push", "main", thismoon, exclude, true},
		{"empty command", "", "main", thismoon, exclude, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCommitPolicyHint(tt.branch, tt.repo, tt.exclude)
			a := h.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/some/repo"})
			if got := a != nil; got != tt.fires {
				t.Fatalf(
					"Check(%q) fired = %v, want %v (advice: %+v)",
					tt.command,
					got,
					tt.fires,
					a,
				)
			}
			if a == nil {
				return
			}
			if a.Hint != CommitPolicyID {
				t.Errorf("advice hint = %q, want %q", a.Hint, CommitPolicyID)
			}
			for _, want := range []string{tt.branch, tt.repo, "git switch -c"} {
				if !strings.Contains(a.Text, want) {
					t.Errorf("advice %q does not mention %q", a.Text, want)
				}
			}
		})
	}
}

// TestCommitPolicyOverlay pins the repo-local .belt.yaml semantics
// (docs/adr/0012): local overrides the machine lists in both directions,
// protected_branches replaces the default set, message is appended, a
// foreign file is a no-op, and a broken file degrades to advisory text.
func TestCommitPolicyOverlay(t *testing.T) {
	const thismoon = "github.com/mad01/thismoon"

	tests := []struct {
		name     string
		overlay  string
		branch   string
		exclude  []string // machine exclude_repos
		fires    bool
		wantText string // required substring when it fires
	}{
		{
			"exclude true silences",
			"hints:\n  commit-policy:\n    exclude: true\n",
			"main", nil, false, "",
		},
		{
			"exclude false overrides machine exclusion",
			"hints:\n  commit-policy:\n    exclude: false\n",
			"main",
			[]string{thismoon},
			true, "feature branch + PR",
		},
		{
			"protected_branches replaces the default set",
			"hints:\n  commit-policy:\n    protected_branches: [\"release/*\"]\n",
			"release/1.2", nil, true, "release/1.2",
		},
		{
			"replaced set drops main",
			"hints:\n  commit-policy:\n    protected_branches: [\"release/*\"]\n",
			"main", nil, false, "",
		},
		{
			"message is appended",
			"hints:\n  commit-policy:\n    message: main is PR-only here.\n",
			"main", nil, true, "Repo policy: main is PR-only here.",
		},
		{
			"foreign file is a no-op",
			"tool: something-else\n",
			"main", nil, true, "feature branch + PR",
		},
		{
			"missing file keeps defaults",
			"",
			"main", nil, true, "feature branch + PR",
		},
		{
			"broken file degrades to advisory",
			"hints: [not a map\n",
			"feat/x", nil, true, "could not evaluate this repo's commit policy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCommitPolicyHint(tt.branch, thismoon, tt.exclude)
			withOverlay(t, h, tt.overlay)
			a := h.Check(Input{Event: EventBash, Command: "git commit -m 'x'", Cwd: "/some/repo"})
			if got := a != nil; got != tt.fires {
				t.Fatalf("fired = %v, want %v (advice: %+v)", got, tt.fires, a)
			}
			if a != nil && !strings.Contains(a.Text, tt.wantText) {
				t.Errorf("advice %q does not mention %q", a.Text, tt.wantText)
			}
		})
	}
}

// TestCommitPolicyDirectMainRepos pins that the shared direct_main_repos
// list silences the hint (its only hint-side reader, docs/adr/0013), and
// that a repo-file opt-in still wins over it (local overrides global,
// docs/adr/0012).
func TestCommitPolicyDirectMainRepos(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	newHint := func() *CommitPolicy {
		h := NewCommitPolicy(config.Config{DirectMainRepos: []string{dotfiles}})
		h.resolveBranch = func(string) string { return "main" }
		h.resolveRepo = func(string) string { return dotfiles }
		h.resolveRoot = func(string) string { return "" }
		h.landed = func(string) bool { return true }
		return h
	}
	in := Input{Event: EventBash, Command: "git commit -m 'x'", Cwd: "/some/repo"}

	if a := newHint().Check(in); a != nil {
		t.Errorf("direct-main repo should be silent, got %+v", a)
	}
	h := newHint()
	withOverlay(t, h, "hints:\n  commit-policy:\n    exclude: false\n")
	if a := h.Check(in); a == nil {
		t.Error("overlay opt-in should override the direct-main silencing")
	}
}

// TestCommitPolicyDirResolution pins which directory the resolvers see: the
// session cwd for a bare commit, the directory a `cd`/`pushd` before it
// landed in (MAD-366), the `git -C` target otherwise, with ~ and $HOME
// expanded the way the shell would (MAD-370). A target only a shell can
// resolve leaves the directory unknown, and the hint stays quiet rather than
// judging the session cwd.
func TestCommitPolicyDirResolution(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	const worklog = "/Users/tester/code/worklog"
	tests := []struct {
		name    string
		command string
		cwd     string
		wantDir string // "" means no lookup and no advice
	}{
		{"bare commit uses cwd", "git commit -m 'x'", "/session/cwd", "/session/cwd"},
		{"git -C wins over cwd", "git -C /other/repo commit -m 'x'", "/session/cwd", "/other/repo"},
		{"tilde -C", "git -C ~/code/worklog commit -m 'x'", "/session/cwd", worklog},
		{"HOME -C", "git -C $HOME/code/worklog commit -m 'x'", "/session/cwd", worklog},
		{"braced HOME -C", "git -C ${HOME}/code/worklog commit -m 'x'", "/session/cwd", worklog},
		{"quoted HOME -C", `git -C "$HOME/code/worklog" commit -m 'x'`, "/session/cwd", worklog},
		{
			"cd then bare commit",
			"cd /other/repo && git commit -m 'x'",
			"/session/cwd",
			"/other/repo",
		},
		{
			"tilde cd into a worktree",
			"cd ~/.worktrees/repo/slug && git commit -m 'x'",
			"/session/cwd",
			"/Users/tester/.worktrees/repo/slug",
		},
		{"unresolvable cd stays silent", "cd $TARGET && git commit -m 'x'", "/session/cwd", ""},
		{"unresolvable -C stays silent", "git -C $TARGET commit -m 'x'", "/session/cwd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			asked := false
			h := newCommitPolicyHint("main", "github.com/mad01/thismoon", nil)
			h.resolveBranch = func(dir string) string {
				asked, got = true, dir
				return "main"
			}
			a := h.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if tt.wantDir == "" {
				if asked || a != nil {
					t.Fatalf("unknown directory: asked=%v advice=%+v, want silence", asked, a)
				}
				return
			}
			if a == nil {
				t.Fatal("Check returned nil, want advice")
			}
			if got != tt.wantDir {
				t.Errorf("resolver saw dir %q, want %q", got, tt.wantDir)
			}
		})
	}
}

// initRepoOnMain creates a git repository at dir on branch main, with one
// commit and the given origin URL.
func initRepoOnMain(t *testing.T, dir, origin string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
		{"remote", "add", "origin", origin},
	} {
		if out, err := gitCommand(dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// TestCommitPolicyWorktreeCommit runs the hint with its real git-backed
// resolvers against the loom layout (MAD-366): the session cwd is the
// canonical checkout on main, and the commit runs inside a linked worktree on
// a feature branch via `cd <worktree> && git commit`. The branch has to be
// read where the commit ran, so the worktree commit stays quiet while a bare
// commit in the session cwd still draws the advice.
func TestCommitPolicyWorktreeCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := isolateGit(t)
	canonical := filepath.Join(home, "canonical")
	worktree := filepath.Join(home, ".worktrees", "repo", "slug")
	initRepoOnMain(t, canonical, "git@github.com:mad01/thismoon.git")
	if out, err := gitCommand(canonical, "worktree", "add", "-q", "-b", "feat/x", worktree).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// The commits initRepoOnMain made seconds ago are the landed commits the
	// firing cases are judged by; this test pins the directory, not the
	// reflog (TestCommitPolicyOnlyLanded).
	h := NewCommitPolicy(config.Config{})
	tests := []struct {
		name    string
		command string
		fires   bool
	}{
		{"commit in the worktree is quiet", "cd " + worktree + " && git commit -m x", false},
		{
			"tilde cd into the worktree is quiet",
			"cd ~/.worktrees/repo/slug && git commit -m x",
			false,
		},
		{"git -C the worktree is quiet", "git -C " + worktree + " commit -m x", false},
		{"bare commit in the session cwd fires", "git commit -m x", true},
		{
			"cd back to the canonical checkout fires",
			"cd " + worktree + " && cd " + canonical + " && git commit -m x",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := h.Check(Input{Event: EventBash, Command: tt.command, Cwd: canonical})
			if got := a != nil; got != tt.fires {
				t.Fatalf(
					"Check(%q) fired = %v, want %v (advice: %+v)",
					tt.command,
					got,
					tt.fires,
					a,
				)
			}
			if a != nil && !strings.Contains(a.Text, "landed on main") {
				t.Errorf("advice %q does not name main", a.Text)
			}
		})
	}
}

// TestRecentCommitEntry pins how a HEAD reflog line reads: every subject git
// commit writes counts when it is no older than the cutoff, and any other
// entry, an old commit, or a line it cannot parse does not.
func TestRecentCommitEntry(t *testing.T) {
	cutoff := time.Unix(1_700_000_000, 0)
	tests := []struct {
		line string
		want bool
	}{
		{"HEAD@{1700000000}\tcommit: add x", true},
		{"HEAD@{1700000005}\tcommit (amend): add x", true},
		{"HEAD@{1700000005}\tcommit (initial): init", true},
		{"HEAD@{1700000005}\tcommit (merge): Merge branch 'x'", true},
		{"HEAD@{1700000005}\tcommit (cherry-pick): add x", true},
		{"HEAD@{1700000005}\tcommit:", true},
		{"HEAD@{1699999999}\tcommit: add x", false},
		{"HEAD@{1700000005}\tcheckout: moving from feat to main", false},
		{"HEAD@{1700000005}\tpull --rebase (finish): returning to refs/heads/main", false},
		{"HEAD@{1700000005}\treset: moving to HEAD~1", false},
		{"HEAD@{1700000005}\tcommitted: not git's", false},
		{"HEAD@{yesterday}\tcommit: add x", false},
		{"HEAD@{1700000005} commit: add x", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := recentCommitEntry(tt.line, cutoff); got != tt.want {
			t.Errorf("recentCommitEntry(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

// TestCommitPolicyNotLanded pins that the advice needs a landed commit: with
// everything else firing, a repo whose reflog shows none stays silent.
func TestCommitPolicyNotLanded(t *testing.T) {
	h := newCommitPolicyHint("main", "github.com/mad01/thismoon", nil)
	h.landed = func(string) bool { return false }
	if a := h.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/some/repo"}); a != nil {
		t.Errorf("Check without a landed commit = %+v, want silence", a)
	}
}

// TestCommitPolicyOnlyLanded runs each command for real in a repo on main
// and then the hint with its real git-backed resolvers: the advice names a
// commit, so only a commit that landed draws it. A dry run, a commit with
// nothing staged, one a pre-commit hook stops, and one whose failure a chain
// hides behind a later exit 0 stay silent. A commit in a chain, an amend,
// one with -n (--no-verify, not a dry run) and one that git -C or a cd
// places still fire. Each case starts from a repo whose newest commit is
// old, so no fixture commit counts as the command's own.
func TestCommitPolicyOnlyLanded(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_ = isolateGit(t)
	const hook = "printf '#!/bin/sh\\nexit 1\\n' > .git/hooks/pre-commit && " +
		"chmod +x .git/hooks/pre-commit"
	tests := []struct {
		name    string
		setup   string // run in the repo first
		command string // {repo} names the repo; run in the parent dir when outside is set
		outside bool
		fails   bool // the command's own exit status, so a case fails for its reason
		fires   bool
	}{
		{name: "dry run on a clean tree", command: "git commit --dry-run -m x", fails: true},
		{
			name:    "dry run with staged changes",
			setup:   "echo a > f && git add f",
			command: "git commit --dry-run -m x",
		},
		{
			name:    "dry run right after a commit",
			setup:   "echo a > f && git add f && git commit -q -m first && echo b > f && git add f",
			command: "git commit --dry-run -m x",
		},
		{name: "nothing staged", command: "git commit -m x", fails: true},
		{
			name:    "pre-commit hook stops it",
			setup:   "echo a > f && git add f && " + hook,
			command: "git commit -m x",
			fails:   true,
		},
		{name: "failed commit inside a chain that exits 0", command: "git commit -m x; echo done"},
		{
			name:    "commit in a chain",
			command: "echo a > f && git add f && git commit -q -m x",
			fires:   true,
		},
		{name: "amend", command: "git commit -q --amend --allow-empty -m x", fires: true},
		{name: "-n is not a dry run", command: "git commit -q -n --allow-empty -m x", fires: true},
		{
			name:    "git -C",
			command: "git -C {repo} commit -q --allow-empty -m x",
			outside: true,
			fires:   true,
		},
		{
			name:    "cd then commit",
			command: "cd {repo} && git commit -q --allow-empty -m x",
			outside: true,
			fires:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			repo := filepath.Join(parent, "repo")
			initRepoOnMain(t, repo, "git@github.com:mad01/thismoon.git")
			ageHead(t, repo)
			if tt.setup != "" {
				if out, err := shell(repo, tt.setup).CombinedOutput(); err != nil {
					t.Fatalf("setup %q: %v\n%s", tt.setup, err, out)
				}
			}
			cwd := repo
			if tt.outside {
				cwd = parent
			}
			command := strings.ReplaceAll(tt.command, "{repo}", repo)
			out, err := shell(cwd, command).CombinedOutput()
			if failed := err != nil; failed != tt.fails {
				t.Fatalf("%q failed = %v, want %v\n%s", command, failed, tt.fails, out)
			}
			a := NewCommitPolicy(
				config.Config{},
			).Check(Input{Event: EventBash, Command: command, Cwd: cwd})
			if got := a != nil; got != tt.fires {
				t.Fatalf("Check(%q) fired = %v, want %v (advice: %+v)", command, got, tt.fires, a)
			}
		})
	}
}

// isolateGit keeps the user's and the system's gitconfig out of a fixture,
// where a commit.gpgsign or hooksPath would break the setup commits or skip
// the repo's own hooks, and names the committer. It returns the temp dir it
// sets as HOME.
func isolateGit(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	return home
}

// ageHead adds an empty commit dated long ago, so the newest HEAD reflog
// entry is a commit older than landedWindow: git stamps the entry with the
// committer date.
func ageHead(t *testing.T, repo string) {
	t.Helper()
	cmd := gitCommand(repo, "commit", "-q", "--allow-empty", "-m", "aged")
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=@1700000000 +0000")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("aged commit: %v\n%s", err, out)
	}
}

// shell runs a command line in dir the way the Bash tool would.
func shell(dir, command string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	return cmd
}
