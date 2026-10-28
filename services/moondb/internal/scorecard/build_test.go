package scorecard

import (
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

func ptr[T any](v T) *T { return &v }

// session is a trace of two turns and two agents: the main conversation
// made 6 model calls and 9 tool calls, 2 failing; the agents 4 and 7, 1
// failing.
func session() trace.SessionTrace {
	tool := func(name string, ms int64, isErr bool) trace.Span {
		return trace.Span{Kind: trace.KindTool, Name: name, DurationMS: ms, IsError: isErr}
	}
	return trace.SessionTrace{
		SessionID: "s1",
		Turns: []trace.TurnTrace{
			{Spans: []trace.Span{
				{Kind: trace.KindModel, Name: "claude-opus-5-5"},
				tool("Read", 20, false), tool("csl_search", 300, false), tool("Bash", 900, true),
			}},
			{Spans: []trace.Span{tool("Read", 40, false), tool("Read", 10, true)}},
		},
		Totals: trace.Totals{
			WallMS: 100_000, ModelMS: 50_000, ToolMS: 20_000, HarnessMS: 5_000, UserMS: 20_000,
			IdleMS: 5_000, Tokens: trace.Tokens{Input: 10, Output: 500, CacheRead: 9000},
			Cost: ptr(3.0), ModelCalls: 6, ToolCalls: 9, ToolErrors: 2,
			TokensMain: &trace.Tokens{Input: 6, Output: 300, CacheRead: 5000},
			CostMain:   ptr(1.0), CostAgents: ptr(2.0),
			TokensAgents:    &trace.Tokens{Input: 4, Output: 200, CacheRead: 4000},
			AgentModelCalls: 4, AgentToolCalls: 7,
		},
		Agents: []trace.AgentRun{
			{
				AgentID: "cheap", Type: "Explore", Span: trace.Span{Cost: ptr(0.5)},
				Totals: trace.Totals{WallMS: 10_000, IdleMS: 1_000, Cost: ptr(0.5), ToolErrors: 1},
			},
			{
				AgentID: "dear", Type: "general-purpose", Span: trace.Span{Cost: ptr(1.5)},
				Totals: trace.Totals{WallMS: 30_000, Cost: ptr(1.5)},
			},
		},
	}
}

func TestBuildTotals(t *testing.T) {
	sc := Build(Input{Trace: session()})
	got := sc.Totals
	if got.Turns != 2 || got.ModelCalls != 10 || got.ToolCalls != 16 || got.ToolErrors != 3 {
		t.Errorf("totals = %+v, want 2 turns, 10 model calls, 16 tool calls, 3 failed", got)
	}
	if got.Main.ActiveMS != 75_000 || got.Main.ToolErrors != 2 || *got.Main.Cost != 1.0 ||
		got.Main.Tokens.Output != 300 {
		t.Errorf("main = %+v", got.Main)
	}
	if got.Agents.ActiveMS != 39_000 || got.Agents.ToolErrors != 1 || *got.Agents.Cost != 2.0 ||
		got.Agents.ModelCalls != 4 || got.Agents.Tokens.Output != 200 {
		t.Errorf("agents = %+v", got.Agents)
	}
	if got.Unpriced == nil {
		t.Error("unpriced = null, want an empty list")
	}
	if sc.Version != Version || sc.SessionID != "s1" || sc.Hints != nil || sc.Flags == nil {
		t.Errorf("scorecard = %+v", sc)
	}
}

func TestBuildTools(t *testing.T) {
	sc := Build(Input{Trace: session(), AgentSpans: map[string][]trace.Span{
		"cheap": {{Kind: trace.KindTool, Name: "csl_search", DurationMS: 100}},
	}})
	want := []ToolStat{
		{Name: "Read", Calls: 3, Errors: 1, DurationMS: 70},
		{Name: "csl_search", Calls: 2, DurationMS: 400, AgentCalls: 1},
		{Name: "Bash", Calls: 1, Errors: 1, DurationMS: 900},
	}
	if !slices.Equal(sc.Tools, want) {
		t.Errorf("tools = %+v, want %+v", sc.Tools, want)
	}
	var runs []string
	for _, r := range sc.Agents.Runs {
		runs = append(runs, r.ID)
	}
	if sc.Agents.Count != 2 || !slices.Equal(runs, []string{"dear", "cheap"}) {
		t.Errorf("agents = %+v, want both, costliest first", sc.Agents)
	}
}

func TestFlags(t *testing.T) {
	below := Scorecard{
		Reads:  ReadStats{Large: moondb.FlagLargeReads - 1},
		Search: SearchStats{Zero: moondb.FlagZeroSearches - 1},
		Agents: AgentStats{Ledger: &Ledger{}},
	}
	if got := flags(below); len(got) != 0 {
		t.Errorf("below every threshold: flags = %q", got)
	}

	at := Scorecard{
		Reads: ReadStats{Large: 2},
		Search: SearchStats{
			Zero: moondb.FlagZeroSearches, Truncated: moondb.FlagTruncatedSearches,
			Errors: moondb.FlagFailedSearches,
		},
		Totals: Totals{ToolErrors: moondb.FlagToolErrors},
		Agents: AgentStats{Ledger: &Ledger{PreCompact: 1, StopFailure: 1, PermissionDenied: 2}},
	}
	want := []string{
		"2 whole-file reads of 300 lines or more",
		"3 csl searches with zero results",
		"3 csl searches cut at the limit",
		"2 csl searches failed",
		"5 tool calls failed",
		"1 context compaction",
		"1 turn ended on an API error",
		"2 tool calls denied by a permission check",
	}
	if got := flags(at); !slices.Equal(got, want) {
		t.Errorf("flags =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTranscriptCountsSource(t *testing.T) {
	if got := (TranscriptCounts{Turns: 3, Agents: 1}).Source(); got.Status != StatusOK {
		t.Errorf("complete = %+v, want ok", got)
	}
	got := TranscriptCounts{
		Turns: 3, Agents: 2, MissingTurns: 1, CutHeads: 1, MissingAgents: 1, CutAgents: 2,
	}.Source()
	want := "1 of 3 turns have no transcript lines; 1 turn keeps only the tail of the lines; " +
		"1 of 2 agents have no agent transcript; 2 agent transcripts were cut short"
	if got.Status != StatusPartial || got.Detail != want {
		t.Errorf("source = %+v, want partial %q", got, want)
	}
}
