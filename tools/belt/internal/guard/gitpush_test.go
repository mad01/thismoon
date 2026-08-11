package guard

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newGitPushGuard(profiles []string, currentBranch string) *GitPushMain {
	g := NewGitPushMain(config.Config{Profiles: profiles})
	g.resolveBranch = func(dir string) string { return currentBranch }
	g.resolveRepo = func(dir string) string { return "" }
	return g
}

func TestGitPushMain(t *testing.T) {
	work := []string{"work"}
	personal := []string{"personal"}
	tests := []struct {
		name          string
		profiles      []string
		command       string
		currentBranch string
		wantDeny      bool
	}{
		{"work push explicit main", work, "git push origin main", "feature", true},
		{"work push explicit master", work, "git push origin master", "feature", true},
		{"work push feature branch", work, "git push origin my-feature", "my-feature", false},
		{"work bare push on main", work, "git push", "main", true},
		{"work bare push on feature", work, "git push", "feature-x", false},
		{"work push -u origin HEAD on main", work, "git push -u origin HEAD", "main", true},
		{"work push -u origin HEAD on feature", work, "git push -u origin HEAD", "fix-1", false},
		{"work push refspec HEAD:main", work, "git push origin HEAD:main", "feature", true},
		{"work push refs/heads/main", work, "git push origin refs/heads/main", "feature", true},
		{"work force push main", work, "git push --force origin main", "feature", true},
		{"work compound command", work, "cd /tmp && git push origin main", "feature", true},
		{"work push after other git cmd", work, "git add . ; git push origin main", "feature", true},
		{"work git -C push bare", work, "git -C /some/repo push", "main", true},
		{"work git --git-dir space form", work, "git --git-dir /some/repo/.git push origin main", "feature", true},
		{"work git --git-dir equals form", work, "git --git-dir=/some/repo/.git push origin main", "feature", true},
		{"work non-push git", work, "git status", "main", false},
		{"work quoted mention not a push", work, `echo "git push origin main"`, "feature", false},
		{"personal push main allowed", personal, "git push origin main", "main", false},
		{"personal bare push on main allowed", personal, "git push", "main", false},
		{"unknown profile fails closed", nil, "git push origin main", "feature", true},
		{"work delete main", work, "git push origin :main", "feature", true},
		{"work push with push-option", work, "git push -o ci.skip origin main", "feature", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newGitPushGuard(tt.profiles, tt.currentBranch)
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
	g := NewGitPushMain(config.Config{Profiles: []string{"work"}})
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
	g := NewGitPushMain(config.Config{Profiles: []string{"work"}})
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
				Profiles: []string{"work"},
				Guards:   map[string]config.Toggle{GitPushMainID: {AllowRepos: tt.allow}},
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
