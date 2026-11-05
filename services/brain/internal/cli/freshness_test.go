package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func TestFlag(t *testing.T) {
	roots := setupVaults(t)
	got := runBrain(
		t,
		"",
		"flag",
		"personal:notes/mine.md",
		"--reason",
		"numbers look off",
		"-o",
		"json",
	)
	if got.code != exitOK {
		t.Fatalf("flag: exit %d, stderr %q", got.code, got.stderr)
	}
	res := decodeStrict[vault.PathResult](t, got.stdout)
	if res.Vault != "personal" || res.Path != "notes/mine.md" || res.SHA == "" {
		t.Errorf("flag result = %+v", res)
	}
	read := runBrain(t, "", "read", "personal:notes/mine.md")
	if !strings.Contains(read.stdout, "DISPUTED on ") ||
		!strings.Contains(read.stdout, "numbers look off") {
		t.Errorf("read after flag shows no disputed banner:\n%s", read.stdout)
	}

	if got := runBrain(t, "", "flag", "personal:notes/mine.md"); got.code != exitUsage {
		t.Errorf("flag without --reason: exit %d, want %d", got.code, exitUsage)
	}

	testgit.Init(t, roots["team"])
	testgit.Run(t, roots["team"], "remote", "add", "origin", "https://example.com/team/notes.git")
	shared := runBrain(t, "", "flag", "team:notes/team-guide.md", "--reason", "outdated")
	if shared.code != exitReadOnly {
		t.Fatalf("flag in a shared vault: exit %d, want %d", shared.code, exitReadOnly)
	}
	for _, want := range []string{"opening an issue", "team:notes/team-guide.md", "https://example.com/team/notes.git"} {
		if !strings.Contains(shared.stderr, want) {
			t.Errorf("shared flag stderr %q lacks %q", shared.stderr, want)
		}
	}
}

func TestInboxListKeepArchive(t *testing.T) {
	root := setupCLI(t, "")
	writeNote(t, root, "inbox/stale-capture.md",
		"---\ntitle: Stale Capture\nstatus: raw\nexpires_at: 2020-01-01\n---\nzebrafish capture\n")

	list := decodeStrict[vault.InboxListing](
		t,
		runBrain(t, "", "inbox", "list", "-o", "json").stdout,
	)
	if list.Total != 3 || list.Notes[0].Path != "inbox/stale-capture.md" || !list.Notes[0].Expired {
		t.Fatalf("inbox list = %+v", list)
	}
	expired := decodeStrict[vault.InboxListing](
		t, runBrain(t, "", "inbox", "list", "--expired", "-o", "json").stdout)
	if expired.Total != 1 {
		t.Errorf("inbox list --expired = %+v, want the stale capture alone", expired)
	}

	keep := runBrain(t, "", "inbox", "keep", "inbox/raw.md", "--days", "10", "-o", "json")
	res := decodeStrict[vault.KeepResult](t, keep.stdout)
	want := lifecycle.Date(time.Now().AddDate(0, 0, 10))
	if keep.code != exitOK || res.ExpiresAt != want {
		t.Errorf("inbox keep: exit %d, %+v; want expires_at %s", keep.code, res, want)
	}
	if got := runBrain(t, "", "inbox", "keep", "notes/alpha.md"); got.code != exitUsage {
		t.Errorf("keep of a validated note: exit %d, want %d", got.code, exitUsage)
	}

	if got := runBrain(t, "", "search", "zebrafish"); !strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Fatalf("search before archive = %q", got.stdout)
	}
	if got := runBrain(t, "", "inbox", "archive"); got.code != exitUsage {
		t.Errorf("archive without --expired: exit %d, want %d", got.code, exitUsage)
	}
	archive := runBrain(t, "", "inbox", "archive", "--expired", "-o", "json")
	report := decodeStrict[ArchiveReport](t, archive.stdout)
	if archive.code != exitOK || len(report.Archived) != 1 ||
		report.Archived[0].ArchivedTo != ".brain/archive/inbox/stale-capture.md" {
		t.Fatalf("archive: exit %d, %+v, stderr %q", archive.code, report, archive.stderr)
	}
	if got := runBrain(t, "", "search", "zebrafish"); strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Errorf("search after archive = %q, want no hit", got.stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".brain", "archive", "inbox", "stale-capture.md")); err != nil {
		t.Errorf("archived file: %v", err)
	}
	if got := runBrain(t, "", "check", "-o", "json"); strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Errorf("check still sees the archived note:\n%s", got.stdout)
	}
}

// writeNote writes content to the vault-relative path rel under root.
func writeNote(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
