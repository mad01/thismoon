package scorecard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Input is everything a scorecard is built from.
type Input struct {
	// Trace is the session's trace (trace.BuildSession).
	Trace trace.SessionTrace
	// AgentSpans are each subagent's spans by agent id (AgentTrace.Spans):
	// a session trace lists the agents' runs without them.
	AgentSpans map[string][]trace.Span
	// Calls are the tool calls of the session's turns and agents
	// (ParseCalls), each agent's marked with its id.
	Calls []Call
	// Transcripts says whether every turn and agent had its lines.
	Transcripts Source
	// Events is what the events service said (ReadEvents).
	Events EventsResult
}

// Build computes a session's scorecard. It never fails: an input that was
// not there leaves its part empty or null, as Sources says.
func Build(in Input) Scorecard {
	sc := Scorecard{
		Version:   Version,
		SessionID: in.Trace.SessionID,
		Sources:   Sources{Transcripts: in.Transcripts, Events: in.Events.Source},
		Totals:    totals(in.Trace),
		Tools:     toolStats(in.Trace, in.AgentSpans),
		Search:    searchStats(in.Calls),
		Reads:     readStats(in.Calls),
		Hints:     hintStats(in.Events.Hints),
		Agents: AgentStats{
			Count: len(in.Trace.Agents), Runs: agentRuns(in.Trace.Agents),
			Ledger: in.Events.Ledger,
		},
	}
	sc.Flags = flags(sc)
	return sc
}

// totals reads the session trace's totals. A trace's call counts are the
// main conversation's, with the agents' apart; the agents' failed calls
// come from their runs.
func totals(st trace.SessionTrace) Totals {
	t := st.Totals
	main := Part{
		ActiveMS:   active(t),
		Tokens:     t.Tokens,
		Cost:       t.Cost,
		ModelCalls: t.ModelCalls,
		ToolCalls:  t.ToolCalls,
		ToolErrors: t.ToolErrors,
	}
	if t.TokensMain != nil {
		main.Tokens, main.Cost = *t.TokensMain, t.CostMain
	}
	agents := Part{
		ModelCalls: t.AgentModelCalls, ToolCalls: t.AgentToolCalls, Cost: t.CostAgents,
	}
	if t.TokensAgents != nil {
		agents.Tokens = *t.TokensAgents
	}
	for _, a := range st.Agents {
		agents.ActiveMS += active(a.Totals)
		agents.ToolErrors += a.Totals.ToolErrors
	}
	unpriced := t.Unpriced
	if unpriced == nil {
		unpriced = []string{}
	}
	return Totals{
		Turns: len(st.Turns), WallMS: t.WallMS, ModelMS: t.ModelMS, ToolMS: t.ToolMS,
		HarnessMS: t.HarnessMS, UserMS: t.UserMS, IdleMS: t.IdleMS,
		Tokens: t.Tokens, Cost: t.Cost, Unpriced: unpriced,
		ModelCalls: main.ModelCalls + agents.ModelCalls,
		ToolCalls:  main.ToolCalls + agents.ToolCalls,
		ToolErrors: main.ToolErrors + agents.ToolErrors,
		Main:       main, Agents: agents,
	}
}

// active is the time work ran: the wall time less the time waiting for
// input and the idle time, never below zero.
func active(t trace.Totals) int64 {
	return max(t.WallMS-t.UserMS-t.IdleMS, 0)
}

// toolStats sums the tool spans of the turns and the agents by tool name,
// most calls first, then longest, then by name.
func toolStats(st trace.SessionTrace, agentSpans map[string][]trace.Span) []ToolStat {
	byName := map[string]*ToolStat{}
	add := func(s trace.Span, agent bool) {
		if s.Kind != trace.KindTool {
			return
		}
		ts := byName[s.Name]
		if ts == nil {
			ts = &ToolStat{Name: s.Name}
			byName[s.Name] = ts
		}
		ts.Calls++
		ts.Errors += count(s.IsError)
		ts.DurationMS += s.DurationMS
		ts.AgentCalls += count(agent)
	}
	for _, tt := range st.Turns {
		for _, s := range tt.Spans {
			add(s, false)
		}
	}
	for _, spans := range agentSpans {
		for _, s := range spans {
			add(s, true)
		}
	}
	out := make([]ToolStat, 0, len(byName))
	for _, ts := range byName {
		out = append(out, *ts)
	}
	slices.SortFunc(out, func(a, b ToolStat) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), cmp.Compare(b.DurationMS, a.DurationMS),
			cmp.Compare(a.Name, b.Name))
	})
	return out
}

// agentRuns lists the subagents' runs costliest first.
func agentRuns(runs []trace.AgentRun) []AgentRun {
	out := make([]AgentRun, 0, len(runs))
	for _, a := range trace.AgentsByCost(runs) {
		out = append(out, AgentRun{
			ID: a.AgentID, Type: a.Type, Name: a.Name, Model: a.Model,
			DurationMS: a.Span.DurationMS, Tokens: a.Totals.Tokens, Cost: a.Totals.Cost,
			ModelCalls: a.Totals.ModelCalls, ToolCalls: a.Totals.ToolCalls,
			ToolErrors: a.Totals.ToolErrors,
		})
	}
	return out
}

// TranscriptCounts counts what a session's transcript lines lack, for the
// transcripts source.
type TranscriptCounts struct {
	Turns, Agents int
	// MissingTurns and MissingAgents have no transcript lines in the store.
	MissingTurns, MissingAgents int
	// CutHeads are the turns whose lines start inside the turn: the size
	// cap kept only their tail.
	CutHeads int
	// CutAgents are the agent transcripts whose head the size cap cut, or
	// whose read stopped before the file's end.
	CutAgents int
}

// Source says whether every turn and agent had all its lines.
func (c TranscriptCounts) Source() Source {
	var parts []string
	if c.MissingTurns > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d turns have no transcript lines",
			c.MissingTurns, c.Turns))
	}
	if c.CutHeads > 0 {
		parts = append(parts, fmt.Sprintf("%d %s only the tail of the lines",
			c.CutHeads, noun(c.CutHeads, "turn keeps", "turns keep")))
	}
	if c.MissingAgents > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d agents have no agent transcript",
			c.MissingAgents, c.Agents))
	}
	if c.CutAgents > 0 {
		parts = append(parts, fmt.Sprintf("%d agent %s cut short",
			c.CutAgents, noun(c.CutAgents, "transcript was", "transcripts were")))
	}
	if len(parts) == 0 {
		return Source{Status: StatusOK}
	}
	return Source{Status: StatusPartial, Detail: strings.Join(parts, "; ")}
}
