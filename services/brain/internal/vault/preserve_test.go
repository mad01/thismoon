package vault

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// styledLine is one line of a hand-written frontmatter and the top-level key
// it belongs to ("" for a comment).
type styledLine struct{ key, text string }

// styledCapture is an inbox note in every shape a re-encode used to rewrite:
// a comment, a key comment, a flow list, quotes, plain dates, 4-space and
// 0-space lists and a folded scalar.
var styledCapture = []styledLine{
	{"", "# kept comment"},
	{"title", `title: "Quoted Title"`},
	{"tags", "tags: [ai, research]"},
	{"created", "created: 2024-01-01"},
	{"updated", "updated: 2024-01-02"},
	{"owner", "owner: alex # who owns it"},
	{"four", "four:"},
	{"four", "    - a"},
	{"four", "    - b"},
	{"zero", "zero:"},
	{"zero", "- x"},
	{"zero", "- y"},
	{"summary", "summary: >"},
	{"summary", "  folded line one"},
	{"summary", "  folded line two"},
	{"id", "id: 01JBRAINSTYLEDCAPTURE0000"},
	{"status", "status: raw"},
	{"expires_at", "expires_at: 2026-12-01"},
}

func styledCaptureNote(eol string) string {
	lines := make([]string, 0, len(styledCapture))
	for _, l := range styledCapture {
		lines = append(lines, l.text)
	}
	return "---" + eol + strings.Join(lines, eol) + eol + "---" + eol + "Body." + eol
}

func TestChangesKeepUntouchedFrontmatterBytes(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// change runs on inbox/capture.md and returns the note it wrote.
		change func(t *testing.T, v *Vault, path string) string
		// touched are the keys the change may rewrite.
		touched []string
	}{
		{
			name: "edit --set",
			change: func(t *testing.T, v *Vault, path string) string {
				_, err := v.Edit(path, EditOptions{
					Set: []markdown.Field{mustField(t, "classification", "internal")}, Now: now,
				})
				return orFail(t, path, err)
			},
			touched: []string{"updated"},
		},
		{
			name: "flag",
			change: func(t *testing.T, v *Vault, path string) string {
				_, err := v.Flag(path, "doubtful", now)
				return orFail(t, path, err)
			},
			touched: []string{"status", "updated", "expires_at"},
		},
		{
			name: "inbox keep",
			change: func(t *testing.T, v *Vault, path string) string {
				_, err := v.Keep(path, 60, now)
				return orFail(t, path, err)
			},
			touched: []string{"updated", "expires_at"},
		},
		{
			name: "supersede",
			change: func(t *testing.T, v *Vault, path string) string {
				_, err := v.Supersede(path, "id:01JBRAINREPLACEMENT000000", now)
				return orFail(t, path, err)
			},
			touched: []string{"status", "updated"},
		},
		{
			name: "promote",
			change: func(t *testing.T, v *Vault, path string) string {
				res, err := v.Promote(path, PromoteOptions{Now: now})
				return orFail(t, res.NewPath, err)
			},
			touched: []string{"status", "updated", "expires_at"},
		},
	}
	for _, eol := range []string{"\n", "\r\n"} {
		for _, tt := range tests {
			t.Run(tt.name+" "+strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
				v := newTestVault(t, map[string]string{"inbox/capture.md": styledCaptureNote(eol)})
				written := tt.change(t, v, filepath.Join(v.Root, "inbox", "capture.md"))
				data, err := os.ReadFile(written)
				if err != nil {
					t.Fatal(err)
				}
				assertKeptLines(t, string(data), eol, tt.touched)
			})
		}
	}
}

// orFail fails the test on err and returns path.
func orFail(t *testing.T, path string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// assertKeptLines checks that every line of styledCapture whose key is not
// in touched is in note, byte for byte and in order, and that every line of
// note ends in eol.
func assertKeptLines(t *testing.T, note, eol string, touched []string) {
	t.Helper()
	lines := strings.SplitAfter(note, "\n")
	pos := 0
	for _, l := range styledCapture {
		if slices.Contains(touched, l.key) {
			continue
		}
		i := slices.Index(lines[pos:], l.text+eol)
		if i < 0 {
			t.Fatalf("line %q is gone or out of order in:\n%s", l.text, note)
		}
		pos += i + 1
	}
	for _, l := range lines {
		if l != "" && (!strings.HasSuffix(l, eol) || (eol == "\n" && strings.HasSuffix(l, "\r\n"))) {
			t.Fatalf("line %q does not end in %q in:\n%q", l, eol, note)
		}
	}
}
