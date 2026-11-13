package semantic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestSyncSkipsLinkedNotes pins that the semantic sync never embeds a note
// read through a symbolic link, as the vault walks do: neither a note that
// is a link nor one under a linked tier directory.
func TestSyncSkipsLinkedNotes(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "notes", "real.md"), "---\ntitle: Real\n---\ninside words\n")
	write(filepath.Join(outside, "secret.md"), "---\ntitle: Secret\n---\noutside words\n")
	write(filepath.Join(outside, "x.md"), "---\ntitle: X\n---\noutside words\n")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "notes", "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outputs")); err != nil {
		t.Fatal(err)
	}
	idx, err := Open(t.TempDir(), hashEmbed)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Sync(context.Background(), idx, root, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := report.Notes["notes/real.md"]; !ok || len(report.Notes) != 1 ||
		report.Embedded != 1 {
		t.Errorf("sync = %+v, want notes/real.md alone embedded", report)
	}
}
