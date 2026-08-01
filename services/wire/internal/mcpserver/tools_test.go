package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/wire/internal/client"
)

// newHandlers points the tools at a stub serve API and records what they ask
// it for, so the tools can be exercised without a real serve process.
func newHandlers(t *testing.T, reply any, seen *http.Request) *handlers {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = *r
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return &handlers{client: client.New(srv.URL), webURL: "http://wire.this"}
}

func TestRegisteredToolNames(t *testing.T) {
	ctx := context.Background()
	srv, err := New("test", Config{Port: 1, BaseURL: "http://wire.this"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Drive the real MCP handshake over an in-memory pair, so this checks what
	// a client actually sees rather than the registry's internals.
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	// A missing or renamed tool silently breaks every agent that calls it, so
	// pin the surface.
	want := []string{"wire_open", "wire_post", "wire_read", "wire_list", "wire_close"}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	if len(got) != len(want) {
		t.Errorf("registered %d tools, want %d: %v", len(got), len(want), got)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %s is not registered", name)
		}
	}
}

func TestOpenReturnsAWatchURL(t *testing.T) {
	var seen http.Request
	h := newHandlers(t, client.Summary{Channel: client.Channel{ID: "ch_1", Name: "handoff"}}, &seen)

	_, out, err := h.handleOpen(context.Background(), nil, openInput{Name: "handoff", From: "planner"})
	if err != nil {
		t.Fatalf("handleOpen: %v", err)
	}
	if out.Channel.Name != "handoff" {
		t.Errorf("opened %+v", out.Channel)
	}
	if out.URL != "http://wire.this/?channel=handoff" {
		t.Errorf("watch URL = %q", out.URL)
	}
	if seen.Method != http.MethodPost || seen.URL.Path != "/api/channels" {
		t.Errorf("called %s %s", seen.Method, seen.URL.Path)
	}
}

func TestPostReportsTheCursor(t *testing.T) {
	var seen http.Request
	h := newHandlers(t, client.Message{ChannelID: "ch_1", Seq: 4, From: "a", Body: "done"}, &seen)

	_, out, err := h.handlePost(context.Background(), nil, postInput{
		Channel: "handoff", From: "a", Body: "done",
	})
	if err != nil {
		t.Fatalf("handlePost: %v", err)
	}
	if out.Cursor != 4 || out.Message.Seq != 4 {
		t.Errorf("post output = %+v, want cursor 4", out)
	}
}

func TestPostForwardsProtocolFields(t *testing.T) {
	var seen http.Request
	var sentBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		sentBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.Message{Seq: 5})
	}))
	t.Cleanup(srv.Close)
	h := &handlers{client: client.New(srv.URL), webURL: "http://wire.this"}

	if _, _, err := h.handlePost(context.Background(), nil, postInput{
		Channel: "typed", From: "a", Body: "7432",
		Kind: "answer", ReplyTo: 3, ReplyNeeded: true,
	}); err != nil {
		t.Fatalf("handlePost: %v", err)
	}
	if seen.URL.Path != "/api/channels/typed/messages" {
		t.Errorf("called %s", seen.URL.Path)
	}
	var body client.PostBody
	if err := json.Unmarshal(sentBody, &body); err != nil {
		t.Fatalf("decode forwarded body: %v", err)
	}
	if body.Kind != "answer" || body.ReplyTo != 3 || !body.ReplyNeeded {
		t.Errorf("forwarded body = %+v, want the protocol fields intact", body)
	}
}

func TestReadCapsTheWait(t *testing.T) {
	var seen http.Request
	h := newHandlers(t, client.Batch{Cursor: 2}, &seen)

	if _, _, err := h.handleRead(context.Background(), nil, readInput{
		Channel: "handoff", Since: 1, Wait: 9999,
	}); err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if got := seen.URL.Query().Get("wait"); got != "120" {
		t.Errorf("wait sent as %q, want it capped to 120", got)
	}
	if got := seen.URL.Query().Get("since"); got != "1" {
		t.Errorf("since sent as %q", got)
	}
}

func TestReadFlagsAClosedChannel(t *testing.T) {
	var seen http.Request
	closed := client.Batch{Channel: client.Channel{Name: "done"}, Cursor: 3}
	now := closed.Channel.CreatedAt
	closed.Channel.ClosedAt = &now
	h := newHandlers(t, closed, &seen)

	_, out, err := h.handleRead(context.Background(), nil, readInput{Channel: "done"})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if !out.Closed {
		t.Error("a closed channel was not flagged, so a caller would keep waiting on it")
	}
}

func TestErrorsReachTheCaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "wire: channel not found: nope"})
	}))
	t.Cleanup(srv.Close)
	h := &handlers{client: client.New(srv.URL), webURL: "http://wire.this"}

	_, _, err := h.handleRead(context.Background(), nil, readInput{Channel: "nope"})
	if err == nil || !strings.Contains(err.Error(), "channel not found") {
		t.Fatalf("err = %v, want the serve error surfaced", err)
	}
}
