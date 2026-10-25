package spool

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var at = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// editEnvelope builds a valid edit envelope whose chunks are the given
// contents, in the order patch, original, observed.
func editEnvelope(id string, contents ...string) (Envelope, map[string][]byte) {
	chunks := map[string][]byte{}
	keys := make([]string, 3)
	for i, c := range contents {
		keys[i] = ChunkKey([]byte(c))
		chunks[keys[i]] = []byte(c)
	}
	env := Envelope{
		Version: Version, Kind: KindEdit, ID: id, At: at, Harness: "claude-code",
		Session: SessionRef{ID: "s1"}, Turn: TurnRef{ID: "p1"},
		Edit: &EditPayload{
			Tool: "Edit", ToolUseID: id, Path: "a.go", Kind: store.EditUpdate,
			PatchChunk: keys[0], OriginalChunk: keys[1], ObservedChunk: keys[2],
		},
	}
	return env, chunks
}

func mustWrite(t *testing.T, w *Writer, env Envelope, chunks map[string][]byte) {
	t.Helper()
	if err := w.Write(env, chunks); err != nil {
		t.Fatalf("Write(%s): %v", env.ID, err)
	}
}

func mustList(t *testing.T, r *Reader) []string {
	t.Helper()
	paths, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return paths
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	env, chunks := editEnvelope("toolu_01:0", "patch", "before", "after")
	mustWrite(t, w, env, chunks)

	paths := mustList(t, r)
	if len(paths) != 1 {
		t.Fatalf("List() = %v, want one envelope", paths)
	}
	name := filepath.Base(paths[0])
	if !regexp.MustCompile(`^\d{20}-edit-toolu_01_0\.json$`).MatchString(name) {
		t.Errorf("envelope name = %q, want <nanos>-edit-toolu_01_0.json", name)
	}
	got, err := r.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.ID != env.ID || got.Kind != KindEdit || !got.At.Equal(at) ||
		!reflect.DeepEqual(got.Edit, env.Edit) {
		t.Errorf("Read() = %+v, want %+v", got, env)
	}
	for _, key := range got.ChunkKeys() {
		data, err := r.Chunk(key)
		if err != nil {
			t.Fatalf("Chunk(%s): %v", key, err)
		}
		if string(data) != string(chunks[key]) {
			t.Errorf("Chunk(%s) = %q, want %q", key, data, chunks[key])
		}
	}
	assertPerm(t, dir, 0o700)
	assertPerm(t, filepath.Join(dir, tmpDir), 0o700)
	assertPerm(t, paths[0], 0o600)
	assertPerm(t, filepath.Join(dir, got.Edit.PatchChunk+chunkSuffix), 0o600)
}

func assertPerm(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %s = %o, want %o", filepath.Base(path), got, want)
	}
}

func TestWriteRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Envelope, map[string][]byte)
	}{
		{"old version", func(e *Envelope, _ map[string][]byte) { e.Version = 0 }},
		{"unknown kind", func(e *Envelope, _ map[string][]byte) { e.Kind = "bogus" }},
		{"no id", func(e *Envelope, _ map[string][]byte) { e.ID = "" }},
		{"no payload", func(e *Envelope, _ map[string][]byte) { e.Edit = nil }},
		{"extra payload", func(e *Envelope, _ map[string][]byte) { e.TurnOpen = &TurnOpen{} }},
		{"kind names another payload", func(e *Envelope, _ map[string][]byte) {
			e.Kind = KindTurnOpen
		}},
		{"chunk key mismatch", func(_ *Envelope, c map[string][]byte) {
			c[ChunkKey([]byte("x"))] = []byte("y")
		}},
		{"chunk key not a key", func(_ *Envelope, c map[string][]byte) {
			c["../escape"] = []byte("y")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env, chunks := editEnvelope("toolu_01:0", "patch")
			tc.mutate(&env, chunks)
			if err := (&Writer{Dir: dir}).Write(env, chunks); err == nil {
				t.Fatal("Write() = nil, want an error")
			}
			if paths := mustList(t, &Reader{Dir: dir}); len(paths) != 0 {
				t.Errorf("List() = %v after a rejected write, want none", paths)
			}
		})
	}
}

func TestWriteSkipsStoredChunk(t *testing.T) {
	root := t.TempDir()
	dir, objects := filepath.Join(root, "spool"), filepath.Join(root, "objects")
	env, chunks := editEnvelope("toolu_01:0", "stored", "fresh")
	stored := env.Edit.PatchChunk
	if err := os.MkdirAll(filepath.Join(objects, stored[:2]), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objects, stored[:2], stored+".gz"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, &Writer{Dir: dir, Objects: objects}, env, chunks)

	if exists(filepath.Join(dir, stored+chunkSuffix)) {
		t.Error("chunk already in the store was written to the spool")
	}
	if !exists(filepath.Join(dir, env.Edit.OriginalChunk+chunkSuffix)) {
		t.Error("chunk missing from the store was not written to the spool")
	}
}

func TestListIsWriteOrder(t *testing.T) {
	dir := t.TempDir()
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	ids := []string{"c", "a", "b"}
	for _, id := range ids {
		env, chunks := editEnvelope(id, "patch-"+id)
		mustWrite(t, w, env, chunks)
	}
	paths := mustList(t, r)
	if len(paths) != len(ids) {
		t.Fatalf("List() = %v, want %d envelopes", paths, len(ids))
	}
	for i, p := range paths {
		env, err := r.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		if env.ID != ids[i] {
			t.Errorf("List()[%d] holds %q, want %q", i, env.ID, ids[i])
		}
	}
	if paths := mustList(t, &Reader{Dir: filepath.Join(dir, "absent")}); len(paths) != 0 {
		t.Errorf("List() of a missing spool = %v, want none", paths)
	}
}

func TestReadRejectsOtherVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1-edit-x.json")
	if err := os.WriteFile(path, []byte(`{"v":2,"kind":"edit","id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Reader{Dir: dir}).Read(path); err == nil {
		t.Error("Read() of a version 2 envelope = nil error, want one")
	}
}

func TestChunkVerifies(t *testing.T) {
	dir := t.TempDir()
	r := &Reader{Dir: dir}
	key := ChunkKey([]byte("content"))
	if _, err := r.Chunk(key); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Chunk(missing) error = %v, want fs.ErrNotExist", err)
	}
	if err := os.WriteFile(filepath.Join(dir, key+chunkSuffix), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Chunk(key); err == nil {
		t.Error("Chunk(tampered) = nil error, want a mismatch")
	}
	if _, err := r.Chunk("../../etc/passwd"); err == nil {
		t.Error("Chunk(non-key) = nil error, want one")
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	env, chunks := editEnvelope("x", "patch")
	mustWrite(t, w, env, chunks)
	path := mustList(t, r)[0]
	for i := range 2 {
		if err := r.Remove(path); err != nil {
			t.Fatalf("Remove() call %d: %v", i+1, err)
		}
	}
	if paths := mustList(t, r); len(paths) != 0 {
		t.Errorf("List() after Remove = %v, want none", paths)
	}
}

func TestSweep(t *testing.T) {
	dir := t.TempDir()
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	ingested, chunks1 := editEnvelope("ingested", "shared", "only-ingested")
	pending, chunks2 := editEnvelope("pending", "shared")
	mustWrite(t, w, ingested, chunks1)
	mustWrite(t, w, pending, chunks2)
	if err := r.Remove(mustList(t, r)[0]); err != nil {
		t.Fatal(err)
	}
	young := ChunkKey([]byte("in flight"))
	writeFile(t, filepath.Join(dir, young+chunkSuffix), "in flight")
	stale := filepath.Join(dir, tmpDir, "w-crashed")
	writeFile(t, stale, "half")
	old := time.Now().Add(-2 * sweepGrace)
	for _, p := range []string{
		filepath.Join(dir, pending.Edit.PatchChunk+chunkSuffix),
		filepath.Join(dir, ingested.Edit.OriginalChunk+chunkSuffix),
		stale,
	} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	if err := r.Sweep(); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	checks := []struct {
		name string
		path string
		want bool
	}{
		{"chunk a pending envelope names", pending.Edit.PatchChunk + chunkSuffix, true},
		{"old chunk of an ingested envelope", ingested.Edit.OriginalChunk + chunkSuffix, false},
		{"young unreferenced chunk", young + chunkSuffix, true},
		{"old temp file", filepath.Join(tmpDir, "w-crashed"), false},
	}
	for _, c := range checks {
		if got := exists(filepath.Join(dir, c.path)); got != c.want {
			t.Errorf("%s exists = %v, want %v", c.name, got, c.want)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSweepKeepsEverythingBesideABadEnvelope(t *testing.T) {
	dir := t.TempDir()
	orphan := ChunkKey([]byte("orphan"))
	path := filepath.Join(dir, orphan+chunkSuffix)
	writeFile(t, path, "orphan")
	writeFile(t, filepath.Join(dir, "1-edit-bad.json"), "{not json")
	old := time.Now().Add(-2 * sweepGrace)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := (&Reader{Dir: dir}).Sweep(); err == nil {
		t.Error("Sweep() with an undecodable envelope = nil error, want one")
	}
	if !exists(path) {
		t.Error("Sweep deleted a chunk while an envelope could not be decoded")
	}
}

// TestWatchConcurrentWriter runs a writer and a watching reader in separate
// goroutines: every envelope reaches fn once, with its chunks readable.
func TestWatchConcurrentWriter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	const n = 40
	early, chunks := editEnvelope("early", "early patch")
	mustWrite(t, w, early, chunks)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := map[string]int{}
	var problems []error
	watched := make(chan error, 1)
	go func() {
		watched <- r.Watch(ctx, func(path string) {
			env, err := r.Read(path)
			if err == nil {
				err = readChunks(r, env)
			}
			if err != nil {
				problems = append(problems, err)
			}
			got[env.ID]++
			if err := r.Remove(path); err != nil {
				problems = append(problems, err)
			}
			if len(got) == n+1 {
				cancel()
			}
		})
	}()
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range n {
			env, chunks := editEnvelope(fmt.Sprintf("e%02d", i), fmt.Sprint("patch ", i), "shared")
			if err := w.Write(env, chunks); err != nil {
				t.Errorf("Write: %v", err)
			}
		}
	})
	wg.Wait()
	if err := <-watched; err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if len(got) != n+1 {
		t.Fatalf("Watch delivered %d envelopes before the deadline, want %d", len(got), n+1)
	}
	for id, count := range got {
		if count != 1 {
			t.Errorf("envelope %s delivered %d times, want once", id, count)
		}
	}
	for _, err := range problems {
		t.Error(err)
	}
}

func readChunks(r *Reader, env Envelope) error {
	for _, key := range env.ChunkKeys() {
		if _, err := r.Chunk(key); err != nil {
			return fmt.Errorf("envelope %s: %w", env.ID, err)
		}
	}
	return nil
}

func TestPollDeliversWithoutWatcher(t *testing.T) {
	dir := t.TempDir()
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.poll(ctx, map[string]bool{}, func(path string) {
			got = append(got, filepath.Base(path))
			if len(got) == 2 {
				cancel()
			}
		}, 5*time.Millisecond)
	}()
	for _, id := range []string{"one", "two"} {
		env, chunks := editEnvelope(id, "patch "+id)
		mustWrite(t, w, env, chunks)
	}
	<-done
	if len(got) != 2 {
		t.Errorf("poll delivered %v, want two envelopes", got)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	w, r := &Writer{Dir: dir}, &Reader{Dir: dir}
	same, head := ChunkKey([]byte("same")), ChunkKey([]byte("head"))
	chunks := map[string][]byte{same: []byte("same"), head: []byte("head")}
	env := Envelope{
		Version: Version, Kind: KindSnapshot, ID: "p1:snap:pre:abc", At: at,
		Harness: "claude-code", Session: SessionRef{ID: "s1"}, Turn: TurnRef{ID: "p1"},
		Snapshot: &Snapshot{
			Location: store.Location{Worktree: "/wt"}, Phase: store.SnapshotPre,
			Started: at.Add(-time.Second), Scope: store.SnapshotScope, StateHash: "h",
			Files: []store.SnapshotFile{
				{Path: "a.go", Chunk: same, Size: 4, HeadBlob: "b1", HeadChunk: head},
				{Path: "b.go", Chunk: same, Size: 4},
				{Path: "big.bin", Size: 2 << 20, Reason: store.ReasonTooLarge},
				{Path: "gone.go", Deleted: true, HeadBlob: "b2", HeadChunk: same},
			},
			Skipped: store.SnapshotSkipped{TooLarge: 1},
		},
	}
	mustWrite(t, w, env, chunks)

	paths := mustList(t, r)
	if len(paths) != 1 {
		t.Fatalf("List() = %v, want one envelope", paths)
	}
	got, err := r.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got.Snapshot, env.Snapshot) {
		t.Errorf("Read().Snapshot = %+v, want %+v", got.Snapshot, env.Snapshot)
	}
	if keys := got.ChunkKeys(); !reflect.DeepEqual(keys, []string{same, head}) {
		t.Errorf("ChunkKeys() = %v, want the shared chunk and the HEAD content", keys)
	}

	env.Snapshot = nil
	if err := w.Write(env, nil); err == nil {
		t.Error("Write of a snapshot envelope without its payload = nil, want an error")
	}
}

func TestStored(t *testing.T) {
	objects := filepath.Join(t.TempDir(), "objects")
	stored, fresh := ChunkKey([]byte("stored")), ChunkKey([]byte("fresh"))
	if err := os.MkdirAll(filepath.Join(objects, stored[:2]), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objects, stored[:2], stored+".gz"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		w    *Writer
		key  string
		want bool
	}{
		{"in the store", &Writer{Objects: objects}, stored, true},
		{"not in the store", &Writer{Objects: objects}, fresh, false},
		{"not a key", &Writer{Objects: objects}, "../" + stored, false},
		{"no store", &Writer{}, stored, false},
	}
	for _, tc := range cases {
		if got := tc.w.Stored(tc.key); got != tc.want {
			t.Errorf("%s: Stored = %v, want %v", tc.name, got, tc.want)
		}
	}
}
