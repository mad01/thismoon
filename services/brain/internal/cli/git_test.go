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
