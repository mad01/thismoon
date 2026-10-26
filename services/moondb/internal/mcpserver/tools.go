package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// handlers carries the dependencies shared by all moondb tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views sessions in a browser
	checks func(ctx context.Context) []doctor.Check
}

// readOnly is the annotation every moondb tool carries: it reads the local
// store and git, and changes neither.
func readOnly() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
}

func registerTools(s *mcp.Server, h *handlers) {
	registerEditTools(s, h)
	registerSessionTools(s, h)
}

// registerEditTools adds the tools that start from a file or a turn:
// why, history and turn.
func registerEditTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_why",
		Description: "Find the agent turn that wrote one line of a file, including changes not committed yet. " +
			"Give `path` (relative to the worktree root) and the 1-based `line`, plus `worktree` (absolute checkout root, where git runs) when the file is on disk. " +
			"When the checkout is gone, give `repo` (host/owner/repo) and the line's `text` instead. " +
			"Returns the edit that produced the line, its turn and session, the commit it landed in when known, and `method`: " +
			"blame (git blame joined a recorded blob), content (the line's text matched a line an edit added; `candidates` lists matches best first), or none (nothing recorded explains the line). " +
			"Follow up with moondb_turn on the returned turn id for the prompt and everything else that turn changed.",
		Annotations: readOnly(),
	}, h.handleWhy)

	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_history",
		Description: "List the recorded edits of one file, newest first: which tool call changed it, in which turn and session, and what kind of change it was. " +
			"`path` is REQUIRED (relative to the worktree root). `worktree` narrows to one checkout; `repo` (host/owner/repo) alone spans every checkout of the repository. " +
			"`limit` caps the list (default 50).",
		Annotations: readOnly(),
	}, h.handleHistory)

	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_turn",
		Description: "Get one turn by id: the prompt it answered, its status, and everything recorded under it: " +
			"edits, unattributed deltas (changes no tool call explains, such as a formatter or a human between turns), " +
			"turn-boundary snapshots, and commit links. `turn.agents` lists the agents that worked in the turn: " +
			"the main conversation first, then each subagent with its model calls, time, edits and cost there. " +
			"`turn.trace_totals.cost` covers them all; `cost_main` and `cost_agents` split it. " +
			"Turn ids come from moondb_why and moondb_history (`turn_id`).",
		Annotations: readOnly(),
	}, h.handleTurn)
}

// registerSessionTools adds the tools that start from sessions or from
// moondb itself: sessions, trace and doctor.
func registerSessionTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_sessions",
		Description: "List recorded agent sessions newest first. Each has a title (the harness title, else its first prompt line), " +
			"a status (running or ended), when it started, was last active and ended, its active time and wall time, its turn, edit, file and agent counts, " +
			"the names of the subagents that ran (`agents`), its tokens and cost, its transcript path, and the repositories and worktrees it edited. " +
			"`active_ms` is the time inside its turns, less the time a turn waited for input, so a pause between turns never counts. " +
			"`duration_ms` is the wall time from the first prompt to the last turn's end, pauses included, and never below `active_ms`. " +
			"An open turn ends at its last edit or agent stop for both. " +
			"The cost covers the main conversation and its subagents; `cost_main` and `cost_agents` split it. " +
			"Filter by `repo` (host/owner/repo; sessions with at least one edit there), `since` (sessions active since then, whenever they started: " +
			"an RFC 3339 time or a duration such as 24h or 7d), and `limit` (default 50).",
		Annotations: readOnly(),
	}, h.handleSessions)

	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_trace",
		Description: "Show what took time in one session and what it cost: wall time split into model, tool, harness " +
			"(hooks and request setup) and user time (waiting for input: between turns, and inside a turn " +
			"after the model stopped until a new input such as a teammate message), tokens by kind, and the USD cost " +
			"at list prices. " +
			fmt.Sprintf(
				"Also lists the %[1]d longest tool calls and the %[1]d most expensive model calls, ",
				moondb.TraceTopSpans,
			) +
			"each with its turn number, prompt and, for a subagent's call, `agent_id` and `agent`. " +
			"`agents` lists the subagents costliest first: type, name, model, the turns they ran under, " +
			"duration, calls, edits, tokens and cost. " +
			"`id` is REQUIRED: a session id from moondb_sessions. " +
			"Cost covers the main conversation and every subagent with a recorded agent transcript; " +
			"`totals.cost_main` and `totals.cost_agents` split it. " +
			"A model with no known price shows in `totals.unpriced` and adds no cost.",
		Annotations: readOnly(),
	}, h.handleTrace)

	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_doctor",
		Description: "Diagnose moondb itself: is serve reachable, is the store readable, is the running build " +
			"the installed one. Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing. " +
			"This is about the service, not the recorded history: use it when a moondb tool errors " +
			"or comes back empty and you need to know whether serve is even up.",
		Annotations: readOnly(),
	}, h.handleDoctor)
}

// sessionURL is the web page that shows one session's turns and edits.
func (h *handlers) sessionURL(id string) string {
	if id == "" {
		return h.webURL
	}
	return h.webURL + "/?session=" + url.QueryEscape(id)
}

// ── why ──

type whyInput struct {
	Worktree string `json:"worktree,omitempty" jsonschema:"absolute root of the checkout the file is read from; git runs here"`
	Repo     string `json:"repo,omitempty"     jsonschema:"repository identity host/owner/repo as the origin remote names it; narrows the content match when the worktree is gone"`
	Path     string `json:"path"               jsonschema:"file path relative to the worktree root (required)"`
	Line     int    `json:"line"               jsonschema:"1-based line number (required)"`
	Text     string `json:"text,omitempty"     jsonschema:"the line's content, when the file is not on disk"`
}

type whyOutput struct {
	Result  query.WhyResult `json:"result"`
	Summary string          `json:"summary"`
	URL     string          `json:"url"     jsonschema:"web page showing the session that made the edit"`
}

func (h *handlers) handleWhy(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in whyInput,
) (*mcp.CallToolResult, whyOutput, error) {
	res, err := h.client.Why(ctx, query.WhyRequest{
		Worktree: in.Worktree,
		Repo:     in.Repo,
		Path:     in.Path,
		Line:     in.Line,
		Text:     in.Text,
	})
	if err != nil {
		return nil, whyOutput{}, err
	}
	sessionID := ""
	if res.Edit != nil {
		sessionID = res.Edit.SessionID
	}
	return nil, whyOutput{Result: res, Summary: whySummary(res), URL: h.sessionURL(sessionID)}, nil
}

// whySummary says in one line which edit explains the line and how it was found.
func whySummary(r query.WhyResult) string {
	where := fmt.Sprintf("line %d of %s", r.Line, r.Path)
	if r.Edit == nil {
		return fmt.Sprintf("no recorded edit explains %s (method %s)", where, r.Method)
	}
	e := r.Edit
	s := fmt.Sprintf("%s: %s %s by %s in turn %s of session %s (method %s)",
		where, e.Kind, e.ID, e.Tool, e.TurnID, e.SessionID, r.Method)
	if r.Commit != "" {
		s += fmt.Sprintf(", committed in %.12s", r.Commit)
	}
	if n := len(r.Candidates); n > 1 {
		s += fmt.Sprintf("; %d candidates, best first", n)
	}
	return s
}

// ── history ──

type historyInput struct {
	Worktree string `json:"worktree,omitempty" jsonschema:"absolute checkout root; narrows to that checkout"`
	Repo     string `json:"repo,omitempty"     jsonschema:"repository identity host/owner/repo; alone spans every checkout"`
	Path     string `json:"path"               jsonschema:"file path relative to the worktree root (required)"`
	Limit    int    `json:"limit,omitempty"    jsonschema:"maximum edits to return, newest first (default 50)"`
}

type editsOutput struct {
	Edits   []store.Edit `json:"edits"`
	Summary string       `json:"summary"`
	URL     string       `json:"url"     jsonschema:"web page where the user can browse sessions"`
}

func (h *handlers) handleHistory(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in historyInput,
) (*mcp.CallToolResult, editsOutput, error) {
	edits, err := h.client.History(ctx, query.HistoryRequest{
		Worktree: in.Worktree,
		Repo:     in.Repo,
		Path:     in.Path,
		Limit:    in.Limit,
	})
	if err != nil {
		return nil, editsOutput{}, err
	}
	if edits == nil {
		edits = []store.Edit{}
	}
	summary := fmt.Sprintf("no recorded edits of %s", in.Path)
	if len(edits) > 0 {
		summary = fmt.Sprintf("%d edits of %s, newest %s in turn %s",
			len(edits), in.Path, edits[0].At.Format("2006-01-02 15:04"), edits[0].TurnID)
	}
	return nil, editsOutput{Edits: edits, Summary: summary, URL: h.webURL}, nil
}

// ── turn ──

// promptExcerpt caps the prompt text a turn result carries, so one long
// pasted prompt cannot crowd out the edits.
const promptExcerpt = 2000

type turnInput struct {
	ID string `json:"id" jsonschema:"turn id, the turn_id of an edit from moondb_why or moondb_history (required)"`
}

type turnOutput struct {
	Turn    server.TurnView `json:"turn"`
	Prompt  string          `json:"prompt,omitempty" jsonschema:"the prompt text, cut at 2000 bytes; absent when no prompt was recorded"`
	Summary string          `json:"summary"`
	URL     string          `json:"url"              jsonschema:"web page showing the turn's session"`
}

func (h *handlers) handleTurn(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in turnInput,
) (*mcp.CallToolResult, turnOutput, error) {
	d, err := h.client.Turn(ctx, in.ID)
	if err != nil {
		return nil, turnOutput{}, err
	}
	prompt := h.prompt(ctx, d.PromptChunk)
	summary := fmt.Sprintf(
		"turn %d of session %s, %s: %d edits, %d unattributed deltas, %d commit links",
		d.Seq,
		d.SessionID,
		d.Status,
		len(d.Edits),
		len(d.Deltas),
		len(d.CommitLinks),
	)
	if tot := d.TraceTotals; tot != nil {
		summary += fmt.Sprintf("; took %s, cost %s",
			trace.FormatDuration(tot.WallMS), trace.FormatTotalsCost(*tot))
	}
	skipped := 0
	for _, s := range d.Skipped {
		skipped += s.Total()
	}
	if skipped > 0 {
		summary += fmt.Sprintf("; %d files skipped", skipped)
	}
	if names := subagentNames(d.Agents); names != "" {
		summary += "; subagents: " + names
	}
	if line := trace.PromptLine(prompt); line != "" {
		summary += fmt.Sprintf("; prompt: %q", line)
	}
	return nil, turnOutput{
		Turn:    d,
		Prompt:  prompt,
		Summary: summary,
		URL:     h.sessionURL(d.SessionID),
	}, nil
}

// subagentNames lists the subagents of an agent listing by name, with the
// type when it differs; "" when only the main conversation ran.
func subagentNames(rows []server.AgentRow) string {
	var names []string
	for _, r := range rows {
		switch {
		case r.Type == server.MainAgentType && r.ID == "":
		case r.Name != "" && r.Name != r.Type:
			names = append(names, fmt.Sprintf("%s (%s)", r.Name, r.Type))
		default:
			names = append(names, r.Type)
		}
	}
	return strings.Join(names, ", ")
}

// prompt fetches a turn's prompt text. Best-effort: a missing or unreadable
// chunk leaves the prompt out rather than failing the whole turn.
func (h *handlers) prompt(ctx context.Context, sha string) string {
	if sha == "" {
		return ""
	}
	data, err := h.client.Chunk(ctx, sha)
	if err != nil {
		return ""
	}
	return truncate(string(data), promptExcerpt)
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}

// ── sessions ──

type sessionsInput struct {
	Repo  string `json:"repo,omitempty"  jsonschema:"repository identity host/owner/repo; keeps sessions with at least one edit there"`
	Since string `json:"since,omitempty" jsonschema:"keeps sessions active since then: an RFC 3339 time, or a duration before now such as 90m, 24h or 7d"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum sessions to return, newest first (default 50)"`
}

type sessionsOutput struct {
	Sessions []server.SessionSummary `json:"sessions"`
	Summary  string                  `json:"summary"`
	URL      string                  `json:"url"      jsonschema:"web page where the user can browse sessions"`
}

func (h *handlers) handleSessions(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in sessionsInput,
) (*mcp.CallToolResult, sessionsOutput, error) {
	list, err := h.client.Sessions(ctx, client.SessionQuery{
		Repo:  in.Repo,
		Since: in.Since,
		Limit: in.Limit,
	})
	if err != nil {
		return nil, sessionsOutput{}, err
	}
	if list == nil {
		list = []server.SessionSummary{}
	}
	summary := "no sessions recorded"
	if len(list) > 0 {
		summary = fmt.Sprintf("%d sessions, newest started %s",
			len(list), list[0].StartedAt.Format("2006-01-02 15:04"))
	}
	return nil, sessionsOutput{Sessions: list, Summary: summary, URL: h.webURL}, nil
}

// ── trace ──

type traceInput struct {
	ID string `json:"id" jsonschema:"session id, from moondb_sessions (required)"`
}

type traceOutput struct {
	SessionID       string         `json:"session_id"`
	Turns           int            `json:"turns"`
	Totals          trace.Totals   `json:"totals"`
	LongestTools    []trace.Ranked `json:"longest_tools"    jsonschema:"the longest tool calls, longest first, each with its turn number and prompt"`
	CostliestModels []trace.Ranked `json:"costliest_models" jsonschema:"the most expensive model calls, most expensive first; unpriced calls rank last by tokens"`
	Agents          []traceAgent   `json:"agents"           jsonschema:"the subagents that ran, costliest first"`
	Summary         string         `json:"summary"`
	URL             string         `json:"url"              jsonschema:"web page with the session's timeline"`
}

func (h *handlers) handleTrace(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in traceInput,
) (*mcp.CallToolResult, traceOutput, error) {
	st, err := h.client.SessionTrace(ctx, in.ID)
	if err != nil {
		return nil, traceOutput{}, err
	}
	return nil, traceOutput{
		SessionID:       st.SessionID,
		Turns:           len(st.Turns),
		Totals:          st.Totals,
		LongestTools:    trace.LongestTools(st, moondb.TraceTopSpans),
		CostliestModels: trace.CostliestModels(st, moondb.TraceTopSpans),
		Agents:          traceAgents(st),
		Summary:         traceSummary(st),
		URL:             h.sessionURL(st.SessionID),
	}, nil
}

// traceAgent is one subagent as moondb_trace lists it.
type traceAgent struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Name       string       `json:"name,omitempty"`
	Model      string       `json:"model,omitempty"`
	Turns      []int        `json:"turns"               jsonschema:"the numbers of the turns it ran under"`
	DurationMS int64        `json:"duration_ms"         jsonschema:"from its spawn to its stop"`
	ModelCalls int          `json:"model_calls"`
	ToolCalls  int          `json:"tool_calls"`
	EditCount  int          `json:"edit_count"`
	Tokens     trace.Tokens `json:"tokens"`
	Cost       *float64     `json:"cost,omitempty"      jsonschema:"USD at list prices; absent when no call is priced"`
	Unpriced   []string     `json:"unpriced,omitempty"`
	LinesCut   int          `json:"lines_cut,omitempty" jsonschema:"lines the agent transcript's size cap cut; their calls add no tokens or cost"`
}

// traceAgents lists a session's subagents for moondb_trace, costliest
// first.
func traceAgents(st trace.SessionTrace) []traceAgent {
	out := []traceAgent{}
	for _, a := range trace.AgentsByCost(st.Agents) {
		out = append(out, traceAgent{
			ID: a.AgentID, Type: a.Type, Name: a.Name, Model: a.Model,
			Turns: st.TurnSeqs(a.TurnIDs), DurationMS: a.Span.DurationMS,
			ModelCalls: a.Totals.ModelCalls, ToolCalls: a.Totals.ToolCalls,
			EditCount: a.Span.EditCount, Tokens: a.Totals.Tokens, Cost: a.Totals.Cost,
			Unpriced: a.Totals.Unpriced, LinesCut: a.LinesCut,
		})
	}
	return out
}

// traceSummary says in one line where a session's time and money went.
func traceSummary(st trace.SessionTrace) string {
	t := st.Totals
	s := fmt.Sprintf(
		"session %s: %d turns, wall %s (model %s, tool %s, harness %s, user %s), %s tokens, cost %s",
		st.SessionID,
		len(st.Turns),
		trace.FormatDuration(t.WallMS),
		trace.FormatDuration(t.ModelMS),
		trace.FormatDuration(t.ToolMS),
		trace.FormatDuration(t.HarnessMS),
		trace.FormatDuration(t.UserMS),
		trace.FormatTokens(t.Tokens.Total()),
		trace.FormatTotalsCost(t),
	)
	if t.TokensAgents != nil {
		s += fmt.Sprintf(" (main %s, %d subagents %s)",
			trace.FormatCost(t.CostMain), len(st.Agents), trace.FormatCost(t.CostAgents))
	}
	if len(t.Unpriced) > 0 {
		s += fmt.Sprintf(" (unpriced: %s)", strings.Join(t.Unpriced, ", "))
	}
	if top := trace.LongestTools(st, 1); len(top) > 0 {
		s += fmt.Sprintf(
			"; longest tool call %s %s in turn %d by %s",
			top[0].Span.Name,
			trace.FormatDuration(top[0].Span.DurationMS),
			top[0].Seq,
			top[0].Who(),
		)
	}
	return s
}

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `moondb doctor` and returns the
// report. A failing check is a result, not a tool error: the caller asked
// what is wrong, and an error would hide the answer behind a transport
// failure.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}
