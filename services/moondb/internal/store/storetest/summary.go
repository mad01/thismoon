package storetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// sessionSummary: a listing row and a session's detail add up the same
// records. A file edited twice counts once, the main conversation is one
// agent, an agent with a record and edits counts once and one without
// edits counts too, the last activity is the newest turn edge, edit or
// agent stop, the last turn's status shows, the wall span and the active
// time come from the turn records, an open turn ending both at its last
// activity, and the usage of every turn and agent with totals merges by
// model, apart for the main conversation and the agents. The attribution
// rule splits the same usage by repository. Every record falls on one
// local day, whose slice is then the whole session.
func sessionSummary(t *testing.T, st store.Store) {
	ctx := context.Background()
	seedSummary(t, st)
	list, err := st.Sessions(ctx, store.SessionFilter{})
	must(t, err)
	wantIDs(t, "Sessions", ids(list, sessionID), "s2", "s1")
	if len(list) != 2 {
		return
	}
	got := list[1]
	merged := []store.ModelUsage{
		usage("model-a", 1150, 115), usage("model-b", 5, 1), usage("model-c", 9, 1),
	}
	// The main conversation edited only repoA. agent-1's one edit in a
	// repository is in repoB, its other outside any. agent-2 edited
	// nothing and its cwd sits in no repository.
	attributed := []store.RepoUsage{
		{
			Repo:  repoA,
			Usage: []store.ModelUsage{usage("model-a", 150, 15), usage("model-b", 5, 1)},
		},
		{Repo: repoB, Usage: []store.ModelUsage{usage("model-a", 1000, 100)}},
		{Repo: store.CostNoRepo, Usage: []store.ModelUsage{usage("model-c", 9, 1)}},
	}
	// t1 and t2 give their totals' 1 s each, and the open t3 runs from
	// minute 30 to agent-2's stop at minute 35.
	active := 2000 + (5 * time.Minute).Milliseconds()
	want := store.SessionSummary{
		Session:   got.Session,
		Repos:     []string{repoA, repoB},
		Worktrees: []string{"/wt/a", "/wt/b"},
		TurnCount: 3, EditCount: 4, FileCount: 3, AgentCount: 3,
		Files: []store.FileKey{
			{
				Path: "/abs/z.txt",
			}, {Worktree: "/wt/a", Path: "x.go"}, {Worktree: "/wt/b", Path: "y.go"},
		},
		LastTurnOpen:   true,
		LastActivityAt: at(35),
		// t1 starts at minute 1, and t3's end at minute 35 ends the wall
		// span too.
		DurationMS:  (34 * time.Minute).Milliseconds(),
		ActiveMS:    active,
		LastTurnEnd: at(35),
		Usage:       merged,
		MainUsage:   []store.ModelUsage{usage("model-a", 150, 15), usage("model-b", 5, 1)},
		AgentUsage:  []store.ModelUsage{usage("model-a", 1000, 100), usage("model-c", 9, 1)},
		AgentLabels: []string{"builder", "Explore"},
		RepoUsage:   attributed,
		Days: []store.DayActivity{{
			Date: store.LocalDay(at(0)), FirstAt: at(0), LastAt: at(35),
			Turns: 3, Edits: 4, Files: 3, Agents: 3, ActiveMS: active,
			Usage: merged, RepoUsage: attributed,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("s1 summary =\n%+v\nwant\n%+v", got, want)
	}
	d, err := st.Session(ctx, "s1")
	must(t, err)
	if !reflect.DeepEqual(d.SessionSummary, got) {
		t.Errorf("Session(s1) summary =\n%+v\nwant the listing row\n%+v", d.SessionSummary, got)
	}
	empty := list[0]
	if empty.TurnCount != 0 || empty.EditCount != 0 || empty.FileCount != 0 ||
		empty.Repos == nil || empty.Worktrees == nil || !empty.LastActivityAt.Equal(at(40)) ||
		len(empty.Days) != 1 || empty.Days[0].Date != store.LocalDay(at(40)) {
		t.Errorf("s2 summary = %+v, want zero counts, empty lists and its start as activity "+
			"and as its one day", empty)
	}
}

// seedSummary stores what sessionSummary adds up: session s1 with three
// turns, edits by the main conversation and by agent-1, and two agents
// with records, and an empty session s2.
func seedSummary(t *testing.T, st store.Store) {
	a, b := loc(repoA, "/wt/a"), loc(repoB, "/wt/b")
	first := turn("t1", "s1", store.TurnClosed, at(1))
	first.EndedAt = new(at(5))
	first.Totals = totals(1, usage("model-a", 100, 10), usage("model-b", 5, 1))
	second := turn("t2", "s1", store.TurnClosed, at(10))
	second.EndedAt = new(at(20))
	second.Totals = totals(1, usage("model-a", 50, 5))
	sub := edit("e3", "s1", "t2", 12, b, "y.go")
	sub.AgentID, sub.AgentType = "agent-1", "Explore"
	outside := edit("e4", "s1", "t3", 31, store.Location{}, "/abs/z.txt")
	outside.AgentID = "agent-1"
	recorded := agent("agent-1", "s1", 11, 32, "t2", "t3")
	recorded.Type, recorded.Name = "general-purpose", "builder"
	recorded.Totals = totals(1, usage("model-a", 1000, 100))
	reviewer := agent("agent-2", "s1", 12, 35, "t2")
	reviewer.Type = "Explore"
	reviewer.Totals = totals(1, usage("model-c", 9, 1))
	seed(t, st,
		session("s1", at(0)), session("s2", at(40)),
		first, second, turn("t3", "s1", store.TurnOpen, at(30)),
		edit("e1", "s1", "t1", 2, a, "x.go"),
		edit("e2", "s1", "t1", 3, a, "x.go"),
		sub, outside, recorded, reviewer,
	)
}

// turnsWithoutTotals: closed and failed turns with no totals, or totals
// older than the version asked for, come newest session first and in Seq
// order within a session; an open turn never does, and limit cuts the
// list.
func turnsWithoutTotals(t *testing.T, st store.Store) {
	ctx := context.Background()
	older := turn("t2", "s1", store.TurnFailed, at(2))
	older.Totals = totals(1)
	current := turn("t3", "s1", store.TurnClosed, at(3))
	current.Totals = totals(2)
	seed(t, st, session("s1", at(0)), session("s2", at(10)),
		turn("t1", "s1", store.TurnClosed, at(1)), older, current,
		turn("t4", "s1", store.TurnOpen, at(4)),
		turn("t5", "s1", store.TurnClosed, at(5)),
		turn("u1", "s2", store.TurnClosed, at(11)),
		turn("u2", "s2", store.TurnClosed, at(12)),
	)
	cases := []struct {
		name           string
		version, limit int
		want           []string
	}{
		{"version 2", 2, 0, []string{"u1", "u2", "t1", "t2", "t5"}},
		{"limit", 2, 3, []string{"u1", "u2", "t1"}},
		{"version 1", 1, 0, []string{"u1", "u2", "t1", "t5"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.TurnsWithoutTotals(ctx, c.version, c.limit)
			must(t, err)
			wantIDs(t, "TurnsWithoutTotals", ids(got, turnID), c.want...)
		})
	}
}
