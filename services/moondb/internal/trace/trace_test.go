package trace

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// t0 is the start of every fixture turn; the fixtures' timestamps count
// from it.
var t0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func after(d time.Duration) *time.Time {
	t := t0.Add(d)
	return &t
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func edit(id, agentID, agentType string, at time.Duration) store.Edit {
	toolUse, _, _ := strings.Cut(id, ":")
	return store.Edit{
		ID: id, ToolUseID: toolUse, AgentID: agentID, AgentType: agentType, At: t0.Add(at),
	}
}

// wantSpan is what a test checks of one child span.
type wantSpan struct {
	kind       Kind
	id         string
	name       string
	durationMS int64
	isError    bool
	unfinished bool
	editIDs    []string
	editCount  int
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name       string
		file       string // "" for a turn without a transcript chunk
		status     store.TurnStatus
		ended      *time.Time
		edits      []store.Edit
		wantTotals Totals
		wantLines  Lines
		wantReason string
		wantSpans  []wantSpan
		wantCost   *float64 // nil means no total cost
	}{
		{
			name:   "plain turn",
			file:   "plain.jsonl",
			status: store.TurnClosed,
			ended:  after(4 * time.Second),
			wantTotals: Totals{
				WallMS: 4000, ModelMS: 3000, HarnessMS: 600, UserMS: 400, ModelCalls: 1,
				Tokens: Tokens{
					Input:        10,
					CacheWrite5m: 1000,
					CacheRead:    20000,
					Output:       100,
					Thinking:     40,
				},
			},
			wantLines: Lines{Read: 5, Events: 4},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_plain_1", name: "claude-opus-5-5", durationMS: 3000},
			},
			// 10 input at $4, 1000 5m writes at $5, 20000 reads at $0.20, 100 out at $20
			wantCost: new(0.01104),
		},
		{
			// A teammate message 30s after the model finished starts the loop
			// again under the same prompt id: the wait after the Stop hooks is
			// user time. No Stop line follows the second call, so the tail to
			// the turn's end is harness time.
			name:   "a turn resumed after Stop counts the wait as user time",
			file:   "resumed.jsonl",
			status: store.TurnClosed,
			ended:  after(35 * time.Second),
			wantTotals: Totals{
				WallMS: 35000, ModelMS: 3300, HarnessMS: 1800, UserMS: 29900, ModelCalls: 2,
				Tokens: Tokens{Input: 200, Output: 100},
			},
			wantLines: Lines{Read: 7, Events: 7},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_resumed_1", name: "claude-opus-5-5", durationMS: 1500},
				{kind: KindModel, id: "msg_resumed_2", name: "claude-opus-5-5", durationMS: 1800},
			},
			// 200 input at $4 and 100 output at $20
			wantCost: new(0.0028),
		},
		{
			name:   "three tool calls, one failing, and a subagent",
			file:   "tools.jsonl",
			status: store.TurnClosed,
			ended:  after(12500 * time.Millisecond),
			edits: []store.Edit{
				edit("toolu_bash:0", "", "", 5*time.Second),
				edit("toolu_sub_1:0", "agent-a", "builder", 5*time.Second),
				edit("toolu_sub_2:0", "agent-a", "builder", 7*time.Second),
			},
			wantTotals: Totals{
				WallMS: 12500, ModelMS: 5700, ToolMS: 5600, HarnessMS: 1200,
				ModelCalls: 3, ToolCalls: 3, ToolErrors: 1,
				Tokens: Tokens{Input: 9, CacheRead: 6000, Output: 180},
			},
			wantLines: Lines{Read: 13, Events: 12},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_tools_1", name: "claude-opus-5-5", durationMS: 2800},
				{kind: KindTool, id: "toolu_read", name: "Read", durationMS: 200},
				{
					kind: KindTool, id: "toolu_bash", name: "Bash", durationMS: 5000,
					editIDs: []string{"toolu_bash:0"},
				},
				{
					kind:       KindSubagent,
					id:         "agent-a",
					name:       "builder",
					durationMS: 2000,
					editCount:  2,
				},
				{kind: KindModel, id: "msg_tools_2", name: "claude-opus-5-5", durationMS: 1700},
				{kind: KindTool, id: "toolu_edit", name: "Edit", durationMS: 400, isError: true},
				{kind: KindModel, id: "msg_tools_3", name: "claude-opus-5-5", durationMS: 1400},
			},
			wantCost: new(0.004836),
		},
		{
			name:   "a streamed message in two lines is one call with the last usage",
			file:   "streamed.jsonl",
			status: store.TurnClosed,
			ended:  after(4200 * time.Millisecond),
			wantTotals: Totals{
				WallMS: 4200, ModelMS: 3500, HarnessMS: 700, ModelCalls: 1,
				Tokens: Tokens{
					Input: 5, CacheWrite5m: 1000, CacheWrite1h: 2000, Output: 60, Thinking: 20,
				},
			},
			wantLines: Lines{Read: 4, Events: 4},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_stream_1", name: "claude-fable-5-1", durationMS: 3500},
			},
			wantCost: new(0.05555),
		},
		{
			name:   "an unknown model has tokens and no cost",
			file:   "unknown.jsonl",
			status: store.TurnClosed,
			ended:  after(2 * time.Second),
			wantTotals: Totals{
				WallMS: 2000, ModelMS: 1500, HarnessMS: 500, ModelCalls: 2,
				Tokens: Tokens{Input: 100, Output: 20}, Unpriced: []string{"claude-nova-9"},
			},
			wantLines: Lines{Read: 4, Events: 4},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_unknown_1", name: "claude-nova-9", durationMS: 1000},
				{kind: KindModel, id: "msg_synthetic_1", name: "<synthetic>", durationMS: 500},
			},
		},
		{
			name:   "a turn whose only model line billed nothing has no cost",
			file:   "synthetic.jsonl",
			status: store.TurnClosed,
			ended:  after(2 * time.Second),
			wantTotals: Totals{
				WallMS: 2000, ModelMS: 500, HarnessMS: 1500, ModelCalls: 1,
			},
			wantLines: Lines{Read: 3, Events: 3},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_synthetic_only", name: "<synthetic>", durationMS: 500},
			},
		},
		{
			name:   "broken lines are skipped and counted",
			file:   "broken.jsonl",
			status: store.TurnClosed,
			ended:  after(4 * time.Second),
			wantTotals: Totals{
				WallMS: 4000, ModelMS: 2000, HarnessMS: 2000, ModelCalls: 1, ToolCalls: 1,
				Tokens: Tokens{Input: 1, Output: 7},
			},
			wantLines: Lines{Read: 5, Events: 3, Skipped: 2},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_broken_1", name: "claude-opus-5-5", durationMS: 2000},
				{
					kind:       KindTool,
					id:         "toolu_broken",
					name:       "Grep",
					durationMS: 1000,
					unfinished: true,
				},
			},
			wantCost: new(0.000144),
		},
		{
			// The size cap kept the last hour's tail: the first kept line is a
			// tool result whose call was in the cut head. The hour before it is
			// harness time, not tool time, and the lost call has no span.
			name:   "a cut head that starts on a tool result counts as harness time",
			file:   "truncated.jsonl",
			status: store.TurnClosed,
			ended:  after(time.Hour + 4*time.Second),
			wantTotals: Totals{
				WallMS: 3_604_000, ModelMS: 2100, ToolMS: 500, HarnessMS: 3_600_500, UserMS: 900,
				ModelCalls: 2, ToolCalls: 1,
				Tokens: Tokens{Input: 2, Output: 10},
			},
			wantLines: Lines{Read: 7, Events: 7, CutHead: true},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_trunc_1", name: "claude-opus-5-5", durationMS: 1700},
				{kind: KindTool, id: "toolu_trunc", name: "Grep", durationMS: 500},
				{kind: KindModel, id: "msg_trunc_2", name: "claude-opus-5-5", durationMS: 400},
			},
			// 2 input at $4 and 10 output at $20
			wantCost: new(0.000208),
		},
		{
			// The first kept line ends a message whose start was cut: its span
			// opens at that line, not at the turn's start 90 minutes earlier.
			name:   "a cut head that starts on an end_turn line counts as harness time",
			file:   "truncated_end_turn.jsonl",
			status: store.TurnClosed,
			ended:  after(90*time.Minute + time.Second),
			wantTotals: Totals{
				WallMS: 5_401_000, HarnessMS: 5_400_200, UserMS: 800, ModelCalls: 1,
				Tokens: Tokens{Input: 10, Output: 50},
			},
			wantLines: Lines{Read: 2, Events: 2, CutHead: true},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_cut_1", name: "claude-opus-5-5", durationMS: 0},
			},
			// 10 input at $4 and 50 output at $20
			wantCost: new(0.00104),
		},
		{
			// 1000.4ms of harness and 1000.7ms of model time in a 2001.1ms
			// turn: each kind alone truncates to 1000ms, one short of the wall.
			name:   "sub-millisecond gaps still add up to the wall time",
			file:   "fractions.jsonl",
			status: store.TurnClosed,
			ended:  after(2001100 * time.Microsecond),
			wantTotals: Totals{
				WallMS: 2001, ModelMS: 1000, HarnessMS: 1001, ModelCalls: 1,
				Tokens: Tokens{Input: 1, Output: 1},
			},
			wantLines: Lines{Read: 3, Events: 3},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_frac_1", name: "claude-opus-5-5", durationMS: 1000},
			},
			wantCost: new(0.000024),
		},
		{
			name:   "US-only inference bills 1.1 times the list price",
			file:   "geo.jsonl",
			status: store.TurnClosed,
			ended:  after(2 * time.Second),
			wantTotals: Totals{
				WallMS: 2000, ModelMS: 1500, HarnessMS: 500, ModelCalls: 1,
				Tokens: Tokens{Input: 1000, CacheRead: 10_000, Output: 100},
			},
			wantLines: Lines{Read: 3, Events: 3},
			wantSpans: []wantSpan{
				{kind: KindModel, id: "msg_geo_1", name: "claude-opus-5-5", durationMS: 1500},
			},
			// (1000 input at $4, 10000 reads at $0.20, 100 output at $20) x 1.1
			wantCost: new(0.0088),
		},
		{
			name:       "a closed turn without a transcript chunk",
			status:     store.TurnClosed,
			ended:      after(time.Minute),
			wantTotals: Totals{WallMS: 60000},
			wantReason: reasonNoChunk,
		},
		{
			name:       "an open turn without a transcript chunk",
			status:     store.TurnOpen,
			wantReason: reasonOpen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []byte
			if tt.file != "" {
				lines = fixture(t, tt.file)
			}
			d := store.TurnDetail{
				Turn: store.Turn{
					ID: "turn-1", Seq: 3, Status: tt.status, StartedAt: t0, EndedAt: tt.ended,
				},
				Edits: tt.edits,
			}
			got := Build(d, lines, Prices{})
			if got.TurnID != "turn-1" || got.Seq != 3 || got.Span.Kind != KindTurn {
				t.Errorf("turn = %s #%d %s", got.TurnID, got.Seq, got.Span.Kind)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Lines != tt.wantLines {
				t.Errorf("Lines = %+v, want %+v", got.Lines, tt.wantLines)
			}
			checkTotals(t, got.Totals, tt.wantTotals, tt.wantCost)
			checkSpans(t, got.Spans, tt.wantSpans)
			checkGaps(t, got)
		})
	}
}

func checkTotals(t *testing.T, got, want Totals, wantCost *float64) {
	t.Helper()
	gotCost := got.Cost
	got.Cost = nil
	if !totalsEqual(got, want) {
		t.Errorf("Totals = %+v\nwant     %+v", got, want)
	}
	switch {
	case wantCost == nil && gotCost != nil:
		t.Errorf("Cost = %v, want none", *gotCost)
	case wantCost != nil && (gotCost == nil || !near(*gotCost, *wantCost)):
		t.Errorf("Cost = %v, want %v", deref(gotCost), *wantCost)
	}
}

func totalsEqual(a, b Totals) bool {
	return a.WallMS == b.WallMS && a.ModelMS == b.ModelMS && a.ToolMS == b.ToolMS &&
		a.HarnessMS == b.HarnessMS && a.UserMS == b.UserMS && a.Tokens == b.Tokens &&
		slices.Equal(a.Unpriced, b.Unpriced) && a.ModelCalls == b.ModelCalls &&
		a.ToolCalls == b.ToolCalls && a.ToolErrors == b.ToolErrors
}

func checkSpans(t *testing.T, got []Span, want []wantSpan) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d child spans, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Kind != w.kind || g.ID != w.id || g.Name != w.name || g.DurationMS != w.durationMS ||
			g.IsError != w.isError || g.Unfinished != w.unfinished ||
			!slices.Equal(g.EditIDs, w.editIDs) || g.EditCount != w.editCount {
			t.Errorf(
				"span %d = %s %s %q %dms error=%v unfinished=%v edits=%v count=%d\nwant      %+v",
				i,
				g.Kind,
				g.ID,
				g.Name,
				g.DurationMS,
				g.IsError,
				g.Unfinished,
				g.EditIDs,
				g.EditCount,
				w,
			)
		}
		if g.Kind == KindModel && g.Tokens == nil {
			t.Errorf("model span %s has no tokens", g.ID)
		}
	}
}

// checkGaps holds every trace to the partition contract: the gaps run
// back to back from the turn's start to its end, neighbours differ in kind,
// and they sum to the wall time, as do the totals by kind.
func checkGaps(t *testing.T, got TurnTrace) {
	t.Helper()
	if len(got.Gaps) == 0 {
		return
	}
	tot := got.Totals
	if sum := tot.ModelMS + tot.ToolMS + tot.HarnessMS + tot.UserMS; sum != tot.WallMS {
		t.Errorf("model, tool, harness and user time sum to %dms; wall is %dms", sum, tot.WallMS)
	}
	prev := got.Span.Start
	var sum time.Duration
	for i, g := range got.Gaps {
		if !g.Start.Equal(prev) || !g.End.After(g.Start) {
			t.Errorf("gap %d %s runs %s to %s after %s", i, g.Kind, g.Start, g.End, prev)
		}
		if i > 0 && got.Gaps[i-1].Kind == g.Kind {
			t.Errorf("gaps %d and %d are both %s; they should merge", i-1, i, g.Kind)
		}
		sum += g.End.Sub(g.Start)
		prev = g.End
	}
	if !prev.Equal(got.Span.End) || sum.Milliseconds() != got.Totals.WallMS {
		t.Errorf("gaps end at %s and sum to %s; turn ends at %s after %dms",
			prev, sum, got.Span.End, got.Totals.WallMS)
	}
}

func TestBuildToolsGapOrder(t *testing.T) {
	d := store.TurnDetail{
		Turn: store.Turn{ID: "t", StartedAt: t0, EndedAt: after(12500 * time.Millisecond)},
	}
	got := Build(d, fixture(t, "tools.jsonl"), Prices{})
	var kinds []GapKind
	for _, g := range got.Gaps {
		kinds = append(kinds, g.Kind)
	}
	want := []GapKind{
		GapHarness, GapModel, GapTool, GapModel, GapTool, GapHarness,
		GapModel, GapTool, GapHarness, GapModel, GapHarness,
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("gap kinds = %v\nwant %v", kinds, want)
	}
}

func TestBuildClampsLinesBeforeTheTurn(t *testing.T) {
	// The prompt hook is async, so the turn can start after its first
	// lines were written; those lines count from the turn's start.
	d := store.TurnDetail{
		Turn: store.Turn{ID: "t", StartedAt: t0.Add(time.Second), EndedAt: after(4 * time.Second)},
	}
	got := Build(d, fixture(t, "plain.jsonl"), Prices{})
	// The Stop line at 3.6s ends 100ms of Stop hooks; the rest is user time.
	tot := got.Totals
	if got.Span.DurationMS != 3000 || tot.ModelMS != 2500 || tot.HarnessMS != 100 ||
		tot.UserMS != 400 {
		t.Errorf("turn %dms, model %dms, harness %dms, user %dms; want 3000, 2500, 100, 400",
			got.Span.DurationMS, tot.ModelMS, tot.HarnessMS, tot.UserMS)
	}
	if m := got.Spans[0]; !m.Start.Equal(d.StartedAt) || m.DurationMS != 2500 {
		t.Errorf(
			"model span starts %s and lasts %dms; want the turn start and 2500ms",
			m.Start,
			m.DurationMS,
		)
	}
}

func TestBuildClampsSubagentSpans(t *testing.T) {
	// The Bash hook is async, so an edit can be recorded after Stop; a
	// subagent span still ends with its turn, and one never starts before it.
	d := store.TurnDetail{
		Turn: store.Turn{ID: "t", StartedAt: t0, EndedAt: after(12500 * time.Millisecond)},
		Edits: []store.Edit{
			edit("toolu_a1:0", "agent-a", "builder", 5*time.Second),
			edit("toolu_a2:0", "agent-a", "builder", 40*time.Second),
			edit("toolu_b1:0", "agent-b", "", -2*time.Second),
			edit("toolu_b2:0", "agent-b", "", time.Second),
		},
	}
	got := Build(d, fixture(t, "tools.jsonl"), Prices{})
	want := map[string][2]time.Time{
		"agent-a": {t0.Add(5 * time.Second), t0.Add(12500 * time.Millisecond)},
		"agent-b": {t0, t0.Add(time.Second)},
	}
	for _, s := range got.Spans {
		if s.Kind != KindSubagent {
			continue
		}
		w, ok := want[s.ID]
		if !ok || !s.Start.Equal(w[0]) || !s.End.Equal(w[1]) ||
			s.DurationMS != w[1].Sub(w[0]).Milliseconds() {
			t.Errorf("subagent %s runs %s to %s (%dms); want %v",
				s.ID, s.Start, s.End, s.DurationMS, w)
		}
		delete(want, s.ID)
	}
	if len(want) > 0 {
		t.Errorf("no span for %v", want)
	}
}

func TestPricesLookup(t *testing.T) {
	tests := []struct {
		model     string
		wantInput float64 // 0 means no price
	}{
		{"claude-opus-5-5", 4},
		{"claude-opus-5-5-20261101", 4}, // not claude-opus-5's $5
		{"claude-opus-4-5-20251101", 5},
		{"claude-sonnet-4-20250514", 3},
		{"us.anthropic.claude-sonnet-4-6", 3},
		{"claude-fable-5-1[1m]", 10},
		{"claude-opus-4-9", 0},
		{"claude-opus-5-6", 0}, // a new version is not claude-opus-5 with a date
		{"claude-opus-5-55", 0},
		{"<synthetic>", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			price, ok := Prices{}.Lookup(tt.model)
			if ok != (tt.wantInput != 0) || price.Input != tt.wantInput {
				t.Errorf(
					"Lookup(%q) = %v, %v; want input %v",
					tt.model,
					price.Input,
					ok,
					tt.wantInput,
				)
			}
		})
	}
}

func TestPricesCost(t *testing.T) {
	overridden := NewPrices(map[string]moondb.Rates{
		"claude-nova-9":            {Input: 1, Output: 2},
		"claude-opus-5-5":          {Input: 100, Output: 100},
		"claude-haiku-5-5":         {Input: 1, Output: 2},
		"claude-opus-5-5-20261101": {Input: 10, Output: 10},
	})
	tests := []struct {
		name   string
		prices Prices
		call   Call
		want   *float64
	}{
		{
			name: "standard",
			call: Call{
				Model:  "claude-sonnet-5-5",
				Tokens: Tokens{Input: 1000, CacheRead: 1_000_000, Output: 1000},
			},
			want: new(0.002 + 0.10 + 0.01),
		},
		{
			name: "haiku 5.5 under the long-prompt threshold",
			call: Call{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 50_000, Output: 1000}},
			want: new(0.0055),
		},
		{
			name: "haiku 5.5 over it bills every token at the long rates",
			call: Call{
				Model:  "claude-haiku-5-5",
				Tokens: Tokens{Input: 1000, CacheRead: 199_000, Output: 1000},
			},
			want: new((1000*0.50 + 199_000*0.05 + 1000*2.50) / 1e6),
		},
		{
			name: "fast mode doubles opus 5.5",
			call: Call{
				Model: "claude-opus-5-5", Speed: "fast",
				Tokens: Tokens{Input: 1_000_000, Output: 1_000_000},
			},
			want: new(8.0 + 40.0),
		},
		{
			name: "fast mode without a fast price has no cost",
			call: Call{Model: "claude-sonnet-5-5", Speed: "fast", Tokens: Tokens{Input: 1}},
		},
		{
			name: "US-only inference bills 1.1 times every rate",
			call: Call{
				Model: "claude-sonnet-5-5", Geo: "us",
				Tokens: Tokens{Input: 1000, CacheRead: 1_000_000, Output: 1000},
			},
			want: new((0.002 + 0.10 + 0.01) * 1.1),
		},
		{
			name: "US-only inference stacks on fast mode",
			call: Call{
				Model: "claude-opus-5-5", Speed: "fast", Geo: "us",
				Tokens: Tokens{Input: 1_000_000, Output: 1_000_000},
			},
			want: new((8.0 + 40.0) * 1.1),
		},
		{
			name: "US-only inference stacks on haiku 5.5's long rates",
			call: Call{
				Model: "claude-haiku-5-5", Geo: "us",
				Tokens: Tokens{Input: 1000, CacheRead: 199_000, Output: 1000},
			},
			want: new((1000*0.50 + 199_000*0.05 + 1000*2.50) / 1e6 * 1.1),
		},
		{
			name: "a model before 4.6 has no US-only premium",
			call: Call{Model: "claude-haiku-4-5", Geo: "us", Tokens: Tokens{Input: 1_000_000}},
			want: new(1.0),
		},
		{
			name: "global inference bills the list price",
			call: Call{Model: "claude-opus-5-5", Geo: "global", Tokens: Tokens{Input: 1_000_000}},
			want: new(4.0),
		},
		{name: "unknown model", call: Call{Model: "claude-nova-9", Tokens: Tokens{Input: 1}}},
		{name: "no tokens costs nothing", call: Call{Model: "claude-nova-9"}, want: new(0.0)},
		{
			name: "an override adds a model", prices: overridden,
			call: Call{Model: "claude-nova-9", Tokens: Tokens{Input: 1_000_000, Output: 1_000_000}},
			want: new(3.0),
		},
		{
			name: "an added model has no fast price", prices: overridden,
			call: Call{Model: "claude-nova-9", Speed: "fast", Tokens: Tokens{Input: 1}},
		},
		{
			name: "an override replaces a row's rates", prices: overridden,
			call: Call{Model: "claude-opus-5-5", Tokens: Tokens{Output: 10_000}},
			want: new(1.0),
		},
		{
			name: "an override keeps the row's fast multiplier", prices: overridden,
			call: Call{Model: "claude-opus-5-5", Speed: "fast", Tokens: Tokens{Output: 10_000}},
			want: new(2.0),
		},
		{
			name: "an override keeps the row's US-only multiplier", prices: overridden,
			call: Call{Model: "claude-opus-5-5", Geo: "us", Tokens: Tokens{Output: 10_000}},
			want: new(1.1),
		},
		{
			name: "an override keeps the row's long-prompt tier", prices: overridden,
			call: Call{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 200_000}},
			want: new(200_000 * 0.50 / 1e6),
		},
		{
			name: "an override under the threshold bills its own rates", prices: overridden,
			call: Call{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 50_000}},
			want: new(0.05),
		},
		{
			name: "an override of a dated id keeps its family's modifiers", prices: overridden,
			call: Call{
				Model: "claude-opus-5-5-20261101", Speed: "fast",
				Tokens: Tokens{Input: 1_000_000},
			},
			want: new(20.0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.prices.Cost(tt.call)
			if (got == nil) != (tt.want == nil) || (got != nil && !near(*got, *tt.want)) {
				t.Errorf("Cost = %v, want %v", deref(got), deref(tt.want))
			}
		})
	}
}

func TestBuildSession(t *testing.T) {
	turn := func(seq int, start, end time.Duration, children ...Span) TurnTrace {
		return TurnTrace{
			TurnID: "turn-" + string(rune('0'+seq)), Seq: seq, Prompt: "prompt",
			Span:  Span{Kind: KindTurn, Start: t0.Add(start), End: t0.Add(end)},
			Spans: children,
			Totals: Totals{
				WallMS: (end - start).Milliseconds(), ModelMS: 1000, ToolMS: 2000, ToolCalls: 1,
				Tokens: Tokens{Input: 10}, Cost: new(0.5),
			},
		}
	}
	tool := func(id string, ms int64) Span { return Span{Kind: KindTool, ID: id, DurationMS: ms} }
	model := func(id string, cost *float64, out int64) Span {
		return Span{Kind: KindModel, ID: id, Cost: cost, Tokens: &Tokens{Output: out}}
	}
	second := turn(2, 10*time.Minute, 15*time.Minute,
		tool("slow", 9000), model("cheap", new(0.1), 10), model("unpriced", nil, 999))
	second.Totals.Cost = nil
	second.Totals.Unpriced = []string{"claude-nova-9"}
	second.Totals.UserMS = 60_000 // a wait inside the turn adds to the gaps between turns
	first := turn(1, 0, 5*time.Minute, tool("fast", 100), model("dear", new(2.0), 1))

	got := BuildSession("sess", []TurnTrace{second, first}, nil)
	if got.Turns[0].Seq != 1 || got.Turns[1].Seq != 2 {
		t.Fatalf("turns out of order: #%d, #%d", got.Turns[0].Seq, got.Turns[1].Seq)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Kind != GapUser || got.Gaps[0].DurationMS != 300_000 {
		t.Errorf("gaps = %+v, want one 5 minute user gap", got.Gaps)
	}
	want := Totals{
		WallMS: 900_000, ModelMS: 2000, ToolMS: 4000, UserMS: 360_000, ToolCalls: 2,
		Tokens: Tokens{Input: 20}, Unpriced: []string{"claude-nova-9"},
	}
	checkTotals(t, got.Totals, want, new(0.5))

	if l := LongestTools(got, 1); len(l) != 1 || l[0].Span.ID != "slow" || l[0].Seq != 2 {
		t.Errorf("LongestTools = %+v, want slow in turn 2", l)
	}
	var ids []string
	for _, r := range CostliestModels(got, 10) {
		ids = append(ids, r.Span.ID)
	}
	if !slices.Equal(ids, []string{"dear", "cheap", "unpriced"}) {
		t.Errorf("CostliestModels = %v, want dear, cheap, unpriced", ids)
	}
}

// TestBuildSessionTimeOrder: a session's turns run in time order, not Seq
// order, since an import numbers the turns it adds after the live ones. A
// user gap runs from the latest end so far to the next start, and the
// session's time split adds up to its wall time, overlapping turns too.
func TestBuildSessionTimeOrder(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 1, h, m, 0, 0, time.UTC) }
	// turn has a wall time split between every kind, as Build's are.
	turn := func(seq int, start, end time.Time) TurnTrace {
		wall := end.Sub(start).Milliseconds()
		return TurnTrace{
			TurnID: "turn-" + string(rune('0'+seq)), Seq: seq,
			Span: Span{Kind: KindTurn, Start: start, End: end},
			Totals: Totals{
				WallMS: wall, ModelMS: wall / 2, ToolMS: wall / 4, HarnessMS: wall / 8,
				UserMS: wall - wall/2 - wall/4 - wall/8,
			},
		}
	}
	// gap names a stretch by its clock times, "08:10-10:00".
	gap := func(from, to time.Time) string { return from.Format("15:04") + "-" + to.Format("15:04") }
	tests := []struct {
		name     string
		turns    []TurnTrace
		wantSeqs []int
		wantGaps []string
		wantWall time.Duration
	}{
		{
			name: "an imported turn ran before the live ones",
			turns: []TurnTrace{
				turn(1, at(10, 0), at(10, 5)),
				turn(2, at(11, 0), at(11, 2)),
				turn(3, at(8, 0), at(8, 10)),
			},
			wantSeqs: []int{3, 1, 2},
			wantGaps: []string{"08:10-10:00", "10:05-11:00"},
			wantWall: at(11, 2).Sub(at(8, 0)),
		},
		{
			name: "a turn inside another adds no gap and counts in both",
			turns: []TurnTrace{
				turn(1, at(10, 0), at(12, 0)),
				turn(2, at(10, 30), at(10, 40)),
				turn(3, at(12, 30), at(12, 35)),
			},
			wantSeqs: []int{1, 2, 3},
			wantGaps: []string{"12:00-12:30"},
			wantWall: at(12, 35).Sub(at(10, 0)) + 10*time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSession("sess", tt.turns, nil)
			var seqs []int
			var turnsUser int64
			for _, x := range got.Turns {
				seqs = append(seqs, x.Seq)
				turnsUser += x.Totals.UserMS
			}
			if !slices.Equal(seqs, tt.wantSeqs) {
				t.Errorf("turn order = %v, want %v", seqs, tt.wantSeqs)
			}
			var gaps []string
			var gapsMS int64
			for _, g := range got.Gaps {
				gaps = append(gaps, gap(g.Start, g.End))
				gapsMS += g.DurationMS
				if g.Kind != GapUser || g.DurationMS != g.End.Sub(g.Start).Milliseconds() {
					t.Errorf("gap %+v is not a user gap of its length", g)
				}
			}
			if !slices.Equal(gaps, tt.wantGaps) {
				t.Errorf("gaps = %v, want %v", gaps, tt.wantGaps)
			}
			tot := got.Totals
			if tot.WallMS != tt.wantWall.Milliseconds() {
				t.Errorf("wall = %dms, want %dms", tot.WallMS, tt.wantWall.Milliseconds())
			}
			if tot.UserMS != turnsUser+gapsMS {
				t.Errorf("user = %dms, want the turns' %dms plus the gaps' %dms",
					tot.UserMS, turnsUser, gapsMS)
			}
			if sum := tot.ModelMS + tot.ToolMS + tot.HarnessMS + tot.UserMS; sum != tot.WallMS {
				t.Errorf("model, tool, harness and user time sum to %dms; wall is %dms",
					sum, tot.WallMS)
			}
		})
	}
}

func TestBuildSessionEmpty(t *testing.T) {
	got := BuildSession("sess", nil, nil)
	if got.Turns == nil || got.Gaps == nil || got.Totals.WallMS != 0 || got.Totals.Cost != nil {
		t.Errorf("empty session trace = %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"agent_longest_tools":[]`, `"agent_costliest_models":[]`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("JSON lacks %s: %s", key, data)
		}
	}
	for name, list := range map[string][]Ranked{
		"LongestTools": LongestTools(got, 10), "CostliestModels": CostliestModels(got, 10),
	} {
		if list == nil {
			t.Errorf("%s = nil, want an empty list", name)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
