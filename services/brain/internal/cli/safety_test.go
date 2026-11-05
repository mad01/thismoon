package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// safetyCase is one command the vault must refuse with a usage error,
// leaving the files it names as they were.
type safetyCase struct {
	name string
	// files are written into the fixture vault first.
	files map[string]string
	args  []string
	// wantStderr is a substring of the one stderr line.
	wantStderr string
	// outside are paths, relative to the vault's parent, that must not
	// exist afterwards.
	outside []string
}

func TestRefusedWritesExitUsage(t *testing.T) {
	tests := []safetyCase{
		{
			name:       "edit unclosed frontmatter",
			files:      map[string]string{"inbox/open.md": "---\ntitle: Open\nno closing fence\n"},
			args:       []string{"edit", "inbox/open.md", "--append", "x"},
			wantStderr: "frontmatter is not closed",
		},
		{
			name:       "write filename outside the vault",
			args:       []string{"write", "--title", "Esc", "--filename", "../../escaped", "--body", "x"},
			wantStderr: "vault: invalid path",
			outside:    []string{"escaped.md", "../escaped.md"},
		},
		{
			name:       "write filename into another tier",
			args:       []string{"write", "--tier", "notes", "--title", "Esc", "--filename", "../inbox/esc"},
			wantStderr: "vault: invalid path",
		},
		{
			name:       "promote moc outside the vault",
			args:       []string{"promote", "inbox/raw.md", "--moc", "../../outside.md"},
			wantStderr: "vault: invalid path",
			outside:    []string{"outside.md"},
		},
		{
			name:       "promote moc in another tier",
			args:       []string{"promote", "inbox/raw.md", "--moc", "notes/beta.md"},
			wantStderr: "is not a note in the vault's MOCs/ tier",
		},
		{
			name:       "write title with a newline",
			args:       []string{"write", "--title", "two\nlines", "--body", "x"},
			wantStderr: "holds a control character",
		},
		{
			name:       "capture title with an escape",
			args:       []string{"capture", "--title", "red \x1b[31m", "--body", "x"},
			wantStderr: "holds a control character",
		},
		{
			name:       "edit --set title with a newline",
			args:       []string{"edit", "notes/beta.md", "--set", `title="two\nlines"`},
			wantStderr: "holds a control character",
		},
		{
			name:       "promote --title with a newline",
			args:       []string{"promote", "inbox/raw.md", "--title", "two\nlines"},
			wantStderr: "holds a control character",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := setupCLI(t, "")
			for rel, content := range tc.files {
				if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := runBrain(t, "", tc.args...)
			if got.code != exitUsage {
				t.Fatalf("exit = %d, want %d; stderr %q", got.code, exitUsage, got.stderr)
			}
			if strings.Count(got.stderr, "\n") != 1 || !strings.Contains(got.stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want one line containing %q", got.stderr, tc.wantStderr)
			}
			for rel, content := range tc.files {
				if data, _ := os.ReadFile(filepath.Join(root, rel)); string(data) != content {
					t.Errorf("%s changed to %q", rel, data)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "inbox", "raw.md")); err != nil {
				t.Errorf("inbox/raw.md moved: %v", err)
			}
			for _, rel := range tc.outside {
				if _, err := os.Stat(filepath.Join(filepath.Dir(root), rel)); err == nil {
					t.Errorf("%s was created outside the vault", rel)
				}
			}
		})
	}
}

func TestCaptureTitleDropsControlCharacters(t *testing.T) {
	if got := captureTitle("", "# Red \x1b[31mtitle\x07\nrest", ""); got != "Red [31mtitle" {
		t.Errorf("captureTitle = %q, want the first line without control characters", got)
	}
	if got := captureTitle("", "\x1b\x07\nSecond line", ""); got != "Second line" {
		t.Errorf("captureTitle = %q, want the first line with text", got)
	}
}
