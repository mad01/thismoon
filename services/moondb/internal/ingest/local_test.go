package ingest

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// TestIngestLocalStore runs the turn and edit kinds against the local
// backend, whose Put replaces the whole record: the merges have to keep
// what earlier envelopes set, survive a replay without writing, and come
// back the same after the store is reopened from its log.
func TestIngestLocalStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	st, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	f.st, f.fake, f.in.Store = st, nil, st

	c := chunkSet{}
	closed := closeEnv(c, "s1", "p1", 5, "done")
	closed.Session.Title, closed.Session.Model = "", "m1"
	next := openEnv(c, "s1", "p2", 6, "next")
	next.Session.Title = ""
	batches := [][]spool.Envelope{
		{
			openEnv(c, "s2", "q1", 0, "other work"),
			editEnv(c, "s2", "q1", "tu0", 0, "b.go", "package b\n", ""),
			// before its open, and disagreeing with the disk read
			editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "package  a\n"),
		},
		{openEnv(c, "s1", "p1", 1, "add a"), closed, next},
	}
	for _, batch := range batches {
		for _, env := range batch {
			f.write(env, c)
		}
		f.drain()
	}
	if n := f.pending(); n != 0 {
		t.Fatalf("%d envelopes left in the spool: %q", n, f.logs)
	}
	checkLocal(t, st)

	size := dirSize(t, dir)
	for _, batch := range batches {
		for _, env := range batch {
			if err := f.in.Ingest(context.Background(), env); err != nil {
				t.Fatalf("replay %s: %v", env.ID, err)
			}
		}
	}
	if got := dirSize(t, dir); got != size {
		t.Errorf("replay grew the store from %d to %d bytes, want no writes", size, got)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	checkLocal(t, reopened)
}

// checkLocal checks what TestIngestLocalStore's envelopes leave behind.
func checkLocal(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	sd, err := st.Session(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s := sd.Session; s.Title != "title s1" || s.Model != "m1" || s.Cwd != worktree ||
		s.Harness != "claude-code" || !s.StartedAt.Equal(at(1)) {
		t.Errorf("session = %+v, want the first title kept, model m1, started %v", s, at(1))
	}
	p1, err := st.Turn(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if tr := p1.Turn; tr.Seq != 1 || tr.SessionID != "s1" || tr.Status != store.TurnClosed ||
		tr.PromptChunk != spool.ChunkKey([]byte("add a")) ||
		tr.FinalChunk != spool.ChunkKey([]byte("done")) || tr.TranscriptOffset != 10 ||
		!tr.StartedAt.Equal(at(1)) || tr.EndedAt == nil || !tr.EndedAt.Equal(at(5)) {
		t.Errorf("turn p1 = %+v, want seq 1, closed, prompt and final kept, %v..%v", tr,
			at(1), at(5))
	}
	if len(p1.Edits) != 1 || p1.Edits[0].ID != "tu1:0" {
		t.Errorf("turn p1 edits = %+v, want tu1:0", p1.Edits)
	}
	if len(p1.Deltas) != 1 || !p1.Deltas[0].Ambiguous {
		t.Errorf("turn p1 deltas = %+v, want one ambiguous delta", p1.Deltas)
	}
	p2, err := st.Turn(ctx, "p2")
	if err != nil {
		t.Fatal(err)
	}
	if p2.Seq != 2 || p2.Status != store.TurnOpen {
		t.Errorf("turn p2 = %+v, want seq 2, open", p2.Turn)
	}
}

// dirSize sums the sizes of the regular files under dir.
func dirSize(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}
