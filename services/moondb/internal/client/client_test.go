package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var testInfo = buildinfo.Info{Version: "test", Commit: "abc123", Tag: "moondb/v0.0.0"}

type harness struct {
	client *Client
	store  *servertest.Store
	query  *servertest.Querier
}

// newHarness runs the real server handler over the fixture store, so these
// tests pin the client to the wire shape the server actually writes.
func newHarness(t *testing.T) harness {
	t.Helper()
	st := servertest.NewStore()
	q := servertest.NewQuerier(st)
	ts := httptest.NewServer(server.New(st, q, testInfo).Handler())
	t.Cleanup(ts.Close)
	return harness{client: New(ts.URL + "/"), store: st, query: q}
}

func TestSessions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	all, err := h.client.Sessions(ctx, SessionQuery{})
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(all) != 2 || all[0].ID != servertest.NewSession || all[0].EditCount != 2 {
		t.Errorf("sessions = %+v", all)
	}
	got, err := h.client.Sessions(
		ctx,
		SessionQuery{Repo: servertest.OtherRepo, Since: "2026-09-30T00:00:00Z", Limit: 5},
	)
	if err != nil {
		t.Fatalf("Sessions filtered: %v", err)
	}
	if len(got) != 1 || got[0].ID != servertest.OldSession {
		t.Errorf("filtered sessions = %+v, want only %s", got, servertest.OldSession)
	}
}

func TestSessionAndTurn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sess, err := h.client.Session(ctx, servertest.NewSession)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if len(sess.Turns) != 1 || sess.Turns[0].ID != servertest.NewTurn {
		t.Errorf("session turns = %+v", sess.Turns)
	}
	turn, err := h.client.Turn(ctx, servertest.NewTurn)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(turn.Edits) != 2 || turn.Edits[0].AfterChunk != servertest.AfterSHA {
		t.Errorf("turn edits = %+v", turn.Edits)
	}
}

func TestMissingRecordsMatchErrNotFound(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, sessErr := h.client.Session(ctx, "nope")
	_, turnErr := h.client.Turn(ctx, "nope")
	_, chunkErr := h.client.Chunk(ctx, strings.Repeat("0", 64))
	for name, err := range map[string]error{"session": sessErr, "turn": turnErr, "chunk": chunkErr} {
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: err = %v, want it to match store.ErrNotFound", name, err)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			t.Errorf("%s: err = %#v, want an *APIError with status 404", name, err)
		}
	}
}

func TestEdits(t *testing.T) {
	h := newHarness(t)
	got, err := h.client.Edits(context.Background(), EditQuery{
		Worktree: servertest.Worktree, Path: servertest.PathB, Session: servertest.NewSession,
		Turn: servertest.NewTurn, Since: "2026-10-01T09:00:00Z", Limit: 3,
	})
	if err != nil {
		t.Fatalf("Edits: %v", err)
	}
	if len(got) != 1 || got[0].ID != servertest.EditB {
		t.Errorf("edits = %+v, want only %s", got, servertest.EditB)
	}
	byBlob, err := h.client.Edits(context.Background(), EditQuery{Blob: "blob-a"})
	if err != nil || len(byBlob) != 1 || byBlob[0].ID != servertest.EditA {
		t.Errorf("Edits by blob = %+v, %v", byBlob, err)
	}
}

func TestWhyAndHistory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	req := query.WhyRequest{
		Worktree: servertest.Worktree, Repo: servertest.Repo, Path: servertest.PathA,
		Line: 1, Text: "package a",
	}
	res, err := h.client.Why(ctx, req)
	if err != nil {
		t.Fatalf("Why: %v", err)
	}
	if res.Edit == nil || res.Edit.ID != servertest.EditA ||
		res.Session.ID != servertest.NewSession {
		t.Errorf("why = %+v", res)
	}
	if got := h.query.LastWhy(); got != req {
		t.Errorf("server got %+v, want %+v", got, req)
	}

	hreq := query.HistoryRequest{Repo: servertest.Repo, Path: servertest.PathB, Limit: 7}
	edits, err := h.client.History(ctx, hreq)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(edits) != 1 || edits[0].ID != servertest.EditB {
		t.Errorf("history = %+v", edits)
	}
	if got := h.query.LastHistory(); got != hreq {
		t.Errorf("server got %+v, want %+v", got, hreq)
	}
}

func TestNotImplementedMatchesSentinel(t *testing.T) {
	h := newHarness(t)
	h.query.SetErr(query.ErrNotImplemented)
	_, err := h.client.Why(context.Background(), query.WhyRequest{Path: "a.go", Line: 1})
	if !errors.Is(err, query.ErrNotImplemented) {
		t.Errorf("err = %v, want it to match query.ErrNotImplemented", err)
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Error("a 501 must not match store.ErrNotFound")
	}
}

func TestBadRequestCarriesServerMessage(t *testing.T) {
	h := newHarness(t)
	_, err := h.client.Why(context.Background(), query.WhyRequest{Line: 1})
	if err == nil || err.Error() != "moondb: path is required" {
		t.Errorf("err = %v, want the server's message verbatim", err)
	}
}

func TestChunk(t *testing.T) {
	h := newHarness(t)
	got, err := h.client.Chunk(context.Background(), servertest.PromptSHA)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if string(got) != servertest.Prompt {
		t.Errorf("chunk = %q, want %q", got, servertest.Prompt)
	}
}

func TestHealthAndVersion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.client.Health(ctx); err != nil {
		t.Errorf("Health: %v", err)
	}
	h.store.SetPingErr(errors.New("disk gone"))
	if err := h.client.Health(ctx); err == nil || !strings.Contains(err.Error(), "disk gone") {
		t.Errorf("Health with a failing store = %v, want the ping error", err)
	}
	info, err := h.client.Version(ctx)
	if err != nil || info != testInfo {
		t.Errorf("Version = %+v, %v; want %+v", info, err, testInfo)
	}
}

func TestUnreachableServeHintsAtDoctor(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	ts.Close()
	err := New(ts.URL).Health(context.Background())
	if err == nil {
		t.Fatal("want an error when serve is down")
	}
	for _, want := range []string{"not reachable", "moondb serve", "moondb doctor"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

func TestCancelledContextIsNotUnreachable(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.client.Sessions(ctx, SessionQuery{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "not reachable") {
		t.Errorf("err = %q, a cancelled call must not blame serve", err)
	}
}

func TestNonJSONErrorFallsBackToStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(ts.Close)
	_, err := New(ts.URL).Session(context.Background(), "x")
	if err == nil || err.Error() != "moondb: serve returned 502 Bad Gateway" {
		t.Errorf("err = %v, want the status fallback", err)
	}
}
