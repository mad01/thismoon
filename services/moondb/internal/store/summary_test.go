package store

import (
	"iter"
	"testing"
	"time"
)

var activeT0 = time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC)

// minutes is the test time m minutes after activeT0.
func minutes(m int) time.Time { return activeT0.Add(time.Duration(m) * time.Minute) }

func ms(m int) int64 { return (time.Duration(m) * time.Minute).Milliseconds() }

// closedTurn is a closed turn from start to end minutes, without totals.
func closedTurn(id string, seq, start, end int) Turn {
	return Turn{
		ID: id, SessionID: "s", Seq: seq, Status: TurnClosed,
		StartedAt: minutes(start), EndedAt: new(minutes(end)),
	}
}

// withTotals gives a turn totals of wall and user minutes.
func withTotals(t Turn, wall, user int) Turn {
	t.Totals = &TurnTotals{WallMS: ms(wall), UserMS: ms(user)}
	return t
}

func openAt(id string, seq, start int) Turn {
	return Turn{ID: id, SessionID: "s", Seq: seq, Status: TurnOpen, StartedAt: minutes(start)}
}

// editsAt yields one edit per minute given.
func editsAt(at ...int) iter.Seq[*Edit] {
	return func(yield func(*Edit) bool) {
		for _, m := range at {
			if !yield(&Edit{ID: "e", SessionID: "s", At: minutes(m)}) {
				return
			}
		}
	}
}

// TestSummarizeActiveMS: active time adds up the time inside the turns,
// less the time a turn waited for input, and never the gaps between
// turns. A turn with totals gives wall less user time, clamped at zero;
// one without gives its recorded span; an open turn ends at the latest
// edit or agent stop before the next turn starts.
func TestSummarizeActiveMS(t *testing.T) {
	tests := []struct {
		name       string
		turns      []Turn
		agents     []Agent
		edits      []int
		wantActive int64
		wantWall   int64
	}{
		{
			name:       "no turns",
			edits:      []int{3},
			wantActive: 0,
			wantWall:   0,
		},
		{
			name:       "turn with totals: wall less user time",
			turns:      []Turn{withTotals(closedTurn("t1", 1, 0, 12), 12, 5)},
			wantActive: ms(7),
			wantWall:   ms(12),
		},
		{
			name:       "turn without totals: its recorded span",
			turns:      []Turn{closedTurn("t1", 1, 1, 9)},
			wantActive: ms(8),
			wantWall:   ms(8),
		},
		{
			name:       "open turn ends at its last edit or agent stop",
			turns:      []Turn{openAt("t1", 1, 0)},
			agents:     []Agent{{ID: "a1", SessionID: "s", EndedAt: minutes(6)}},
			edits:      []int{2, 4},
			wantActive: ms(6),
			wantWall:   0,
		},
		{
			name: "a 10-hour gap between turns adds nothing",
			turns: []Turn{
				closedTurn("t1", 1, 0, 30),
				withTotals(closedTurn("t2", 2, 630, 660), 30, 0),
			},
			wantActive: ms(60),
			wantWall:   ms(660),
		},
		{
			name: "user time above wall time clamps at zero",
			turns: []Turn{
				withTotals(closedTurn("t1", 1, 0, 5), 5, 9),
				closedTurn("t2", 2, 10, 13),
			},
			wantActive: ms(3),
			wantWall:   ms(13),
		},
		{
			name: "an open turn before a later one stops at its last activity",
			turns: []Turn{
				openAt("t1", 1, 0),
				closedTurn("t2", 2, 600, 605),
			},
			edits:      []int{3, 602},
			wantActive: ms(3 + 5),
			wantWall:   ms(605),
		},
		{
			name: "turns out of Seq order find the next start by time",
			turns: []Turn{
				closedTurn("t2", 2, 600, 605),
				openAt("t1", 1, 0),
			},
			edits:      []int{4, 603},
			wantActive: ms(4 + 5),
			wantWall:   ms(605),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum := Summarize(Session{ID: "s", StartedAt: minutes(0)}, tc.turns, tc.agents,
				editsAt(tc.edits...))
			if sum.ActiveMS != tc.wantActive {
				t.Errorf("ActiveMS = %d, want %d", sum.ActiveMS, tc.wantActive)
			}
			if sum.DurationMS != tc.wantWall {
				t.Errorf("DurationMS = %d, want %d", sum.DurationMS, tc.wantWall)
			}
		})
	}
}
