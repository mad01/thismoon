package feedstore

import (
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

var t0 = time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)

func card(id, text string) protocol.Card {
	return protocol.Card{
		ID:    id,
		Lines: []protocol.Line{{{Text: text, Color: protocol.ColorDefault}}},
		Tone:  protocol.ColorDefault,
	}
}

// apply sends each message to s, the nth one arriving n minutes after t0.
func apply(t *testing.T, s *Store, msgs ...protocol.Message) {
	t.Helper()
	for i, m := range msgs {
		if err := s.Apply(m, t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("Apply(%#v) = %v", m, err)
		}
	}
}

// texts returns the first line of each visible entry, oldest first.
func texts(s *Store) []string {
	var out []string
	for _, e := range s.Visible() {
		out = append(out, e.Card.Lines[0].Text())
	}
	return out
}

func assertTexts(t *testing.T, s *Store, want ...string) {
	t.Helper()
	if got := texts(s); !slices.Equal(got, want) {
		t.Errorf("visible = %q, want %q", got, want)
	}
}

func TestCardsAppendInArrivalOrder(t *testing.T) {
	s := New(DefaultMax)
	apply(t, s, card("a", "deploy started"), card("b", "deploy finished"))

	assertTexts(t, s, "deploy started", "deploy finished")
	got := s.Visible()
	if got[0].Seq >= got[1].Seq {
		t.Errorf("Seq = %d then %d, want them to rise in arrival order", got[0].Seq, got[1].Seq)
	}
	if !got[1].Arrived.Equal(t0.Add(time.Minute)) {
		t.Errorf("second entry Arrived = %v, want the time its line arrived", got[1].Arrived)
	}
}

func TestRepeatedIDReplacesInPlace(t *testing.T) {
	s := New(DefaultMax)
	apply(t, s,
		card("a", "acme/api#12 opened"),
		card("b", "acme/api#13 opened"),
		card("a", "acme/api#12 merged"),
	)

	assertTexts(t, s, "acme/api#12 merged", "acme/api#13 opened")
	first := s.Visible()[0]
	if first.Seq != 1 || !first.Arrived.Equal(t0) {
		t.Errorf("replaced entry = Seq %d, Arrived %v; want it to keep Seq 1 and %v",
			first.Seq, first.Arrived, t0)
	}
	if s.Len() != 2 {
		t.Errorf("Len = %d, want 2: a repeated id adds no entry", s.Len())
	}
}

func TestEmptyIDAlwaysAppends(t *testing.T) {
	s := New(DefaultMax)
	apply(t, s, card("", "tick"), card("", "tick"))

	assertTexts(t, s, "tick", "tick")
}

func TestLimitDropsOldest(t *testing.T) {
	s := New(3)
	apply(t, s, card("a", "1"), card("b", "2"), card("c", "3"), card("d", "4"), card("", "5"))

	assertTexts(t, s, "3", "4", "5")
	// The dropped id is gone, so sending it again appends.
	apply(t, s, card("a", "1 again"))
	assertTexts(t, s, "4", "5", "1 again")
}

func TestLimitBelowOneKeepsOne(t *testing.T) {
	s := New(0)
	apply(t, s, card("a", "1"), card("b", "2"))

	assertTexts(t, s, "2")
}

func TestFilterKeepsArrivalOrder(t *testing.T) {
	s := New(DefaultMax)
	apply(t, s,
		card("a", "login failed for alice"),
		card("b", "deploy finished"),
		card("c", "login ok for alice"),
	)
	s.SetQuery("lgn alice")

	assertTexts(t, s, "login failed for alice", "login ok for alice")
	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3: the filter hides entries, it does not drop them", s.Len())
	}
	s.SetQuery("")
	assertTexts(t, s, "login failed for alice", "deploy finished", "login ok for alice")
}

func TestFilterUsesMatchText(t *testing.T) {
	s := New(DefaultMax)
	c := card("a", "acme/api#12")
	c.Match = "alice"
	apply(t, s, c, card("b", "acme/api#13"))
	s.SetQuery("alice")

	assertTexts(t, s, "acme/api#12")
}

func TestSnapshotReplacesAll(t *testing.T) {
	s := New(2)
	apply(t, s,
		card("a", "old"),
		protocol.Snapshot{Cards: []protocol.Card{
			card("x", "x1"), card("y", "y1"), card("x", "x2"), card("z", "z1"),
		}},
	)

	// x keeps its first place and takes its last content, then the limit
	// keeps the newest two.
	assertTexts(t, s, "y1", "z1")
}

func TestRemoveAndClear(t *testing.T) {
	s := New(DefaultMax)
	header := protocol.Line{{Text: "Deploys", Color: protocol.ColorDefault}}
	apply(t, s,
		protocol.Header{Text: header},
		card("a", "1"), card("b", "2"), card("c", "3"),
		protocol.Remove{ID: "b"},
		protocol.Remove{ID: "missing"},
	)
	assertTexts(t, s, "1", "3")

	apply(t, s, protocol.Clear{})
	if s.Len() != 0 || len(s.Visible()) != 0 {
		t.Errorf("after clear Len = %d, Visible = %v; want no entries", s.Len(), s.Visible())
	}
	if got := s.Header().Text(); got != "Deploys" {
		t.Errorf("header after clear = %q, want it kept", got)
	}
}

func TestHoldsFollowsEveryChange(t *testing.T) {
	s := New(2)
	apply(t, s, card("a", "1"), card("b", "2"), card("c", "3"))
	s.SetQuery("4")
	apply(t, s, protocol.Remove{ID: "b"}, card("d", "4"))
	// a went to the limit and b to the remove; c and d are held, c hidden.
	for seq, want := range map[uint64]bool{0: false, 1: false, 2: false, 3: true, 4: true, 5: false} {
		if got := s.Holds(seq); got != want {
			t.Errorf("Holds(%d) = %v, want %v", seq, got, want)
		}
	}

	apply(t, s, protocol.Snapshot{Cards: []protocol.Card{card("c", "3")}})
	if s.Holds(3) || !s.Holds(5) {
		t.Errorf("after a snapshot Holds(3) = %v and Holds(5) = %v, want false and true",
			s.Holds(3), s.Holds(5))
	}
}

func TestEntryTime(t *testing.T) {
	at := time.Date(2026, 11, 20, 23, 59, 0, 0, time.UTC)
	with := Entry{Card: protocol.Card{At: at}, Arrived: t0}
	without := Entry{Arrived: t0}

	if got := with.Time(); !got.Equal(at) {
		t.Errorf("Time with at = %v, want %v", got, at)
	}
	if got := without.Time(); !got.Equal(t0) {
		t.Errorf("Time without at = %v, want the arrival time %v", got, t0)
	}
}
