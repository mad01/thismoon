package derive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// minutes gives a time relative to the fixture's edit time, at.
func minutes(m int) time.Time { return at.Add(time.Duration(m) * time.Minute) }

// pastEdit is the latest earlier edit a fake store holds for the path.
type pastEdit struct {
	at              int
	after, observed string
}

// pastSnap is the latest snapshot a fake store holds for the worktree:
// files maps a path to its content; a missing path was clean, and nil
// content marks a deleted file.
type pastSnap struct {
	at        int
	truncated bool
	files     map[string]*string
}

// fakeStore is the part of the store a Deriver reads, over memChunks.
type fakeStore struct {
	c           memChunks
	edit        *pastEdit
	snap        *pastSnap
	contentless *int // the time of an edit with no after-content, if any
}

func (f *fakeStore) put(s string) string {
	k, _ := f.c.Put(context.Background(), []byte(s))
	return k
}

func (f *fakeStore) deriver() *Deriver {
	return &Deriver{
		Chunks: f.c,
		LatestAfter: func(_ context.Context, q store.ChainQuery) (store.Edit, error) {
			if f.edit == nil || minutes(f.edit.at).After(q.At) {
				return store.Edit{}, store.ErrNotFound
			}
			e := store.Edit{
				ID:         "toolu_0:0",
				At:         minutes(f.edit.at),
				AfterChunk: f.put(f.edit.after),
			}
			if f.edit.observed != "" {
				e.ObservedChunk = f.put(f.edit.observed)
			}
			return e, nil
		},
		LatestSnapshot: func(_ context.Context, q store.SnapshotQuery) (store.Snapshot, error) {
			if f.snap == nil || q.Worktree != "/wt" || minutes(f.snap.at).After(q.At) {
				return store.Snapshot{}, store.ErrNotFound
			}
			return f.snapshot(), nil
		},
		Edits: func(_ context.Context, q store.EditQuery) ([]store.Edit, error) {
			if f.contentless == nil || minutes(*f.contentless).Before(q.Since) {
				return nil, nil
			}
			return []store.Edit{{ID: "toolu_9:0", At: minutes(*f.contentless)}}, nil
		},
	}
}

func (f *fakeStore) snapshot() store.Snapshot {
	var list []store.SnapshotFile
	for p, content := range f.snap.files {
		if content == nil {
			list = append(list, store.SnapshotFile{Path: p, Deleted: true})
			continue
		}
		list = append(list, store.SnapshotFile{Path: p, Chunk: f.put(*content)})
	}
	raw, _ := json.Marshal(list)
	return store.Snapshot{
		ID: "prev", At: minutes(f.snap.at), Location: store.Location{Worktree: "/wt"},
		MapChunk: f.put(string(raw)), Truncated: f.snap.truncated,
	}
}

func TestEditSnapshotBefore(t *testing.T) {
	// appended still takes editHunks, so only the order of the sources
	// decides between it and before.
	appended := before + "// appended\n"
	tests := []struct {
		name   string
		edit   *pastEdit
		snap   *pastSnap
		source store.BeforeSource
		want   string
	}{
		{
			name:   "a snapshot taken after the chain's edit wins",
			edit:   &pastEdit{at: -10, after: appended},
			snap:   &pastSnap{at: -5, files: map[string]*string{"a.go": ptr(before)}},
			source: store.BeforeSnapshot, want: before,
		},
		{
			name:   "a snapshot older than the chain's edit yields to it",
			edit:   &pastEdit{at: -5, after: appended},
			snap:   &pastSnap{at: -10, files: map[string]*string{"a.go": ptr(before)}},
			source: store.BeforeChain, want: appended,
		},
		{
			name:   "a newer whole snapshot without the path: it was clean, so HEAD",
			edit:   &pastEdit{at: -10, after: appended},
			snap:   &pastSnap{at: -5, files: map[string]*string{"b.go": ptr("b")}},
			source: store.BeforeHead, want: before,
		},
		{
			name:   "a newer truncated snapshot without the path says nothing",
			edit:   &pastEdit{at: -10, after: appended},
			snap:   &pastSnap{at: -5, truncated: true},
			source: store.BeforeChain, want: appended,
		},
		{
			name:   "no edit before, the snapshot holds the path",
			snap:   &pastSnap{at: -5, files: map[string]*string{"a.go": ptr(appended)}},
			source: store.BeforeSnapshot, want: appended,
		},
		{
			name:   "a snapshot copy the hunks do not fit falls through",
			snap:   &pastSnap{at: -5, files: map[string]*string{"a.go": ptr(stale)}},
			source: store.BeforeHead, want: before,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStore{c: memChunks{}, edit: tt.edit, snap: tt.snap}
			raw, _ := json.Marshal(editHunks)
			p := &spool.EditPayload{
				Tool: "Edit", ToolUseID: "toolu_1", Path: "a.go", Kind: store.EditUpdate,
				Location: store.Location{Worktree: "/wt"}, Input: []byte(editInput),
				PatchChunk: f.put(string(raw)), HeadChunk: f.put(before),
			}
			e, _, err := f.deriver().Edit(context.Background(), envelope(p))
			if err != nil {
				t.Fatal(err)
			}
			if e.BeforeSource != tt.source || string(f.c[e.BeforeChunk]) != tt.want {
				t.Errorf("before = %s %q, want %s %q",
					e.BeforeSource, f.c[e.BeforeChunk], tt.source, tt.want)
			}
		})
	}
}

func TestBashChangedPathTakesNewerSnapshot(t *testing.T) {
	// A formatter changed the file between turns: the snapshot after the
	// last edit holds what the command started from.
	f := &fakeStore{
		c:    memChunks{},
		edit: &pastEdit{at: -10, after: stale},
		snap: &pastSnap{at: -5, files: map[string]*string{"a.go": ptr(before)}},
	}
	p := &spool.EditPayload{
		Tool: "Bash", ToolUseID: "toolu_1", Path: "a.go", Kind: store.EditUpdate,
		Location: store.Location{Worktree: "/wt"}, ObservedChunk: f.put(after),
	}
	e, delta, err := f.deriver().Edit(context.Background(), envelope(p))
	if err != nil || delta != nil {
		t.Fatalf("Edit: delta %+v, err %v", delta, err)
	}
	if e.BeforeSource != store.BeforeSnapshot || string(f.c[e.BeforeChunk]) != before ||
		string(f.c[e.AfterChunk]) != after || e.Kind != store.EditUpdate {
		t.Errorf("edit = %+v, want snapshot before, disk read after", e)
	}
}

func TestSnapshotRecord(t *testing.T) {
	f := &fakeStore{c: memChunks{}}
	loc := store.Location{Repo: "github.com/o/r", Worktree: "/wt", Head: "abc"}
	env := snapshotEnvelope(loc,
		store.SnapshotFile{Path: "z.go", Chunk: f.put("z\n"), Size: 2, Status: "??"},
		store.SnapshotFile{Path: "a.go", Size: 2 << 20, Status: ".M"},
	)
	env.Snapshot.Truncated = true
	env.Snapshot.Skipped = store.SnapshotSkipped{Timeout: 3, TooLarge: 1}
	snap, deltas, err := f.deriver().Snapshot(context.Background(), env)
	if err != nil || len(deltas) != 0 {
		t.Fatalf("Snapshot: %d deltas, err %v", len(deltas), err)
	}
	want := store.Snapshot{
		ID: "turn-1:snap:post:wt", SessionID: "sess-1", TurnID: "turn-1",
		Phase: store.SnapshotPost, At: at, Location: loc, Scope: store.SnapshotScope,
		MapChunk: snap.MapChunk, FileCount: 2, Truncated: true,
		Skipped: store.SnapshotSkipped{Timeout: 3, TooLarge: 1},
	}
	if snap != want {
		t.Errorf("snapshot =\n%+v\nwant\n%+v", snap, want)
	}
	var files []store.SnapshotFile
	if err := json.Unmarshal(f.c[snap.MapChunk], &files); err != nil ||
		len(files) != 2 || files[0].Path != "a.go" || files[1].Path != "z.go" {
		t.Errorf("file map = %s (%v), want both files sorted by path", f.c[snap.MapChunk], err)
	}
	if _, _, err := f.deriver().Snapshot(context.Background(), envelope(nil)); err == nil {
		t.Error("an envelope without a snapshot payload derived without error")
	}
}

func TestSnapshotDeltas(t *testing.T) {
	const x, y, z = "x\n", "y\n", "z\n"
	deleted := (*string)(nil)
	tests := []struct {
		name        string
		edit        *pastEdit
		snap        *pastSnap
		contentless *int
		now         *string // the file as this snapshot found it; nil: deleted
		from        string  // "" means no delta
	}{
		{
			name: "changed since the last edit",
			edit: &pastEdit{at: -9, after: x},
			now:  ptr(y),
			from: x,
		},
		{name: "unchanged since the last edit", edit: &pastEdit{at: -9, after: x}, now: ptr(x)},
		{
			// The edit's disk read already differed, and that delta is
			// recorded with the edit: the snapshot does not repeat it.
			name: "the edit's disk read is the known content",
			edit: &pastEdit{at: -9, after: x, observed: y}, now: ptr(y),
		},
		{
			name: "the previous snapshot is newer than the edit",
			edit: &pastEdit{at: -9, after: x},
			snap: &pastSnap{at: -5, files: map[string]*string{"a.go": ptr(y)}},
			now:  ptr(z), from: y,
		},
		{
			name: "the previous snapshot is older than the edit",
			edit: &pastEdit{at: -5, after: x},
			snap: &pastSnap{at: -9, files: map[string]*string{"a.go": ptr(y)}},
			now:  ptr(z), from: x,
		},
		{
			name: "the newer previous snapshot had the path clean",
			edit: &pastEdit{at: -9, after: x},
			snap: &pastSnap{at: -5, files: map[string]*string{}},
			now:  ptr(z),
		},
		{
			name: "the newer previous snapshot was cut short",
			edit: &pastEdit{at: -9, after: x},
			snap: &pastSnap{at: -5, truncated: true},
			now:  ptr(z), from: x,
		},
		{
			name: "the newer previous snapshot had it deleted",
			snap: &pastSnap{at: -5, files: map[string]*string{"a.go": deleted}},
			now:  ptr(z),
		},
		{
			name:        "an edit since left no after-content",
			edit:        &pastEdit{at: -9, after: x},
			contentless: new(-3),
			now:         ptr(z),
		},
		{
			name: "deleted since the last edit",
			edit: &pastEdit{at: -9, after: x},
			now:  deleted,
			from: x,
		},
		{name: "nothing known before", now: ptr(z)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStore{
				c:           memChunks{},
				edit:        tt.edit,
				snap:        tt.snap,
				contentless: tt.contentless,
			}
			file := store.SnapshotFile{Path: "a.go", Deleted: tt.now == nil}
			if tt.now != nil {
				file.Chunk = f.put(*tt.now)
			}
			env := snapshotEnvelope(store.Location{Worktree: "/wt"}, file)
			_, deltas, err := f.deriver().Snapshot(context.Background(), env)
			if err != nil {
				t.Fatal(err)
			}
			if tt.from == "" {
				if len(deltas) != 0 {
					t.Errorf("deltas = %+v, want none", deltas)
				}
				return
			}
			if len(deltas) != 1 {
				t.Fatalf("deltas = %+v, want one", deltas)
			}
			d := deltas[0]
			if d.ID != "turn-1:snap:post:wt:a.go" || d.Cause != store.DeltaSnapshot ||
				d.TurnID != "turn-1" || d.SessionID != "sess-1" || !d.At.Equal(at) ||
				d.Path != "a.go" || d.Worktree != "/wt" {
				t.Errorf("delta identity = %+v", d)
			}
			if string(f.c[d.FromChunk]) != tt.from || d.ToChunk != file.Chunk {
				t.Errorf("delta from %q to %q, want from %q to the snapshot's chunk",
					f.c[d.FromChunk], d.ToChunk, tt.from)
			}
			diff := string(f.c[d.PatchChunk])
			wantDiff := "--- a/a.go\n+++ b/a.go\n@@ -1 +0,0 @@\n-" + tt.from
			if tt.now != nil {
				wantDiff = "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-" + tt.from + "+" + *tt.now
			}
			if diff != wantDiff {
				t.Errorf("delta patch =\n%s\nwant\n%s", diff, wantDiff)
			}
		})
	}
}

func TestSnapshotLookupErrors(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeStore{c: memChunks{}}
	d := f.deriver()
	d.LatestSnapshot = func(context.Context, store.SnapshotQuery) (store.Snapshot, error) {
		return store.Snapshot{}, boom
	}
	env := snapshotEnvelope(store.Location{Worktree: "/wt"},
		store.SnapshotFile{Path: "a.go", Chunk: f.put("a\n")})
	if _, _, err := d.Snapshot(context.Background(), env); !errors.Is(err, boom) {
		t.Errorf("snapshot lookup failure: err = %v, want it to wrap %v", err, boom)
	}
}

func snapshotEnvelope(loc store.Location, files ...store.SnapshotFile) spool.Envelope {
	return spool.Envelope{
		Version: spool.Version, Kind: spool.KindSnapshot, ID: "turn-1:snap:post:wt", At: at,
		Harness: "test-harness", Session: spool.SessionRef{ID: "sess-1"},
		Turn: spool.TurnRef{ID: "turn-1"},
		Snapshot: &spool.Snapshot{
			Location: loc, Phase: store.SnapshotPost, Scope: store.SnapshotScope, Files: files,
		},
	}
}
