package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/mad01/thismoon/services/brain/internal/check"
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
		{
			name:       "edit --status superseded",
			args:       []string{"edit", "notes/beta.md", "--status", "superseded"},
			wantStderr: "brain supersede",
		},
		{
			name:       "edit --set status unknown",
			args:       []string{"edit", "notes/beta.md", "--set", "status=finished"},
			wantStderr: "is not one of raw, validated, promoting, disputed",
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

func TestLinkedNotesAreRefusedAndChecked(t *testing.T) {
	root := setupCLI(t, "")
	secret := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(secret, []byte("aws_secret=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "notes", "leak.md")); err != nil {
		t.Fatal(err)
	}
	got := runBrain(t, "", "read", "notes/leak.md")
	if got.code != exitUsage || strings.Contains(got.stdout+got.stderr, "hunter2") ||
		!strings.Contains(got.stderr, "symbolic link") {
		t.Errorf("read = %d %q %q, want exit 2 naming the link, no content", got.code, got.stdout, got.stderr)
	}
	got = runBrain(t, "", "check", "-o", "json")
	if got.code != exitError {
		t.Fatalf("check exit = %d, want 1; stdout %s", got.code, got.stdout)
	}
	report := decodeStrict[check.Report](t, got.stdout)
	found := false
	for _, f := range report.Findings {
		if f.Path == "notes/leak.md" && f.Level == check.Error && strings.Contains(f.Message, "symbolic link") {
			found = true
		}
	}
	if !found || strings.Contains(got.stdout, "hunter2") {
		t.Errorf("check findings %+v, want an error for notes/leak.md and no content", report.Findings)
	}
}

func TestNULBytesAreRefusedAndChecked(t *testing.T) {
	root := setupCLI(t, "")
	got := runBrain(t, "x\x00y\n", "write", "--title", "Nul")
	if got.code != exitUsage || !strings.Contains(got.stderr, "NUL byte") {
		t.Errorf("write = %d %q, want exit 2 naming the NUL byte", got.code, got.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "nul.md")); err == nil {
		t.Error("inbox/nul.md was written")
	}
	held := "---\ntitle: Held\n---\nx\x00y\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "held.md"), []byte(held), 0o644); err != nil {
		t.Fatal(err)
	}
	got = runBrain(t, "", "check", "-o", "json", "notes/held.md")
	if got.code != exitError {
		t.Fatalf("check exit = %d, want 1; stdout %s", got.code, got.stdout)
	}
	report := decodeStrict[check.Report](t, got.stdout)
	found := false
	for _, f := range report.Findings {
		found = found || (f.Level == check.Error && f.Line == 4 && strings.HasPrefix(f.Message, "nul-byte: "))
	}
	if !found {
		t.Errorf("check findings %+v, want a nul-byte error on line 4", report.Findings)
	}
}

func TestListAndPeekEscapeControlCharacters(t *testing.T) {
	root := setupCLI(t, "")
	note := "---\ntitle: \"red \\e[31m\\ntwo\"\nstatus: disputed\nstatus_reason: \"bad\\rreason\"\n---\nbody \x1b[2J line\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "ansi.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list", "notes"}, {"peek", "notes/ansi.md"}} {
		got := runBrain(t, "", args...)
		if got.code != exitOK {
			t.Fatalf("%v exit = %d; stderr %q", args, got.code, got.stderr)
		}
		for _, r := range got.stdout {
			if r != '\n' && r != '\t' && unicode.IsControl(r) {
				t.Errorf("%v printed control character %U in %q", args, r, got.stdout)
				break
			}
		}
		if args[0] == "list" && !strings.Contains(got.stdout, `red \x1b[31m\ntwo`) {
			t.Errorf("list = %q, want the title with its control characters escaped", got.stdout)
		}
		if args[0] == "peek" && !strings.Contains(got.stdout, `body \x1b[2J line`) {
			t.Errorf("peek = %q, want the body line escaped", got.stdout)
		}
	}
}
