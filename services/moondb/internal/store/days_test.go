package store

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// local is a test time in the server's zone, which LocalDay cuts days by:
// day d of the test month at hour h and minute m.
func local(d, h, m int) time.Time {
	return time.Date(2026, 10, d, h, m, 0, 0, time.Local)
}

func day(d int) string { return LocalDay(local(d, 0, 0)) }

// dayView is a DayActivity as a test reads it: its usage as a token
// total and its attributed parts as "repo:tokens" pairs.
type dayView struct {
	Date                        string
	First, Last                 time.Time
	Turns, Edits, Files, Agents int
	ActiveMS                    int64
	Tokens                      int64
	Repos                       string
}

func viewOf(d DayActivity) dayView {
	var tokens int64
	for _, u := range d.Usage {
		tokens += u.Tokens.Total()
	}
	parts := []string{}
	for _, p := range d.RepoUsage {
		var n int64
		for _, u := range p.Usage {
			n += u.Tokens.Total()
		}
		parts = append(parts, fmt.Sprintf("%s:%d", p.Repo, n))
	}
	return dayView{
		Date: d.Date, First: d.FirstAt, Last: d.LastAt,
		Turns: d.Turns, Edits: d.Edits, Files: d.Files, Agents: d.Agents,
		ActiveMS: d.ActiveMS, Tokens: tokens, Repos: strings.Join(parts, " "),
	}
}

// TestSummarizeDays: a session has a day for every local day a record
// falls on, its start included, oldest first. A turn's count, active time
// and usage go to the day it started, an edit's to the day it was made,
// and an agent's usage to the day it started, split by its edits that
// day. A day that holds only a turn's end or an agent's stop is a day
// with no counts.
func TestSummarizeDays(t *testing.T) {
	loc := Location{Repo: "r", Worktree: "/w"}
	s := Session{ID: "s", StartedAt: local(1, 9, 0)}
	// t1 closes on day 1 with totals; t2 opens late on day 2 and its last
	// activity, agent a1's edit, lands on day 3; a1 starts on day 1 and
	// stops on day 2.
	t1 := Turn{
		ID: "t1", SessionID: "s", Seq: 1, Status: TurnClosed,
		StartedAt: local(1, 9, 5), EndedAt: new(local(1, 9, 30)),
		Totals: &TurnTotals{
			WallMS: ms(25), UserMS: ms(5),
			Usage: []ModelUsage{{Model: "m", Tokens: Tokens{Input: 10}}},
		},
	}
	t2 := Turn{ID: "t2", SessionID: "s", Seq: 2, Status: TurnOpen, StartedAt: local(2, 23, 50)}
	a1 := Agent{
		ID: "a1", SessionID: "s", Type: "general-purpose",
		StartedAt: local(1, 9, 10), EndedAt: local(2, 8, 0),
		Totals: &TurnTotals{Usage: []ModelUsage{{Model: "m", Tokens: Tokens{Output: 7}}}},
	}
	edits := slices.Values([]*Edit{
		{ID: "e1", SessionID: "s", At: local(1, 9, 20), Location: loc, Path: "x.go"},
		{ID: "e2", SessionID: "s", AgentID: "a1", At: local(3, 0, 10), Location: loc, Path: "y.go"},
	})
	sum := Summarize(s, []Turn{t1, t2}, []Agent{a1}, edits)
	want := []dayView{
		// The main conversation edited r that day, so its tokens go there;
		// a1 had no edit yet and no location, so its go to no_repo. The
		// parts come sorted by name.
		{
			Date: day(1), First: local(1, 9, 0), Last: local(1, 9, 30),
			Turns: 1, Edits: 1, Files: 1, Agents: 2, ActiveMS: ms(20),
			Tokens: 17, Repos: "no_repo:7 r:10",
		},
		// a1's stop opens the day; t2 starts on it and its 20 active
		// minutes run past midnight.
		{
			Date: day(2), First: local(2, 8, 0), Last: local(2, 23, 50),
			Turns: 1, ActiveMS: ms(20),
		},
		{
			Date: day(3), First: local(3, 0, 10), Last: local(3, 0, 10),
			Edits: 1, Files: 1, Agents: 1,
		},
	}
	got := []dayView{}
	for _, d := range sum.Days {
		got = append(got, viewOf(d))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("days =\n%+v\nwant\n%+v", got, want)
	}
	if !sum.LastActivityAt.Equal(local(3, 0, 10)) {
		t.Errorf("last activity = %v, want the edit on day 3", sum.LastActivityAt)
	}
}

// TestSummarizeDaysMainSplit: a day's main usage is the split of the
// turns that started that day, by the weights the whole session gives
// them: a turn without edits takes the previous day's, and a turn's edit
// made after midnight still splits the turn on its start day. The day's
// agents split by their edits that day.
func TestSummarizeDaysMainSplit(t *testing.T) {
	turn := func(id string, seq int, start time.Time, input int64) Turn {
		return Turn{
			ID: id, SessionID: "s", Seq: seq, Status: TurnClosed,
			StartedAt: start, EndedAt: new(start.Add(30 * time.Minute)),
			Totals: &TurnTotals{Usage: []ModelUsage{{Model: "m", Tokens: Tokens{Input: input}}}},
		}
	}
	turns := []Turn{
		turn("t1", 1, local(1, 9, 0), 10),
		turn("t2", 2, local(2, 10, 0), 20),
		turn("t3", 3, local(2, 23, 50), 40),
	}
	a1 := Agent{
		ID: "a1", SessionID: "s", Type: "general-purpose",
		StartedAt: local(3, 0, 5), EndedAt: local(3, 0, 15),
		Totals: &TurnTotals{Usage: []ModelUsage{{Model: "m", Tokens: Tokens{Input: 7}}}},
	}
	edits := []*Edit{
		{ID: "e1", TurnID: "t1", At: local(1, 9, 10), Location: Location{Repo: repoA}},
		{ID: "e2", TurnID: "t3", At: local(3, 0, 10), Location: Location{Repo: repoB}},
	}
	sum := Summarize(Session{ID: "s", StartedAt: local(1, 9, 0)}, turns, []Agent{a1},
		slices.Values(edits))
	got := map[string]string{}
	for _, d := range sum.Days {
		got[d.Date] = viewOf(d).Repos
	}
	want := map[string]string{
		day(1): repoA + ":10",
		day(2): repoA + ":20 " + repoB + ":40",
		day(3): CostNoRepo + ":7",
	}
	if !maps.Equal(got, want) {
		t.Errorf("day parts = %v, want %v", got, want)
	}
}

// TestSummarizeDaysStartOnly: a session with no record but its start has
// its start day, with nothing counted.
func TestSummarizeDaysStartOnly(t *testing.T) {
	s := Session{ID: "s", StartedAt: local(5, 14, 0)}
	sum := Summarize(s, nil, nil, slices.Values([]*Edit{}))
	want := []dayView{{Date: day(5), First: s.StartedAt, Last: s.StartedAt}}
	got := []dayView{}
	for _, d := range sum.Days {
		got = append(got, viewOf(d))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("days = %+v, want %+v", got, want)
	}
}
