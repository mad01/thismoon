package vault

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// inboxNotes are raw captures around revokeDay (2026-11-05): one expired,
// one live, one without a date, and an inbox note that is no longer raw.
var inboxNotes = map[string]string{
	"inbox/old.md":     "---\ntitle: Old\nstatus: raw\ncaptured_by: user\nexpires_at: 2026-10-01\n---\nold\n",
	"inbox/fresh.md":   "---\ntitle: Fresh\nexpires_at: 2026-12-01\n---\nfresh\n",
	"inbox/undated.md": "---\ntitle: Undated\nstatus: raw\n---\nno date\n",
	"inbox/checked.md": "---\ntitle: Checked\nstatus: validated\nexpires_at: 2026-01-01\n---\n" +
		"not raw\n",
}

func TestInboxListsRawNotesSoonestFirst(t *testing.T) {
	v := newTestVault(t, inboxNotes)
	v.Name = "personal"
	got, err := NewSet(v).Inbox(revokeDay, false)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, n := range got.Notes {
		paths = append(paths, n.Path)
	}
	want := []string{"inbox/old.md", "inbox/fresh.md", "inbox/undated.md"}
	if !slices.Equal(paths, want) || got.Total != 3 {
		t.Fatalf("inbox = %v (total %d), want %v", paths, got.Total, want)
	}
	if !got.Notes[0].Expired || got.Notes[1].Expired || got.Notes[0].CapturedBy != "user" {
		t.Errorf("notes = %+v", got.Notes)
	}
	expired, err := NewSet(v).Inbox(revokeDay, true)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Total != 1 || expired.Notes[0].Path != "inbox/old.md" {
		t.Errorf("expired = %+v, want inbox/old.md alone", expired.Notes)
	}
}

func TestKeepExtendsExpiryOnly(t *testing.T) {
	v := newTestVault(t, inboxNotes)
	old := filepath.Join(v.Root, "inbox", "old.md")
	res, err := v.Keep(old, 30, revokeDay)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExpiresAt != "2026-12-05" || readLifecycle(t, v, old).ExpiresAt != "2026-12-05" {
		t.Errorf("keep 30 = %+v, want 2026-12-05", res)
	}
	fresh := filepath.Join(v.Root, "inbox", "fresh.md")
	if res, err := v.Keep(fresh, 1, revokeDay); err != nil || res.ExpiresAt != "2026-12-01" {
		t.Errorf("keep 1 on a later expiry = %+v, %v; want 2026-12-01 kept", res, err)
	}
	checked := filepath.Join(v.Root, "inbox", "checked.md")
	if _, err := v.Keep(checked, 30, revokeDay); !errors.Is(err, ErrInvalidStatusChange) {
		t.Errorf("keep of a validated note = %v, want ErrInvalidStatusChange", err)
	}
	v.Shared = true
	if _, err := v.Keep(old, 30, revokeDay); !errors.Is(err, ErrReadOnly) {
		t.Errorf("keep in a shared vault = %v, want ErrReadOnly", err)
	}
}

func TestArchiveExpiredMovesOutOfTheWalk(t *testing.T) {
	v := newTestVault(t, inboxNotes)
	v.Name = "personal"
	dry, err := v.ArchiveExpired(revokeDay, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry) != 1 || dry[0].ArchivedTo != ".brain/archive/inbox/old.md" {
		t.Fatalf("dry run = %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(v.Root, "inbox", "old.md")); err != nil {
		t.Fatalf("a dry run moved the note: %v", err)
	}

	// An earlier archive of the same name keeps its file.
	earlier := filepath.Join(v.Root, ".brain", "archive", "inbox", "old.md")
	if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(earlier, []byte("earlier\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := v.ArchiveExpired(revokeDay, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0].ArchivedTo != ".brain/archive/inbox/old-2.md" {
		t.Fatalf("archive = %+v", moved)
	}
	if data, _ := os.ReadFile(earlier); string(data) != "earlier\n" {
		t.Errorf("archive replaced an earlier archived file: %q", data)
	}
	if _, err := os.Stat(filepath.Join(v.Root, "inbox", "old.md")); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Errorf("inbox/old.md still there: %v", err)
	}
	sn, err := NewSet(v).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range sn.Notes() {
		if n.Ref.Path != "" && filepath.Base(n.Ref.Path) == "old-2.md" {
			t.Errorf("the walk still sees the archived note %s", n.Ref.Path)
		}
	}
	v.Shared = true
	if _, err := v.ArchiveExpired(revokeDay, false); !errors.Is(err, ErrReadOnly) {
		t.Errorf("archive in a shared vault = %v, want ErrReadOnly", err)
	}
}
