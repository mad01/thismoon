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
			for _, rel := range tc.outside {
				if _, err := os.Stat(filepath.Join(filepath.Dir(root), rel)); err == nil {
					t.Errorf("%s was created outside the vault", rel)
				}
			}
		})
	}
}
