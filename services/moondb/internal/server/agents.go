package server

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// MainAgentType is the type of the row that stands for the main
// conversation in an agent listing; its id is "".
const MainAgentType = trace.MainAgent

// AgentRow is one agent of a session or a turn: the main conversation or a
// subagent. In a session it covers the agent's whole run, in a turn its
// work under that turn. A subagent known only from its edits has no
// record behind it (Recorded false), so no calls, time or cost.
type AgentRow struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	Model       string    `json:"model,omitempty"`
	TurnIDs     []string  `json:"turn_ids"`
	StartedAt   time.Time `json:"started_at,omitzero"`
	EndedAt     time.Time `json:"ended_at,omitzero"`
	DurationMS  int64     `json:"duration_ms"`
	ModelCalls  int       `json:"model_calls"`
	ToolCalls   int       `json:"tool_calls"`
	ToolErrors  int       `json:"tool_errors"`
	EditCount   int       `json:"edit_count"`
	// Repos are the repository identities the agent's edits went to.
	Repos []string `json:"repos"`
	// CostRepo is where the attribution rule sends a session row's cost:
	// a repository, orchestration or no_repo (store.MainCostRepo,
	// store.AgentCostRepo). A row without tokens adds nothing there and
	// leaves it empty, as do a turn's rows.
	CostRepo string       `json:"cost_repo,omitempty"`
	Tokens   store.Tokens `json:"tokens"`
	// Cost is the USD of the priced tokens, nil when none is priced.
	Cost     *float64 `json:"cost,omitempty"`
	Unpriced []string `json:"unpriced,omitempty"`
	Recorded bool     `json:"recorded"`
	// LinesCut counts the lines the agent transcript's size cap cut, whose
	// calls the row leaves out.
	LinesCut int `json:"lines_cut,omitempty"`
}

// AgentView is one agent as GET /api/agents/{id} answers it: the record,
// its session row and its trace.
type AgentView struct {
	store.Agent
	Row   AgentRow         `json:"row"`
	Trace trace.AgentTrace `json:"trace"`
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	a, at, err := s.query.AgentTraceByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	edits, err := s.store.Edits(r.Context(), store.EditQuery{SessionID: a.SessionID})
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, AgentView{
		Agent: a, Row: s.recordedRow(a, query.AgentEdits(edits, a.ID)), Trace: at,
	})
}

// sessionAgents lists a session's agents: the main conversation first,
// then the subagents with a record by start, then those known only from
// their edits.
func (s *Server) sessionAgents(ctx context.Context, d store.SessionDetail) ([]AgentRow, error) {
	edits, err := s.store.Edits(ctx, store.EditQuery{SessionID: d.ID})
	if err != nil {
		return nil, err
	}
	rows := []AgentRow{s.mainRow(d, query.AgentEdits(edits, ""))}
	for _, a := range d.Agents {
		rows = append(rows, s.recordedRow(a, query.AgentEdits(edits, a.ID)))
	}
	return append(rows, editOnlyRows(edits, rows)...), nil
}

// mainRow is the main conversation's row of a session: its turns' calls,
// time and usage, and the edits it made itself.
func (s *Server) mainRow(d store.SessionDetail, edits []store.Edit) AgentRow {
	row := AgentRow{
		Type: MainAgentType, Model: d.Model, TurnIDs: []string{}, Recorded: true,
		EditCount: len(edits), Repos: editRepos(edits),
	}
	for _, t := range d.Turns {
		row.TurnIDs = append(row.TurnIDs, t.ID)
		if t.Totals != nil {
			row.ModelCalls += t.Totals.ModelCalls
			row.ToolCalls += t.Totals.ToolCalls
			row.ToolErrors += t.Totals.ToolErrors
		}
	}
	if len(d.Turns) > 0 {
		row.StartedAt, row.EndedAt = d.Turns[0].StartedAt, d.LastTurnEnd
		row.DurationMS = d.DurationMS
	}
	row.Tokens = tokensOf(d.MainUsage)
	row.Cost, row.Unpriced = s.query.UsageCost(d.MainUsage)
	if row.Tokens.Total() > 0 {
		row.CostRepo = store.MainCostRepo(edits)
	}
	return row
}

// recordedRow is a subagent's row from its record and the edits it made.
func (s *Server) recordedRow(a store.Agent, edits []store.Edit) AgentRow {
	row := AgentRow{
		ID: a.ID, Type: a.Type, Name: a.Name, Description: a.Description, Model: a.Model,
		TurnIDs: orEmpty(a.TurnIDs), StartedAt: a.StartedAt, EndedAt: a.EndedAt,
		DurationMS: a.EndedAt.Sub(a.StartedAt).Milliseconds(),
		EditCount:  len(edits), Repos: editRepos(edits), Recorded: true, LinesCut: a.Lines.Cut,
	}
	if t := a.Totals; t != nil {
		row.ModelCalls, row.ToolCalls, row.ToolErrors = t.ModelCalls, t.ToolCalls, t.ToolErrors
		row.Tokens = tokensOf(t.Usage)
		row.Cost, row.Unpriced = s.query.UsageCost(t.Usage)
	}
	if row.Tokens.Total() > 0 {
		row.CostRepo = store.AgentCostRepo(a, edits)
	}
	return row
}

// editOnlyRows are the rows of the subagents that made an edit but have no
// row in listed: each one's type, turns, edits and the time between its
// first edit and its last.
func editOnlyRows(edits []store.Edit, listed []AgentRow) []AgentRow {
	byID := map[string]*AgentRow{}
	var order []string
	for _, e := range edits {
		if e.AgentID == "" || slices.ContainsFunc(listed, func(r AgentRow) bool {
			return r.ID == e.AgentID
		}) {
			continue
		}
		row, ok := byID[e.AgentID]
		if !ok {
			row = &AgentRow{
				ID: e.AgentID, Type: cmp.Or(e.AgentType, "subagent"), TurnIDs: []string{},
				StartedAt: e.At, EndedAt: e.At,
			}
			byID[e.AgentID] = row
			order = append(order, e.AgentID)
		}
		row.EditCount++
		row.StartedAt, row.EndedAt = earlier(row.StartedAt, e.At), later(row.EndedAt, e.At)
		if !slices.Contains(row.TurnIDs, e.TurnID) {
			row.TurnIDs = append(row.TurnIDs, e.TurnID)
		}
	}
	out := make([]AgentRow, 0, len(order))
	for _, id := range order {
		row := byID[id]
		row.DurationMS = row.EndedAt.Sub(row.StartedAt).Milliseconds()
		row.Repos = editRepos(query.AgentEdits(edits, id))
		out = append(out, *row)
	}
	slices.SortStableFunc(out, func(a, b AgentRow) int { return a.StartedAt.Compare(b.StartedAt) })
	return out
}

// turnAgents lists the agents that worked in a turn: the main conversation
// first, from tt's own totals; then each subagent span of tt, which
// WithAgents made from a record's segment under the turn or Build from the
// agent's edits; then the subagents with an edit in the turn and no span.
func (s *Server) turnAgents(
	d store.TurnDetail,
	tt trace.TurnTrace,
	runs []trace.AgentRun,
) []AgentRow {
	main := query.AgentEdits(d.Edits, "")
	tot := tt.Totals
	rows := []AgentRow{{
		Type: MainAgentType, TurnIDs: []string{d.ID}, StartedAt: tt.Span.Start,
		EndedAt: tt.Span.End, DurationMS: tt.Span.DurationMS, ModelCalls: tot.ModelCalls,
		ToolCalls: tot.ToolCalls, ToolErrors: tot.ToolErrors, EditCount: len(main),
		Repos: editRepos(main), Tokens: *cmp.Or(tot.TokensMain, &tot.Tokens),
		Cost: mainCost(tot), Unpriced: mainUnpriced(tot), Recorded: true,
	}}
	for _, sp := range tt.Spans {
		if sp.Kind == trace.KindSubagent {
			rows = append(rows, spanRow(sp, d, runs))
		}
	}
	return append(rows, editOnlyRows(d.Edits, rows)...)
}

// mainCost is the main conversation's part of a turn's cost.
func mainCost(t trace.Totals) *float64 {
	if t.TokensMain != nil {
		return t.CostMain
	}
	return t.Cost
}

// mainUnpriced names the models the main conversation's part of a turn's
// cost leaves out, without the ones only its subagents used.
func mainUnpriced(t trace.Totals) []string {
	if t.TokensMain != nil {
		return t.UnpricedMain
	}
	return t.Unpriced
}

// spanRow is a turn's row for one subagent span, with the run's record
// fields when the agent has a record.
func spanRow(sp trace.Span, d store.TurnDetail, runs []trace.AgentRun) AgentRow {
	edits := query.AgentEdits(d.Edits, sp.ID)
	row := AgentRow{
		ID: sp.ID, Type: sp.Name, TurnIDs: []string{d.ID}, StartedAt: sp.Start, EndedAt: sp.End,
		DurationMS: sp.DurationMS, ModelCalls: sp.ModelCalls, ToolCalls: sp.ToolCalls,
		EditCount: len(edits), Repos: editRepos(edits), Cost: sp.Cost,
	}
	if sp.Tokens != nil {
		row.Tokens = *sp.Tokens
	}
	if i := slices.IndexFunc(runs, func(r trace.AgentRun) bool { return r.AgentID == sp.ID }); i >= 0 {
		r := runs[i]
		row.Type, row.Name, row.Model, row.Recorded, row.LinesCut = r.Type, r.Name, r.Model, true,
			r.LinesCut
	} else if len(edits) > 0 {
		row.Type = cmp.Or(edits[0].AgentType, "subagent")
	}
	return row
}

// orEmpty is list, or an empty one for nil, so the JSON says [].
func orEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// editRepos lists the repositories edits went to, sorted.
func editRepos(edits []store.Edit) []string {
	repos := []string{}
	for _, e := range edits {
		if e.Repo != "" && !slices.Contains(repos, e.Repo) {
			repos = append(repos, e.Repo)
		}
	}
	slices.Sort(repos)
	return repos
}

func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
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
