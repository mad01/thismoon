package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/search"
)

// partialArchiveVault returns a vault with two expired raw notes where the
// second cannot move: the archive path its directory needs is a file.
func partialArchiveVault(t *testing.T) string {
	t.Helper()
	root := setupCLI(t, "")
	writeNote(t, root, "inbox/a-first.md",
		"---\ntitle: First\nstatus: raw\nexpires_at: 2020-01-01\n---\nquokka one\n")
	writeNote(t, root, "inbox/sub/b-second.md",
		"---\ntitle: Second\nstatus: raw\nexpires_at: 2020-01-01\n---\nquokka two\n")
	writeNote(t, root, ".brain/archive/inbox/sub", "not a directory\n")
	if got := runBrain(t, "", "index", "--literal-only"); got.code != exitOK {
		t.Fatalf("index: exit %d, stderr %q", got.code, got.stderr)
	}
	return root
}

func TestInboxArchiveReportsAndPurgesAPartialRun(t *testing.T) {
	root := partialArchiveVault(t)
	got := runBrain(t, "", "inbox", "archive", "--expired", "-o", "json")
	if got.code != exitError {
		t.Fatalf("exit %d, want %d; stderr %q", got.code, exitError, got.stderr)
	}
	report := decodeStrict[ArchiveReport](t, got.stdout)
	if len(report.Archived) != 1 || report.Archived[0].Path != "inbox/a-first.md" {
		t.Fatalf("archived = %+v, want inbox/a-first.md alone", report.Archived)
	}
	if report.Error == nil || report.Error.Code != "error" ||
		!strings.Contains(report.Error.Message, "sub") {
		t.Errorf("report error = %+v, want the failed move", report.Error)
	}
	if !strings.HasPrefix(got.stderr, "brain inbox archive: ") {
		t.Errorf("stderr = %q", got.stderr)
	}
	assertLiteralIndexFresh(t, root)
}

func TestInboxArchiveTextListsMovesBeforeAFailure(t *testing.T) {
	partialArchiveVault(t)
	got := runBrain(t, "", "inbox", "archive", "--expired")
	if got.code != exitError || !strings.Contains(got.stdout, "archived inbox/a-first.md") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

// assertLiteralIndexFresh fails unless the default vault's literal index
// was rebuilt against the vault as it is now.
func assertLiteralIndexFresh(t *testing.T, root string) {
	t.Helper()
	dir, err := brain.IndexDir("default")
	if err != nil {
		t.Fatal(err)
	}
	state, err := search.LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, _, err := search.ComputeFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if search.IsStale(state, fingerprint) {
		t.Error("the literal index still holds the archived note (not purged)")
	}
}

func TestInboxArchiveSkipHintForSharedVaults(t *testing.T) {
	setupVaults(t)
	got := runBrain(t, "", "inbox", "archive", "--expired", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	report := decodeStrict[ArchiveReport](t, got.stdout)
	if len(report.Skipped) != 1 {
		t.Fatalf("skipped = %+v, want the shared vault", report.Skipped)
	}
	reason := report.Skipped[0].Reason
	if strings.Contains(reason, "--vault") || !strings.Contains(reason, "read-only") {
		t.Errorf("skip reason = %q, want one that does not send the user to --vault", reason)
	}
	named := runBrain(t, "", "inbox", "archive", "--expired", "--vault", "team", "-o", "json")
	refused := decodeStrict[ArchiveReport](t, named.stdout)
	if named.code != exitReadOnly || refused.Error == nil || refused.Error.Code != "read_only" {
		t.Errorf("--vault team: exit %d, %+v", named.code, refused)
	}
}
