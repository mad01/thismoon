package ingest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/ingest/faketest"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const (
	repo     = "github.com/o/r"
	worktree = "/wt/a"
)

var t0 = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func at(minute int) time.Time { return t0.Add(time.Duration(minute) * time.Minute) }

// fixture is an Ingester over a fake store and a real spool directory.
type fixture struct {
	t *testing.T
	// st is the store under test; fake is the same store when it is the
	// in-memory one, nil otherwise.
	st   store.Store
	fake *faketest.Store
	w    *spool.Writer
	in   *Ingester
	now  time.Time

	mu   sync.Mutex
	logs []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spool")
	fake := faketest.New()
	f := &fixture{t: t, st: fake, fake: fake, w: &spool.Writer{Dir: dir}, now: at(60)}
	f.in = &Ingester{
		Store: fake,
		Spool: &spool.Reader{Dir: dir},
		Now:   func() time.Time { return f.now },
		Logf: func(format string, args ...any) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.logs = append(f.logs, fmt.Sprintf(format, args...))
		},
	}
	return f
}

// write puts env and its chunks into the spool and returns its path.
func (f *fixture) write(env spool.Envelope, c chunkSet) string {
	f.t.Helper()
	if err := f.w.Write(env, c); err != nil {
		f.t.Fatalf("write %s: %v", env.ID, err)
	}
	return f.pathOf(env.ID)
}

func (f *fixture) pathOf(id string) string {
	f.t.Helper()
	paths, err := f.in.Spool.List()
	if err != nil {
		f.t.Fatal(err)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "-"+strings.ReplaceAll(id, ":", "_")+".json") {
			return p
		}
	}
	return ""
}

func (f *fixture) drain() {
	f.t.Helper()
	r := &runner{in: f.in, attempts: map[string]int{}, retryAt: map[string]time.Time{}}
	r.drain(context.Background())
}

func (f *fixture) pending() int {
	f.t.Helper()
	paths, err := f.in.Spool.List()
	if err != nil {
		f.t.Fatal(err)
	}
	return len(paths)
}

func (f *fixture) logged(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func (f *fixture) turn(id string) store.TurnDetail {
	f.t.Helper()
	td, err := f.st.Turn(context.Background(), id)
	if err != nil {
		f.t.Fatalf("turn %s: %v", id, err)
	}
	return td
}

func (f *fixture) session(id string) store.Session {
	f.t.Helper()
	sd, err := f.st.Session(context.Background(), id)
	if err != nil {
		f.t.Fatalf("session %s: %v", id, err)
	}
	return sd.Session
}

// chunkSet collects the chunks an envelope points at.
type chunkSet map[string][]byte

func (c chunkSet) add(s string) string {
	key := spool.ChunkKey([]byte(s))
	c[key] = []byte(s)
	return key
}

func base(kind spool.Kind, id, session, turn string, minute int) spool.Envelope {
	return spool.Envelope{
		Version: spool.Version,
		Kind:    kind,
		ID:      id,
		At:      at(minute),
		Harness: "claude-code",
		Session: spool.SessionRef{ID: session, Cwd: worktree, Title: "title " + session},
		Turn:    spool.TurnRef{ID: turn, TranscriptOffset: 10},
	}
}

func openEnv(c chunkSet, session, turn string, minute int, prompt string) spool.Envelope {
	env := base(spool.KindTurnOpen, turn+":open", session, turn, minute)
	env.TurnOpen = &spool.TurnOpen{PromptChunk: c.add(prompt)}
	return env
}

func closeEnv(c chunkSet, session, turn string, minute int, final string) spool.Envelope {
	env := base(spool.KindTurnClose, turn+":close", session, turn, minute)
	env.TurnClose = &spool.TurnClose{
		FinalChunk:      c.add(final),
		TranscriptChunk: c.add("{\"line\":1}\n"),
		Status:          store.TurnClosed,
	}
	return env
}

// editEnv is a Write that created path with content; observed is the disk
// read, "" for none.
func editEnv(
	c chunkSet,
	session, turn, toolUseID string,
	minute int,
	path, content, observed string,
) spool.Envelope {
	env := base(spool.KindEdit, toolUseID+":0", session, turn, minute)
	env.Edit = &spool.EditPayload{
		Tool:      "Write",
		ToolUseID: toolUseID,
		AbsPath:   worktree + "/" + path,
		Path:      path,
		Location: store.Location{
			Repo: repo, Worktree: worktree, Canonical: worktree, Branch: "main", Head: "h0",
		},
		Kind:         store.EditCreate,
		ContentChunk: c.add(content),
	}
	if observed != "" {
		env.Edit.ObservedChunk = c.add(observed)
	}
	return env
}

func headEnv(session, turn string, minute int, files ...spool.ChangedFile) spool.Envelope {
	env := base(spool.KindHeadMoved, turn+":head:new", session, turn, minute)
	env.HeadMoved = &spool.HeadMoved{
		Location: store.Location{Repo: repo, Worktree: worktree, Branch: "feat", Head: "new"},
		OldHead:  "old",
		NewHead:  "new",
		Files:    files,
	}
	return env
}

func TestTurnOpenThenClose(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "fix the bug"), c)
	f.write(closeEnv(c, "s1", "p1", 5, "fixed"), c)
	f.drain()

	if n := f.pending(); n != 0 {
		t.Errorf("%d envelopes left in the spool, want 0", n)
	}
	s := f.session("s1")
	if s.Harness != "claude-code" || s.Cwd != worktree || !s.StartedAt.Equal(at(1)) {
		t.Errorf("session = %+v", s)
	}
	tr := f.turn("p1").Turn
	want := store.Turn{
		ID: "p1", SessionID: "s1", Seq: 1, Status: store.TurnClosed, TranscriptOffset: 10,
		PromptChunk:     spool.ChunkKey([]byte("fix the bug")),
		FinalChunk:      spool.ChunkKey([]byte("fixed")),
		TranscriptChunk: spool.ChunkKey([]byte("{\"line\":1}\n")),
	}
	if tr.EndedAt == nil || !tr.EndedAt.Equal(at(5)) || !tr.StartedAt.Equal(at(1)) {
		t.Errorf("turn times = %v .. %v, want %v .. %v", tr.StartedAt, tr.EndedAt, at(1), at(5))
	}
	tr.StartedAt, tr.EndedAt = time.Time{}, nil
	if tr != want {
		t.Errorf("turn = %+v\nwant %+v", tr, want)
	}
	for _, key := range []string{want.PromptChunk, want.FinalChunk, want.TranscriptChunk} {
		if ok, _ := f.st.Chunks().Has(context.Background(), key); !ok {
			t.Errorf("chunk %s not copied into the store", key)
		}
	}
}

func TestCloseWithoutOpenCreatesTurnAndSession(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(closeEnv(c, "s1", "p1", 5, "done"), c)
	f.drain()

	tr := f.turn("p1").Turn
	if tr.Status != store.TurnClosed || !tr.StartedAt.Equal(at(5)) || tr.SessionID != "s1" {
		t.Errorf("turn = %+v, want closed, started at the close", tr)
	}
	if s := f.session("s1"); !s.StartedAt.Equal(at(5)) {
		t.Errorf("session started %v, want %v", s.StartedAt, at(5))
	}
}

func TestEditBeforeOpen(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "package a\n"), c)
	f.drain()

	placeholder := f.turn("p1")
	if placeholder.Status != store.TurnOpen || len(placeholder.Edits) != 1 {
		t.Fatalf("placeholder = %+v with %d edits, want open with 1", placeholder.Turn,
			len(placeholder.Edits))
	}
	f.write(openEnv(c, "s1", "p1", 1, "add package a"), c)
	f.drain()

	tr := f.turn("p1").Turn
	if tr.PromptChunk != spool.ChunkKey([]byte("add package a")) || !tr.StartedAt.Equal(at(1)) {
		t.Errorf("turn after open = %+v, want the prompt and StartedAt %v", tr, at(1))
	}
	if s := f.session("s1"); !s.StartedAt.Equal(at(1)) {
		t.Errorf("session started %v, want %v", s.StartedAt, at(1))
	}
	e := placeholder.Edits[0]
	if e.ID != "tu1:0" || e.AfterBlob != gitlink.BlobID([]byte("package a\n")) {
		t.Errorf("edit = %s with blob %s", e.ID, e.AfterBlob)
	}
}

func TestEditDeltaAmbiguity(t *testing.T) {
	tests := []struct {
		name          string
		otherSession  bool
		wantAmbiguous bool
	}{
		{"alone in the worktree", false, false},
		{"another session has an open turn there", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			c := chunkSet{}
			if tc.otherSession {
				f.write(openEnv(c, "s2", "q1", 0, "other work"), c)
				f.write(editEnv(c, "s2", "q1", "tu0", 0, "b.go", "package b\n", ""), c)
			}
			f.write(openEnv(c, "s1", "p1", 1, "add a"), c)
			f.write(editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "package  a\n"), c)
			f.drain()

			deltas := f.turn("p1").Deltas
			if len(deltas) != 1 {
				t.Fatalf("got %d deltas, want 1", len(deltas))
			}
			if d := deltas[0]; d.Cause != store.DeltaObservedMismatch ||
				d.Ambiguous != tc.wantAmbiguous {
				t.Errorf("delta = %+v, want observed-mismatch ambiguous=%v", d, tc.wantAmbiguous)
			}
		})
	}
}

func TestHeadMovedLinksCommit(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "add a"), c)
	f.write(editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", ""), c)
	f.write(headEnv("s1", "p1", 3,
		spool.ChangedFile{Path: "a.go", Blob: gitlink.BlobID([]byte("package a\n")), Status: "A"},
		spool.ChangedFile{Path: "b.go", Blob: gitlink.BlobID([]byte("unrecorded")), Status: "A"},
		spool.ChangedFile{Path: "c.go", Status: "D"},
	), nil)
	f.drain()

	links := f.turn("p1").CommitLinks
	want := store.CommitLink{
		ID: "new:a.go", Repo: repo, Worktree: worktree, Branch: "feat", Commit: "new",
		Parent: "old", Path: "a.go", Blob: gitlink.BlobID([]byte("package a\n")),
		EditID: "tu1:0", TurnID: "p1", SessionID: "s1", At: at(3),
	}
	if len(links) != 1 || links[0] != want {
		t.Errorf("commit links = %+v\nwant [%+v]", links, want)
	}
}

func TestReplayIsNoOp(t *testing.T) {
	c := chunkSet{}
	envs := []spool.Envelope{
		openEnv(c, "s1", "p1", 1, "prompt"),
		editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "package  a\n"),
		closeEnv(c, "s1", "p1", 3, "final"),
		headEnv("s1", "p1", 4, spool.ChangedFile{
			Path: "a.go", Blob: gitlink.BlobID([]byte("package a\n")), Status: "A",
		}),
	}
	f := newFixture(t)
	for _, env := range envs {
		f.write(env, c)
	}
	f.drain()
	before := f.fake.Writes()
	for _, env := range envs {
		f.write(env, c)
		if err := f.in.Ingest(context.Background(), env); err != nil {
			t.Fatalf("replay %s: %v", env.ID, err)
		}
		if got := f.fake.Writes(); got != before {
			t.Errorf("replaying %s wrote %d records, want none", env.ID, got-before)
		}
	}
}

func TestEditChunkMissingKeepsEnvelope(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := editEnv(c, "s1", "p1", "tu1", 2, "a.go", "package a\n", "")
	path := f.write(env, c)
	if err := os.Remove(filepath.Join(f.w.Dir, env.Edit.ContentChunk+".chunk")); err != nil {
		t.Fatal(err)
	}

	if err := f.in.Ingest(context.Background(), env); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Ingest = %v, want a missing-chunk error", err)
	}
	f.drain()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("envelope gone after a failed ingest: %v", err)
	}
	if edits, _ := f.st.Edits(context.Background(), store.EditQuery{}); len(edits) != 0 {
		t.Errorf("recorded %d edits without their content", len(edits))
	}
	if !f.logged("attempt 1 of 3") {
		t.Errorf("failure not logged: %q", f.logs)
	}
}

func TestTurnChunkMissingWarns(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := openEnv(c, "s1", "p1", 1, "lost prompt")
	f.write(env, c)
	if err := os.Remove(filepath.Join(f.w.Dir, env.TurnOpen.PromptChunk+".chunk")); err != nil {
		t.Fatal(err)
	}
	f.drain()

	if tr := f.turn("p1").Turn; tr.PromptChunk != "" || tr.Status != store.TurnOpen {
		t.Errorf("turn = %+v, want open without a prompt chunk", tr)
	}
	if f.pending() != 0 || !f.logged("recording without it") {
		t.Errorf("pending %d, logs %q; want the envelope ingested with a warning", f.pending(),
			f.logs)
	}
}

func TestPoisonEnvelopeMovesAside(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "good"), c)
	poison := filepath.Join(f.w.Dir, "00000000000000000001-edit-bad.json")
	if err := os.WriteFile(poison, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &runner{in: f.in, attempts: map[string]int{}, retryAt: map[string]time.Time{}}
	ctx := context.Background()

	r.drain(ctx)
	f.turn("p1") // the good envelope behind the poison one went in
	r.drain(ctx) // within the retry delay: not attempted again
	if r.attempts[poison] != 1 {
		t.Fatalf("attempts after two drains inside the delay = %d, want 1", r.attempts[poison])
	}
	for range maxAttempts - 1 {
		f.now = f.now.Add(retryDelay)
		r.drain(ctx)
	}

	moved := filepath.Join(f.w.Dir, failedDir, filepath.Base(poison))
	if _, err := os.Stat(moved); err != nil {
		t.Errorf("poison envelope not in failed/: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(moved)); err != nil || info.Mode().Perm() != failedPerm {
		t.Errorf("failed/ = %v, %v; want mode %o", info, err, failedPerm)
	}
	if f.pending() != 0 || len(r.attempts) != 0 {
		t.Errorf("pending %d, attempts %v; want both empty", f.pending(), r.attempts)
	}
}

// downStore is a store that is unreachable: every write and Ping fail.
type downStore struct{ *faketest.Store }

var errDown = errors.New("store down")

func (downStore) PutSession(context.Context, store.Session) error { return errDown }
func (downStore) Ping(context.Context) error                      { return errDown }

func TestStoreDownCountsNoFailures(t *testing.T) {
	f := newFixture(t)
	f.in.Store = downStore{f.fake}
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "prompt"), c)
	r := &runner{in: f.in, attempts: map[string]int{}, retryAt: map[string]time.Time{}}
	for range maxAttempts + 1 {
		r.drain(context.Background())
		f.now = f.now.Add(retryDelay)
	}
	if f.pending() != 1 || len(r.attempts) != 0 {
		t.Errorf("pending %d, attempts %v; want the envelope kept and uncounted", f.pending(),
			r.attempts)
	}
	if !f.logged("store unavailable") {
		t.Errorf("logs = %q, want a store-unavailable line", f.logs)
	}
}

func TestRunDrainsThenWatches(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "before start"), c)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.in.Run(ctx) }()

	waitFor(t, "the envelope present at start", func() bool { return hasTurn(f.st, "p1") })
	f.write(openEnv(c, "s2", "p2", 2, "after start"), c)
	waitFor(t, "the envelope written after start", func() bool { return hasTurn(f.st, "p2") })
	waitFor(t, "the spool to empty", func() bool { return f.pending() == 0 })

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func hasTurn(st store.Store, id string) bool {
	_, err := st.Turn(context.Background(), id)
	return err == nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
