package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seqReader is a deterministic, non-repeating byte source. A constant source
// would make every generated id identical and spin the collision retry.
type seqReader struct{ n byte }

func (r *seqReader) Read(p []byte) (int, error) {
	for i := range p {
		r.n++
		p[i] = r.n
	}
	return len(p), nil
}

// newTestStore returns a store in a temp dir with deterministic ids and clock,
// so tests never touch a real workdir.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.rnd = &seqReader{}
	tick := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time {
		tick = tick.Add(time.Second)
		return tick
	}
	return s
}

func mustOpen(t *testing.T, s *Store, in OpenInput) Channel {
	t.Helper()
	c, err := s.Open(in)
	if err != nil {
		t.Fatalf("Open(%+v): %v", in, err)
	}
	return c
}

func mustPost(t *testing.T, s *Store, ref, from, body string) Message {
	t.Helper()
	m, err := s.Post(ref, PostInput{From: from, Body: body})
	if err != nil {
		t.Fatalf("Post(%s): %v", ref, err)
	}
	return m
}

func TestOpenGeneratesNameAndID(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{From: "planner"})

	if !strings.HasPrefix(c.ID, "ch_") {
		t.Errorf("id %q lacks the ch_ prefix", c.ID)
	}
	if _, err := NormalizeName(c.Name); err != nil || c.Name == "" {
		t.Errorf("generated name %q is not a valid handle: %v", c.Name, err)
	}
	if c.OpenedBy != "planner" {
		t.Errorf("opened_by = %q, want planner", c.OpenedBy)
	}
	if c.Closed() {
		t.Error("a fresh channel is closed")
	}
}

func TestOpenRejectsDuplicateName(t *testing.T) {
	s := newTestStore(t)
	mustOpen(t, s, OpenInput{Name: "refactor-auth"})

	_, err := s.Open(OpenInput{Name: "Refactor-Auth"})
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("second open err = %v, want ErrNameTaken (names are case-insensitive)", err)
	}
}

func TestOpenRejectsInvalidName(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"has space", "under_score", "-leading", strings.Repeat("a", 65)} {
		if _, err := s.Open(OpenInput{Name: name}); err == nil {
			t.Errorf("Open(%q) succeeded, want a validation error", name)
		}
	}
}

func TestResolveByIDAndName(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "handoff"})

	for _, ref := range []string{c.ID, "handoff", "HANDOFF"} {
		got, err := s.Resolve(ref)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", ref, err)
		}
		if got.ID != c.ID {
			t.Errorf("Resolve(%q) = %s, want %s", ref, got.ID, c.ID)
		}
	}
	if _, err := s.Resolve("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Resolve(nope) err = %v, want ErrNotFound", err)
	}
}

func TestPostAssignsContiguousSeq(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "seq"})

	for i := int64(1); i <= 3; i++ {
		if m := mustPost(t, s, c.Name, "a", "line"); m.Seq != i {
			t.Errorf("message %d has seq %d", i, m.Seq)
		}
	}
}

func TestPostRequiresFromAndBody(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "guard"})

	if _, err := s.Post(c.Name, PostInput{From: "", Body: "hi"}); err == nil {
		t.Error("post with no from succeeded")
	}
	if _, err := s.Post(c.Name, PostInput{From: "a", Body: "   "}); err == nil {
		t.Error("post with a blank body succeeded")
	}
	big := strings.Repeat("x", maxBodyBytes+1)
	if _, err := s.Post(c.Name, PostInput{From: "a", Body: big}); err == nil {
		t.Error("post over the body limit succeeded")
	}
}

func TestPostValidatesKindAndReplyTo(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "protocol"})
	mustPost(t, s, c.Name, "a", "first")

	if _, err := s.Post(c.Name, PostInput{From: "a", Body: "x", Kind: "banter"}); err == nil {
		t.Error("post with an unknown kind succeeded")
	}
	if _, err := s.Post(c.Name, PostInput{From: "a", Body: "x", ReplyTo: 99}); err == nil {
		t.Error("post replying to a seq the channel does not have succeeded")
	}
	if _, err := s.Post(c.Name, PostInput{From: "a", Body: "x", ReplyTo: -1}); err == nil {
		t.Error("post with a negative reply_to succeeded")
	}

	m, err := s.Post(c.Name, PostInput{
		From: "b", Body: "why?", Kind: "Question", ReplyTo: 1, ReplyNeeded: true,
	})
	if err != nil {
		t.Fatalf("Post with protocol fields: %v", err)
	}
	if m.Kind != "question" || m.ReplyTo != 1 || !m.ReplyNeeded {
		t.Errorf(
			"posted message = %+v, want kind question (lowercased), reply_to 1, reply_needed",
			m,
		)
	}
}

func TestAwaitingReplyTracksOpenQuestions(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "owed"})
	mustPost(t, s, c.Name, "a", "context")
	if _, err := s.Post(c.Name, PostInput{
		From: "a", Body: "which port?", Kind: "question", ReplyNeeded: true,
	}); err != nil {
		t.Fatalf("Post question: %v", err)
	}
	if _, err := s.Post(c.Name, PostInput{
		From: "a", Body: "run the tests", Kind: "task", ReplyNeeded: true,
	}); err != nil {
		t.Fatalf("Post task: %v", err)
	}

	sum, err := s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(sum.AwaitingReply) != 2 || sum.AwaitingReply[0] != 2 || sum.AwaitingReply[1] != 3 {
		t.Fatalf("awaiting_reply = %v, want [2 3]", sum.AwaitingReply)
	}

	// Answering the question settles it; the task stays owed.
	if _, err := s.Post(c.Name, PostInput{
		From: "b", Body: "7432", Kind: "answer", ReplyTo: 2,
	}); err != nil {
		t.Fatalf("Post answer: %v", err)
	}
	b, err := s.Read(c.Name, 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(b.AwaitingReply) != 1 || b.AwaitingReply[0] != 3 {
		t.Fatalf("awaiting_reply after the answer = %v, want [3]", b.AwaitingReply)
	}
}

func TestReadFromCursor(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "cursor"})
	mustPost(t, s, c.Name, "a", "one")
	mustPost(t, s, c.Name, "b", "two")
	mustPost(t, s, c.Name, "a", "three")

	b, err := s.Read(c.Name, 1, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(b.Messages) != 2 || b.Messages[0].Body != "two" || b.Messages[1].Body != "three" {
		t.Fatalf("Read(since=1) = %+v, want the last two messages", b.Messages)
	}
	if b.Cursor != 3 {
		t.Errorf("cursor = %d, want 3", b.Cursor)
	}

	b, err = s.Read(c.Name, 1, 1)
	if err != nil {
		t.Fatalf("Read with limit: %v", err)
	}
	if len(b.Messages) != 1 || b.Messages[0].Body != "two" {
		t.Fatalf("Read(since=1, limit=1) = %+v, want just the second message", b.Messages)
	}
	if b.Cursor != 2 {
		t.Errorf("cursor after a limited read = %d, want 2", b.Cursor)
	}

	b, _ = s.Read(c.Name, 99, 0)
	if len(b.Messages) != 0 {
		t.Errorf("Read past the end returned %d messages", len(b.Messages))
	}
	if b.Cursor != 3 {
		t.Errorf("a cursor past the end stayed at %d, want it clamped to 3", b.Cursor)
	}
}

func TestReadIsIsolatedFromLaterPosts(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "isolation"})
	mustPost(t, s, c.Name, "a", "one")

	b, _ := s.Read(c.Name, 0, 0)
	mustPost(t, s, c.Name, "b", "two")
	if len(b.Messages) != 1 {
		t.Fatalf("a returned slice grew to %d after a later post", len(b.Messages))
	}
}

func TestWaitWakesOnPost(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "wait"})

	done := make(chan []Message, 1)
	go func() {
		b, err := s.Wait(context.Background(), c.Name, 0, 0, 5*time.Second)
		if err != nil {
			t.Errorf("Wait: %v", err)
		}
		done <- b.Messages
	}()

	// The waiter has to be parked before the post for the wake-up to be the
	// thing under test; poll the watcher registry rather than sleeping blind.
	waitForWatcher(t, s, c.ID)
	mustPost(t, s, c.Name, "peer", "reply")

	select {
	case msgs := <-done:
		if len(msgs) != 1 || msgs[0].Body != "reply" {
			t.Fatalf("Wait returned %+v, want the posted message", msgs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after a post")
	}
}

func TestWaitReturnsEmptyOnTimeout(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "timeout"})

	start := time.Now()
	b, err := s.Wait(context.Background(), c.Name, 0, 0, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(b.Messages) != 0 {
		t.Fatalf("Wait returned %d messages on an empty channel", len(b.Messages))
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Error("Wait returned before its timeout")
	}
}

func TestWaitReturnsWhenChannelCloses(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "closing"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := s.Wait(context.Background(), c.Name, 0, 0, 5*time.Second); err != nil {
			t.Errorf("Wait: %v", err)
		}
	}()

	waitForWatcher(t, s, c.ID)
	if _, err := s.Close(c.Name, "done here"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked past the channel closing")
	}
}

func TestWaitStopsOnContextCancel(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "cancel"})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.Wait(ctx, c.Name, 0, 0, time.Minute)
		done <- err
	}()

	waitForWatcher(t, s, c.ID)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait ignored a cancelled context")
	}
}

// waitForWatcher blocks until a waiter has registered on the channel, so a
// test can post without racing the waiter's arrival.
func waitForWatcher(t *testing.T, s *Store, channelID string) {
	t.Helper()
	for range 500 {
		s.mu.Lock()
		_, ok := s.watchers[channelID]
		s.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no waiter registered")
}

func TestCloseIsTerminalAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "terminal"})
	mustPost(t, s, c.Name, "a", "before")

	closed, err := s.Close(c.Name, "shipped")
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed.Closed() || closed.CloseNote != "shipped" {
		t.Fatalf("Close produced %+v", closed)
	}

	if _, err := s.Post(c.Name, PostInput{From: "a", Body: "after"}); !errors.Is(err, ErrClosed) {
		t.Errorf("post to a closed channel err = %v, want ErrClosed", err)
	}

	again, err := s.Close(c.Name, "different note")
	if err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if again.CloseNote != "shipped" {
		t.Errorf("second close overwrote the note with %q", again.CloseNote)
	}

	// A closed channel is still readable — the transcript is the point.
	if b, err := s.Read(c.Name, 0, 0); err != nil || len(b.Messages) != 1 {
		t.Errorf("reading a closed channel gave %d messages, err %v", len(b.Messages), err)
	}
}

func TestListOrdersByActivityAndHidesClosed(t *testing.T) {
	s := newTestStore(t)
	quiet := mustOpen(t, s, OpenInput{Name: "quiet"})
	busy := mustOpen(t, s, OpenInput{Name: "busy"})
	gone := mustOpen(t, s, OpenInput{Name: "gone"})
	mustPost(t, s, busy.Name, "a", "hello there")
	if _, err := s.Close(gone.Name, ""); err != nil {
		t.Fatalf("Close: %v", err)
	}

	open := s.List(false)
	if len(open) != 2 {
		t.Fatalf("List(false) returned %d channels, want 2", len(open))
	}
	if open[0].Name != busy.Name {
		t.Errorf("most recently active channel is %q, want %q", open[0].Name, busy.Name)
	}
	if open[0].Messages != 1 || open[0].Cursor != 1 {
		t.Errorf("busy summary = %+v, want 1 message and cursor 1", open[0])
	}
	if len(open[0].Participants) != 1 || open[0].Participants[0] != "a" {
		t.Errorf("participants = %v, want [a]", open[0].Participants)
	}
	if open[1].Name != quiet.Name {
		t.Errorf("second channel is %q, want %q", open[1].Name, quiet.Name)
	}

	if all := s.List(true); len(all) != 3 {
		t.Errorf("List(true) returned %d channels, want 3", len(all))
	}
}

func TestReloadFromDisk(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := mustOpen(
		t,
		s,
		OpenInput{Name: "persisted", Topic: "handoff", Conventions: "tag your posts"},
	)
	mustPost(t, s, c.Name, "a", "one")
	if _, err := s.Post(c.Name, PostInput{
		From: "b", Body: "two", Kind: "answer", ReplyTo: 1, ReplyNeeded: true,
	}); err != nil {
		t.Fatalf("Post with protocol fields: %v", err)
	}
	if _, err := s.Close(c.Name, "wrapped"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, err := reloaded.Get("persisted")
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.ID != c.ID || got.Topic != "handoff" {
		t.Errorf("reloaded channel = %+v, want id %s and topic handoff", got, c.ID)
	}
	if got.Conventions != "tag your posts" {
		t.Errorf("reloaded conventions = %q, want the opener's ground rules", got.Conventions)
	}
	if !got.Closed() || got.CloseNote != "wrapped" {
		t.Errorf("close state did not survive the reload: %+v", got)
	}
	if got.Messages != 2 {
		t.Fatalf("reloaded %d messages, want 2", got.Messages)
	}
	b, err := reloaded.Read("persisted", 0, 0)
	if err != nil {
		t.Fatalf("Read after reload: %v", err)
	}
	if b.Messages[0].Body != "one" || b.Messages[1].Seq != 2 {
		t.Errorf("reloaded messages out of order: %+v", b.Messages)
	}
	second := b.Messages[1]
	if second.Kind != "answer" || second.ReplyTo != 1 || !second.ReplyNeeded {
		t.Errorf("protocol fields did not survive the reload: %+v", second)
	}
}

func TestLoadDropsTornFinalLine(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := mustOpen(t, s, OpenInput{Name: "torn"})
	mustPost(t, s, c.Name, "a", "complete")

	// Simulate an append interrupted mid-write: a partial record with no
	// trailing newline, the crash artifact the loader has to survive.
	path := filepath.Join(dir, messagesDirName, messagesFileName(c.ID))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open message log: %v", err)
	}
	if _, err := f.WriteString(`{"channel_id":"` + c.ID + `","seq":2,"fr`); err != nil {
		t.Fatalf("write torn line: %v", err)
	}
	_ = f.Close()

	var warnings []string
	restore := logf
	logf = func(format string, args ...any) { warnings = append(warnings, format) }
	defer func() { logf = restore }()

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reload with a torn line: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("got %d warnings, want 1 about the torn line", len(warnings))
	}
	got, err := reloaded.Get("torn")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Messages != 1 {
		t.Fatalf("kept %d messages, want only the complete one", got.Messages)
	}

	// The torn bytes are truncated away, so the next append starts a clean line.
	mustPost(t, reloaded, "torn", "b", "after recovery")
	third, err := New(dir)
	if err != nil {
		t.Fatalf("second reload: %v", err)
	}
	again, err := third.Get("torn")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if again.Messages != 2 {
		t.Fatalf("after recovery there are %d messages, want 2", again.Messages)
	}
}

func TestPreviewCollapsesAndClips(t *testing.T) {
	if got := preview("one\n  two   three "); got != "one two three" {
		t.Errorf("preview collapsed to %q", got)
	}
	long := strings.Repeat("é", previewRunes+10)
	got := preview(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a clipped preview lost its ellipsis: %q", got)
	}
	if n := len([]rune(got)); n != previewRunes+1 {
		t.Errorf("clipped preview is %d runes, want %d", n, previewRunes+1)
	}
}

func TestJoinAndLeaveDriveTheRoster(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "swarm", From: "planner"})

	sum, err := s.Join(c.Name, "worker", "ready for tasks")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if got := sum.Members; len(got) != 2 || got[0] != "planner" || got[1] != "worker" {
		t.Errorf("members after join = %v, want [planner worker]", got)
	}
	if sum.Messages != 1 || sum.LastBody != "ready for tasks" {
		t.Errorf("join summary = %+v, want the join message with its note as body", sum)
	}

	// Joining again says nothing new: no duplicate message, same roster.
	again, err := s.Join(c.Name, "worker", "")
	if err != nil {
		t.Fatalf("Join twice: %v", err)
	}
	if again.Messages != 1 || len(again.Members) != 2 {
		t.Errorf("second join = %+v, want it idempotent", again)
	}

	left, err := s.Leave(c.Name, "worker", "done here")
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if got := left.Members; len(got) != 1 || got[0] != "planner" {
		t.Errorf("members after leave = %v, want [planner]", got)
	}
	if left.Messages != 2 {
		t.Errorf("leave summary has %d messages, want the leave recorded", left.Messages)
	}

	// Leaving when already gone is a no-op, and rejoining works.
	if sum, err = s.Leave(c.Name, "worker", ""); err != nil || sum.Messages != 2 {
		t.Errorf("second leave = %+v, %v; want it idempotent", sum, err)
	}
	if sum, err = s.Join(c.Name, "worker", ""); err != nil || len(sum.Members) != 2 {
		t.Errorf("rejoin = %+v, %v; want worker back on the roster", sum, err)
	}
}

func TestJoinRejectsClosedChannel(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "done", From: "planner"})
	if _, err := s.Close(c.Name, ""); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.Join(c.Name, "late", ""); !errors.Is(err, ErrClosed) {
		t.Errorf("Join on closed = %v, want ErrClosed", err)
	}
}

func TestAwaitingReplyByGroupsByAddressee(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "swarm", From: "planner"})

	q1, err := s.Post(c.Name, PostInput{
		From: "planner", To: "worker", Body: "status?", Kind: "question", ReplyNeeded: true,
	})
	if err != nil {
		t.Fatalf("Post addressed question: %v", err)
	}
	q2, err := s.Post(c.Name, PostInput{
		From: "planner", Body: "anyone seen the logs?", Kind: "question", ReplyNeeded: true,
	})
	if err != nil {
		t.Fatalf("Post broadcast question: %v", err)
	}

	sum, err := s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(sum.AwaitingReply) != 2 {
		t.Errorf("awaiting_reply = %v, want both questions open", sum.AwaitingReply)
	}
	if got := sum.AwaitingReplyBy["worker"]; len(got) != 1 || got[0] != q1.Seq {
		t.Errorf("awaiting_reply_by[worker] = %v, want [%d]", got, q1.Seq)
	}
	if _, ok := sum.AwaitingReplyBy[""]; ok {
		t.Error("the broadcast question was grouped under an empty addressee")
	}

	// Answering the addressed question clears it from the map; the broadcast
	// one stays in the flat list.
	if _, err := s.Post(c.Name, PostInput{
		From: "worker", Body: "on it", Kind: "answer", ReplyTo: q1.Seq,
	}); err != nil {
		t.Fatalf("Post answer: %v", err)
	}
	sum, err = s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get after answer: %v", err)
	}
	if sum.AwaitingReplyBy != nil {
		t.Errorf(
			"awaiting_reply_by = %v, want it empty once the addressed debt settles",
			sum.AwaitingReplyBy,
		)
	}
	if len(sum.AwaitingReply) != 1 || sum.AwaitingReply[0] != q2.Seq {
		t.Errorf("awaiting_reply = %v, want just the broadcast question", sum.AwaitingReply)
	}
}

func TestAwaitingReplyOffRosterFlagsUnknownAddressees(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "swarm", From: "planner"})
	if _, err := s.Join(c.Name, "worker", ""); err != nil {
		t.Fatalf("Join: %v", err)
	}

	q1, err := s.Post(c.Name, PostInput{
		From: "planner", To: "worker", Body: "status?", Kind: "question", ReplyNeeded: true,
	})
	if err != nil {
		t.Fatalf("Post to a member: %v", err)
	}
	q2, err := s.Post(c.Name, PostInput{
		From: "planner", To: "quil", Body: "and you?", Kind: "question", ReplyNeeded: true,
	})
	if err != nil {
		t.Fatalf("Post to a name not on the roster: %v", err)
	}

	sum, err := s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := sum.AwaitingReplyOffRoster; len(got) != 1 || got[0] != "quil" {
		t.Errorf("off-roster = %v, want [quil]: the member's debt must not be flagged", got)
	}

	// Leaving strands the member's obligation under a departed name.
	if _, err := s.Leave(c.Name, "worker", ""); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	sum, err = s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get after leave: %v", err)
	}
	if got := sum.AwaitingReplyOffRoster; len(got) != 2 || got[0] != "quil" || got[1] != "worker" {
		t.Errorf("off-roster after leave = %v, want [quil worker]", got)
	}

	// A pre-join handoff resolves itself when the addressee joins.
	if _, err := s.Join(c.Name, "worker", ""); err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	// Answering settles the typo'd obligation like any other.
	if _, err := s.Post(c.Name, PostInput{
		From: "quill", Body: "fine", Kind: "answer", ReplyTo: q2.Seq,
	}); err != nil {
		t.Fatalf("Post answer: %v", err)
	}
	sum, err = s.Get(c.Name)
	if err != nil {
		t.Fatalf("Get after rejoin and answer: %v", err)
	}
	if sum.AwaitingReplyOffRoster != nil {
		t.Errorf("off-roster = %v, want nil once every addressee is back on the roster",
			sum.AwaitingReplyOffRoster)
	}
	if got := sum.AwaitingReplyBy["worker"]; len(got) != 1 || got[0] != q1.Seq {
		t.Errorf("awaiting_reply_by[worker] = %v, want [%d] still open", got, q1.Seq)
	}

	// The read batch carries the same signal.
	if _, err := s.Leave(c.Name, "worker", ""); err != nil {
		t.Fatalf("Leave again: %v", err)
	}
	b, err := s.Read(c.Name, 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := b.AwaitingReplyOffRoster; len(got) != 1 || got[0] != "worker" {
		t.Errorf("batch off-roster = %v, want [worker]", got)
	}
}

func TestPostBoundsTo(t *testing.T) {
	s := newTestStore(t)
	c := mustOpen(t, s, OpenInput{Name: "swarm", From: "planner"})
	long := strings.Repeat("x", 65)
	if _, err := s.Post(c.Name, PostInput{From: "a", To: long, Body: "hi"}); err == nil {
		t.Error("a 65-rune to was accepted")
	}
	if _, err := s.Post(c.Name, PostInput{From: "a", To: "  worker  ", Body: "hi"}); err != nil {
		t.Errorf("a padded to was rejected: %v", err)
	}
}
