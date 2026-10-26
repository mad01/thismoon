package query

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// traceLines is turn p1's transcript: one model call asking for an Edit,
// and its result. The million input tokens on claude-opus-5-5 cost $4.
var traceLines = strings.Join([]string{
	`{"type":"api-request","timestamp":"2026-10-25T12:00:01Z"}`,
	`{"type":"assistant","timestamp":"2026-10-25T12:00:31Z","message":{"id":"m1",` +
		`"model":"claude-opus-5-5","content":[{"type":"tool_use","id":"tu1","name":"Edit"}],` +
		`"usage":{"input_tokens":1000000,"output_tokens":0}}}`,
	`{"type":"user","timestamp":"2026-10-25T12:01:00Z",` +
		`"message":{"content":[{"type":"tool_result","tool_use_id":"tu1"}]}}`,
}, "\n")

// traceFixture is newFixture with p1 closed over traceLines and a second
// turn p2 without a transcript, eight minutes after p1 ended.
func traceFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	end1, end2 := at(2), at(11)
	f.seed(
		store.Turn{
			ID: "p1", SessionID: "s1", Status: store.TurnClosed, StartedAt: at(0), EndedAt: &end1,
			PromptChunk: f.chunk(
				"\n  fix the bug\nwith detail",
			), TranscriptChunk: f.chunk(traceLines),
		},
		store.Turn{
			ID:        "p2",
			SessionID: "s1",
			Status:    store.TurnClosed,
			StartedAt: at(10),
			EndedAt:   &end2,
		},
		store.Edit{ID: "tu1:0", ToolUseID: "tu1", SessionID: "s1", TurnID: "p1", At: at(0)},
	)
	return f
}

func TestSessionTrace(t *testing.T) {
	f := traceFixture(t)
	got, err := f.svc.SessionTrace(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 2 || got.Turns[0].TurnID != "p1" || got.Turns[1].TurnID != "p2" {
		t.Fatalf("turns = %+v, want p1 then p2", got.Turns)
	}
	p1, p2 := got.Turns[0], got.Turns[1]
	if p1.Prompt != "fix the bug" || p2.Prompt != "" {
		t.Errorf("prompts = %q, %q; want the first line of p1's and none", p1.Prompt, p2.Prompt)
	}
	if p2.Reason == "" {
		t.Error("p2 has no transcript, so its trace should say why it is empty")
	}
	tool := p1.Spans[1]
	if tool.Kind != trace.KindTool || !slices.Equal(tool.EditIDs, []string{"tu1:0"}) {
		t.Errorf("tool span = %+v, want tu1 with its edit", tool)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].DurationMS != (8*time.Minute).Milliseconds() {
		t.Errorf("gaps = %+v, want the 8 minutes between the turns", got.Gaps)
	}
	if c := got.Totals.Cost; c == nil || math.Abs(*c-4) > 1e-9 {
		t.Errorf("session cost = %v, want 4", c)
	}
	if got.Totals.WallMS != (11 * time.Minute).Milliseconds() {
		t.Errorf("session wall = %dms, want 11 minutes", got.Totals.WallMS)
	}
}

func TestSessionTraceUnknownSession(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.SessionTrace(context.Background(), "nope"); !errors.Is(
		err,
		store.ErrNotFound,
	) {
		t.Errorf("err = %v, want store.ErrNotFound", err)
	}
}

func TestTurnTraceCacheSeesLateEdits(t *testing.T) {
	f := traceFixture(t)
	ctx := context.Background()
	turnTrace := func() trace.TurnTrace {
		t.Helper()
		d, err := f.st.Turn(ctx, "p1")
		if err != nil {
			t.Fatal(err)
		}
		tt, err := f.svc.TurnTrace(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		return tt
	}
	turnTrace()
	turnTrace()
	if n := f.svc.traces().count(); n != 1 {
		t.Errorf("cache holds %d traces after reading one turn twice, want 1", n)
	}
	f.seed(store.Edit{ID: "tu1:1", ToolUseID: "tu1", SessionID: "s1", TurnID: "p1", At: at(1)})
	if ids := turnTrace().Spans[1].EditIDs; !slices.Equal(ids, []string{"tu1:0", "tu1:1"}) {
		t.Errorf("edit ids after a late edit = %v, want both", ids)
	}
}

func TestTurnTraceMissingChunk(t *testing.T) {
	f := newFixture(t)
	d := store.TurnDetail{Turn: store.Turn{ID: "p9", TranscriptChunk: strings.Repeat("ab", 32)}}
	if _, err := f.svc.TurnTrace(context.Background(), d); err == nil {
		t.Error("a transcript chunk the store lacks should be an error")
	}
}

func TestTraceCacheEvictsLeastRecentlyRead(t *testing.T) {
	c := newTraceCache[trace.TurnTrace](2)
	c.put("a", trace.TurnTrace{TurnID: "a"})
	c.put("b", trace.TurnTrace{TurnID: "b"})
	if _, ok := c.get("a"); !ok {
		t.Fatal("a missing before any eviction")
	}
	c.put("c", trace.TurnTrace{TurnID: "c"})
	if _, ok := c.get("b"); ok {
		t.Error("b was read least recently and should be gone")
	}
	for _, key := range []string{"a", "c"} {
		if tt, ok := c.get(key); !ok || tt.TurnID != key {
			t.Errorf("get(%q) = %v, %v", key, tt.TurnID, ok)
		}
	}
	if c.count() != 2 {
		t.Errorf("count = %d, want 2", c.count())
	}
}

// agentLines is agent a1's transcript: a teammate messaged under p1 whose
// one model call reads half a million input tokens, $2 on
// claude-opus-5-5, and makes the edit tu9.
var agentLines = strings.Join([]string{
	`{"type":"user","isSidechain":true,"promptId":"p1","timestamp":"2026-10-25T12:00:05Z",` +
		`"message":{"content":"go"}}`,
	`{"type":"assistant","isSidechain":true,"timestamp":"2026-10-25T12:00:20Z","message":` +
		`{"id":"am1","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"tu9",` +
		`"name":"Write"}],"usage":{"input_tokens":500000,"output_tokens":0}}}`,
	`{"type":"user","isSidechain":true,"promptId":"p1","timestamp":"2026-10-25T12:00:30Z",` +
		`"message":{"content":[{"type":"tool_result","tool_use_id":"tu9"}]}}`,
}, "\n")

// agentFixture is traceFixture with agent a1, which ran under p1 past its
// end and made one edit.
func agentFixture(t *testing.T) *fixture {
	t.Helper()
	f := traceFixture(t)
	f.seed(
		store.Agent{
			ID: "a1", SessionID: "s1", Type: "general-purpose", Name: "builder",
			TurnIDs: []string{"p1"}, StartedAt: at(0), EndedAt: at(4),
			TranscriptChunk: f.chunk(agentLines),
		},
		store.Edit{
			ID: "tu9:0", ToolUseID: "tu9", SessionID: "s1", TurnID: "p1", AgentID: "a1",
			AgentType: "builder", At: at(0),
		},
	)
	return f
}

// TestSessionTraceWithAgents: the session's cost adds the agent's run to
// the turns', the agent's work shows under the turn it ran in, and its
// trace is cached like a turn's.
func TestSessionTraceWithAgents(t *testing.T) {
	f := agentFixture(t)
	got, err := f.svc.SessionTrace(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Totals.Cost; c == nil || math.Abs(*c-6) > 1e-9 {
		t.Errorf("session cost = %v, want 4 for the turns and 2 for the agent", c)
	}
	if len(got.Agents) != 1 || got.Agents[0].Span.EditCount != 1 {
		t.Errorf("agents = %+v, want a1 with its edit", got.Agents)
	}
	i := slices.IndexFunc(got.Turns[0].Spans, func(s trace.Span) bool {
		return s.Kind == trace.KindSubagent
	})
	if i < 0 || got.Turns[0].Spans[i].ModelCalls != 1 || got.Turns[0].Spans[i].Name != "builder" {
		t.Errorf("p1 spans = %+v, want the builder's work with its call", got.Turns[0].Spans)
	}
	if _, err := f.svc.SessionTrace(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if n := f.svc.agentTraces.count(); n != 1 {
		t.Errorf("agent cache holds %d traces after two reads, want 1", n)
	}
}

// TestAgentTraceByID loads one agent with the edits it made; an unknown id
// is ErrNotFound.
func TestAgentTraceByID(t *testing.T) {
	f := agentFixture(t)
	a, at, err := f.svc.AgentTraceByID(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "builder" || at.Totals.ModelCalls != 1 ||
		!slices.Equal(at.Spans[1].EditIDs, []string{"tu9:0"}) {
		t.Errorf("AgentTraceByID = %+v, %+v", a, at.AgentRun)
	}
	if _, _, err := f.svc.AgentTraceByID(context.Background(), "nope"); !errors.Is(
		err, store.ErrNotFound) {
		t.Errorf("err = %v, want store.ErrNotFound", err)
	}
}

// TestTurnAgents lists the agents that ran under a turn, and none for a
// turn no agent ran under.
func TestTurnAgents(t *testing.T) {
	f := agentFixture(t)
	ctx := context.Background()
	for _, c := range []struct {
		turn string
		want int
	}{{"p1", 1}, {"p2", 0}} {
		d, err := f.st.Turn(ctx, c.turn)
		if err != nil {
			t.Fatal(err)
		}
		runs, err := f.svc.TurnAgents(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != c.want {
			t.Errorf("TurnAgents(%s) = %d runs, want %d", c.turn, len(runs), c.want)
		}
	}
}
