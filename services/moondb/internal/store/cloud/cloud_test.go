package cloud

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
	"github.com/mad01/thismoon/services/moondb/internal/store/storetest"
)

// TestConformance runs the store contract against the cloud backend, with
// the bucket faked over httptest. Each data directory gets its own bucket,
// and a store reopened on a directory reaches the bucket it had.
func TestConformance(t *testing.T) {
	var mu sync.Mutex
	buckets := map[string]*Bucket{}
	storetest.Run(t, func(t *testing.T, dir string) store.Store {
		t.Helper()
		mu.Lock()
		b, ok := buckets[dir]
		if !ok {
			b = newFakeGCS("moondb-test").serve(t)
			buckets[dir] = b
		}
		mu.Unlock()
		return mustOpen(t, dir, b)
	})
}

func mustOpen(t *testing.T, dir string, b *Bucket) *Store {
	t.Helper()
	s, err := Open(
		Config{Dir: dir, Bucket: b, Logf: t.Logf, PollEvery: time.Hour, CopyEvery: time.Hour},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestChunkPutCreatesOnlyWhenAbsent(t *testing.T) {
	ctx := context.Background()
	f := newFakeGCS("b")
	s := mustOpen(t, t.TempDir(), f.serve(t))
	t.Cleanup(func() { _ = s.Close() })
	data := []byte("kept once")
	sha := store.ChunkKey(data)
	held, err := gzipBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	f.put(chunkPrefix+sha, held)

	for range 2 {
		got, err := s.Chunks().Put(ctx, data)
		if err != nil || got != sha {
			t.Fatalf("Put = %q, %v; want %q", got, err, sha)
		}
	}
	if n := f.callsTo("POST " + chunkPrefix); n != 1 {
		t.Errorf("uploads = %d, want 1: the 412 makes the key known", n)
	}
	if ok, err := s.Chunks().Has(ctx, sha); !ok || err != nil {
		t.Errorf("Has = %v, %v; want true", ok, err)
	}
	if n := f.callsTo("GET " + chunkPrefix); n != 0 {
		t.Errorf("metadata reads = %d, want 0 for a known key", n)
	}
}

func TestChunkGetDetectsCorruption(t *testing.T) {
	f := newFakeGCS("b")
	s := mustOpen(t, t.TempDir(), f.serve(t))
	t.Cleanup(func() { _ = s.Close() })
	sha := store.ChunkKey([]byte("wanted"))
	other, err := gzipBytes([]byte("something else"))
	if err != nil {
		t.Fatal(err)
	}
	f.put(chunkPrefix+sha, other)

	if _, err := s.Chunks().Get(context.Background(), sha); !errors.Is(err, errCorruptChunk) {
		t.Errorf("Get = %v, want errCorruptChunk", err)
	}
}

func TestBucketRetries(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		statuses []int
		wantErr  bool
	}{
		{"recovers after 503 and 429", []int{503, 429}, false},
		{"gives up after maxAttempts", []int{503, 503, 503, 503}, true},
		{"does not retry 403", []int{403}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeGCS("b")
			b := f.serve(t)
			f.failWith(c.statuses...)
			err := b.create(ctx, upload{object: "o", size: 1, open: bytesBody([]byte("x"))})
			if (err != nil) != c.wantErr {
				t.Fatalf("create = %v, wantErr %v", err, c.wantErr)
			}
			if _, ok := f.object("o"); ok == c.wantErr {
				t.Errorf("object stored = %v, want %v", ok, !c.wantErr)
			}
		})
	}
}

func TestPingNamesAMissingBucket(t *testing.T) {
	f := newFakeGCS("other")
	b := f.serve(t)
	b.name = "missing"
	s := mustOpen(t, t.TempDir(), b)
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Ping(context.Background()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Ping = %v, want a not-found bucket", err)
	}
}

// seedSegments writes a closed first segment holding one edit and an empty
// second one, the state of a log that has rolled once.
func seedSegments(t *testing.T, dir string) {
	t.Helper()
	logDir := filepath.Join(dir, "log")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"t":"edit","r":{"id":"e1","session_id":"s1","turn_id":"t1",` +
		`"at":"2026-10-01T09:00:00Z","path":"x.go"}}` + "\n"
	for name, data := range map[string]string{"000001.jsonl": line, "000002.jsonl": ""} {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// waitFor polls until ok holds, failing t after a few seconds.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestLogCopyRestores checks the copy of the log end to end: a closed
// segment is copied at start, the open one on Close, a restart sends
// neither again, and a store opened from the bucket's copy holds every
// record.
func TestLogCopyRestores(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seedSegments(t, dir)
	f := newFakeGCS("b")
	b := f.serve(t)

	s := mustOpen(t, dir, b)
	waitFor(t, "the closed segment in the bucket", func() bool {
		_, ok := f.object("log/000001.jsonl")
		return ok
	})
	edit := store.Edit{ID: "e2", SessionID: "s1", TurnID: "t1", At: time.Now().UTC(), Path: "y.go"}
	if err := s.AppendEdit(ctx, edit); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := mustOpen(t, dir, b).Close(); err != nil {
		t.Fatalf("Close after reopen: %v", err)
	}
	if n := f.callsTo("POST log/"); n != 2 {
		t.Errorf("segment uploads = %d, want 2: one per segment, none after the restart", n)
	}

	restored := t.TempDir()
	for _, name := range []string{"000001.jsonl", "000002.jsonl"} {
		data, _ := f.object("log/" + name)
		if err := os.MkdirAll(filepath.Join(restored, "log"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(restored, "log", name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ls, err := local.Open(restored)
	if err != nil {
		t.Fatalf("open the restored log: %v", err)
	}
	t.Cleanup(func() { _ = ls.Close() })
	edits, err := ls.Edits(ctx, store.EditQuery{SessionID: "s1"})
	if err != nil || len(edits) != 2 {
		t.Errorf("restored edits = %d, %v; want 2", len(edits), err)
	}
}

func TestReadMapsStatus(t *testing.T) {
	f := newFakeGCS("b")
	b := f.serve(t)
	if _, err := b.read(context.Background(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("read of a missing object = %v, want ErrNotFound", err)
	}
	f.failWith(http.StatusForbidden)
	if _, err := b.read(context.Background(), "nope"); err == nil ||
		errors.Is(err, store.ErrNotFound) {
		t.Errorf("read on 403 = %v, want a plain error", err)
	}
}
