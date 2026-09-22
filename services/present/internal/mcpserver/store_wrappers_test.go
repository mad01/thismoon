package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// handlersOver builds shared-mode handlers over the store wrap returns,
// sharing one movable clock with the wrapper. It mirrors what serve wires
// in shared mode, where the tools never see the raw store.
func handlersOver(
	t *testing.T,
	wrap func(store.Store, func() time.Time) store.Store,
) (*handlers, *time.Time) {
	t.Helper()
	fs, err := store.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	h := &handlers{
		store: wrap(fs, clock),
		mode:  ModeShared,
		now:   clock,
		open:  func(string) error { return nil },
	}
	return h, &now
}

// TestSharedCreateRefusesAnOversizedPage pins the size cap on the MCP path:
// a shared instance serves through the size-limited store, so present_create
// is refused by the same rule as an HTTP push, whichever store is underneath.
func TestSharedCreateRefusesAnOversizedPage(t *testing.T) {
	h, _ := handlersOver(t, func(s store.Store, _ func() time.Time) store.Store {
		return store.WithSizeLimit(s, present.MaxPageBytes)
	})
	ctx := context.Background()
	key := author.NewKey()

	_, _, err := h.handleCreateShared(ctx, request(key, nil), sharedCreateInput{
		createInput: createInput{
			Title:   "Huge",
			Content: "<p>" + strings.Repeat("x", present.MaxPageBytes) + "</p>",
		},
	})
	if !errors.Is(err, store.ErrTooLarge) {
		t.Fatalf("oversized create: err = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error must name the limit: %v", err)
	}

	_, out, err := h.handleCreateShared(ctx, request(key, nil), sharedCreateInput{
		createInput: createInput{Title: "Small", Content: "<p>fits</p>"},
	})
	if err != nil || out.ID == "" {
		t.Fatalf("create under the limit = %+v, %v", out, err)
	}
}

// TestSharedToolsHonourExpiry pins the expiry wrapper through the tools: an
// ephemeral page past its expiry is gone for every tool, not just for the
// HTTP page view, even though the sweeper has not deleted it yet.
func TestSharedToolsHonourExpiry(t *testing.T) {
	h, now := handlersOver(t, store.WithoutExpired)
	ctx := context.Background()
	key := author.NewKey()

	_, created, err := h.handleCreateShared(ctx, request(key, nil), sharedCreateInput{
		createInput: createInput{Title: "Temp", Content: "<p>v1</p>"},
		Ephemeral:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.handleRead(ctx, request(key, nil), readInput{ID: created.ID}); err != nil {
		t.Fatalf("read before expiry: %v", err)
	}

	*now = now.Add(present.SharedTTL + time.Minute)

	if _, _, err := h.handleRead(ctx, request(key, nil), readInput{ID: created.ID}); !errors.Is(
		err, store.ErrNotFound,
	) {
		t.Errorf("read after expiry: err = %v, want ErrNotFound", err)
	}
	if _, _, err := h.handleSource(ctx, request(key, nil), sourceInput{ID: created.ID}); !errors.Is(
		err, store.ErrNotFound,
	) {
		t.Errorf("source after expiry: err = %v, want ErrNotFound", err)
	}
	in := updateInput{ID: created.ID, Content: ptrStr("<p>v2</p>")}
	if _, _, err := h.handleUpdate(ctx, request(key, nil), in); !errors.Is(
		err, store.ErrNotFound,
	) {
		t.Errorf("update after expiry: err = %v, want ErrNotFound", err)
	}
}
