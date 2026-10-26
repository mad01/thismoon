package trace

import (
	"slices"
	"sort"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// reasonNoAgentChunk says why an agent's trace is empty.
const reasonNoAgentChunk = "no agent transcript was recorded for this agent"

// AgentRun is one subagent's run as a session trace lists it: the run's
// span, its totals and its segments, without its model and tool spans.
type AgentRun struct {
	AgentID string   `json:"agent_id"`
	Type    string   `json:"type"`
	Name    string   `json:"name,omitempty"`
	Model   string   `json:"model,omitempty"`
	TurnIDs []string `json:"turn_ids"`
	// Span runs from the spawn to the stop, with the run's calls, tokens,
	// cost and edits.
	Span     Span      `json:"span"`
	Totals   Totals    `json:"totals"`
	Segments []Segment `json:"segments"`
	Lines    Lines     `json:"lines"`
	// LinesCut counts the lines the agent transcript's size cap cut from
	// its head (store.AgentLines.Cut): their calls add no tokens or cost.
	// Partial is set when the capture's budget ran out before the file's
	// end.
	LinesCut int    `json:"lines_cut,omitempty"`
	Partial  bool   `json:"partial,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// AgentTrace is the trace of one subagent: its run, and under it the spans
// of its model and tool calls and its time split by gap kind.
type AgentTrace struct {
	AgentRun
	Spans []Span `json:"spans"`
	Gaps  []Gap  `json:"gaps"`
}

// Segment is the part of an agent's run under one main turn: the lines
// from one promptId to the next. A teammate runs under every turn that
// sent it a message, so its run has a segment per stretch of work; an
// agent the main conversation waits on has one.
type Segment struct {
	TurnID     string    `json:"turn_id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	DurationMS int64     `json:"duration_ms"`
	ModelCalls int       `json:"model_calls"`
	ToolCalls  int       `json:"tool_calls"`
	ToolErrors int       `json:"tool_errors"`
	Tokens     Tokens    `json:"tokens"`
	// Cost is the USD of the segment's priced calls, nil when none is
	// priced; Unpriced names the models it leaves out.
	Cost     *float64 `json:"cost,omitempty"`
	Unpriced []string `json:"unpriced,omitempty"`
}

// BuildAgent computes the trace of one subagent from its agent transcript.
// The run's span goes from StartedAt to EndedAt, stretched to the last
// line when one is later; its model spans, tool spans and gaps are made
// as Build makes a turn's, from the same parser. edits are the edits the
// agent made, for the tool spans that made them and the run's edit count.
// The segments split the run by the turn each line came in under.
func BuildAgent(a store.Agent, lines []byte, edits []store.Edit, prices Prices) AgentTrace {
	events, counts := parseLines(lines, true)
	start, end := bounds(a.StartedAt, a.EndedAt, events)
	at := AgentTrace{
		AgentRun: AgentRun{
			AgentID: a.ID, Type: a.Type, Name: a.Name, Model: a.Model,
			TurnIDs:  slices.Clone(a.TurnIDs),
			Span:     span(KindSubagent, a.ID, a.Label(), start, end),
			Segments: []Segment{},
			Lines:    counts,
			LinesCut: a.Lines.Cut,
			Partial:  a.Lines.Partial,
		},
		Spans: []Span{},
		Gaps:  []Gap{},
	}
	at.Span.EditCount = len(edits)
	if len(events) == 0 {
		at.Totals = Totals{WallMS: at.Span.DurationMS}
		at.Reason = agentReason(lines)
		return at
	}
	models, calls := modelSpans(events, start, end, prices)
	tools := toolSpans(events, start, end, edits)
	at.Spans = byStart(slices.Concat(models, tools))
	at.Gaps = partition(events, start, end, counts.CutHead)
	at.Totals = turnTotals(at.Span, at.Gaps, models, tools)
	at.Totals.Usage = prices.Usage(calls)
	at.Segments = segments(events, at.Span, at.Spans)
	at.Span.Tokens, at.Span.Cost = &at.Totals.Tokens, at.Totals.Cost
	at.Span.ModelCalls, at.Span.ToolCalls = at.Totals.ModelCalls, at.Totals.ToolCalls
	return at
}

func agentReason(lines []byte) string {
	if len(lines) == 0 {
		return reasonNoAgentChunk
	}
	return reasonNoEvents
}

// byStart sorts spans by start time, ties in the order given.
func byStart(spans []Span) []Span {
	slices.SortStableFunc(spans, func(a, b Span) int { return a.Start.Compare(b.Start) })
	return spans
}

// segments splits a run at each line whose promptId differs from the one
// before it; a line without one, every assistant line among them, stays in
// the segment it follows. The first segment starts with the run and the
// last ends with it. Each span goes to the segment its start falls in.
func segments(events []event, run Span, spans []Span) []Segment {
	var segs []Segment
	cur := ""
	for _, ev := range events {
		id := ev.promptID
		if id == "" {
			id = cur
		}
		if len(segs) == 0 || id != cur {
			segs = append(segs, Segment{TurnID: id, Start: ev.at})
			cur = id
		}
		segs[len(segs)-1].End = ev.at
	}
	segs[0].Start = run.Start
	segs[len(segs)-1].End = run.End
	for _, s := range spans {
		segs[segmentAt(segs, s.Start)].add(s)
	}
	for i := range segs {
		segs[i].DurationMS = segs[i].End.Sub(segs[i].Start).Milliseconds()
	}
	return segs
}

// add counts one model or tool span in the segment.
func (g *Segment) add(s Span) {
	switch s.Kind {
	case KindModel:
		g.ModelCalls++
		g.Tokens = g.Tokens.Add(*s.Tokens)
		if s.Tokens.Total() > 0 {
			g.Cost, g.Unpriced = addCost(g.Cost, g.Unpriced, s.Cost, s.Name)
		}
	case KindTool:
		g.ToolCalls++
		if s.IsError {
			g.ToolErrors++
		}
	}
}

// segmentAt is the index of the segment t falls in: the last one starting
// at or before it, the first when t is earlier than every one.
func segmentAt(segs []Segment, t time.Time) int {
	i := sort.Search(len(segs), func(j int) bool { return segs[j].Start.After(t) })
	return max(i-1, 0)
}

// WithAgents returns tt with the work of the agents that ran under the
// turn. Each agent with a segment under the turn gets one subagent span,
// from the first such segment's start to the last one's end, with their
// calls, tokens and cost. It replaces the span the agent's edits made and
// keeps that span's edit count. It is not kept inside the turn's span: a
// background agent runs past the turn's Stop. The turn's totals gain the
// agents' tokens and cost (withAgentWork). tt itself is not changed, so a
// cached trace stays as Build made it.
func WithAgents(tt TurnTrace, agents []AgentRun) TurnTrace {
	var added []Span
	var work []Totals
	for _, a := range agents {
		if s, w, ok := a.turnSpan(tt.TurnID); ok {
			added, work = append(added, s), append(work, w)
		}
	}
	if len(added) == 0 {
		return tt
	}
	spans := make([]Span, 0, len(tt.Spans)+len(added))
	for _, s := range tt.Spans {
		i := slices.IndexFunc(added, func(a Span) bool { return a.ID == s.ID })
		if s.Kind == KindSubagent && i >= 0 {
			added[i].EditCount = s.EditCount
			continue
		}
		spans = append(spans, s)
	}
	tt.Spans = byStart(append(spans, added...))
	tt.Totals = withAgentWork(tt.Totals, work)
	return tt
}

// turnSpan is the span of a's work under turnID and its totals; false when
// no segment of a ran under it.
func (a AgentRun) turnSpan(turnID string) (Span, Totals, bool) {
	var s Span
	var w Totals
	found := false
	for _, g := range a.Segments {
		if turnID == "" || g.TurnID != turnID {
			continue
		}
		if !found {
			s = Span{Kind: KindSubagent, ID: a.AgentID, Name: a.Span.Name, Start: g.Start}
			found = true
		}
		s.Start, s.End = earlier(s.Start, g.Start), later(s.End, g.End)
		w.ModelCalls += g.ModelCalls
		w.ToolCalls += g.ToolCalls
		w.Tokens = w.Tokens.Add(g.Tokens)
		if g.Cost != nil {
			w.addCost(g.Cost, "")
		}
		for _, m := range g.Unpriced {
			w.addCost(nil, m)
		}
	}
	s.DurationMS = s.End.Sub(s.Start).Milliseconds()
	s.Tokens, s.Cost = &w.Tokens, w.Cost
	s.ModelCalls, s.ToolCalls = w.ModelCalls, w.ToolCalls
	return s, w, found
}

// withAgentWork adds the agents' work to the main conversation's totals t.
// Tokens, Cost and Usage gain the agents', which the *Main and *Agents
// fields keep apart, and the agents' calls are counted apart. With no
// work, t stays as it is.
func withAgentWork(t Totals, work []Totals) Totals {
	if len(work) == 0 {
		return t
	}
	mainTokens, mainCost := t.Tokens, t.Cost
	var agents Totals
	usage := [][]store.ModelUsage{t.Usage}
	for _, w := range work {
		agents.Tokens = agents.Tokens.Add(w.Tokens)
		if w.Cost != nil {
			agents.addCost(w.Cost, "")
		}
		for _, m := range w.Unpriced {
			t.addCost(nil, m)
		}
		t.AgentModelCalls += w.ModelCalls
		t.AgentToolCalls += w.ToolCalls
		usage = append(usage, w.Usage)
	}
	t.TokensMain, t.TokensAgents = &mainTokens, &agents.Tokens
	t.CostMain, t.CostAgents = mainCost, agents.Cost
	t.Tokens = mainTokens.Add(agents.Tokens)
	if agents.Cost != nil {
		t.addCost(agents.Cost, "")
	}
	t.Usage = store.MergeUsage(usage...)
	return t
}

// addAgents adds the agents to st: their runs, their work to each turn it
// ran under, their whole runs to the session's totals, and their top
// spans.
func (st *SessionTrace) addAgents(agents []AgentTrace) {
	st.Agents = make([]AgentRun, 0, len(agents))
	work := make([]Totals, 0, len(agents))
	for _, a := range agents {
		st.Agents = append(st.Agents, a.AgentRun)
		work = append(work, a.Totals)
	}
	st.Totals = withAgentWork(st.Totals, work)
	for i, tt := range st.Turns {
		st.Turns[i] = WithAgents(tt, st.Agents)
	}
	tools, models := agentRanked(agents, st.Turns)
	st.AgentLongestTools = top(tools, nil, moondb.TraceTopSpans, longer)
	st.AgentCostliestModels = top(models, nil, moondb.TraceTopSpans, costlier)
}

// agentRanked lists the agents' tool and model spans, each with the turn
// its segment ran under and that turn's number and prompt.
func agentRanked(agents []AgentTrace, turns []TurnTrace) (tools, models []Ranked) {
	byID := make(map[string]TurnTrace, len(turns))
	for _, tt := range turns {
		byID[tt.TurnID] = tt
	}
	tools, models = []Ranked{}, []Ranked{}
	for _, a := range agents {
		for _, s := range a.Spans {
			turnID := a.Segments[segmentAt(a.Segments, s.Start)].TurnID
			tt := byID[turnID]
			r := Ranked{
				Seq: tt.Seq, TurnID: turnID, Prompt: tt.Prompt,
				AgentID: a.AgentID, Agent: a.Span.Name, Span: s,
			}
			switch s.Kind {
			case KindTool:
				tools = append(tools, r)
			case KindModel:
				models = append(models, r)
			}
		}
	}
	return tools, models
}

func earlier(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b
	}
	return a
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// MainAgent names the main conversation where agents are listed beside
// it.
const MainAgent = "main"

// Who names the agent that made a ranked call: the subagent's label, or
// MainAgent for the main conversation.
func (r Ranked) Who() string {
	if r.AgentID == "" {
		return MainAgent
	}
	return r.Agent
}

// AgentsByCost returns a copy of runs, the costliest first; a run with no
// priced call ranks last by tokens.
func AgentsByCost(runs []AgentRun) []AgentRun {
	out := slices.Clone(runs)
	slices.SortStableFunc(out, func(a, b AgentRun) int { return costlier(a.Span, b.Span) })
	return out
}

// TurnSeqs are the numbers of the turns ids name, in the order given; an
// id the trace has no turn for is left out.
func (st SessionTrace) TurnSeqs(ids []string) []int {
	seqs := make([]int, 0, len(ids))
	for _, id := range ids {
		if i := slices.IndexFunc(st.Turns, func(t TurnTrace) bool { return t.TurnID == id }); i >= 0 {
			seqs = append(seqs, st.Turns[i].Seq)
		}
	}
	return seqs
}

// MainEdits counts the edits the main conversation's tool calls made in
// the session's turns.
func (st SessionTrace) MainEdits() int {
	n := 0
	for _, tt := range st.Turns {
		for _, s := range tt.Spans {
			if s.Kind == KindTool {
				n += len(s.EditIDs)
			}
		}
	}
	return n
}
