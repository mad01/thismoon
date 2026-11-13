package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestIndexVaultKeepsThePreviousIndexOnAWalkError pins that a walk that
// fails commits nothing: the shards of the last good build still answer.
func TestIndexVaultKeepsThePreviousIndexOnAWalkError(t *testing.T) {
	tests := []struct {
		name       string
		breakVault func(t *testing.T, root string)
	}{
		{"vault root gone", func(t *testing.T, root string) {
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory unreadable", func(t *testing.T, root string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads a directory whatever its mode")
			}
			locked := filepath.Join(root, "notes", "locked")
			writeTestNote(t, filepath.Join(locked, "secret.md"), "---\ntitle: S\n---\nwalrus\n")
			if err := os.Chmod(locked, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "vault")
			writeTestNote(
				t,
				filepath.Join(root, "notes", "kept.md"),
				"---\ntitle: Kept\n---\nwalrus\n",
			)
			target := Target{Name: "v", Root: root, IndexDir: t.TempDir()}
			if err := IndexVault(target, nil); err != nil {
				t.Fatal(err)
			}
			tt.breakVault(t, root)
			if err := IndexVault(target, nil); err == nil {
				t.Fatal("IndexVault of a vault it cannot walk returned nil")
			}
			// IncludeRevoked skips the frontmatter read that drops a hit
			// whose file is gone, so the hits are the index's own.
			matches, err := Search(context.Background(), target.IndexDir,
				SearchOptions{Pattern: "walrus", IncludeRevoked: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 1 || matches[0].Path != "notes/kept.md" {
				t.Errorf("matches = %+v, want the previous index's notes/kept.md", matches)
			}
		})
	}
}

func writeTestNote(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
