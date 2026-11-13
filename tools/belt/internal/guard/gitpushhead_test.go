package guard

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// switchTestBranches are the local branches the repo of the branch switch
// tests has (GitPushMain.resolveHasBranch); gone is a name it lacks.
var switchTestBranches = wordSet("main master feature fix/x fix/y")

// branchSwitches (MAD-579) pin how a git checkout or switch earlier in the
// command changes the current branch a later push reads. A switch to a
// default branch counts as that branch, and one the guard cannot read counts
// as unknown. The branch git reads before the command still counts, unless
// the push runs only after a sure switch in its own directory succeeded,
// joined to it by && with nothing between that may move HEAD another way.
// Every push runs in /tmp unless the row moves it.
var branchSwitches = []struct {
	current, command string
	deny             bool
}{
	// A switch to a default branch, or one the guard cannot read.
	{"feature", "git checkout main && git push", true},
	{"feature", "git checkout main && git push origin HEAD", true},
	{"feature", "git switch main && git push -u origin HEAD", true},
	{"feature", "git switch main; git push origin HEAD", true},
	{"feature", `git checkout main; git push origin "$(git branch --show-current)"`, true},
	{"feature", "git checkout main && git pull && git push", true},
	{"feature", "git checkout main && git merge fix/x && git push", true},
	{"feature", "git checkout -B main origin/main && git push origin HEAD", true},
	{"feature", "git switch -t origin/main && git push", true},
	{"feature", "git checkout master && git push", true},
	{"feature", "git checkout master && git push origin @", true},
	{"feature", "git checkout refs/heads/main && git push", true},
	{"feature", `git checkout "$B" && git push`, true},
	{"feature", "git checkout - && git push", true},
	{"feature", "git checkout @{-1} && git push", true},
	{"feature", "git switch --detach && git push", true},
	{"feature", "git switch -c main && git push -u origin HEAD", true},
	{"feature", "git checkout -b master && git push", true},
	{"feature", "git switch --create=main && git push -u origin HEAD", true},
	{"feature", "git switch --track origin/main && git push origin HEAD", true},
	{"feature", "git checkout -t origin/main && git push", true},
	{"feature", "git switch -fc main && git push origin HEAD", true},
	{"feature", "(git checkout main) && git push", true},
	{"feature", "echo $(git switch main) && git push", true},
	{"feature", "sh -c 'git switch main' && git push", true},
	{"feature", "git switch main & git push", true},
	{"feature", "git switch -c f && git switch main && git push origin HEAD", true},

	// A sure switch replaces the branch only when && alone joins it to the push.
	{"main", "git switch -c feature; git push -u origin HEAD", true},
	{"main", "true || git switch -c feature && git push -u origin HEAD", true},
	{"main", "git switch -c feature | cat && git push -u origin HEAD", true},
	{"main", "git switch -c feature && true; git push -u origin HEAD", true},
	{"main", "git switch -c x 2>/dev/null || true && git push origin HEAD", true},
	{"main", "git switch -c x || git push origin HEAD", true},
	{"main", "( git switch -c x ) && git push origin HEAD", true},
	{"main", "echo $(git switch -c x) && git push origin HEAD", true},
	{"main", "git switch -c f && (git checkout main) && git push origin HEAD", true},

	// Finding 1: only a switch in the push's own known directory counts.
	{"main", "git switch -c zz && git -C /other push origin HEAD", true},
	{"main", "cd /a && git switch -c zz && cd /b && git push origin HEAD", true},
	{"main", "git switch -c zz && cd /b && git push", true},
	{"main", "git -C /other switch -c zz && git push origin HEAD", true},
	{"main", "git switch -c zz && git -C /other checkout main && git push origin HEAD", true},
	{"main", "GIT_DIR=/r/.git git -C /tmp switch -c zz && git push origin HEAD", true},
	{"main", "git -C /tmp switch -c zz && git push origin HEAD", false},
	{"main", "git -C /a switch -c zz && git -C /a push origin HEAD", false},
	{"main", "cd /a && git switch -c zz && cd /a && git push origin HEAD", false},
	{"feature", "git -C /other checkout main && git push", false},
	{"feature", "cd /other && git checkout main && cd /tmp && git push", false},

	// Finding 2: the switch must run as git itself, not negated.
	{"main", "echo git checkout -b zz && git push -u origin HEAD", true},
	{"main", ": git switch -c zz && git push origin HEAD", true},
	{"main", "! git switch -c feature && git push origin HEAD", true},
	{"main", "xargs git switch -c zz && git push origin HEAD", true},
	{"main", "if git switch -c feature; then git push origin HEAD; fi", true},
	{"main", "FOO=1 git switch -c zz && git push origin HEAD", false},
	{"main", "env git switch -c zz && git push origin HEAD", false},
	{"main", "timeout 5 git switch -c zz && git push origin HEAD", false},

	// Finding 3: git checkout <name> is sure once <name> is a local branch.
	{
		"feature",
		"git checkout main && git pull --rebase && git checkout fix/x && git rebase main && git push --force-with-lease",
		false,
	},
	{
		"feature",
		"git checkout main && git pull --rebase && git checkout gone && git rebase main && git push --force-with-lease",
		true,
	},
	{"main", "git checkout fix/x && git push", false},
	{"main", "git checkout gone && git push", true},
	{"main", "git checkout fix/x; git push", true},
	{"feature", "git checkout main && git pull && git switch fix/x && git push", false},
	{
		"feature",
		"git stash && git checkout main && git pull && git checkout - && git stash pop && git push",
		true,
	},

	// Finding 5: a bundle of known short options reads.
	{"main", "git switch -qc zz && git push -u origin HEAD", false},
	{"main", "git switch -qczz && git push -u origin HEAD", false},
	{"main", "git checkout -qb zz && git push -u origin HEAD", false},
	{"main", "git switch -cq zz && git push -u origin HEAD", false},
	{"main", "git switch -qx zz && git push -u origin HEAD", true},
	{"main", "git switch -tc zz && git push -u origin HEAD", true},
	{"feature", "git switch -qc main && git push origin HEAD", true},
	{"feature", "git checkout -qp && git push", false},

	// A step between the switch and the push that may move HEAD another way.
	{"main", "git switch -c x && git co main && git push origin HEAD", true},
	{"main", "git switch -c x && git branch -M main && git push -u origin HEAD", true},
	{"main", "git switch -c x && ./release.sh && git push origin HEAD", true},
	{"main", "git switch -c x && go test ./... && git push -u origin HEAD", true},
	{"main", "git switch -c x && git rebase origin/main main && git push origin HEAD", true},
	{"main", "git switch -c x && git rebase --root main && git push origin HEAD", true},
	{"main", "git switch -c x && git stash branch y && git push origin HEAD", true},
	{"main", "git switch -c x && echo ref > .git/HEAD && git push origin HEAD", true},
	{
		"main",
		"git checkout -b fix/y && git add . && git commit -m x && git push -u origin HEAD",
		false,
	},
	{"main", "git switch fix/x && git rebase main && git push -f", false},
	{"main", "git switch -c x && git branch --merged && git push origin HEAD", true},
	{"main", "git switch -c x && git checkout -- f.txt && git push origin HEAD", true},
	{"main", "git switch -c x && xargs git add && git push origin HEAD", true},
	{
		"main",
		"git switch -c x && git fetch && git pull && git merge y && git push origin HEAD",
		false,
	},
	{
		"main",
		"git switch -c x && git status && git log -1 && git diff && git push origin HEAD",
		false,
	},
	{"main", "git switch -c x && echo done && git push origin HEAD", false},

	// A shell function defined anywhere in the command may stand in for git or
	// a listed step, so no push is sure. A listed git step with a -c override
	// of its own, an --output option or a path in .git may move HEAD.
	{"main", `git() { command git "$@"; }; git switch -c x && git push origin HEAD`, true},
	{"main", `function git { command git "$@"; }; git switch -c x && git push origin HEAD`, true},
	{
		"main",
		"cat() { git checkout main; }; git switch -c x && cat f && git push origin HEAD",
		true,
	},
	{"main", "cd() { git checkout main; }; git switch -c x && cd . && git push origin HEAD", true},
	{"main", "git switch -c x && git push origin HEAD && cat() { :; }", true},
	{"main", `git switch -c x && git commit -m "fix parse() crash" && git push origin HEAD`, false},
	{
		"main",
		"git switch -c x && git -c core.hooksPath=/tmp/h commit -m x && git push origin HEAD",
		true,
	},
	{"main", "git switch -c x && git log -1 --output=.git/HEAD && git push origin HEAD", true},
	{"main", "git switch -c x && git diff --output /tmp/d.txt && git push origin HEAD", true},
	{"main", "git switch -c x && git diff --out=/tmp/d.txt && git push origin HEAD", true},
	{"main", "git switch -c x && git log --oneline -3 && git push origin HEAD", false},

	// A step after the push that may move HEAD may run beside it. The cost:
	// returning to main after the push, or opening the PR, in the same command.
	{"main", "git switch -c x && git push origin HEAD & git checkout main", true},
	{"main", "git switch -c x && { git push origin HEAD; } & git co main", true},
	{"main", "git switch -c x && git push -u origin HEAD && git checkout main", true},
	{"main", "git switch -c x && git push -u origin HEAD && gh pr create --fill", true},
	{"main", "git switch -c x && git push -u origin HEAD 2>&1 | tail -3", false},
	{"main", "git switch -c x && git push -u origin HEAD && git status", false},

	// Passes.
	{"feature", "git checkout -b feature2 && git push -u origin feature2", false},
	{"main", "git checkout -b feature && git push -u origin feature", false},
	{"main", "git switch -c feature && git push -u origin HEAD", false},
	{"main", "git checkout -b feature && git push -u origin HEAD", false},
	{"main", "git fetch && git switch -c feature && git push -u origin HEAD", false},
	{"main", `git switch -c feature && git push -u origin "$(git branch --show-current)"`, false},
	{
		"main",
		"git checkout main && git pull && git checkout -b fix && git push -u origin fix",
		false,
	},
	{
		"main",
		"git checkout main && git pull && git checkout -b fix && git push -u origin HEAD",
		false,
	},
	{"feature", "git checkout feature && git push", false},
	{"feature", "git checkout fix/y && git push -u origin HEAD", false},
	{"feature", "git checkout -- file.txt && git push", false},
	{"feature", "git checkout main -- file.txt && git push", false},
	{"feature", "git checkout main && git pull", false},
	{"feature", "git switch main && git push origin feature", false},
	{"feature", "git push && git checkout main", false},
	{
		"feature",
		"git stash && git checkout main && git pull && git checkout - && git stash pop",
		false,
	},
	{"feature", "git fetch && git rebase origin/main && git push --force-with-lease", false},
	{"feature", "cat <<'EOF'\ngit checkout main\nEOF\ngit push", false},
}

func newSwitchGuard(current string) *GitPushMain {
	g := newGitPushGuard(current)
	g.resolveHasBranch = func(_, name string) bool { return switchTestBranches[name] }
	return g
}

func TestGitPushMainBranchSwitch(t *testing.T) {
	for _, tt := range branchSwitches {
		t.Run(tt.current+": "+tt.command, func(t *testing.T) {
			d := newSwitchGuard(
				tt.current,
			).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.deny {
				t.Errorf(
					"on %s, Check(%q) denial = %v, want deny %v",
					tt.current,
					tt.command,
					d,
					tt.deny,
				)
			}
		})
	}
}

// TestGitPushMainBranchSwitchReason pins that a denial a switch causes names
// that switch and what kept it from being sure, so the reason points at the
// real cause.
func TestGitPushMainBranchSwitchReason(t *testing.T) {
	tests := []struct{ current, command, want string }{
		{
			"feature",
			"git checkout main && git pull --rebase && git checkout gone && git push",
			"`git checkout main` earlier",
		},
		{"feature", "git checkout main && git push origin HEAD", "`git checkout main` earlier"},
		{
			"feature",
			"git stash && git checkout main && git checkout - && git push",
			"`git checkout -` earlier",
		},
		{"feature", `git switch -c "$B" && git push origin HEAD`, "`git switch -c \"$B\"` earlier"},
		{"main", "git checkout gone && git push", "no local branch gone"},
		{"main", "git switch -c x; git push origin HEAD", "does not run only after the switch"},
		{
			"main",
			"git switch -c x && git -C /other switch -c y && git push origin HEAD",
			"`git switch -c y` in another directory",
		},
		{
			"main",
			"git switch -c x && git co main && git push origin HEAD",
			"`git co main` runs between the switch and the push",
		},
		{
			"main",
			"git switch -c x && git branch -M main && git push -u origin HEAD",
			"`git branch -M main` runs between",
		},
		{
			"main",
			"git switch -c x && git push origin HEAD & git checkout main",
			"`git checkout main` comes after the push",
		},
		{
			"main",
			"git switch -c x && git push -u origin HEAD && git checkout main",
			"`git checkout main` comes after the push in the same command and may move the branch",
		},
		{
			"main",
			"git switch -c x && git push -u origin HEAD && gh pr create --fill",
			"run that step in a command of its own",
		},
		{
			"main",
			`git() { command git "$@"; }; git switch -c x && git push origin HEAD`,
			"defines a shell function",
		},
		{
			"main",
			"git switch -c x && git -c core.hooksPath=/tmp/h commit -m x && git push origin HEAD",
			"runs between the switch and the push",
		},
	}
	for _, tt := range tests {
		d := newSwitchGuard(
			tt.current,
		).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if d == nil || !strings.Contains(d.Reason, tt.want) {
			t.Errorf(
				"on %s, Check(%q) = %v, want a reason with %q",
				tt.current,
				tt.command,
				d,
				tt.want,
			)
		}
	}
}

// TestGitPushMainBranchSwitchExempt pins finding 4: in an exempt repo a
// switch never turns an allow into a denial, so the exemption decides as it
// does for the branch git reads. A push the exemption does not cover is
// still denied.
func TestGitPushMainBranchSwitchExempt(t *testing.T) {
	tests := []struct {
		current, command string
		deny             bool
	}{
		{"feature", "git switch main && git push origin HEAD", false},
		{"feature", `git checkout "$B" && git push origin HEAD`, false},
		{"feature", "git switch --detach && git push origin HEAD", false},
		{"main", "git checkout master && git push", false},
		{
			"feature",
			"git stash && git checkout main && git pull && git checkout - && git stash pop && git push",
			false,
		},
		{
			"main",
			"git stash && git checkout main && git pull && git checkout - && git stash pop && git push",
			false,
		},
		{"feature", "git checkout @{-1} && git push", false},
		{"feature", "git switch -qc zz && git push -u origin HEAD", false},
		{"feature", "git checkout main && git push upstream HEAD", false},
		{"main", "git switch -c zz && git push upstream HEAD", false},
		{"main", "git switch -c zz && git push upstream main", true},
		{"main", "git checkout main && git -c remote.origin.pushurl=x push origin main", true},
		{"main", "git checkout gone && git push upstream HEAD", true},
		{"main", "git checkout fix/x && git push upstream HEAD", false},
	}
	for _, tt := range tests {
		g := NewGitPushMain(config.Config{DirectMainRepos: []string{"github.com/mad01/dotfiles"}})
		g.resolveBranch = func(string) string { return tt.current }
		g.resolveRepo = func(string) string { return "github.com/mad01/dotfiles" }
		g.resolveHasBranch = func(_, name string) bool { return switchTestBranches[name] }
		// master has no branch in this repo, so git finds no push remote for it.
		g.resolvePushRemote = func(_, branch string) string {
			if branch == "master" {
				return ""
			}
			return "origin"
		}
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf(
				"exempt repo on %s, Check(%q) denial = %v, want deny %v",
				tt.current,
				tt.command,
				d,
				tt.deny,
			)
		}
	}
}

// TestGitHasBranch pins the one git call the guard makes for a switch: a
// local branch of that exact name reads as there, and a prefix of one, a
// missing branch, a directory with no repository or none at all as not.
func TestGitHasBranch(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		base := []string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}
		if out, err := exec.Command("git", append(base, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "first")
	git("branch", "fix/x")
	for name, want := range map[string]bool{"fix/x": true, "main": true, "fix": false, "gone": false} {
		if got := gitHasBranch(dir, name); got != want {
			t.Errorf("gitHasBranch(repo, %q) = %v, want %v", name, got, want)
		}
	}
	if gitHasBranch(t.TempDir(), "main") || gitHasBranch("", "main") {
		t.Error("gitHasBranch outside a repo = true, want false")
	}
}
