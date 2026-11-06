package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const outsideSecret = "---\ntitle: Credentials\n---\naws_secret=hunter2\n"

// linkedVault is a vault whose notes/leak.md links to a file outside it and
// whose notes/away/ links to a directory outside it.
func linkedVault(t *testing.T) (*Vault, string) {
	t.Helper()
	v := newTestVault(t, map[string]string{"notes/real.md": "---\ntitle: Real\n---\nSee [[leak]].\n"})
	outside := t.TempDir()
	secret := filepath.Join(outside, "credentials")
	if err := os.WriteFile(secret, []byte(outsideSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "x.md"), []byte(outsideSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(v.Root, "notes", "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(v.Root, "notes", "away")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(v.Root, "notes", "real.md"), filepath.Join(v.Root, "notes", "alias.md")); err != nil {
		t.Fatal(err)
	}
	return v, secret
}

func TestNotesAreNeverReadOrWrittenThroughALink(t *testing.T) {
	v, secret := linkedVault(t)
	for _, rel := range []string{"notes/leak.md", "notes/away/x.md", "notes/alias.md"} {
		t.Run(rel, func(t *testing.T) {
			if _, err := v.Read(rel, 0, 0); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("Read err = %v, want ErrInvalidPath", err)
			}
			if _, err := v.Peek(rel, 0); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("Peek err = %v, want ErrInvalidPath", err)
			}
			if _, err := NewSet(v).Locate(rel); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("Locate err = %v, want ErrInvalidPath", err)
			}
			abs := filepath.Join(v.Root, filepath.FromSlash(rel))
			if _, err := v.Edit(abs, EditOptions{Append: "x"}); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("Edit err = %v, want ErrInvalidPath", err)
			}
			if _, err := v.Flag(abs, "doubt", time.Now()); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("Flag err = %v, want ErrInvalidPath", err)
			}
		})
	}
	if data, _ := os.ReadFile(secret); string(data) != outsideSecret {
		t.Errorf("the linked file changed to %q", data)
	}
}

func TestWriteRefusesALinkedTierDirectory(t *testing.T) {
	v := newTestVault(t, nil)
	outside := t.TempDir()
	if err := os.Remove(filepath.Join(v.Root, "outputs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(v.Root, "outputs")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Write(WriteOptions{Tier: TierOutputs, Title: "Away"}); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Write err = %v, want ErrInvalidPath", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("Write put %d files outside the vault", len(entries))
	}
}

func TestWalksSkipLinkedNotes(t *testing.T) {
	v, _ := linkedVault(t)
	listing, err := v.List(ListOptions{Tier: TierNotes})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Notes) != 1 || listing.Notes[0].Path != "notes/real.md" {
		t.Errorf("List = %+v, want notes/real.md alone", listing.Notes)
	}
	if _, err := v.FindByTitle("Credentials"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByTitle through a link err = %v, want ErrNotFound", err)
	}
	report, err := NewSet(v).IDs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Notes != 1 {
		t.Errorf("IDs counted %d notes, want 1", report.Notes)
	}
	sn, err := NewSet(v).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var linked []string
	for _, n := range sn.Notes() {
		if strings.Contains(string(n.Data), "hunter2") {
			t.Errorf("%s was read through its link", n.Ref.Path)
		}
		if n.ParseErr != nil && strings.Contains(n.ParseErr.Error(), "symbolic link") {
			linked = append(linked, n.Ref.Path)
		}
	}
	if strings.Join(linked, ",") != "notes/alias.md,notes/leak.md" {
		t.Errorf("snapshot reports linked notes %v, want notes/alias.md and notes/leak.md", linked)
	}
	for _, c := range sn.Citations(mustNote(t, sn, v, "notes/real.md")) {
		if c.To != nil {
			t.Errorf("[[%s]] resolved to %s, want it dangling", c.Target, c.To.Ref.Path)
		}
	}
}

func mustNote(t *testing.T, sn *Snapshot, v *Vault, rel string) *NoteView {
	t.Helper()
	n, err := sn.Note(Ref{Vault: v, Path: rel})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
