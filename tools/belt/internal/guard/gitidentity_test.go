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
		{
			"matching email allows",
			rules,
			`git commit -m "x"`,
			"github.com/mad01/thismoon",
			"personal@example.com",
			false,
			false,
		},
		{
			"wrong email denies",
			rules,
			`git commit -m "x"`,
			"github.com/mad01/thismoon",
			"work@example.com",
			true,
			false,
		},
		{
			"work repo wrong email denies",
			rules,
			`git commit -m "x"`,
			"work-host.example/org/repo",
			"personal@example.com",
			true,
			false,
		},
		{
			"work repo right email allows",
			rules,
			`git commit -m "x"`,
			"work-host.example/org/repo",
			"work@example.com",
			false,
			false,
		},
		{
			"uncovered repo allows",
			rules,
			`git commit -m "x"`,
			"gitlab.com/other/repo",
			"anything@example.com",
			false,
			false,
		},
		{
			"unresolved repo allows",
			rules,
			`git commit -m "x"`,
			"",
			"work@example.com",
			false,
			false,
		},
		{
			"unresolved email fails open",
			rules,
			`git commit -m "x"`,
			"github.com/mad01/thismoon",
			"",
			false,
			false,
		},
		{
			"no config is a no-op",
			nil,
			`git commit -m "x"`,
			"github.com/mad01/thismoon",
			"work@example.com",
			false,
			false,
		},
		{
			"non-commit git allows",
			rules,
			"git status",
			"github.com/mad01/thismoon",
			"work@example.com",
			false,
			false,
		},
		{
			"quoted mention not a commit",
			rules,
			`echo "git commit -m x"`,
			"github.com/mad01/thismoon",
			"work@example.com",
			false,
			false,
		},
		{
			"compound command caught",
			rules,
			`git add . && git commit -m "x"`,
			"github.com/mad01/thismoon",
			"work@example.com",
			true,
			false,
		},
		{
			"soft mode warns and allows",
			[]config.GitIdentity{
				{
					Repos: []string{"github.com/mad01/*"},
					Email: "personal@example.com",
					Mode:  "soft",
				},
			},
			`git commit -m "x"`, "github.com/mad01/thismoon", "work@example.com", false, true,
		},
		{
			"empty repos list covers everything",
			[]config.GitIdentity{{Email: "personal@example.com"}},
			`git commit -m "x"`, "gitlab.com/other/repo", "work@example.com", true, false,
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

// TestGitIdentityDirResolution pins which directory the resolvers see: the
// session cwd for a bare commit, the directory a `cd`/`pushd` before it
// landed in (MAD-366), the `git -C` target otherwise, with ~ and $HOME
// expanded the way the shell would (MAD-370). A target only a shell can
// resolve leaves the directory unknown, and the guard fails open without
// asking the resolvers about a guessed location.
func TestGitIdentityDirResolution(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	cfg := config.Config{GitIdentity: []config.GitIdentity{{Email: "x@example.com", Mode: "hard"}}}
	tests := []struct {
		name    string
		command string
		cwd     string
		wantDir string // "" means no lookup and no denial
	}{
		{"bare commit uses cwd", `git commit -m "x"`, "/session/cwd", "/session/cwd"},
		{"absolute -C", `git -C /other/repo commit -m "x"`, "/session/cwd", "/other/repo"},
		{
			"tilde -C",
			`git -C ~/code/worklog commit -m "x"`,
			"/session/cwd",
			"/Users/tester/code/worklog",
		},
		{
			"HOME -C",
			`git -C $HOME/code/worklog commit -m "x"`,
			"/session/cwd",
			"/Users/tester/code/worklog",
		},
		{
			"braced HOME -C",
			`git -C ${HOME}/code/worklog commit -m "x"`,
			"/session/cwd",
			"/Users/tester/code/worklog",
		},
		{
			"cd then bare commit",
			`cd /other/repo && git commit -m "x"`,
			"/session/cwd",
			"/other/repo",
		},
		{
			"tilde cd into a worktree",
			`cd ~/.worktrees/repo/slug && git commit -m "x"`,
			"/session/cwd",
			"/Users/tester/.worktrees/repo/slug",
		},
		{"unresolvable cd fails open", `cd $TARGET && git commit -m "x"`, "/session/cwd", ""},
		{"unresolvable -C fails open", `git -C $TARGET commit -m "x"`, "/session/cwd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newIdentityGuard(cfg, "github.com/mad01/thismoon", "other@example.com", &warned)
			var gotDir string
			asked := false
			g.resolveRepo = func(dir string) string {
				asked, gotDir = true, dir
				return "github.com/mad01/thismoon"
			}
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if tt.wantDir == "" {
				if asked {
					t.Errorf("resolver asked about %q, want no lookup", gotDir)
				}
				if d != nil {
					t.Errorf("unknown directory denied: %v", d)
				}
				return
			}
			if gotDir != tt.wantDir {
				t.Errorf("resolver dir = %q, want %q", gotDir, tt.wantDir)
			}
			if d == nil {
				t.Error("want a denial for the mismatched email")
			}
		})
	}
}
