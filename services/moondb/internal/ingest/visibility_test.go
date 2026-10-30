package ingest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// visibilityEnv asks to hide or show session at minute.
func visibilityEnv(session string, minute int, hidden bool) spool.Envelope {
	env := base(spool.KindVisibility, fmt.Sprintf("vis:%s:%d", session, minute), session, "",
		minute)
	env.Visibility = &spool.Visibility{Hidden: hidden}
	return env
}

// TestVisibilityHidesAndShows: a hide sets Hidden on a held session, a
// later envelope's upsert keeps it, and a show clears it.
func TestVisibilityHidesAndShows(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "prompt"), c)
	f.write(visibilityEnv("s1", 2, true), c)
	later := openEnv(c, "s1", "p2", 3, "next")
	later.Session.Model = "model-b"
	f.write(later, c)
	f.drain()
	if s := f.session("s1"); !s.Hidden || s.Model != "model-b" {
		t.Fatalf("session = hidden %v, model %q; want hidden after the later upsert of model-b",
			s.Hidden, s.Model)
	}

	f.write(visibilityEnv("s1", 4, false), c)
	f.drain()
	if s := f.session("s1"); s.Hidden {
		t.Error("session still hidden after a show")
	}
}

// TestVisibilityReplayWritesNothing: asking for the state a session is
// already in writes no record.
func TestVisibilityReplayWritesNothing(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "prompt"), c)
	f.write(visibilityEnv("s1", 2, true), c)
	f.drain()
	before := f.fake.Writes()
	if err := f.in.Ingest(context.Background(), visibilityEnv("s1", 2, true)); err != nil {
		t.Fatal(err)
	}
	if got := f.fake.Writes(); got != before {
		t.Errorf("replayed hide wrote %d records, want none", got-before)
	}
}

// TestVisibilityOfUnknownSessionDrops: a session the store does not hold
// is not created; the envelope is logged and leaves the spool.
func TestVisibilityOfUnknownSessionDrops(t *testing.T) {
	f := newFixture(t)
	f.write(visibilityEnv("ghost", 1, true), chunkSet{})
	f.drain()
	if _, err := f.st.Session(context.Background(), "ghost"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("session ghost: %v; want none", err)
	}
	if n := f.pending(); n != 0 {
		t.Errorf("%d envelopes pending, want the hide gone from the spool", n)
	}
	if !f.logged(`session "ghost"`) {
		t.Errorf("drop not logged: %q", f.logs)
	}
}
