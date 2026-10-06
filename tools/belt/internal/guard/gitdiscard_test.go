package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// discardFixture describes the repo state the guard sees: the canonical repo
// per directory and whether its tree is dirty. A directory missing from
// dirty reads as clean; one listed in broken makes git status fail.
type discardFixture struct {
	repo   string
	dirty  map[string]bool
	broken map[string]bool
	// ignoredDirty marks directories whose only changes are ignored files,
	// visible to `git status --porcelain --ignored` alone.
	ignoredDirty map[string]bool
}

// newDiscardGuard builds the guard with injected resolvers and a warning
// recorder, so no test shells out to git or dials the events service.
func newDiscardGuard(cfg config.Config, f discardFixture, warned *[]string) *GitDiscard {
	g := NewGitDiscard(cfg)
	g.resolveRepo = func(string) string { return f.repo }
	g.status = func(dir string, extra ...string) (string, bool) {
		if f.broken[dir] {
			return "", false
		}
		if f.dirty[dir] {
			return " M file.go", true
		}
		if len(extra) > 0 && f.ignoredDirty[dir] {
			return "!! build/", true
		}
		return "", true
	}
	g.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return g
}

func TestGitDiscardCommands(t *testing.T) {
	dirty := discardFixture{repo: "github.com/mad01/thismoon", dirty: map[string]bool{"/repo": true}}
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"git stash", true},
		{"git stash -u", true},
		{"git stash push -m wip", true},
		{"git stash save wip", true},
		{"git stash list", false},
		{"git stash show -p", false},
		{"git stash apply", false},
		{"git stash pop", false},
		{"git stash branch tmp", false},
		{"git reset --hard", true},
		{"git reset --hard origin/main", true},
		{"git reset --soft HEAD~1", false},
		{"git reset HEAD file.go", false},
		{"git checkout -- file.go", true},
		{"git checkout .", true},
		{"git checkout main -- file.go", true},
		{"git checkout -f main", true},
		{"git checkout main", false},
		{"git checkout -b feat", false},
		{"git switch --discard-changes main", true},
		{"git switch main", false},
		{"git restore file.go", true},
		{"git restore --staged file.go", false},
		{"git restore -S file.go", false},
		{"git restore --staged --worktree file.go", true},
		{"git restore --source=HEAD~1 file.go", true},
		{"git clean -f", true},
		{"git clean -fdx", true},
		{"git clean --force", true},
		{"git clean -n", false},
		{"git clean -fn", false},
		{"git clean -d", false},
		{"git status", false},
		{"git commit -m 'git stash'", false},
		{"echo git reset --hard", true}, // token parser: a bare git anywhere in the segment counts
		{"make test && git stash", true},
		{"git -C /repo stash", true},
		{"cd /repo && git reset --hard", true},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, dirty, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.HasPrefix(d.Reason, "belt[git-discard]: this discards uncommitted work.") {
				t.Errorf("reason = %q, want the git-discard prefix and message", d.Reason)
			}
		})
	}
}

func TestGitDiscardDecision(t *testing.T) {
	const repo = "github.com/mad01/thismoon"
	tests := []struct {
		name     string
		cfg      config.Config
		fixture  discardFixture
		command  string
		cwd      string
		wantDeny bool
		wantWarn bool
	}{
		{
			name:    "clean tree allows",
			fixture: discardFixture{repo: repo},
			command: "git reset --hard", cwd: "/repo",
		},
		{
			name:    "dirty tree denies",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git reset --hard", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "judged in the git -C directory, not the session cwd",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/other": true}},
			command: "git -C /other stash", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean -C target allows despite a dirty session cwd",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git -C /other stash", cwd: "/repo",
		},
		{
			name:    "tracked cd places the command",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/other": true}},
			command: "cd /other && git checkout .", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "unresolvable directory denies",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git stash`, cwd: "/repo", wantDeny: true,
		},
		{
			name:    "git status failure denies",
			fixture: discardFixture{repo: repo, broken: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean -x checks ignored files too",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git clean -fdx", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean without -x ignores ignored files",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git clean -fd", cwd: "/repo",
		},
		{
			name: "allow_repos exempts",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo",
		},
		{
			name: "allow_repos for another repo still denies",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/dotfiles"}},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo", wantDeny: true,
		},
		{
			name: "soft mode warns instead of denying",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git reset --hard", cwd: "/repo", wantWarn: true,
		},
		{
			name: "soft mode stays quiet on a clean tree",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo},
			command: "git reset --hard", cwd: "/repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(tt.cfg, tt.fixture, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if (len(warned) > 0) != tt.wantWarn {
				t.Errorf("Check(%q) warnings = %v, wantWarn %v", tt.command, warned, tt.wantWarn)
			}
		})
	}
}

// TestGitStatusPorcelain runs the real backend against a scratch repo: clean
// reads as clean, an untracked file as dirty, and a non-repo as a failure.
func TestGitStatusPorcelain(t *testing.T) {
	dir := t.TempDir()
	if out, ok := gitStatusPorcelain(dir); ok {
		t.Fatalf("non-repo status = %q, ok; want a failure", out)
	}
	if err := gitCmd(dir, "init", "-q").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if out, ok := gitStatusPorcelain(dir); !ok || out != "" {
		t.Errorf("fresh repo status = %q, %v; want clean", out, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, ok := gitStatusPorcelain(dir); !ok || out == "" {
		t.Errorf("status with an untracked file = %q, %v; want dirty", out, ok)
	}
	if _, ok := gitStatusPorcelain(""); ok {
		t.Error("empty dir must fail, never run git in belt's own cwd")
	}
}
