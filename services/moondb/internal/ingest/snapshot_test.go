package ingest

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// snapEnv is a snapshot of the fixture worktree holding files, path to
// content.
func snapEnv(
	c chunkSet,
	session, turn string,
	phase store.SnapshotPhase,
	minute int,
	files map[string]string,
) spool.Envelope {
	env := base(spool.KindSnapshot, turn+":snap:"+string(phase), session, turn, minute)
	var list []store.SnapshotFile
	for _, p := range slices.Sorted(maps.Keys(files)) {
		list = append(list, store.SnapshotFile{
			Path: p, Chunk: c.add(files[p]), Size: int64(len(files[p])), Status: ".M",
		})
	}
	env.Snapshot = &spool.Snapshot{
		Location: store.Location{Repo: repo, Worktree: worktree, Canonical: worktree, Head: "h0"},
		Phase:    phase, Scope: store.SnapshotScope, Files: list, StateHash: "state-" + turn,
	}
	return env
}

// replaceEnv is an Edit of path that swaps old for new, with neither the
// harness's original nor HEAD content, so the store's sources decide its
// before.
func replaceEnv(session, turn, toolUseID string, minute int, path, old, new string) spool.Envelope {
	env := base(spool.KindEdit, toolUseID+":0", session, turn, minute)
	input, _ := json.Marshal(map[string]string{
		"file_path": worktree + "/" + path, "old_string": old, "new_string": new,
	})
	env.Edit = &spool.EditPayload{
		Tool: "Edit", ToolUseID: toolUseID, AbsPath: worktree + "/" + path, Path: path,
		Location: store.Location{Repo: repo, Worktree: worktree, Canonical: worktree, Head: "h0"},
		Kind:     store.EditUpdate, Input: input,
	}
	return env
}

func TestSnapshotsAcrossTurns(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	envs := []spool.Envelope{
		openEnv(c, "s1", "p1", 1, "add a"),
		editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "package a\n"),
		closeEnv(c, "s1", "p1", 3, "done"),
		snapEnv(c, "s1", "p1", store.SnapshotPost, 3, map[string]string{"a.go": "package a\n"}),
		// Between the turns a formatter rewrote a.go.
		openEnv(c, "s1", "p2", 10, "edit a"),
		snapEnv(c, "s1", "p2", store.SnapshotPre, 10,
			map[string]string{"a.go": "package a // formatted\n"}),
		replaceEnv("s1", "p2", "tu2", 11, "a.go", "formatted", "edited"),
	}
	for _, env := range envs {
		f.write(env, c)
	}
	f.drain()
	if n := f.pending(); n != 0 {
		t.Fatalf("%d envelopes left in the spool, want none", n)
	}

	first := f.turn("p1")
	if len(first.Snapshots) != 1 || len(first.Deltas) != 0 {
		t.Errorf("turn p1: %d snapshots, deltas %+v; want one snapshot and no delta",
			len(first.Snapshots), first.Deltas)
	}
	second := f.turn("p2")
	if len(second.Snapshots) != 1 || second.Snapshots[0].Phase != store.SnapshotPre ||
		second.Snapshots[0].FileCount != 1 || second.Snapshots[0].MapChunk == "" {
		t.Errorf("turn p2 snapshots = %+v, want the pre one with one file", second.Snapshots)
	}
	if len(second.Deltas) != 1 {
		t.Fatalf("turn p2 deltas = %+v, want the formatter's", second.Deltas)
	}
	d := second.Deltas[0]
	if d.Cause != store.DeltaSnapshot || d.Path != "a.go" || d.Ambiguous ||
		d.FromChunk != c.add("package a\n") || d.ToChunk != c.add("package a // formatted\n") {
		t.Errorf("delta = %+v, want a.go from the edit's content to the formatted one", d)
	}
	if len(second.Edits) != 1 || second.Edits[0].BeforeSource != store.BeforeSnapshot ||
		second.Edits[0].BeforeChunk != c.add("package a // formatted\n") {
		t.Errorf("edit = %+v, want its before from the pre snapshot", second.Edits)
	}

	before := f.fake.Writes()
	for _, env := range envs {
		f.write(env, c)
		if err := f.in.Ingest(context.Background(), env); err != nil {
			t.Fatalf("replay %s: %v", env.ID, err)
		}
	}
	if got := f.fake.Writes(); got != before {
		t.Errorf("replaying wrote %d records, want none", got-before)
	}
}

func TestSnapshotDeltaAmbiguity(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s2", "q1", 0, "other work"), c)
	f.write(editEnv(c, "s2", "q1", "tu0", 0, "b.go", "package b\n", ""), c)
	f.write(openEnv(c, "s1", "p1", 1, "add a"), c)
	f.write(editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", ""), c)
	f.write(
		snapEnv(c, "s1", "p1", store.SnapshotPost, 3, map[string]string{"a.go": "changed\n"}),
		c,
	)
	f.drain()

	deltas := f.turn("p1").Deltas
	if len(deltas) != 1 || !deltas[0].Ambiguous || deltas[0].Cause != store.DeltaSnapshot {
		t.Errorf("deltas = %+v, want one ambiguous snapshot delta", deltas)
	}
}

func TestSnapshotChunkMissingKeepsTheEntry(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := snapEnv(c, "s1", "p1", store.SnapshotPre, 1, map[string]string{"a.go": "lost\n"})
	env.Snapshot.Files[0].HeadBlob, env.Snapshot.Files[0].HeadChunk = "b0", c.add("head lost\n")
	f.write(env, c)
	for _, key := range []string{env.Snapshot.Files[0].Chunk, env.Snapshot.Files[0].HeadChunk} {
		if err := os.Remove(filepath.Join(f.w.Dir, key+".chunk")); err != nil {
			t.Fatal(err)
		}
	}
	f.drain()

	snaps := f.turn("p1").Snapshots
	if f.pending() != 0 || len(snaps) != 1 || snaps[0].FileCount != 1 {
		t.Fatalf("snapshots = %+v with %d pending, want the snapshot recorded", snaps, f.pending())
	}
	raw, err := f.st.Chunks().Get(context.Background(), snaps[0].MapChunk)
	if err != nil {
		t.Fatal(err)
	}
	var files []store.SnapshotFile
	if err := json.Unmarshal(raw, &files); err != nil || len(files) != 1 || files[0].Chunk != "" ||
		files[0].HeadChunk != "" || files[0].HeadBlob != "b0" {
		t.Errorf("file map = %s (%v), want a.go with its HEAD blob and neither chunk", raw, err)
	}
	if !f.logged("in neither spool nor store") {
		t.Errorf("logs = %q, want a warning about the lost chunk", f.logs)
	}
}
