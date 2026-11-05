package store_test

import (
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestCapCheckCuts: a turn whose stored lines fit within the cap while
// the next turn's offset puts its range past it was cut; a range at the
// cap, or lines past it, were not. The last turn is a tail to judge
// against the transcript's size unless a turn is still open. Only the
// first turn trusts an offset of zero, and a turn without lines of a
// known size is not judged.
func TestCapCheckCuts(t *testing.T) {
	kept := map[string]int64{"k50": 50, "k30": 30, "k120": 120, "k150": 150}
	check := store.CapCheck{Cap: 100, Kept: func(chunk string) (int64, bool) {
		n, ok := kept[chunk]
		return n, ok
	}}
	cases := []struct {
		name      string
		turns     []store.Turn
		wantTurns []string
		wantTail  *store.CutTail
	}{
		{
			name: "range past the cap, and a tail",
			turns: []store.Turn{
				cutTurn("a", 1, 0, "k50", store.TurnFailed),
				cutTurn("b", 2, 150, "k30", ""),
			},
			wantTurns: []string{"a"},
			wantTail:  &store.CutTail{TurnID: "b", Offset: 150},
		},
		{
			name:  "range at the cap, tail past it",
			turns: []store.Turn{cutTurn("a", 1, 0, "k50", ""), cutTurn("b", 2, 100, "k120", "")},
		},
		{
			name:  "lines past the cap",
			turns: []store.Turn{cutTurn("a", 1, 0, "k150", ""), cutTurn("b", 2, 400, "k120", "")},
		},
		{
			name: "an open turn bounds the range and leaves no tail",
			turns: []store.Turn{
				cutTurn("a", 1, 10, "k50", ""), cutTurn("b", 2, 200, "", store.TurnOpen),
			},
			wantTurns: []string{"a"},
		},
		{
			name: "only the first turn trusts a zero offset",
			turns: []store.Turn{
				cutTurn("a", 1, 0, "k50", ""), cutTurn("b", 2, 0, "k50", ""),
				cutTurn("c", 3, 300, "k120", ""),
			},
			wantTurns: []string{"a"},
		},
		{
			name:  "no lines, or lines of unknown size",
			turns: []store.Turn{cutTurn("a", 1, 0, "", ""), cutTurn("b", 2, 300, "gone", "")},
		},
	}
	s := store.Session{ID: "s1", TranscriptPath: "/t/s1.jsonl"}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := check.Cuts(s, c.turns)
			if want := len(c.wantTurns) > 0 || c.wantTail != nil; found != want {
				t.Errorf("found = %v, want %v", found, want)
			}
			if got.SessionID != "s1" || got.TranscriptPath != "/t/s1.jsonl" {
				t.Errorf("session = %q %q, want s1 /t/s1.jsonl", got.SessionID, got.TranscriptPath)
			}
			if !slices.Equal(got.Turns, c.wantTurns) {
				t.Errorf("turns = %q, want %q", got.Turns, c.wantTurns)
			}
			if (got.Tail == nil) != (c.wantTail == nil) ||
				(got.Tail != nil && *got.Tail != *c.wantTail) {
				t.Errorf("tail = %+v, want %+v", got.Tail, c.wantTail)
			}
		})
	}
}

// cutTurn is a fixture turn started seq minutes into the day, closed
// unless status says otherwise.
func cutTurn(id string, seq int, offset int64, chunk string, status store.TurnStatus) store.Turn {
	if status == "" {
		status = store.TurnClosed
	}
	return store.Turn{
		ID: id, SessionID: "s1", Seq: seq, Status: status, TranscriptChunk: chunk,
		TranscriptOffset: offset,
		StartedAt:        time.Date(2026, 10, 1, 9, seq, 0, 0, time.UTC),
	}
}
