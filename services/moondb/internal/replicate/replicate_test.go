package replicate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/replicate"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

const key = "replicate-test-key"

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func openStore(t *testing.T, dir string) *local.Store {
	t.Helper()
	st, err := local.Open(dir)
	if err != nil {
		t.Fatalf("open %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seed writes one of every record kind into st, each pointing at chunks it
// stored, a snapshot's files included. It returns every chunk key.
func seed(t *testing.T, st *local.Store) []string {
	t.Helper()
	ctx := context.Background()
	var keys []string
	put := func(s string) string {
		sha, err := st.Chunks().Put(ctx, []byte(s))
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, sha)
		return sha
	}
	file, head := put("package x\n"), put("package x // head\n")
	files, _ := json.Marshal(
		[]store.SnapshotFile{{Path: "x.go", Chunk: file, HeadChunk: head, Size: 10}},
	)
	loc := store.Location{
		Repo:      "example.com/o/r",
		Worktree:  "/w/r",
		Canonical: "/w/r",
		Branch:    "main",
	}
	writes := []error{
		st.PutSession(ctx, store.Session{ID: "s1", Harness: "claude-code", StartedAt: t0}),
		st.PutTurn(ctx, store.Turn{
			ID: "t1", SessionID: "s1", Status: store.TurnClosed,
			StartedAt: t0, PromptChunk: put("fix it"), TranscriptChunk: put(`{"type":"user"}`),
		}),
		st.PutAgent(ctx, store.Agent{
			ID: "a1", SessionID: "s1", Type: "Explore",
			TurnIDs: []string{"t1"}, TranscriptChunk: put(`{"type":"assistant"}`),
		}),
		st.AppendEdit(ctx, store.Edit{
			ID: "e1", SessionID: "s1", TurnID: "t1", At: t0,
			Location: loc, Path: "x.go", BeforeChunk: put("a\n"), AfterChunk: put("b\n"),
		}),
		st.AppendDelta(ctx, store.UnattributedDelta{
			ID: "d1", SessionID: "s1", TurnID: "t1",
			At: t0, Path: "x.go", FromChunk: put("b\n"), ToChunk: put("c\n"),
		}),
		st.AppendSnapshot(ctx, store.Snapshot{
			ID: "sn1", SessionID: "s1", TurnID: "t1", At: t0,
			Location: loc, MapChunk: put(string(files)), FileCount: 1,
		}),
		st.AppendCommitLink(ctx, store.CommitLink{
			ID: "l1", Repo: loc.Repo, Commit: "abc",
			Path: "x.go", EditID: "e1", TurnID: "t1", SessionID: "s1", At: t0,
		}),
	}
	for _, err := range writes {
		if err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// shared serves dst the way `moondb serve --shared` does, accepting key.
func shared(t *testing.T, dst store.Store) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256([]byte(key))
	keys, err := server.ParseKeys(strings.NewReader(hex.EncodeToString(sum[:])))
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(dst, &query.Service{Store: dst, NoCheckouts: true}, buildinfo.Info{})
	ts := httptest.NewServer(s.SharedHandler(server.SharedConfig{Keys: keys}))
	t.Cleanup(ts.Close)
	return ts
}

func newReplicator(
	t *testing.T,
	dir string,
	src *local.Store,
	target, k, state string,
) *replicate.Replicator {
	t.Helper()
	r, err := replicate.New(replicate.Config{
		Dir: dir, Chunks: src.Chunks(), Target: target, Key: k, State: state, Logf: t.Logf,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// drain runs rounds until one sends nothing and returns the records sent.
func drain(t *testing.T, r *replicate.Replicator) int {
	t.Helper()
	total := 0
	for range 100 {
		n, err := r.Round(context.Background())
		if err != nil {
			t.Fatalf("Round: %v", err)
		}
		if n == 0 {
			return total
		}
		total += n
	}
	t.Fatal("rounds never ran out of records")
	return total
}

// view is the shared answers a reader sees for the seeded session.
func view(t *testing.T, st store.Store) string {
	t.Helper()
	ctx := context.Background()
	s, err := st.Session(ctx, "s1")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	td, err := st.Turn(ctx, "t1")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	raw, _ := json.Marshal([]any{s, td})
	return string(raw)
}

func segment(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(local.SegmentPath(dir, 1))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestReplicateCopiesTheStore sends every record and chunk, so the shared
// store answers as the local one does and holds its log byte for byte.
// Sent again from the start, the log changes no answer.
func TestReplicateCopiesTheStore(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src, dst := openStore(t, srcDir), openStore(t, dstDir)
	chunks := seed(t, src)
	ts := shared(t, dst)
	state := filepath.Join(t.TempDir(), "replicate.json")

	if n := drain(t, newReplicator(t, srcDir, src, ts.URL, key, state)); n != 7 {
		t.Errorf("sent %d records, want 7", n)
	}
	for _, sha := range chunks {
		if ok, err := dst.Chunks().Has(context.Background(), sha); !ok || err != nil {
			t.Errorf("shared store lacks chunk %s: %v", sha, err)
		}
	}
	if got, want := view(t, dst), view(t, src); got != want {
		t.Errorf("shared view differs:\n got %s\nwant %s", got, want)
	}
	if !bytes.Equal(segment(t, dstDir), segment(t, srcDir)) {
		t.Error("shared log differs from the local one")
	}

	if n := drain(t, newReplicator(t, srcDir, src, ts.URL, key, state)); n != 0 {
		t.Errorf("a saved cursor sent %d records again, want 0", n)
	}
	before := view(t, dst)
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	drain(t, newReplicator(t, srcDir, src, ts.URL, key, state))
	if after := view(t, dst); after != before {
		t.Errorf("resending the log changed the shared view:\n got %s\nwant %s", after, before)
	}
}

func TestReplicateNeedsTheKey(t *testing.T) {
	srcDir := t.TempDir()
	src := openStore(t, srcDir)
	seed(t, src)
	ts := shared(t, openStore(t, t.TempDir()))
	r := newReplicator(t, srcDir, src, ts.URL, "wrong", filepath.Join(t.TempDir(), "state"))
	if _, err := r.Round(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "401") {
		t.Errorf("Round with a wrong key = %v, want a 401", err)
	}
}

// TestReplicateSkipsUnknownKinds sends a record of a kind the instance
// does not know, which it skips, and the cursor moves past it.
func TestReplicateSkipsUnknownKinds(t *testing.T) {
	srcDir := t.TempDir()
	src := openStore(t, srcDir)
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(local.SegmentPath(srcDir, 1), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"t":"future","r":{"id":"f1"}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	src = openStore(t, srcDir)
	ts := shared(t, openStore(t, t.TempDir()))
	r := newReplicator(t, srcDir, src, ts.URL, key, filepath.Join(t.TempDir(), "state"))
	if n := drain(t, r); n != 1 {
		t.Errorf("sent %d records, want the 1 unknown one", n)
	}
}

func TestNewRefusesABadTarget(t *testing.T) {
	src := openStore(t, t.TempDir())
	for _, target := range []string{"", "moondb.example", "ftp://moondb.example"} {
		_, err := replicate.New(replicate.Config{
			Dir: "d", Chunks: src.Chunks(), Target: target, Key: key, State: "s",
		})
		if err == nil {
			t.Errorf("New(target %q) succeeded, want an error", target)
		}
	}
}

// TestRunSendsUntilCanceled runs the loop as serve does: it sends the log,
// keeps polling, and returns once its context ends.
func TestRunSendsUntilCanceled(t *testing.T) {
	srcDir := t.TempDir()
	src, dst := openStore(t, srcDir), openStore(t, t.TempDir())
	seed(t, src)
	ts := shared(t, dst)
	r, err := replicate.New(replicate.Config{
		Dir: srcDir, Chunks: src.Chunks(), Target: ts.URL, Key: key,
		State: filepath.Join(t.TempDir(), "state"), Logf: t.Logf, Every: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := dst.Session(context.Background(), "s1"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the loop never sent the session")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}
