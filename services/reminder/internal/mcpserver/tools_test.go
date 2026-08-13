package mcpserver

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/reminder/internal/client"
	"github.com/mad01/thismoon/services/reminder/internal/server"
	"github.com/mad01/thismoon/services/reminder/internal/store"
)

// fakeNotifier swallows notifications so tool tests never fire a real macOS
// notification; it records the count so test/fire delivery can be asserted.
type fakeNotifier struct{ count int }

func (f *fakeNotifier) Notify(string, string) error { f.count++; return nil }

// testHandlers spins up a real serve handler over a temp store and points the
// MCP handlers at it, so tool calls exercise the full client→API→store path.
func testHandlers(t *testing.T) (*handlers, *fakeNotifier, func()) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	fn := &fakeNotifier{}
	ts := httptest.NewServer(server.New(st, buildinfo.Info{Version: "test"}, fn).Handler())
	h := &handlers{client: client.New(ts.URL), webURL: ts.URL}
	return h, fn, ts.Close
}

func TestCreateThenGetAndList(t *testing.T) {
	h, _, done := testHandlers(t)
	defer done()
	ctx := context.Background()

	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "ship", In: "1h", Repeat: "daily"},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Reminder.ID == "" || created.Reminder.Repeat != "daily" {
		t.Fatalf("unexpected created: %+v", created.Reminder)
	}

	_, got, err := h.handleGet(ctx, nil, idInput{ID: created.Reminder.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Reminder.Title != "ship" {
		t.Errorf("get title = %q", got.Reminder.Title)
	}

	_, list, err := h.handleList(ctx, nil, listInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Reminders) != 1 {
		t.Errorf("list len = %d, want 1", len(list.Reminders))
	}
}

func TestEditAndCancel(t *testing.T) {
	h, _, done := testHandlers(t)
	defer done()
	ctx := context.Background()

	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "x", In: "1h"})
	id := created.Reminder.ID

	newTitle := "renamed"
	_, edited, err := h.handleEdit(ctx, nil, editInput{ID: id, Title: &newTitle})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.Reminder.Title != "renamed" {
		t.Errorf("edit title = %q", edited.Reminder.Title)
	}

	_, cancelled, err := h.handleCancel(ctx, nil, idInput{ID: id})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Reminder.Status != store.StatusCancelled {
		t.Errorf("cancel status = %q", cancelled.Reminder.Status)
	}
}

func TestTestToolDoesNotMutate(t *testing.T) {
	h, fn, done := testHandlers(t)
	defer done()
	ctx := context.Background()

	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "ping", In: "1h"})
	id := created.Reminder.ID

	_, res, err := h.handleTest(ctx, nil, testInput{ID: id})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if !res.OK || res.Reminder == nil || res.Reminder.Status != store.StatusPending {
		t.Fatalf("unexpected test result: %+v", res)
	}
	if fn.count != 1 {
		t.Errorf("notifier count = %d, want 1", fn.count)
	}

	// Global test: no id, no reminder in the result.
	_, gres, err := h.handleTest(ctx, nil, testInput{})
	if err != nil {
		t.Fatalf("global test: %v", err)
	}
	if !gres.OK || gres.Reminder != nil {
		t.Errorf("global test result = %+v, want ok with no reminder", gres)
	}
	if fn.count != 2 {
		t.Errorf("notifier count = %d, want 2", fn.count)
	}
}

func TestFireToolAdvancesState(t *testing.T) {
	h, fn, done := testHandlers(t)
	defer done()
	ctx := context.Background()

	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "x", In: "1h"})

	_, fired, err := h.handleFire(ctx, nil, idInput{ID: created.Reminder.ID})
	if err != nil {
		t.Fatalf("fire: %v", err)
	}
	if fired.Reminder.Status != store.StatusFired {
		t.Errorf("status after fire = %q, want fired", fired.Reminder.Status)
	}
	if fn.count != 1 {
		t.Errorf("notifier count = %d, want 1", fn.count)
	}
}

func TestServeUnreachableError(t *testing.T) {
	// No server listening on this port → a descriptive, actionable error.
	h := &handlers{client: client.New("http://127.0.0.1:0"), webURL: ""}
	_, _, err := h.handleList(context.Background(), nil, listInput{})
	if err == nil {
		t.Fatal("want error when serve is unreachable")
	}
}
