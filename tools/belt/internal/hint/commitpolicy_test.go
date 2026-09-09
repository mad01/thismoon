package hint

import (
	"os"
	"path/filepath"
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
	h.resolveRoot = func(string) string { return "" } // no overlay unless a test sets one
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
