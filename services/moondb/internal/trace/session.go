package trace

import (
	"cmp"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// BuildSession puts a session's turn traces in order and adds what lies
// between them: the time from one turn's end to the next one's start is
// user time, the session's and not either turn's. The session's user time
// is those gaps plus the user time inside its turns. turns are the turns'
// own traces (Build); agents are the traces of the session's subagents
// with a record (BuildAgent), whose work BuildSession adds to each turn it
// ran under and whose whole runs it adds to the session's tokens and cost.
func BuildSession(sessionID string, turns []TurnTrace, agents []AgentTrace) SessionTrace {
	turns = slices.Clone(turns)
	slices.SortStableFunc(turns, func(a, b TurnTrace) int { return cmp.Compare(a.Seq, b.Seq) })
	st := SessionTrace{SessionID: sessionID, Turns: turns, Gaps: []Gap{}}
	if turns == nil {
		st.Turns = []TurnTrace{}
	}
	var user time.Duration
	for i := 1; i < len(turns); i++ {
		from, to := turns[i-1].Span.End, turns[i].Span.Start
		if to.After(from) {
			st.Gaps = append(st.Gaps, Gap{
				Kind: GapUser, Start: from, End: to, DurationMS: to.Sub(from).Milliseconds(),
			})
			user += to.Sub(from)
		}
	}
	st.Totals = sessionTotals(turns)
	st.Totals.UserMS += user.Milliseconds()
	st.addAgents(agents)
	return st
}

// sessionTotals sums the turns; the wall time runs from the first turn's
// start to the last one's end.
func sessionTotals(turns []TurnTrace) Totals {
	var t Totals
	if len(turns) == 0 {
		return t
	}
	first, last := turns[0].Span.Start, turns[0].Span.End
	usage := make([][]store.ModelUsage, 0, len(turns))
	for _, tt := range turns {
		if tt.Span.Start.Before(first) {
			first = tt.Span.Start
		}
		if tt.Span.End.After(last) {
			last = tt.Span.End
		}
		x := tt.Totals
		t.ModelMS += x.ModelMS
		t.ToolMS += x.ToolMS
		t.HarnessMS += x.HarnessMS
		t.UserMS += x.UserMS
		t.Tokens = t.Tokens.Add(x.Tokens)
		t.ModelCalls += x.ModelCalls
		t.ToolCalls += x.ToolCalls
		t.ToolErrors += x.ToolErrors
		usage = append(usage, x.Usage)
		if x.Cost != nil {
			t.addCost(x.Cost, "")
		}
		for _, m := range x.Unpriced {
			t.addCost(nil, m)
		}
	}
	t.WallMS = last.Sub(first).Milliseconds()
	t.Usage = store.MergeUsage(usage...)
	return t
}

// LongestTools returns a session's n longest tool spans, longest first,
// the agents' among them (SessionTrace.AgentLongestTools).
func LongestTools(st SessionTrace, n int) []Ranked {
	return top(turnSpans(st, KindTool), st.AgentLongestTools, n, longer)
}

// CostliestModels returns a session's n most expensive model spans, most
// expensive first, the agents' among them
// (SessionTrace.AgentCostliestModels). A span with no price ranks below
// every priced one, by its token count.
func CostliestModels(st SessionTrace, n int) []Ranked {
	return top(turnSpans(st, KindModel), st.AgentCostliestModels, n, costlier)
}

func longer(a, b Span) int { return cmp.Compare(b.DurationMS, a.DurationMS) }

func costlier(a, b Span) int {
	if c := cmp.Compare(costOf(b), costOf(a)); c != 0 {
		return c
	}
	return cmp.Compare(tokensOf(b), tokensOf(a))
}

// costOf is a span's cost for ranking, -1 when it has none.
func costOf(s Span) float64 {
	if s.Cost == nil {
		return -1
	}
	return *s.Cost
}

func tokensOf(s Span) int64 {
	if s.Tokens == nil {
		return 0
	}
	return s.Tokens.Total()
}

// turnSpans lists the spans of kind in the session's turns, in timeline
// order.
func turnSpans(st SessionTrace, kind Kind) []Ranked {
	out := []Ranked{}
	for _, tt := range st.Turns {
		for _, s := range tt.Spans {
			if s.Kind == kind {
				out = append(
					out,
					Ranked{Seq: tt.Seq, TurnID: tt.TurnID, Prompt: tt.Prompt, Span: s},
				)
			}
		}
	}
	return out
}

// top ranks the lists together by order and keeps the first n, ties in
// the order given: the main conversation's first.
func top(main, agents []Ranked, n int, order func(a, b Span) int) []Ranked {
	out := slices.Concat(main, agents)
	slices.SortStableFunc(out, func(a, b Ranked) int { return order(a.Span, b.Span) })
	return out[:min(n, len(out))]
}
