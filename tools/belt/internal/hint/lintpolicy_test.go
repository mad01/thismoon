package hint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newLintPolicyHint(repo string, exclude []string) *LintPolicy {
	h := NewLintPolicy(config.Config{
		Hints: map[string]config.Toggle{LintPolicyID: {ExcludeRepos: exclude}},
	})
	h.resolveRepo = func(string) string { return repo }
	h.resolveRoot = func(string) string { return "" } // no overlay unless a test sets one
	return h
}

// withLintOverlay points the hint's overlay lookup at a temp repo root
// holding the given .belt.yaml content, or an empty root when content is
// empty.
func withLintOverlay(t *testing.T, h *LintPolicy, content string) {
	t.Helper()
	root := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(root, OverlayFileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.resolveRoot = func(string) string { return root }
}

const lintOverlay = "hints:\n  lint-policy:\n    message: run make fmt && make lint per component.\n"

func TestLintPolicy(t *testing.T) {
	const thismoon = "github.com/mad01/thismoon"

	tests := []struct {
		name     string
		command  string
		overlay  string
		repo     string
		exclude  []string // machine exclude_repos
		fires    bool
		wantText string // required substring when it fires
	}{
		{
			"declared message fires",
			"git commit -m 'x'", lintOverlay, thismoon, nil,
			true, "run make fmt && make lint per component.",
		},
		{
			"missing overlay is silent",
			"git commit -m 'x'", "", thismoon, nil, false, "",
		},
		{
			"overlay without message is silent",
			"git commit -m 'x'", "hints:\n  commit-policy:\n    message: other hint's key.\n",
			thismoon, nil, false, "",
		},
		{
			"non-commit git is silent",
			"git status", lintOverlay, thismoon, nil, false, "",
		},
		{
			"quoted mention is not a commit",
			`echo "git commit -m x"`, lintOverlay, thismoon, nil, false, "",
		},
		{
			"compound command fires",
			"git add -A && git commit -m 'x'", lintOverlay, thismoon, nil, true, thismoon,
		},
		{
			"machine exclude silences",
			"git commit -m 'x'", lintOverlay, thismoon,
			[]string{thismoon},
			false, "",
		},
		{
			"org wildcard excludes",
			"git commit -m 'x'", lintOverlay, thismoon,
			[]string{"github.com/mad01/*"},
			false, "",
		},
		{
			"overlay exclude true silences",
			lintCmd, lintOverlayWith("exclude: true"), thismoon, nil, false, "",
		},
		{
			"overlay exclude false overrides machine exclusion",
			lintCmd, lintOverlayWith("exclude: false"), thismoon,
			[]string{thismoon},
			true, "lint/format policy",
		},
		{
			"unresolved repo is silent",
			"git commit -m 'x'", lintOverlay, "", nil, false, "",
		},
		{
			"lint in the command suppresses",
			"make lint && git commit -m 'x'", lintOverlay, thismoon, nil, false, "",
		},
		{
			"fmt in the command suppresses",
			"gofmt -w . && git commit -m 'x'", lintOverlay, thismoon, nil, false, "",
		},
		{
			"lint only inside quotes still fires",
			`git commit -m "fix lint warnings"`, lintOverlay, thismoon, nil, true, "lint/format policy",
		},
		{
			"empty command is silent",
			"", lintOverlay, thismoon, nil, false, "",
		},
		{
			"broken overlay degrades to advisory",
			"git commit -m 'x'", "hints: [not a map\n", thismoon, nil,
			true, "could not evaluate this repo's lint policy",
		},
		{
			"foreign file is a no-op",
			"git commit -m 'x'", "tool: something-else\n", thismoon, nil, false, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			h := newLintPolicyHint(tt.repo, tt.exclude)
			withLintOverlay(t, h, tt.overlay)
			a := h.Check(Input{
				Event: EventBash, Command: tt.command,
				Cwd: "/some/repo", SessionID: "lint-policy-test",
			})
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
			if a.Hint != LintPolicyID {
				t.Errorf("advice hint = %q, want %q", a.Hint, LintPolicyID)
			}
			if !strings.Contains(a.Text, tt.wantText) {
				t.Errorf("advice %q does not mention %q", a.Text, tt.wantText)
			}
		})
	}
}

const lintCmd = "git commit -m 'x'"

// lintOverlayWith appends one extra key to the standard lint-policy overlay.
func lintOverlayWith(line string) string {
	return lintOverlay + "    " + line + "\n"
}

// TestLintPolicyOncePerSession pins the dedupe contract: one nudge per repo
// per session, a suppressed trigger does not spend it, and no session id
// means no nudge at all rather than a nudge on every commit.
func TestLintPolicyOncePerSession(t *testing.T) {
	const thismoon = "github.com/mad01/thismoon"
	in := func(command, session string) Input {
		return Input{Event: EventBash, Command: command, Cwd: "/some/repo", SessionID: session}
	}

	t.Run("second commit is silent", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		h := newLintPolicyHint(thismoon, nil)
		withLintOverlay(t, h, lintOverlay)
		if a := h.Check(in(lintCmd, "s1")); a == nil {
			t.Fatal("first commit should fire")
		}
		if a := h.Check(in(lintCmd, "s1")); a != nil {
			t.Errorf("second commit should be silent, got %+v", a)
		}
	})
	t.Run("suppressed trigger keeps the nudge", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		h := newLintPolicyHint(thismoon, nil)
		withLintOverlay(t, h, lintOverlay)
		if a := h.Check(in("make lint && git commit -m 'x'", "s2")); a != nil {
			t.Fatalf("toolchain trigger should be silent, got %+v", a)
		}
		if a := h.Check(in(lintCmd, "s2")); a == nil {
			t.Error("bare commit after a suppressed trigger should still fire")
		}
	})
	t.Run("no session id is silent", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		h := newLintPolicyHint(thismoon, nil)
		withLintOverlay(t, h, lintOverlay)
		if a := h.Check(in(lintCmd, "")); a != nil {
			t.Errorf("no session id should mean no nudge, got %+v", a)
		}
	})
}
