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

// withIdle gives a turn with totals idle minutes.
func withIdle(t Turn, idle int) Turn {
	tot := *t.Totals
	tot.IdleMS = ms(idle)
	t.Totals = &tot
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
// less the time a turn waited for input or sat idle, and never the gaps
// between turns. A turn with totals gives wall less user and idle time,
// clamped at zero; one without gives its recorded span. An open turn ends
// at the latest edit or agent stop before the next turn starts, for the
// wall span as for the active time, and the active time never exceeds the
// wall span.
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
			// A laptop asleep inside a turn: 5 hours with no transcript line,
			// past the first 30 minutes, is idle time and never active.
			name: "turn with idle time: wall less user and idle time",
			turns: []Turn{withIdle(
				withTotals(closedTurn("t1", 1, 0, 330), 330, 2), 270,
			)},
			wantActive: ms(330 - 2 - 270),
			wantWall:   ms(330),
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
			wantWall:   ms(6),
		},
		{
			name:       "a running first turn ends both times at its last edit",
			turns:      []Turn{openAt("t1", 1, 0)},
			edits:      []int{10, 45},
			wantActive: ms(45),
			wantWall:   ms(45),
		},
		{
			name: "an open last turn ends the wall span at its last edit",
			turns: []Turn{
				closedTurn("t1", 1, 0, 1),
				openAt("t2", 2, 2),
			},
			edits:      []int{60},
			wantActive: ms(1 + 58),
			wantWall:   ms(60),
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
		{
			name: "two open turns sharing a start count their overlap once",
			turns: []Turn{
				openAt("t1", 1, 0),
				openAt("t2", 2, 0),
			},
			edits:      []int{10},
			wantActive: ms(10),
			wantWall:   ms(10),
		},
		{
			name: "overlapping closed turns cap at the wall span",
			turns: []Turn{
				closedTurn("t1", 1, 0, 30),
				closedTurn("t2", 2, 10, 40),
			},
			wantActive: ms(40),
			wantWall:   ms(40),
		},
		{
			name:       "totals outlasting the recorded span cap at the wall span",
			turns:      []Turn{withTotals(closedTurn("t1", 1, 0, 5), 8, 0)},
			wantActive: ms(5),
			wantWall:   ms(5),
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
			if sum.ActiveMS > sum.DurationMS {
				t.Errorf("ActiveMS %d exceeds DurationMS %d", sum.ActiveMS, sum.DurationMS)
			}
		})
	}
}

// TestSummarizeLastTurnOpen: the last turn is the one that started last,
// the higher Seq on a tie, so an imported turn numbered after the live
// ones does not hide an open live turn.
func TestSummarizeLastTurnOpen(t *testing.T) {
	tests := []struct {
		name  string
		turns []Turn
		want  bool
	}{
		{name: "no turns"},
		{
			name:  "the latest turn is open",
			turns: []Turn{closedTurn("t1", 1, 0, 5), openAt("t2", 2, 10)},
			want:  true,
		},
		{
			name:  "an earlier turn is open",
			turns: []Turn{openAt("t1", 1, 0), closedTurn("t2", 2, 10, 15)},
		},
		{
			name: "an imported turn with a higher Seq ran first",
			turns: []Turn{
				closedTurn("t1", 1, 30, 35), openAt("t2", 2, 40), closedTurn("t3", 3, 0, 10),
			},
			want: true,
		},
		{
			name:  "a tie on start goes to the higher Seq",
			turns: []Turn{openAt("t2", 2, 10), closedTurn("t1", 1, 10, 12)},
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum := Summarize(Session{ID: "s", StartedAt: minutes(0)}, tc.turns, nil, editsAt())
			if sum.LastTurnOpen != tc.want {
				t.Errorf("LastTurnOpen = %v, want %v", sum.LastTurnOpen, tc.want)
			}
		})
	}
}
