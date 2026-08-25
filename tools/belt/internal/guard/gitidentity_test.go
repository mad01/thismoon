package guard

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// newIdentityGuard builds the guard with injected resolvers and a warning
// recorder, so no test shells out to git or dials the events service.
func newIdentityGuard(cfg config.Config, repo, email string, warned *[]string) *GitIdentity {
	g := NewGitIdentity(cfg)
	g.resolveRepo = func(dir string) string { return repo }
	g.resolveEmail = func(dir string) string { return email }
	g.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return g
}

func TestGitIdentity(t *testing.T) {
	rules := []config.GitIdentity{
		{Repos: []string{"github.com/mad01/*"}, Email: "personal@example.com", Mode: "hard"},
		{Repos: []string{"work-host.example/*"}, Email: "work@example.com", Mode: "hard"},
	}
	tests := []struct {
		name     string
		rules    []config.GitIdentity
		command  string
		repo     string
		email    string
		wantDeny bool
		wantWarn bool
	}{
		{"matching email allows", rules, `git commit -m "x"`, "github.com/mad01/thismoon", "personal@example.com", false, false},
		{"wrong email denies", rules, `git commit -m "x"`, "github.com/mad01/thismoon", "work@example.com", true, false},
		{"work repo wrong email denies", rules, `git commit -m "x"`, "work-host.example/org/repo", "personal@example.com", true, false},
		{"work repo right email allows", rules, `git commit -m "x"`, "work-host.example/org/repo", "work@example.com", false, false},
		{"uncovered repo allows", rules, `git commit -m "x"`, "gitlab.com/other/repo", "anything@example.com", false, false},
		{"unresolved repo allows", rules, `git commit -m "x"`, "", "work@example.com", false, false},
		{"unresolved email fails open", rules, `git commit -m "x"`, "github.com/mad01/thismoon", "", false, false},
		{"no config is a no-op", nil, `git commit -m "x"`, "github.com/mad01/thismoon", "work@example.com", false, false},
		{"non-commit git allows", rules, "git status", "github.com/mad01/thismoon", "work@example.com", false, false},
		{"quoted mention not a commit", rules, `echo "git commit -m x"`, "github.com/mad01/thismoon", "work@example.com", false, false},
		{"compound command caught", rules, `git add . && git commit -m "x"`, "github.com/mad01/thismoon", "work@example.com", true, false},
		{
			"soft mode warns and allows",
			[]config.GitIdentity{{Repos: []string{"github.com/mad01/*"}, Email: "personal@example.com", Mode: "soft"}},
			`git commit -m "x"`, "github.com/mad01/thismoon", "work@example.com", false, true,
		},
		{
			"empty repos list covers everything",
			[]config.GitIdentity{{Email: "personal@example.com"}},
			`git commit -m "x"`, "gitlab.com/other/repo", "work@example.com", true, false,
		},
		{
			"profile-gated rule skipped without profile",
			[]config.GitIdentity{{Profile: "work", Email: "work@example.com"}},
			`git commit -m "x"`, "github.com/mad01/thismoon", "personal@example.com", false, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newIdentityGuard(config.Config{GitIdentity: tt.rules}, tt.repo, tt.email, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if (len(warned) > 0) != tt.wantWarn {
				t.Errorf("Check(%q) warnings = %v, wantWarn %v", tt.command, warned, tt.wantWarn)
			}
			if d != nil && !strings.Contains(d.Reason, GitIdentityID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
		})
	}
}

func TestGitIdentityProfileGatedRuleApplies(t *testing.T) {
	var warned []string
	cfg := config.Config{
		Profiles:    []string{"work"},
		GitIdentity: []config.GitIdentity{{Profile: "work", Email: "work@example.com"}},
	}
	g := newIdentityGuard(cfg, "github.com/mad01/thismoon", "personal@example.com", &warned)
	if d := g.Check(Input{Event: EventBash, Command: `git commit -m "x"`, Cwd: "/tmp"}); d == nil {
		t.Error("profile-gated rule did not apply on a machine carrying the profile")
	}
}

func TestGitIdentityFirstRuleWins(t *testing.T) {
	var warned []string
	cfg := config.Config{GitIdentity: []config.GitIdentity{
		{Repos: []string{"github.com/mad01/special"}, Email: "special@example.com"},
		{Repos: []string{"github.com/mad01/*"}, Email: "personal@example.com"},
	}}
	g := newIdentityGuard(cfg, "github.com/mad01/special", "special@example.com", &warned)
	if d := g.Check(Input{Event: EventBash, Command: `git commit -m "x"`, Cwd: "/tmp"}); d != nil {
		t.Errorf("narrower first rule not preferred: %v", d)
	}
}

func TestGitIdentityUsesDashCDir(t *testing.T) {
	g := NewGitIdentity(config.Config{GitIdentity: []config.GitIdentity{{Email: "x@example.com"}}})
	var gotDir string
	g.resolveRepo = func(dir string) string {
		gotDir = dir
		return "github.com/mad01/thismoon"
	}
	g.resolveEmail = func(string) string { return "x@example.com" }
	g.Check(Input{Event: EventBash, Command: `git -C /other/repo commit -m "x"`, Cwd: "/session/cwd"})
	if gotDir != "/other/repo" {
		t.Errorf("resolver dir = %q, want /other/repo", gotDir)
	}
}
