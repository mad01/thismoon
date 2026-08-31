package hint

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newCommitPolicyHint(branch, repo string, exclude []string) *CommitPolicy {
	h := NewCommitPolicy(config.Config{
		Hints: map[string]config.Toggle{CommitPolicyID: {ExcludeRepos: exclude}},
	})
	h.resolveBranch = func(string) string { return branch }
	h.resolveRepo = func(string) string { return repo }
	return h
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
		{"org wildcard excludes", "git commit -m 'x'", "main", thismoon, []string{"github.com/mad01/*"}, false},
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
				t.Fatalf("Check(%q) fired = %v, want %v (advice: %+v)", tt.command, got, tt.fires, a)
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

// TestCommitPolicyDirResolution pins which directory the resolvers see: the
// `git -C` dir when one is given, the session cwd otherwise.
func TestCommitPolicyDirResolution(t *testing.T) {
	tests := []struct {
		name    string
		command string
		cwd     string
		wantDir string
	}{
		{"bare commit uses cwd", "git commit -m 'x'", "/session/cwd", "/session/cwd"},
		{"git -C wins over cwd", "git -C /other/repo commit -m 'x'", "/session/cwd", "/other/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			h := newCommitPolicyHint("main", "github.com/mad01/thismoon", nil)
			h.resolveBranch = func(dir string) string {
				got = dir
				return "main"
			}
			if a := h.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd}); a == nil {
				t.Fatal("Check returned nil, want advice")
			}
			if got != tt.wantDir {
				t.Errorf("resolver saw dir %q, want %q", got, tt.wantDir)
			}
		})
	}
}
