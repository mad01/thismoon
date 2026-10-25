package trace

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Reasons a turn's trace is empty.
const (
	reasonOpen     = "the turn is still open; its transcript lines are recorded at Stop"
	reasonNoChunk  = "no transcript lines were recorded for this turn"
	reasonNoEvents = "the transcript lines hold no timestamped user, assistant, API request or Stop line"
)

// Build computes the trace of one turn from the transcript lines recorded
// under it. The turn span runs from StartedAt to EndedAt, stretched to the
// last line when one is later. Model spans run from the event before an
// assistant message to its last line, tool spans from the line that asked
// for the call to the line that carried its result, and a subagent span
// from the subagent's first recorded edit to its last, inside the turn's
// span: its own transcript is a separate file the turn does not keep. The
// gaps split the turn by the event that ends each stretch.
//
// Build never fails on its input: broken lines are counted in Lines and
// read past, and a turn without lines gets an empty trace with a Reason.
// Lines whose head the size cap cut (Lines.CutHead) give the stretch
// before the first kept line to harness time.
func Build(d store.TurnDetail, lines []byte, prices Prices) TurnTrace {
	events, counts := parseLines(lines)
	start, end := bounds(d.Turn, events)
	tt := TurnTrace{
		TurnID: d.ID,
		Seq:    d.Seq,
		Span:   span(KindTurn, d.ID, "turn", start, end),
		Spans:  []Span{},
		Gaps:   []Gap{},
		Lines:  counts,
		Reason: reason(d.Turn, lines, counts),
	}
	if len(events) == 0 {
		tt.Totals = Totals{WallMS: tt.Span.DurationMS}
		return tt
	}
	models, calls := modelSpans(events, start, end, prices)
	tools := toolSpans(events, start, end, d.Edits)
	tt.Spans = slices.Concat(models, tools, subagentSpans(d.Edits, start, end))
	slices.SortStableFunc(tt.Spans, func(a, b Span) int { return a.Start.Compare(b.Start) })
	tt.Gaps = partition(events, start, end, counts.CutHead)
	tt.Totals = turnTotals(tt.Span, tt.Gaps, models, tools)
	tt.Totals.Usage = prices.Usage(calls)
	return tt
}

// bounds is the turn's start and end. A turn with no start time starts at
// its first event; one with no end ends at its last event.
func bounds(t store.Turn, events []event) (start, end time.Time) {
	start = t.StartedAt
	if start.IsZero() && len(events) > 0 {
		start = events[0].at
	}
	end = start
	if t.EndedAt != nil && t.EndedAt.After(start) {
		end = *t.EndedAt
	}
	if n := len(events); n > 0 && events[n-1].at.After(end) {
		end = events[n-1].at
	}
	return start, end
}

func reason(t store.Turn, lines []byte, counts Lines) string {
	switch {
	case len(lines) == 0 && t.Status == store.TurnOpen:
		return reasonOpen
	case len(lines) == 0:
		return reasonNoChunk
	case counts.Events == 0:
		return reasonNoEvents
	}
	return ""
}

// clamp keeps t inside [lo, hi].
func clamp(t, lo, hi time.Time) time.Time {
	if t.Before(lo) {
		return lo
	}
	if t.After(hi) {
		return hi
	}
	return t
}

// message is one assistant message: the indexes of its first and last
// event on the timeline, and its last line in chunk order, whose usage is
// the final one.
type message struct {
	first, last, final int
}

// modelSpans makes one span per assistant message, and returns the call
// each one billed beside it. Lines of a streamed message share its id; a
// line without one is a message of its own. A message whose first line
// opens the timeline lost the event before it to a cut head, so its span
// starts at that line, never before it.
func modelSpans(events []event, start, end time.Time, prices Prices) ([]Span, []Call) {
	var order []string
	byID := map[string]*message{}
	for i, ev := range events {
		if ev.kind != evAssistant {
			continue
		}
		key := ev.msgID
		if key == "" {
			key = fmt.Sprintf("line %d", ev.line)
		}
		m, ok := byID[key]
		if !ok {
			m = &message{first: i, last: i, final: i}
			byID[key] = m
			order = append(order, key)
		}
		m.last = i
		if ev.line > events[m.final].line {
			m.final = i
		}
	}
	spans := make([]Span, 0, len(order))
	calls := make([]Call, 0, len(order))
	for _, key := range order {
		m := byID[key]
		from := events[m.first].at
		if m.first > 0 {
			from = events[m.first-1].at
		}
		final := events[m.final]
		s := span(KindModel, final.msgID, final.call.Model,
			clamp(from, start, end), clamp(events[m.last].at, start, end))
		s.Tokens = &final.call.Tokens
		s.Cost = prices.Cost(final.call)
		if speed := final.call.Speed; speed != "" && speed != speedStandard {
			s.Speed = speed
		}
		spans = append(spans, s)
		calls = append(calls, final.call)
	}
	return spans, calls
}

// toolSpans makes one span per tool call, ended by its result or, without
// one, by the turn's end.
func toolSpans(events []event, start, end time.Time, edits []store.Edit) []Span {
	results := map[string]event{}
	for _, ev := range events {
		for _, r := range ev.results {
			if _, seen := results[r.id]; !seen {
				results[r.id] = ev
			}
		}
	}
	seen := map[string]bool{}
	var spans []Span
	for _, ev := range events {
		for _, u := range ev.uses {
			if seen[u.id] {
				continue
			}
			seen[u.id] = true
			spans = append(spans, toolSpan(u, ev.at, results, start, end, edits))
		}
	}
	return spans
}

func toolSpan(
	u toolUse,
	at time.Time,
	results map[string]event,
	start, end time.Time,
	edits []store.Edit,
) Span {
	from := clamp(at, start, end)
	res, ok := results[u.id]
	to := end
	if ok {
		to = clamp(res.at, from, end)
	}
	s := span(KindTool, u.id, u.name, from, to)
	s.Unfinished = !ok
	if ok {
		i := slices.IndexFunc(res.results, func(r toolResult) bool { return r.id == u.id })
		s.IsError = res.results[i].isError
	}
	for _, e := range edits {
		if e.ToolUseID == u.id {
			s.EditIDs = append(s.EditIDs, e.ID)
		}
	}
	slices.Sort(s.EditIDs)
	return s
}

// subagentSpans makes one span per subagent that recorded an edit in the
// turn, from its first edit to its last, kept inside [start, end]: the Bash
// hook is async, so an edit can be recorded after the turn's Stop.
func subagentSpans(edits []store.Edit, start, end time.Time) []Span {
	byAgent := map[string]*Span{}
	var order []string
	for _, e := range edits {
		if e.AgentID == "" {
			continue
		}
		s, ok := byAgent[e.AgentID]
		if !ok {
			s = &Span{
				Kind: KindSubagent, ID: e.AgentID, Name: agentName(e.AgentType),
				Start: e.At, End: e.At,
			}
			byAgent[e.AgentID] = s
			order = append(order, e.AgentID)
		}
		s.EditCount++
		if e.At.Before(s.Start) {
			s.Start = e.At
		}
		if e.At.After(s.End) {
			s.End = e.At
		}
	}
	spans := make([]Span, 0, len(order))
	for _, id := range order {
		s := byAgent[id]
		s.Start, s.End = clamp(s.Start, start, end), clamp(s.End, start, end)
		s.DurationMS = s.End.Sub(s.Start).Milliseconds()
		spans = append(spans, *s)
	}
	return spans
}

// agentName is a subagent's type, or "subagent" when the hook named none.
func agentName(agentType string) string {
	return cmp.Or(agentType, "subagent")
}

// partition splits [start, end] into gaps, each classed by the event that
// ends it: an assistant line ends model time, a tool result ends tool
// time, and a new input (a user line that answers no tool call) after the
// model finished ends user time. Anything else is harness time: hooks,
// attachments, request setup, and the Stop hooks up to the Stop line. The
// stretch after the last line is user time once the Stop line was seen,
// and harness time before it. A moment when a tool and the model both ran
// counts once, by the event that ends it. With cutHead, the stretch before
// the first event is harness time whatever that event is: what ran in it
// went with the cut lines.
//
// User time inside a turn is real: the harness keeps one prompt id across
// a Stop when a teammate message or a task notification starts the loop
// again, so the wait before that input lands in the same turn.
func partition(events []event, start, end time.Time, cutHead bool) []Gap {
	gaps := []Gap{}
	prev := start
	finished, stopped := false, false
	for i, ev := range events {
		at := clamp(ev.at, prev, end)
		kind := gapKind(ev, finished)
		if i == 0 && cutHead {
			kind = GapHarness
		}
		gaps = addGap(gaps, kind, prev, at)
		prev = at
		switch ev.kind {
		case evAssistant:
			finished, stopped = finishReasons[ev.stopReason], false
		case evStop:
			finished, stopped = true, true
		case evUser:
			stopped = stopped && len(ev.results) > 0
		}
	}
	tail := GapHarness
	if stopped {
		tail = GapUser
	}
	return addGap(gaps, tail, prev, end)
}

// finishReasons are the stop reasons that end the model's work until new
// input arrives. tool_use and pause_turn go on by themselves, and a line
// without a reason says nothing.
var finishReasons = map[string]bool{"end_turn": true, "stop_sequence": true, "refusal": true}

// gapKind classes the stretch ev ends; finished says the model had
// stopped for input before it.
func gapKind(ev event, finished bool) GapKind {
	switch {
	case ev.kind == evAssistant:
		return GapModel
	case len(ev.results) > 0:
		return GapTool
	case ev.kind == evUser && finished:
		return GapUser
	}
	return GapHarness
}

// addGap appends the stretch [from, to] of kind, merged into the last gap
// when that one has the same kind. An empty stretch adds nothing.
func addGap(gaps []Gap, kind GapKind, from, to time.Time) []Gap {
	if !to.After(from) {
		return gaps
	}
	if n := len(gaps); n > 0 && gaps[n-1].Kind == kind && gaps[n-1].End.Equal(from) {
		gaps[n-1].End = to
		gaps[n-1].DurationMS = to.Sub(gaps[n-1].Start).Milliseconds()
		return gaps
	}
	return append(gaps, Gap{
		Kind: kind, Start: from, End: to, DurationMS: to.Sub(from).Milliseconds(),
	})
}

// gapOrder is the order Totals lists the gap kinds in.
var gapOrder = []GapKind{GapModel, GapTool, GapHarness, GapUser}

// splitMS turns the time by gap kind into whole milliseconds that add up to
// the wall time exactly. It truncates the running sum of the kinds, in
// gapOrder, once per kind and gives each kind the milliseconds its step
// added, so the remainder of one kind carries to the next and the last
// step ends on the wall's milliseconds. No kind is off by a millisecond or
// more, and a kind with no time gets none.
func splitMS(byKind map[GapKind]time.Duration) map[GapKind]int64 {
	out := make(map[GapKind]int64, len(gapOrder))
	var sum time.Duration
	var sumMS int64
	for _, k := range gapOrder {
		sum += byKind[k]
		out[k] = sum.Milliseconds() - sumMS
		sumMS = sum.Milliseconds()
	}
	return out
}

// turnTotals sums a turn: its time by gap kind, and its model and tool
// calls. The gaps cover the turn, so the kinds add up to WallMS. A turn
// whose calls billed no token has no cost, never a zero one, as its
// listing row has none.
func turnTotals(turn Span, gaps []Gap, models, tools []Span) Totals {
	byKind := map[GapKind]time.Duration{}
	for _, g := range gaps {
		byKind[g.Kind] += g.End.Sub(g.Start)
	}
	ms := splitMS(byKind)
	t := Totals{
		WallMS:     turn.DurationMS,
		ModelMS:    ms[GapModel],
		ToolMS:     ms[GapTool],
		HarnessMS:  ms[GapHarness],
		UserMS:     ms[GapUser],
		ModelCalls: len(models),
		ToolCalls:  len(tools),
	}
	for _, s := range models {
		t.Tokens = t.Tokens.Add(*s.Tokens)
		if s.Tokens.Total() > 0 { // a call that billed nothing prices nothing
			t.addCost(s.Cost, s.Name)
		}
	}
	for _, s := range tools {
		if s.IsError {
			t.ToolErrors++
		}
	}
	return t
}

// addCost adds one priced amount, or records model as unpriced.
func (t *Totals) addCost(cost *float64, model string) {
	if cost == nil {
		if !slices.Contains(t.Unpriced, model) {
			t.Unpriced = append(t.Unpriced, model)
			slices.Sort(t.Unpriced)
		}
		return
	}
	sum := *cost
	if t.Cost != nil {
		sum += *t.Cost
	}
	t.Cost = &sum
}
