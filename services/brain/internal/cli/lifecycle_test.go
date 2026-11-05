package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func TestRevocationCommands(t *testing.T) {
	setupCLI(t, "")
	if got := runBrain(t, "", "supersede", "notes/beta.md", "--by", "notes/alpha.md"); got.code != exitUsage ||
		!strings.Contains(got.stderr, "has no id") {
		t.Fatalf("supersede by a note without id: exit %d, stderr %q", got.code, got.stderr)
	}
	if got := runBrain(t, "", "migrate", "--ids"); got.code != exitOK {
		t.Fatalf("migrate: exit %d, stderr %q", got.code, got.stderr)
	}
	tests := []struct {
		name     string
		args     []string
		wantCode int
		banner   string
	}{
		{
			"supersede",
			[]string{"supersede", "notes/beta.md", "--by", "notes/alpha.md"},
			exitOK,
			"SUPERSEDED on ",
		},
		{
			"supersede itself",
			[]string{"supersede", "notes/alpha.md", "--by", "notes/alpha.md"},
			exitUsage,
			"",
		},
		{"supersede without by", []string{"supersede", "notes/alpha.md"}, exitUsage, ""},
		{
			"retract bad reason",
			[]string{"retract", "inbox/raw.md", "--reason", "boredom"},
			exitUsage,
			"",
		},
		{
			"retract",
			[]string{
				"retract",
				"inbox/raw.md",
				"--reason",
				"unverifiable",
				"--note",
				"source is gone",
			},
			exitOK, "RETRACTED on ",
		},
		{"withdraw", []string{"withdraw", "inbox/dup.md"}, exitOK, "WITHDRAWN on "},
		{
			"retract a tombstone",
			[]string{"retract", "inbox/dup.md", "--reason", "error"},
			exitUsage,
			"",
		},
	}
	for _, tc := range tests {
		got := runBrain(t, "", append(tc.args, "-o", "json")...)
		if got.code != tc.wantCode {
			t.Fatalf("%s: exit %d, want %d; stderr %q", tc.name, got.code, tc.wantCode, got.stderr)
		}
		if tc.wantCode != exitOK {
			continue
		}
		res := decodeStrict[vault.PathResult](t, got.stdout)
		read := runBrain(t, "", "read", res.Path)
		if first, _, _ := strings.Cut(read.stdout, "\n"); !strings.HasPrefix(first, tc.banner) {
			t.Errorf("%s: read starts %q, want the banner %q", tc.name, first, tc.banner)
		}
	}
	got := runBrain(t, "", "read", "notes/beta.md", "-o", "json")
	note := decodeStrict[vault.Note](t, got.stdout)
	by, _ := note.Frontmatter.Extra["superseded_by"].(string)
	if !strings.HasPrefix(by, "id:") || !strings.Contains(note.Banner, by) {
		t.Errorf("superseded_by %q, banner %q: want an id ref named in the banner", by, note.Banner)
	}
}
