package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

// TestPurgeDropsAWithdrawnNoteAtOnce pins that withdraw leaves no chunk
// behind before any sync runs, and that the next sync keeps it that way
// without embedding the tombstone.
func TestPurgeDropsAWithdrawnNoteAtOnce(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	lib, v := f.open("model-a")
	f.refresh(lib, v, false)
	dir, err := lib.Config().Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := v.Withdraw(filepath.Join(f.root, "notes", "b.md"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := lib.Purge(context.Background(), v, "notes/b.md"); err != nil {
		t.Fatal(err)
	}
	idx, err := semantic.Open(dir, f.embed.embed)
	if err != nil {
		t.Fatal(err)
	}
	if n := idx.Count(); n != 2 {
		t.Errorf("index holds %d chunks after the purge, want 2", n)
	}
	m, _, err := semantic.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Notes["notes/b.md"]; ok {
		t.Error("manifest still lists the withdrawn note")
	}

	before := f.embed.calls.Load()
	if res := f.refresh(lib, v, false); res.Notes != 0 {
		t.Errorf("sync after a withdraw = %+v, want the tombstone skipped", res)
	}
	if calls := f.embed.calls.Load() - before; calls != 0 {
		t.Errorf("sync embedded %d chunk(s) for a tombstone", calls)
	}
	if got := f.paths(lib, "zebra stripes"); got["notes/b.md"] {
		t.Errorf("a withdrawn note still answers: %v", got)
	}
}
