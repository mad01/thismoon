package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/library"
)

func TestSimilarWarnsAndExitsZero(t *testing.T) {
	roots := setupVaults(t)
	writeNote(t, roots["personal"], "notes/guide-copy.md",
		"---\ntitle: My Guide\naliases: [Team Guide]\n---\nmine\n")

	got := runBrain(t, "", "similar", "personal:notes/shared-name.md", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("similar: exit %d, stderr %q", got.code, got.stderr)
	}
	report := decodeStrict[library.SimilarReport](t, got.stdout)
	if len(report.Similar) != 1 || report.Similar[0].Vault != "team" ||
		report.Similar[0].Match != library.MatchTitle {
		t.Errorf("similar = %+v", report)
	}

	text := runBrain(t, "", "similar", "team:notes/team-guide.md")
	if text.code != exitOK ||
		text.stdout != "personal:notes/guide-copy.md  (an alias of \"My Guide\")\n" {
		t.Errorf("similar text: exit %d, stdout %q", text.code, text.stdout)
	}
	gh := runBrain(t, "", "similar", "team:notes/team-guide.md", "-o", "github")
	if gh.code != exitOK ||
		!strings.HasPrefix(gh.stdout, "::warning file=notes/team-guide.md,title=brain similar::") ||
		!strings.Contains(gh.stdout, "personal:notes/guide-copy.md") {
		t.Errorf("similar -o github: exit %d, stdout %q", gh.code, gh.stdout)
	}
	none := runBrain(t, "", "similar", "personal:notes/mine.md")
	if none.code != exitOK || none.stdout != "no note looks like personal:notes/mine.md\n" {
		t.Errorf("no similar notes: exit %d, stdout %q", none.code, none.stdout)
	}
	if got := runBrain(t, "", "list", "-o", "github"); got.code != exitUsage {
		t.Errorf("list -o github: exit %d, want %d", got.code, exitUsage)
	}
	if got := runBrain(t, "", "similar", "notes/missing.md"); got.code != exitNotFound {
		t.Errorf("similar of a missing note: exit %d, want %d", got.code, exitNotFound)
	}
}
