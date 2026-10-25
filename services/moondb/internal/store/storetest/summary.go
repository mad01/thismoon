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
// agent, the last activity is the newest turn edge or edit, the last
// turn's status shows, the duration comes from the turn records, and the
// usage of every turn with totals merges by model.
func sessionSummary(t *testing.T, st store.Store) {
	ctx := context.Background()
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
	seed(t, st,
		session("s1", at(0)), session("s2", at(40)),
		first, second, turn("t3", "s1", store.TurnOpen, at(30)),
		edit("e1", "s1", "t1", 2, a, "x.go"),
		edit("e2", "s1", "t1", 3, a, "x.go"),
		sub, outside,
	)
	list, err := st.Sessions(ctx, store.SessionFilter{})
	must(t, err)
	wantIDs(t, "Sessions", ids(list, sessionID), "s2", "s1")
	if len(list) != 2 {
		return
	}
	got := list[1]
	want := store.SessionSummary{
		Session:   got.Session,
		Repos:     []string{repoA, repoB},
		Worktrees: []string{"/wt/a", "/wt/b"},
		TurnCount: 3, EditCount: 4, FileCount: 3, AgentCount: 2,
		Files: []store.FileKey{
			{
				Path: "/abs/z.txt",
			}, {Worktree: "/wt/a", Path: "x.go"}, {Worktree: "/wt/b", Path: "y.go"},
		},
		LastTurnOpen:   true,
		LastActivityAt: at(31),
		DurationMS:     (29 * time.Minute).Milliseconds(),
		Usage:          []store.ModelUsage{usage("model-a", 150, 15), usage("model-b", 5, 1)},
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
		empty.Repos == nil || empty.Worktrees == nil || !empty.LastActivityAt.Equal(at(40)) {
		t.Errorf("s2 summary = %+v, want zero counts, empty lists and its start as activity", empty)
	}
}

// turnsWithoutTotals: closed and failed turns with no totals, or totals
// older than the version asked for, come newest first; an open turn never
// does, and limit cuts the list.
func turnsWithoutTotals(t *testing.T, st store.Store) {
	ctx := context.Background()
	older := turn("t2", "s1", store.TurnFailed, at(2))
	older.Totals = totals(1)
	current := turn("t3", "s1", store.TurnClosed, at(3))
	current.Totals = totals(2)
	seed(t, st, session("s1", at(0)),
		turn("t1", "s1", store.TurnClosed, at(1)), older, current,
		turn("t4", "s1", store.TurnOpen, at(4)),
		turn("t5", "s1", store.TurnClosed, at(5)),
	)
	cases := []struct {
		name           string
		version, limit int
		want           []string
	}{
		{"version 2", 2, 0, []string{"t5", "t2", "t1"}},
		{"limit", 2, 2, []string{"t5", "t2"}},
		{"version 1", 1, 0, []string{"t5", "t1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.TurnsWithoutTotals(ctx, c.version, c.limit)
			must(t, err)
			wantIDs(t, "TurnsWithoutTotals", ids(got, turnID), c.want...)
		})
	}
}
