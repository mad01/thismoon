package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/testgit"
)

// gitVault is setupCLI with the vault made a git repo holding the fixture
// notes in one commit. extraConfig is appended to the config.
func gitVault(t *testing.T, extraConfig string) string {
	t.Helper()
	testgit.Isolate(t)
	root := setupCLI(t, extraConfig)
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", nil)
	return root
}

func TestEveryBrainChangeCommits(t *testing.T) {
	root := gitVault(t, "")
	// A person's unrelated edits, staged and not, which brain must leave.
	writeNote(t, root, "notes/beta.md", "---\ntitle: Beta\n---\nedited by hand\n")
	writeNote(t, root, "notes/hand.md", "---\ntitle: Hand\n---\nstaged by hand\n")
	testgit.Run(t, root, "add", "notes/hand.md")

	steps := [][]string{
		{"write", "--tier", "notes", "--title", "Fresh Note", "--body", "fresh"},
		{"capture", "--title", "Quick Thought", "--body", "quick"},
		{"edit", "notes/fresh-note.md", "--append", "more"},
		{"promote", "inbox/raw.md", "--title", "Raw Promoted"},
		{"flag", "notes/alpha.md", "--reason", "numbers look off"},
		{"inbox", "keep", "inbox/quick-thought.md", "--days", "60"},
		{"retract", "notes/fresh-note.md", "--reason", "error"},
	}
	for _, args := range steps {
		if got := runBrain(t, "", args...); got.code != exitOK {
			t.Fatalf("%v: exit %d, stderr %q", args, got.code, got.stderr)
		}
	}
	want := []string{
		"brain: retract notes/fresh-note.md",
		"brain: keep inbox/quick-thought.md",
		"brain: flag notes/alpha.md",
		"brain: promote notes/raw-promoted.md",
		"brain: edit notes/fresh-note.md",
		"brain: capture inbox/quick-thought.md",
		"brain: write notes/fresh-note.md",
		"seed",
	}
	if got := testgit.Subjects(t, root); !slices.Equal(got, want) {
		t.Errorf("commits = %q\nwant %q", got, want)
	}
	status := testgit.Status(t, root)
	if !strings.Contains(status, "M notes/beta.md") ||
		!strings.Contains(status, "A  notes/hand.md") {
		t.Errorf("status %q lost the hand edits", status)
	}
	if strings.Contains(status, "raw") || strings.Contains(status, "fresh") {
		t.Errorf("status %q shows a brain change left uncommitted", status)
	}
}

// TestFailedCommitsAreReported runs every write path that commits, in a
// vault whose commits fail two ways: a pre-commit hook refuses them, and git
// cannot read its global config. Each command fails, says the note is
// changed on disk but not committed, and leaves nothing staged.
func TestFailedCommitsAreReported(t *testing.T) {
	tests := []struct {
		name   string
		breaks func(t *testing.T, root string)
		want   string
	}{
		{
			"hook refuses",
			func(t *testing.T, root string) {
				testgit.Hook(t, root, "pre-commit", "echo 'hook says no' >&2\nexit 1\n")
			},
			"hook says no",
		},
		{
			"git cannot read its config",
			func(t *testing.T, _ string) { testgit.UnreadableGlobalConfig(t) },
			"unable to access",
		},
	}
	steps := []struct {
		args []string
		file string // the note the step leaves changed on disk
	}{
		{
			[]string{"capture", "--title", "Quick Thought", "--body", "quick"},
			"inbox/quick-thought.md",
		},
		{
			[]string{"write", "--tier", "notes", "--title", "Fresh Note", "--body", "fresh"},
			"notes/fresh-note.md",
		},
		{[]string{"edit", "notes/alpha.md", "--append", "more"}, "notes/alpha.md"},
		{[]string{"promote", "inbox/raw.md", "--title", "Raw Promoted"}, "notes/raw-promoted.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := gitVault(t, "")
			tt.breaks(t, root)
			for _, step := range steps {
				got := runBrain(t, "", append(step.args, "-o", "json")...)
				if got.code != exitError ||
					!strings.Contains(
						got.stdout,
						step.file+" is changed on disk, but the git commit failed",
					) ||
					!strings.Contains(got.stdout, tt.want) {
					t.Errorf("%v: exit %d, stdout %q; want exit %d naming %s and %q",
						step.args, got.code, got.stdout, exitError, step.file, tt.want)
				}
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(step.file))); err != nil {
					t.Errorf("%v: %s is not on disk: %v", step.args, step.file, err)
				}
			}
			if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"seed"}) {
				t.Errorf("commits = %q, want the seed alone", got)
			}
			if staged := testgit.Staged(t, root); len(staged) != 0 {
				t.Errorf("staged = %q, want nothing", staged)
			}
		})
	}
}

func TestAutocommitOffAndPlainVaults(t *testing.T) {
	testgit.Isolate(t)
	root := setupCLI(t, "")
	if got := runBrain(t, "", "write", "--tier", "notes", "--title", "Plain", "--body", "x"); got.code != exitOK {
		t.Fatalf("write in a plain vault: exit %d, stderr %q", got.code, got.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("a plain vault became a repo: %v", err)
	}

	cfg := "vaults:\n  - {name: personal, path: " + root + ", autocommit: false}\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".config", "brain", "config.yaml"),
		[]byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testgit.Init(t, root)
	testgit.Commit(t, root, "seed", nil)
	if got := runBrain(t, "", "write", "--tier", "notes", "--title", "Off", "--body", "x"); got.code != exitOK {
		t.Fatalf("write with autocommit off: exit %d, stderr %q", got.code, got.stderr)
	}
	if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"seed"}) {
		t.Errorf("autocommit false committed: %q", got)
	}
	if !strings.Contains(testgit.Status(t, root), "notes/off.md") {
		t.Errorf("the note is missing from the status: %q", testgit.Status(t, root))
	}
}
