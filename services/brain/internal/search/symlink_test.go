package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// linkedFixture is a vault with one real note, a note that is a symbolic
// link to a file outside the vault, and a tier directory that links to a
// directory outside it. Both outside files hold the word "outsideword".
func linkedFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	outside := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "notes", "real.md"), "---\ntitle: Real\n---\ninsideword\n")
	write(filepath.Join(outside, "secret.md"), "---\ntitle: Secret\n---\noutsideword\n")
	write(filepath.Join(outside, "dir", "x.md"), "---\ntitle: X\n---\noutsideword\n")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "notes", "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, "notes", "away")); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestLiteralIndexSkipsLinkedNotes pins that the literal index, and the
// fingerprint that decides when to rebuild it, never read a note through a
// symbolic link, as the vault walks do.
func TestLiteralIndexSkipsLinkedNotes(t *testing.T) {
	root := linkedFixture(t)
	_, files, err := ComputeFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("fingerprint files = %v, want notes/real.md alone", files)
	}
	indexDir := t.TempDir()
	if err := IndexVault(Target{Name: "v", Root: root, IndexDir: indexDir}, nil); err != nil {
		t.Fatal(err)
	}
	matches, err := Search(
		context.Background(),
		indexDir,
		SearchOptions{Pattern: "outsideword", Limit: 50},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("search found linked notes: %v", pathsOf(matches))
	}
	matches, err = Search(
		context.Background(),
		indexDir,
		SearchOptions{Pattern: "insideword", Limit: 50},
	)
	if err != nil || len(matches) != 1 {
		t.Errorf("search insideword = %v, %v; want notes/real.md", pathsOf(matches), err)
	}
}

// TestReadsAfterIndexingSkipALinkedNote pins that a note which became a
// symbolic link after it was indexed is not read through the link: the
// hit metadata and the frontmatter filters see no note there.
func TestReadsAfterIndexingSkipALinkedNote(t *testing.T) {
	root := linkedFixture(t)
	indexDir := t.TempDir()
	if err := IndexVault(Target{Name: "v", Root: root, IndexDir: indexDir}, nil); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "swapped.md")
	swapped := "---\ntitle: Swapped\nstatus: disputed\ntags: [away]\n---\ninsideword\n"
	if err := os.WriteFile(outside, []byte(swapped), 0o644); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "notes", "real.md")
	if err := os.Remove(real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, real); err != nil {
		t.Fatal(err)
	}
	if m := NewMetaReader(map[string]string{"v": root}, time.Now()).Meta("v", "notes/real.md"); m != (NoteMeta{}) {
		t.Errorf("Meta = %+v, want none for a note that is now a link", m)
	}
	matches, err := Search(context.Background(), indexDir, SearchOptions{
		Pattern: "insideword", Tag: "away", Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("tag filter read the link: %v", pathsOf(matches))
	}
}
