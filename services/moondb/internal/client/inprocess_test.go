package client

import (
	"context"
	"errors"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestNewForHandler reads through a handler in process: the answers and
// the error mapping match a client of the same server over the network.
func TestNewForHandler(t *testing.T) {
	ctx := context.Background()
	st := servertest.NewStore()
	c := NewForHandler(server.New(st, servertest.NewQuerier(st), testInfo).APIHandler())

	sessions, err := c.Sessions(ctx, SessionQuery{})
	if err != nil || len(sessions) != 2 || sessions[0].ID != servertest.NewSession {
		t.Fatalf("Sessions = %+v, %v; want the two fixture sessions, newest first", sessions, err)
	}
	got, err := c.Chunk(ctx, servertest.PromptSHA)
	if err != nil || string(got) != servertest.Prompt {
		t.Errorf("Chunk = %q, %v; want the prompt", got, err)
	}
	if _, err := c.Turn(ctx, "no-such-turn"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Turn(unknown) = %v, want ErrNotFound", err)
	}
	if info, err := c.Version(ctx); err != nil || info.Commit != testInfo.Commit {
		t.Errorf("Version = %+v, %v; want %+v", info, err, testInfo)
	}
}
