package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stub records the last request and replies with a canned body, so the client
// can be tested without a real serve process.
type stub struct {
	method string
	path   string
	query  string
	body   map[string]any

	status int
	reply  any
}

func (s *stub) server(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.method, s.path, s.query = r.Method, r.URL.Path, r.URL.RawQuery
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&s.body)
		}
		w.Header().Set("Content-Type", "application/json")
		if s.status != 0 {
			w.WriteHeader(s.status)
		}
		_ = json.NewEncoder(w).Encode(s.reply)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestOpenSendsFields(t *testing.T) {
	s := &stub{reply: Summary{Channel: Channel{ID: "ch_1", Name: "handoff"}}}
	c := s.server(t)

	got, err := c.Open(context.Background(), OpenBody{Name: "handoff", Topic: "auth work", From: "planner"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Name != "handoff" {
		t.Errorf("Open returned %+v", got)
	}
	if s.method != http.MethodPost || s.path != "/api/channels" {
		t.Errorf("Open called %s %s", s.method, s.path)
	}
	if s.body["topic"] != "auth work" || s.body["from"] != "planner" {
		t.Errorf("Open body = %v", s.body)
	}
}

func TestReadBuildsQuery(t *testing.T) {
	s := &stub{reply: Batch{Cursor: 3}}
	c := s.server(t)

	if _, err := c.Read(context.Background(), "handoff", ReadOptions{Since: 2, Limit: 10, Wait: 30}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, want := range []string{"since=2", "limit=10", "wait=30"} {
		if !strings.Contains(s.query, want) {
			t.Errorf("query %q is missing %q", s.query, want)
		}
	}
	if s.path != "/api/channels/handoff/messages" {
		t.Errorf("Read called %s", s.path)
	}
}

func TestReadOmitsZeroOptions(t *testing.T) {
	s := &stub{reply: Batch{}}
	c := s.server(t)

	if _, err := c.Read(context.Background(), "handoff", ReadOptions{}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.query != "" {
		t.Errorf("a plain read sent query %q, want none", s.query)
	}
}

// A blocking read must outlive the server-side wait, or the client abandons a
// request the server is still legitimately holding open.
func TestReadTimeoutOutlastsTheServerWait(t *testing.T) {
	if got := readTimeout(0); got != requestTimeout {
		t.Errorf("readTimeout(0) = %v, want the plain request budget", got)
	}
	if got := readTimeout(30); got <= 30*time.Second {
		t.Errorf("readTimeout(30) = %v, want more than the 30s the server may park", got)
	}
}

func TestAPIErrorSurfacesUnwrapped(t *testing.T) {
	s := &stub{status: http.StatusNotFound, reply: map[string]string{"error": "wire: channel not found: nope"}}
	c := s.server(t)

	_, err := c.Get(context.Background(), "nope")
	if err == nil || err.Error() != "wire: channel not found: nope" {
		t.Fatalf("Get err = %v, want the server's message verbatim", err)
	}
}

func TestUnreachableServeIsExplained(t *testing.T) {
	// Port 0 never accepts a connection, so this is the serve-is-down path.
	c := New("http://127.0.0.1:0")
	_, err := c.List(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "t-man status wire") {
		t.Fatalf("err = %v, want the hint about the serve agent", err)
	}
}

func TestChannelPathEscapesRef(t *testing.T) {
	got, err := channelPath("a b")
	if err != nil {
		t.Fatalf("channelPath: %v", err)
	}
	if got != "/api/channels/a%20b" {
		t.Errorf("channelPath = %q", got)
	}
}

// A connection string has to work anywhere a channel name does — that is the
// point of handing one to another session.
func TestChannelPathAcceptsAConnectionString(t *testing.T) {
	got, err := channelPath("wire://localhost:7432/refactor-auth")
	if err != nil {
		t.Fatalf("channelPath: %v", err)
	}
	if got != "/api/channels/refactor-auth" {
		t.Errorf("channelPath = %q, want the connection string reduced to its channel", got)
	}
}

func TestReadAcceptsAConnectionString(t *testing.T) {
	s := &stub{reply: Batch{Cursor: 1}}
	c := s.server(t)

	if _, err := c.Read(context.Background(), "wire://localhost:7432/handoff", ReadOptions{}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.path != "/api/channels/handoff/messages" {
		t.Errorf("Read called %s", s.path)
	}
}

func TestBadConnectionStringFailsBeforeTheRequest(t *testing.T) {
	s := &stub{reply: Summary{}}
	c := s.server(t)

	if _, err := c.Get(context.Background(), "http://localhost:7432/handoff"); err == nil {
		t.Fatal("a non-wire URL was accepted as a channel")
	}
	if s.path != "" {
		t.Errorf("a bad reference still reached the server at %s", s.path)
	}
}
