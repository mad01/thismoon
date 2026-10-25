package trace

import (
	"cmp"
	"slices"
	"time"
)

// BuildSession puts a session's turn traces in order and adds what lies
// between them: the time from one turn's end to the next one's start is
// user time, the session's and not either turn's. The session's user time
// is those gaps plus the user time inside its turns.
func BuildSession(sessionID string, turns []TurnTrace) SessionTrace {
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
		if x.Cost != nil {
			t.addCost(x.Cost, "")
		}
		for _, m := range x.Unpriced {
			t.addCost(nil, m)
		}
	}
	t.WallMS = last.Sub(first).Milliseconds()
	return t
}

// LongestTools returns a session's n longest tool spans, longest first.
func LongestTools(st SessionTrace, n int) []Ranked {
	return top(st, KindTool, n, func(a, b Span) int {
		return cmp.Compare(b.DurationMS, a.DurationMS)
	})
}

// CostliestModels returns a session's n most expensive model spans, most
// expensive first. A span with no price ranks below every priced one, by
// its token count.
func CostliestModels(st SessionTrace, n int) []Ranked {
	return top(st, KindModel, n, func(a, b Span) int {
		if c := cmp.Compare(costOf(b), costOf(a)); c != 0 {
			return c
		}
		return cmp.Compare(tokensOf(b), tokensOf(a))
	})
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

// top ranks the session's spans of kind by order and keeps the first n,
// ties in timeline order.
func top(st SessionTrace, kind Kind, n int, order func(a, b Span) int) []Ranked {
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
	slices.SortStableFunc(out, func(a, b Ranked) int { return order(a.Span, b.Span) })
	return out[:min(n, len(out))]
}
