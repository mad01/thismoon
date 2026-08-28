package guard

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newGitPushGuard(currentBranch string) *GitPushMain {
	g := NewGitPushMain(config.Config{})
	g.resolveBranch = func(dir string) string { return currentBranch }
	g.resolveRepo = func(dir string) string { return "" }
	return g
}

func TestGitPushMain(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		currentBranch string
		wantDeny      bool
	}{
		{"push explicit main", "git push origin main", "feature", true},
		{"push explicit master", "git push origin master", "feature", true},
		{"push feature branch", "git push origin my-feature", "my-feature", false},
		{"bare push on main", "git push", "main", true},
		{"bare push on feature", "git push", "feature-x", false},
		{"push -u origin HEAD on main", "git push -u origin HEAD", "main", true},
		{"push -u origin HEAD on feature", "git push -u origin HEAD", "fix-1", false},
		{"push refspec HEAD:main", "git push origin HEAD:main", "feature", true},
		{"push refs/heads/main", "git push origin refs/heads/main", "feature", true},
		{"force push main", "git push --force origin main", "feature", true},
		{"compound command", "cd /tmp && git push origin main", "feature", true},
		{"push after other git cmd", "git add . ; git push origin main", "feature", true},
		{"git -C push bare", "git -C /some/repo push", "main", true},
		{"git --git-dir space form", "git --git-dir /some/repo/.git push origin main", "feature", true},
		{"git --git-dir equals form", "git --git-dir=/some/repo/.git push origin main", "feature", true},
		{"non-push git", "git status", "main", false},
		{"quoted mention not a push", `echo "git push origin main"`, "feature", false},
		{"empty config fails closed", "git push origin main", "feature", true},
		{"delete main", "git push origin :main", "feature", true},
		{"push with push-option", "git push -o ci.skip origin main", "feature", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newGitPushGuard(tt.currentBranch)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, GitPushMainID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
		})
	}
}

func TestGitPushUsesCwdForBarePush(t *testing.T) {
	g := NewGitPushMain(config.Config{})
	var gotDir string
	g.resolveBranch = func(dir string) string {
		gotDir = dir
		return "feature"
	}
	g.Check(Input{Event: EventBash, Command: "git push", Cwd: "/session/cwd"})
	if gotDir != "/session/cwd" {
		t.Errorf("resolver dir = %q, want /session/cwd", gotDir)
	}
}

func TestGitPushPrefersDashCDir(t *testing.T) {
	g := NewGitPushMain(config.Config{})
	var gotDir string
	g.resolveBranch = func(dir string) string {
		gotDir = dir
		return "feature"
	}
	g.Check(Input{Event: EventBash, Command: "git -C /other/repo push", Cwd: "/session/cwd"})
	if gotDir != "/other/repo" {
		t.Errorf("resolver dir = %q, want /other/repo", gotDir)
	}
}

func TestGitPushMainAllowRepos(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	allow := []string{dotfiles}
	tests := []struct {
		name          string
		command       string
		currentBranch string
		repo          string
		allow         []string
		wantDeny      bool
	}{
		{"dotfiles bare push on main allowed", "git push", "main", dotfiles, allow, false},
		{"dotfiles explicit origin main allowed", "git push origin main", "feature", dotfiles, allow, false},
		{"dotfiles HEAD:main allowed", "git push origin HEAD:main", "feature", dotfiles, allow, false},
		{"dotfiles git -C push allowed", "git -C /repo push", "main", dotfiles, allow, false},
		{"non-allowlisted repo denied", "git push origin main", "feature", "github.com/mad01/thismoon", allow, true},
		{"unresolved repo denied", "git push origin main", "feature", "", allow, true},
		{"empty allowlist denied", "git push origin main", "feature", dotfiles, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitPushMain(config.Config{
				Guards: map[string]config.Toggle{GitPushMainID: {AllowRepos: tt.allow}},
			})
			g.resolveBranch = func(string) string { return tt.currentBranch }
			g.resolveRepo = func(string) string { return tt.repo }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
		})
	}
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
		if got := canonicalRepo(tt.in); got != tt.want {
			t.Errorf("canonicalRepo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
