package cli

import (
	"strings"
	"testing"

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
