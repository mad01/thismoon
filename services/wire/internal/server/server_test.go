package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/wire/internal/store"
)

// newTestServer returns a live server over a temp workdir, so tests never
// touch a real store.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	srv := httptest.NewServer(New(st, "test", 7432).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// do issues a request and decodes the JSON response, failing the test on a
// transport error. The status is returned so callers can assert on it.
func do(t *testing.T, srv *httptest.Server, method, path string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
	return res.StatusCode
}

// apiChannel mirrors what the API returns for a channel: the store summary
// plus the connection string the server stamps on.
type apiChannel struct {
	store.Summary
	Connect string `json:"connect"`
}

func openChannel(t *testing.T, srv *httptest.Server, name string) apiChannel {
	t.Helper()
	var sum apiChannel
	code := do(t, srv, http.MethodPost, "/api/channels",
		map[string]string{"name": name, "from": "planner"}, &sum)
	if code != http.StatusCreated {
		t.Fatalf("open %q returned %d", name, code)
	}
	return sum
}

func TestOpenPostRead(t *testing.T) {
	srv := newTestServer(t)
	sum := openChannel(t, srv, "handoff")
	if sum.ID == "" || sum.Name != "handoff" || sum.Messages != 0 {
		t.Fatalf("opened channel = %+v", sum)
	}
	// The connection string is the whole handoff, so it has to come back from
	// the call that creates the channel.
	if sum.Connect != "wire://localhost:7432/handoff" {
		t.Errorf("connect = %q", sum.Connect)
	}

	var m store.Message
	code := do(t, srv, http.MethodPost, "/api/channels/handoff/messages",
		map[string]string{"from": "planner", "body": "schema is done, your turn"}, &m)
	if code != http.StatusCreated || m.Seq != 1 {
		t.Fatalf("post returned %d, message %+v", code, m)
	}

	var batch store.Batch
	if code := do(t, srv, http.MethodGet, "/api/channels/handoff/messages", nil, &batch); code != 200 {
		t.Fatalf("read returned %d", code)
	}
	if len(batch.Messages) != 1 || batch.Cursor != 1 {
		t.Fatalf("read = %+v, want one message and cursor 1", batch)
	}

	// The same channel resolves by id as well as by name.
	if code := do(t, srv, http.MethodGet, "/api/channels/"+sum.ID, nil, &apiChannel{}); code != 200 {
		t.Errorf("get by id returned %d", code)
	}
}

func TestProtocolFieldsRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	var sum apiChannel
	code := do(t, srv, http.MethodPost, "/api/channels",
		map[string]string{"name": "typed", "from": "a", "conventions": "answer questions first"},
		&sum)
	if code != http.StatusCreated || sum.Conventions != "answer questions first" {
		t.Fatalf("open with conventions returned %d, channel %+v", code, sum)
	}

	var q store.Message
	code = do(t, srv, http.MethodPost, "/api/channels/typed/messages",
		map[string]any{
			"from": "a", "body": "which port?", "kind": "question", "reply_needed": true,
		}, &q)
	if code != http.StatusCreated || q.Kind != "question" || !q.ReplyNeeded {
		t.Fatalf("post question returned %d, message %+v", code, q)
	}

	var batch store.Batch
	do(t, srv, http.MethodGet, "/api/channels/typed/messages", nil, &batch)
	if len(batch.AwaitingReply) != 1 || batch.AwaitingReply[0] != q.Seq {
		t.Fatalf("awaiting_reply = %v, want the unanswered question", batch.AwaitingReply)
	}

	var a store.Message
	code = do(t, srv, http.MethodPost, "/api/channels/typed/messages",
		map[string]any{"from": "b", "body": "7432", "kind": "answer", "reply_to": q.Seq}, &a)
	if code != http.StatusCreated || a.ReplyTo != q.Seq {
		t.Fatalf("post answer returned %d, message %+v", code, a)
	}
	var after store.Batch
	do(t, srv, http.MethodGet, "/api/channels/typed/messages", nil, &after)
	if len(after.AwaitingReply) != 0 {
		t.Fatalf("awaiting_reply after the answer = %v, want none", after.AwaitingReply)
	}

	code = do(t, srv, http.MethodPost, "/api/channels/typed/messages",
		map[string]any{"from": "a", "body": "x", "kind": "banter"}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("unknown kind returned %d, want 400", code)
	}
	code = do(t, srv, http.MethodPost, "/api/channels/typed/messages",
		map[string]any{"from": "a", "body": "x", "reply_to": 99}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("reply_to a missing seq returned %d, want 400", code)
	}
}

func TestReadFromCursorSkipsSeenMessages(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "cursor")
	for _, body := range []string{"one", "two"} {
		do(t, srv, http.MethodPost, "/api/channels/cursor/messages",
			map[string]string{"from": "a", "body": body}, nil)
	}

	var batch store.Batch
	do(t, srv, http.MethodGet, "/api/channels/cursor/messages?since=1", nil, &batch)
	if len(batch.Messages) != 1 || batch.Messages[0].Body != "two" {
		t.Fatalf("read from cursor = %+v, want only the second message", batch.Messages)
	}
}

func TestLongPollWakesOnPost(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "poll")

	done := make(chan store.Batch, 1)
	go func() {
		var batch store.Batch
		do(t, srv, http.MethodGet, "/api/channels/poll/messages?wait=5", nil, &batch)
		done <- batch
	}()

	// Give the waiting read time to reach the store before answering it.
	time.Sleep(50 * time.Millisecond)
	do(t, srv, http.MethodPost, "/api/channels/poll/messages",
		map[string]string{"from": "peer", "body": "here"}, nil)

	select {
	case batch := <-done:
		if len(batch.Messages) != 1 || batch.Messages[0].Body != "here" {
			t.Fatalf("long poll returned %+v", batch)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("long poll did not return after a post")
	}
}

func TestLongPollTimesOutEmpty(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "quiet")

	var batch store.Batch
	code := do(t, srv, http.MethodGet, "/api/channels/quiet/messages?wait=1", nil, &batch)
	if code != http.StatusOK {
		t.Fatalf("expired wait returned %d, want 200 with an empty batch", code)
	}
	if len(batch.Messages) != 0 {
		t.Fatalf("expired wait returned %d messages", len(batch.Messages))
	}
}

func TestStreamDeliversBacklogAndLiveMessages(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "stream")
	do(t, srv, http.MethodPost, "/api/channels/stream/messages",
		map[string]string{"from": "a", "body": "backlog"}, nil)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/channels/stream/stream", nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("stream content-type = %q", ct)
	}

	frames := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				frames <- strings.TrimPrefix(line, "data: ")
			}
		}
		_ = sc.Err() // the scan ends when the test closes the response body
	}()

	if body := nextBody(t, frames); body != "backlog" {
		t.Fatalf("first frame body = %q, want the backlog message", body)
	}

	do(t, srv, http.MethodPost, "/api/channels/stream/messages",
		map[string]string{"from": "b", "body": "live"}, nil)
	if body := nextBody(t, frames); body != "live" {
		t.Fatalf("second frame body = %q, want the live message", body)
	}
}

// nextBody reads the next streamed message and returns its body.
func nextBody(t *testing.T, frames <-chan string) string {
	t.Helper()
	select {
	case raw := <-frames:
		var m store.Message
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("decode frame %q: %v", raw, err)
		}
		return m.Body
	case <-time.After(10 * time.Second):
		t.Fatal("no stream frame arrived")
		return ""
	}
}

func TestCloseEndsTheChannel(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "wrapup")

	var sum apiChannel
	if code := do(t, srv, http.MethodPost, "/api/channels/wrapup/close",
		map[string]string{"note": "shipped"}, &sum); code != http.StatusOK {
		t.Fatalf("close returned %d", code)
	}
	if sum.ClosedAt == nil || sum.CloseNote != "shipped" {
		t.Fatalf("closed channel = %+v", sum)
	}

	code := do(t, srv, http.MethodPost, "/api/channels/wrapup/messages",
		map[string]string{"from": "a", "body": "late"}, nil)
	if code != http.StatusConflict {
		t.Errorf("posting to a closed channel returned %d, want 409", code)
	}
}

func TestErrorStatuses(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "taken")

	if code := do(t, srv, http.MethodGet, "/api/channels/missing", nil, nil); code != http.StatusNotFound {
		t.Errorf("missing channel returned %d, want 404", code)
	}
	code := do(t, srv, http.MethodPost, "/api/channels", map[string]string{"name": "taken"}, nil)
	if code != http.StatusConflict {
		t.Errorf("duplicate name returned %d, want 409", code)
	}
	code = do(
		t,
		srv,
		http.MethodPost,
		"/api/channels",
		map[string]string{"name": "Not A Name"},
		nil,
	)
	if code != http.StatusBadRequest {
		t.Errorf("invalid name returned %d, want 400", code)
	}
	code = do(t, srv, http.MethodPost, "/api/channels/taken/messages",
		map[string]string{"body": "unsigned"}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("post with no from returned %d, want 400", code)
	}
	code = do(t, srv, http.MethodGet, "/api/channels/taken/messages?since=-1", nil, nil)
	if code != http.StatusBadRequest {
		t.Errorf("negative cursor returned %d, want 400", code)
	}
}

func TestListHidesClosedUnlessAsked(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "live-one")
	openChannel(t, srv, "done-one")
	do(t, srv, http.MethodPost, "/api/channels/done-one/close", nil, nil)

	var out struct {
		Channels []apiChannel `json:"channels"`
	}
	do(t, srv, http.MethodGet, "/api/channels", nil, &out)
	if len(out.Channels) != 1 || out.Channels[0].Name != "live-one" {
		t.Fatalf("default list = %+v, want only the open channel", out.Channels)
	}
	do(t, srv, http.MethodGet, "/api/channels?all=1", nil, &out)
	if len(out.Channels) != 2 {
		t.Fatalf("list with all=1 returned %d channels, want 2", len(out.Channels))
	}
}

func TestStaticRoutes(t *testing.T) {
	srv := newTestServer(t)
	for path, want := range map[string]string{
		"/":        "wk-header",
		"/app.js":  "EventSource",
		"/version": `"version":"test"`,
	} {
		res, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if !strings.Contains(string(raw), want) {
			t.Errorf("GET %s does not contain %q", path, want)
		}
	}
	res, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("GET /healthz returned %d", res.StatusCode)
	}
}

func TestWaitIsCapped(t *testing.T) {
	if got := waitFor(0); got != 0 {
		t.Errorf("waitFor(0) = %v, want no wait", got)
	}
	if got := waitFor(5); got != 5*time.Second {
		t.Errorf("waitFor(5) = %v", got)
	}
	if got := waitFor(9999); got != maxWait {
		t.Errorf("waitFor(9999) = %v, want the %v cap", got, maxWait)
	}
}

func TestJoinLeaveEndpoints(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "swarm")

	var sum apiChannel
	code := do(t, srv, http.MethodPost, "/api/channels/swarm/join",
		map[string]string{"from": "worker", "note": "ready"}, &sum)
	if code != http.StatusOK {
		t.Fatalf("join returned %d", code)
	}
	if got := sum.Members; len(got) != 2 || got[0] != "planner" || got[1] != "worker" {
		t.Errorf("members after join = %v, want [planner worker]", got)
	}
	if sum.Messages != 1 || sum.LastBody != "ready" {
		t.Errorf("join summary = %+v, want the join message recorded", sum.Summary)
	}

	code = do(t, srv, http.MethodPost, "/api/channels/swarm/leave",
		map[string]string{"from": "worker"}, &sum)
	if code != http.StatusOK {
		t.Fatalf("leave returned %d", code)
	}
	if got := sum.Members; len(got) != 1 || got[0] != "planner" {
		t.Errorf("members after leave = %v, want [planner]", got)
	}

	// The roster ops surface store errors like every other endpoint.
	if code = do(t, srv, http.MethodPost, "/api/channels/nope/join",
		map[string]string{"from": "worker"}, nil); code != http.StatusNotFound {
		t.Errorf("join on a missing channel returned %d, want 404", code)
	}
}

func TestAddressedObligationsRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	openChannel(t, srv, "typed")

	var posted store.Message
	code := do(t, srv, http.MethodPost, "/api/channels/typed/messages", map[string]any{
		"from": "planner", "to": "worker", "body": "status?",
		"kind": "question", "reply_needed": true,
	}, &posted)
	if code != http.StatusCreated {
		t.Fatalf("post returned %d", code)
	}
	if posted.To != "worker" {
		t.Errorf("posted to = %q, want worker", posted.To)
	}

	var batch store.Batch
	if code = do(t, srv, http.MethodGet, "/api/channels/typed/messages", nil, &batch); code != http.StatusOK {
		t.Fatalf("read returned %d", code)
	}
	if got := batch.AwaitingReplyBy["worker"]; len(got) != 1 || got[0] != posted.Seq {
		t.Errorf("awaiting_reply_by = %v, want the worker's debt listed", batch.AwaitingReplyBy)
	}
	if len(batch.Members) != 1 || batch.Members[0] != "planner" {
		t.Errorf("members = %v, want the opener on the roster", batch.Members)
	}
	if got := batch.AwaitingReplyOffRoster; len(got) != 1 || got[0] != "worker" {
		t.Errorf("awaiting_reply_off_roster = %v, want [worker]: nobody by that name has joined",
			got)
	}
}
