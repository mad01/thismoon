package trace

import (
	"cmp"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// BuildSession puts a session's turn traces in time order (inTimeOrder) and
// adds what lies between them: the time from the latest turn end so far to
// the next turn's start is user time, the session's and not either turn's.
// turns are the turns' own traces (Build); agents are the traces of the
// session's subagents with a record (BuildAgent), whose work BuildSession
// adds to each turn it ran under and whose whole runs it adds to the
// session's tokens and cost.
func BuildSession(sessionID string, turns []TurnTrace, agents []AgentTrace) SessionTrace {
	turns = slices.Clone(turns)
	slices.SortStableFunc(turns, inTimeOrder)
	st := SessionTrace{SessionID: sessionID, Turns: turns, Gaps: betweenTurns(turns)}
	if turns == nil {
		st.Turns = []TurnTrace{}
	}
	st.Totals = sessionTotals(turns, st.Gaps)
	st.addAgents(agents)
	return st
}

// inTimeOrder orders turns as they ran: by start, then by Seq. Seq is not
// time order: an import numbers the turns it adds after the live ones,
// though they may have run before them.
func inTimeOrder(a, b TurnTrace) int {
	return cmp.Or(a.Span.Start.Compare(b.Span.Start), cmp.Compare(a.Seq, b.Seq))
}

// betweenTurns returns the user gaps between turns in time order: from the
// latest end so far to the next turn's start, when that start is later. A
// turn that starts before an earlier one ended adds none.
func betweenTurns(turns []TurnTrace) []Gap {
	gaps := []Gap{}
	var end time.Time
	for i, tt := range turns {
		if from, to := end, tt.Span.Start; i > 0 && to.After(from) {
			gaps = append(gaps, Gap{
				Kind: GapUser, Start: from, End: to, DurationMS: to.Sub(from).Milliseconds(),
			})
		}
		end = later(end, tt.Span.End)
	}
	return gaps
}

// sessionTotals sums the turns and the gaps between them, the gaps as user
// time. The wall time is the turns' wall times plus the gaps, so the time
// split adds up to it whenever each turn's does. Without overlapping turns
// that runs from the first start to the last end; a stretch two turns
// overlap counts once in each.
func sessionTotals(turns []TurnTrace, gaps []Gap) Totals {
	var t Totals
	if len(turns) == 0 {
		return t
	}
	for _, g := range gaps {
		t.WallMS += g.DurationMS
		t.UserMS += g.DurationMS
	}
	usage := make([][]store.ModelUsage, 0, len(turns))
	for _, tt := range turns {
		x := tt.Totals
		t.WallMS += x.WallMS
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
// The list is never nil, so the JSON says [] when both are empty.
func top(main, agents []Ranked, n int, order func(a, b Span) int) []Ranked {
	out := append(append(make([]Ranked, 0, len(main)+len(agents)), main...), agents...)
	slices.SortStableFunc(out, func(a, b Ranked) int { return order(a.Span, b.Span) })
	return out[:min(n, len(out))]
}
