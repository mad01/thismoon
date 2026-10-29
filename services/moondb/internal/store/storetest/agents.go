package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// agent is a fixture agent that ran from minute start to minute end under
// the turns named.
func agent(id, sessionID string, start, end int, turnIDs ...string) store.Agent {
	return store.Agent{
		ID: id, SessionID: sessionID, Type: "general-purpose", TurnIDs: turnIDs,
		StartedAt: at(start), EndedAt: at(end),
		Lines: store.AgentLines{Read: 10, Kept: 6},
	}
}

func agentID(a store.Agent) string { return a.ID }

// putAgentUpserts: a second Put replaces the agent whole, Agents lists a
// session's agents by start whatever order they arrived in, Agent loads
// one, an unknown id is ErrNotFound, and an agent cannot move to another
// session.
func putAgentUpserts(t *testing.T, st store.Store) {
	ctx := context.Background()
	first := agent("a1", "s1", 5, 9, "t1")
	first.Name = "reviewer"
	resumed := first
	resumed.EndedAt, resumed.TurnIDs = at(20), []string{"t1", "t2"}
	resumed.Lines = store.AgentLines{Read: 30, Kept: 18, Cut: 2}
	resumed.Totals = totals(1, usage("model-a", 7, 3))
	seed(t, st, session("s1", at(0)), session("s2", at(0)),
		first, agent("a0", "s1", 2, 4, "t1"), agent("b1", "s2", 1, 2), resumed)

	list, err := st.Agents(ctx, "s1")
	must(t, err)
	wantIDs(t, "Agents(s1)", ids(list, agentID), "a0", "a1")
	got, err := st.Agent(ctx, "a1")
	must(t, err)
	if !got.EndedAt.Equal(at(20)) || len(got.TurnIDs) != 2 || got.Lines.Cut != 2 ||
		got.Totals == nil || got.Name != "reviewer" {
		t.Errorf("Agent(a1) = %+v, want the resumed record whole", got)
	}
	if _, err := st.Agent(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Agent(nope) error = %v, want ErrNotFound", err)
	}
	moved := resumed
	moved.SessionID = "s2"
	if err := st.PutAgent(ctx, moved); err == nil {
		t.Error("PutAgent moved a1 to s2 without an error")
	}
	empty, err := st.Agents(ctx, "unknown")
	must(t, err)
	wantIDs(t, "Agents(unknown)", ids(empty, agentID))
	d, err := st.Session(ctx, "s1")
	must(t, err)
	wantIDs(t, "Session(s1).Agents", ids(d.Agents, agentID), "a0", "a1")
}

// agentsWithoutTotals: agents with no totals, or totals older than the
// version asked for, come newest session first and by start within a
// session, and limit cuts the list.
func agentsWithoutTotals(t *testing.T, st store.Store) {
	ctx := context.Background()
	older := agent("a2", "s1", 2, 3)
	older.Totals = totals(1)
	current := agent("a3", "s1", 3, 4)
	current.Totals = totals(2)
	seed(t, st, session("s1", at(0)), session("s2", at(10)),
		agent("a1", "s1", 1, 2), older, current,
		agent("b2", "s2", 12, 13), agent("b1", "s2", 11, 12))
	cases := []struct {
		name           string
		version, limit int
		want           []string
	}{
		{"version 2", 2, 0, []string{"b1", "b2", "a1", "a2"}},
		{"limit", 2, 3, []string{"b1", "b2", "a1"}},
		{"version 1", 1, 0, []string{"b1", "b2", "a1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.AgentsWithoutTotals(ctx, c.version, c.limit)
			must(t, err)
			wantIDs(t, "AgentsWithoutTotals", ids(got, agentID), c.want...)
		})
	}
}

// countAgentsWithoutBrief counts the agents an import could still give a
// brief: one with a brief, an internal agent, one that read no line and
// one that ended before since are left out.
func countAgentsWithoutBrief(t *testing.T, st store.Store) {
	ctx := context.Background()
	briefed := agent("a2", "s1", 2, 3)
	briefed.BriefChunk = "brief-sha"
	internal := agent("a3", "s1", 3, 4)
	internal.Type = store.InternalAgent
	unread := agent("a4", "s1", 4, 5)
	unread.Lines = store.AgentLines{}
	seed(t, st, session("s1", at(0)), session("s2", at(10)),
		agent("old", "s1", 0, 1), agent("a1", "s1", 1, 2), briefed, internal, unread,
		agent("b1", "s2", 11, 12))
	got, err := st.CountAgentsWithoutBrief(ctx, at(2))
	must(t, err)
	if got != 2 {
		t.Errorf("CountAgentsWithoutBrief(at 2) = %d, want 2 (a1, b1)", got)
	}
}
