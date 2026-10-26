package trace

import (
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// fixtureAgent is the teammate of testdata/agent.jsonl: messaged in turn
// p1, idle, then messaged again in turn p2, stopped two seconds after its
// last line.
func fixtureAgent() store.Agent {
	return store.Agent{
		ID: "agent-w", SessionID: "s", Type: "general-purpose", Name: "worker",
		Model: "claude-opus-5-5", TurnIDs: []string{"p1", "p2"},
		StartedAt: t0, EndedAt: t0.Add(68 * time.Second),
		Lines: store.AgentLines{Read: 12, Kept: 12},
	}
}

// Costs at Opus 5.5 list prices, $4 per million input tokens and $20 per
// million output tokens, per message of the fixture.
const (
	costA1 = 10*4e-6 + 100*20e-6
	costA2 = 10*4e-6 + 50*20e-6
	costA3 = 20*4e-6 + 200*20e-6
	costA4 = 5*4e-6 + 10*20e-6
)

func buildFixtureAgent(t *testing.T) AgentTrace {
	t.Helper()
	edits := []store.Edit{edit("toolu_a2:0", "agent-w", "worker", 63*time.Second)}
	return BuildAgent(fixtureAgent(), fixture(t, "agent.jsonl"), edits, Prices{})
}

// TestBuildAgent: an agent's run spans its spawn to its stop, its lines
// make model and tool spans as a turn's do, and the wait between two
// messages is user time.
func TestBuildAgent(t *testing.T) {
	at := buildFixtureAgent(t)
	if at.Span.Kind != KindSubagent || at.Span.Name != "worker" || at.Span.DurationMS != 68000 {
		t.Errorf("Span = %+v, want the worker's 68s run", at.Span)
	}
	want := Totals{
		WallMS: 68000, ModelMS: 8600, ToolMS: 4000, HarnessMS: 2900, UserMS: 52500,
		ModelCalls: 4, ToolCalls: 2, ToolErrors: 1,
		Tokens: Tokens{Input: 45, Output: 360},
	}
	checkTotals(t, at.Totals, want, new(costA1+costA2+costA3+costA4))
	if at.Span.ModelCalls != 4 || at.Span.ToolCalls != 2 || at.Span.EditCount != 1 ||
		at.Span.Cost == nil || !near(*at.Span.Cost, *at.Totals.Cost) {
		t.Errorf(
			"Span counts = %+v, want 4 model calls, 2 tool calls, 1 edit and the cost",
			at.Span,
		)
	}
	checkSpans(t, at.Spans, []wantSpan{
		{kind: KindModel, id: "msg_a1", name: "claude-opus-5-5", durationMS: 1800},
		{kind: KindTool, id: "toolu_a1", name: "Bash", durationMS: 3000},
		{kind: KindModel, id: "msg_a2", name: "claude-opus-5-5", durationMS: 1900},
		{kind: KindModel, id: "msg_a3", name: "claude-opus-5-5", durationMS: 2900},
		{
			kind: KindTool, id: "toolu_a2", name: "Edit", durationMS: 1000, isError: true,
			editIDs: []string{"toolu_a2:0"},
		},
		{kind: KindModel, id: "msg_a4", name: "claude-opus-5-5", durationMS: 2000},
	})
	if at.Lines.Events != 12 || at.Reason != "" {
		t.Errorf("Lines = %+v, Reason = %q; want 12 events and no reason", at.Lines, at.Reason)
	}
}

// TestBuildAgentSegments splits the run by the turn each message came in
// under: the first segment starts at the spawn, the last ends at the stop,
// and each call counts in the segment it started in.
func TestBuildAgentSegments(t *testing.T) {
	segs := buildFixtureAgent(t).Segments
	if len(segs) != 2 {
		t.Fatalf("segments = %+v, want 2", segs)
	}
	first, second := segs[0], segs[1]
	if first.TurnID != "p1" || !first.Start.Equal(t0) || first.DurationMS != 7500 ||
		first.ModelCalls != 2 || first.ToolCalls != 1 || !near(deref1(first.Cost), costA1+costA2) {
		t.Errorf("first segment = %+v", first)
	}
	if second.TurnID != "p2" || !second.Start.Equal(t0.Add(time.Minute)) ||
		second.DurationMS != 8000 || second.ModelCalls != 2 || second.ToolCalls != 1 ||
		second.ToolErrors != 1 || !near(deref1(second.Cost), costA3+costA4) {
		t.Errorf("second segment = %+v", second)
	}
}

func deref1(f *float64) float64 {
	if f == nil {
		return -1
	}
	return *f
}

// TestBuildAgentEmpty: an agent without a transcript has a run span and a
// reason, and nothing under it.
func TestBuildAgentEmpty(t *testing.T) {
	at := BuildAgent(fixtureAgent(), nil, nil, Prices{})
	if at.Reason != reasonNoAgentChunk || at.Totals.WallMS != 68000 || len(at.Segments) != 0 ||
		at.Spans == nil {
		t.Errorf("BuildAgent(no lines) = %+v", at.AgentRun)
	}
}

// TestBuildSkipsAgentLines: a turn's trace reads past the lines a
// subagent wrote.
func TestBuildSkipsAgentLines(t *testing.T) {
	d := store.TurnDetail{Turn: store.Turn{ID: "p1", StartedAt: t0, EndedAt: after(time.Minute)}}
	if got := Build(d, fixture(t, "agent.jsonl"), Prices{}); got.Lines.Events != 0 {
		t.Errorf("Build read %d events from an agent transcript, want 0", got.Lines.Events)
	}
}

// TestWithAgents: the turn an agent ran under gets one span for the
// agent's work there, in place of the span its edits made and keeping
// their count, past the turn's end, and the turn's cost gains the agent's
// beside its own. A turn the agent did not run under is left as it is.
func TestWithAgents(t *testing.T) {
	run := buildFixtureAgent(t).AgentRun
	d := store.TurnDetail{
		Turn:  store.Turn{ID: "p1", StartedAt: t0, EndedAt: after(4 * time.Second)},
		Edits: []store.Edit{edit("toolu_x:0", "agent-w", "worker", 3*time.Second)},
	}
	own := Build(d, fixture(t, "plain.jsonl"), Prices{})
	got := WithAgents(own, []AgentRun{run})

	var sub []Span
	for _, s := range got.Spans {
		if s.Kind == KindSubagent {
			sub = append(sub, s)
		}
	}
	if len(sub) != 1 || sub[0].ID != "agent-w" || sub[0].EditCount != 1 ||
		sub[0].DurationMS != 7500 || sub[0].ModelCalls != 2 || sub[0].ToolCalls != 1 {
		t.Fatalf("subagent spans = %+v, want the worker's p1 segment with 1 edit", sub)
	}
	tot := got.Totals
	if !near(deref1(tot.CostMain), 0.01104) || !near(deref1(tot.CostAgents), costA1+costA2) ||
		!near(deref1(tot.Cost), 0.01104+costA1+costA2) || tot.AgentModelCalls != 2 ||
		tot.ModelCalls != 1 || tot.TokensAgents.Output != 150 {
		t.Errorf("Totals = %+v, want main and agent cost apart and summed", tot)
	}
	if own.Totals.CostAgents != nil || len(own.Spans) == len(got.Spans)+1 {
		t.Error("WithAgents changed the trace it was given")
	}
	other := own
	other.TurnID = "p9"
	if again := WithAgents(other, []AgentRun{run}); again.Totals.CostAgents != nil {
		t.Errorf("a turn the agent never ran under gained %v", deref1(again.Totals.CostAgents))
	}
}

// TestWithAgentWorkKeepsMainUnpriced: a model only a subagent used is
// unpriced in the turn's totals and not in the main conversation's part.
func TestWithAgentWorkKeepsMainUnpriced(t *testing.T) {
	main := Totals{Cost: new(0.5)}
	got := withAgentWork(main, []Totals{{Unpriced: []string{"agent-only"}}})
	if !slices.Equal(got.Unpriced, []string{"agent-only"}) || got.UnpricedMain != nil ||
		deref1(got.CostMain) != 0.5 {
		t.Errorf("Totals = %+v, want agent-only unpriced overall and not in main", got)
	}
}

// TestBuildSessionWithAgents: the session's cost adds every agent's whole
// run to the turns', its rankings take the agents' spans, and each turn
// carries the agent's work under it.
func TestBuildSessionWithAgents(t *testing.T) {
	at := buildFixtureAgent(t)
	p1 := Build(store.TurnDetail{Turn: store.Turn{
		ID: "p1", Seq: 1, StartedAt: t0, EndedAt: after(4 * time.Second),
	}}, fixture(t, "plain.jsonl"), Prices{})
	p1.Prompt = "build it"
	p2 := Build(store.TurnDetail{Turn: store.Turn{
		ID: "p2", Seq: 2, StartedAt: t0.Add(time.Minute), EndedAt: after(61 * time.Second),
	}}, nil, Prices{})
	st := BuildSession("s", []TurnTrace{p2, p1}, []AgentTrace{at})

	agents := costA1 + costA2 + costA3 + costA4
	if !near(deref1(st.Totals.Cost), 0.01104+agents) ||
		!near(deref1(st.Totals.CostAgents), agents) ||
		!near(deref1(st.Totals.CostMain), 0.01104) ||
		st.Totals.AgentToolCalls != 2 {
		t.Errorf("Totals = %+v, want the turns' and the agent's cost", st.Totals)
	}
	if len(st.Agents) != 1 || st.Agents[0].AgentID != "agent-w" {
		t.Errorf("Agents = %+v, want the worker", st.Agents)
	}
	if c := st.Turns[1].Totals.CostAgents; c == nil || !near(*c, costA3+costA4) {
		t.Errorf("turn p2 agent cost = %v, want its segment's", deref1(c))
	}
	models := CostliestModels(st, 2)
	if len(models) != 2 || models[0].Span.ID != "msg_plain_1" || models[1].Span.ID != "msg_a3" ||
		models[1].AgentID != "agent-w" || models[1].Agent != "worker" || models[1].Seq != 2 {
		t.Errorf("CostliestModels = %+v, want the main call, then the agent's p2 call", models)
	}
	tools := LongestTools(st, 1)
	if len(tools) != 1 || tools[0].Span.ID != "toolu_a1" || tools[0].Prompt != "build it" {
		t.Errorf("LongestTools = %+v, want the agent's Bash call in turn 1", tools)
	}
}

// TestAgentHelpers: a ranked call names its agent, runs sort costliest
// first, and turn ids map to their numbers.
func TestAgentHelpers(t *testing.T) {
	if got := (Ranked{}).Who(); got != MainAgent {
		t.Errorf("main call Who = %q", got)
	}
	if got := (Ranked{AgentID: "a", Agent: "worker"}).Who(); got != "worker" {
		t.Errorf("agent call Who = %q", got)
	}
	cheap := AgentRun{AgentID: "cheap", Span: Span{Cost: new(1.0)}}
	dear := AgentRun{AgentID: "dear", Span: Span{Cost: new(2.0)}}
	none := AgentRun{AgentID: "none"}
	runs := []AgentRun{none, cheap, dear}
	got := AgentsByCost(runs)
	if got[0].AgentID != "dear" || got[1].AgentID != "cheap" || got[2].AgentID != "none" ||
		runs[0].AgentID != "none" {
		t.Errorf("AgentsByCost = %v, want dear, cheap, none and the input kept", got)
	}
	st := SessionTrace{Turns: []TurnTrace{{TurnID: "p1", Seq: 1}, {TurnID: "p2", Seq: 2}}}
	if seqs := st.TurnSeqs([]string{"p2", "gone", "p1"}); len(seqs) != 2 || seqs[0] != 2 ||
		seqs[1] != 1 {
		t.Errorf("TurnSeqs = %v, want [2 1]", seqs)
	}
	st.Turns[0].Spans = []Span{
		{Kind: KindTool, EditIDs: []string{"e1", "e2"}},
		{Kind: KindSubagent, EditCount: 3},
	}
	if n := st.MainEdits(); n != 2 {
		t.Errorf("MainEdits = %d, want the tool call's 2 and none of the subagent's", n)
	}
}
