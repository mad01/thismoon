// Package scorecard says how a session spent its context: what its tokens,
// cost and time went to, how its tool calls went, whether its csl searches
// came back empty or cut, how much its reads pulled in, which belt hints it
// was shown, and how its subagents ran. An eval suite scores sessions
// against it, so the JSON shape carries a Version and changes by adding
// fields.
//
// Build is a pure function of a session trace, the tool calls parsed from
// the session's transcript lines (ParseCalls), and what the events service
// said (ReadEvents, over Peers). Each outside input reports in Sources
// whether it was reachable. One that was not leaves its part null and never
// fails the scorecard.
package scorecard

import (
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Version is the scorecard's JSON shape. Raise it when a field changes
// meaning or goes away; a new field keeps it. Version 2 dropped the
// assertion-store source and the hints block it fed.
const Version = 2

// Scorecard is one session's account of its context use.
type Scorecard struct {
	Version   int     `json:"version"`
	SessionID string  `json:"session_id"`
	Sources   Sources `json:"sources"`
	Totals    Totals  `json:"totals"`
	// Tools has one row per tool name, most calls first.
	Tools  []ToolStat  `json:"tools"`
	Search SearchStats `json:"search"`
	Reads  ReadStats   `json:"reads"`
	// Hints is null when the events service did not answer.
	Hints  *HintStats `json:"hints"`
	Agents AgentStats `json:"agents"`
	// Flags name the costliest patterns found, one short sentence each,
	// costliest kind first. Empty when none crossed its threshold.
	Flags []string `json:"flags"`
}

// Status says whether an input of the scorecard was there.
type Status string

const (
	// StatusOK: the input answered in full.
	StatusOK Status = "ok"
	// StatusPartial: the input answered, but some of it is missing; Detail
	// says what.
	StatusPartial Status = "partial"
	// StatusUnavailable: the input did not answer; its part is null.
	StatusUnavailable Status = "unavailable"
	// StatusSkipped: nothing needed the input.
	StatusSkipped Status = "skipped"
)

// Source is one input's status and, when it is not ok, why.
type Source struct {
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Sources says which inputs a scorecard was built from.
type Sources struct {
	// Transcripts are the turns' and agents' transcript lines in the store.
	Transcripts Source `json:"transcripts"`
	// Events is the events service: belt hints and belt ledger entries.
	Events Source `json:"events"`
}

// Totals is the session's time, tokens, cost and calls. The time split is
// the main conversation's, as in a session trace; the tokens, cost and call
// counts cover the main conversation and every subagent, and Main and
// Agents split them.
type Totals struct {
	Turns      int          `json:"turns"`
	WallMS     int64        `json:"wall_ms"`
	ModelMS    int64        `json:"model_ms"`
	ToolMS     int64        `json:"tool_ms"`
	HarnessMS  int64        `json:"harness_ms"`
	UserMS     int64        `json:"user_ms"`
	IdleMS     int64        `json:"idle_ms"`
	Tokens     trace.Tokens `json:"tokens"`
	Cost       *float64     `json:"cost"`
	Unpriced   []string     `json:"unpriced"`
	ModelCalls int          `json:"model_calls"`
	ToolCalls  int          `json:"tool_calls"`
	ToolErrors int          `json:"tool_errors"`
	Main       Part         `json:"main"`
	Agents     Part         `json:"agents"`
}

// Part is the main conversation's or the subagents' share of the totals.
// ActiveMS is the time model, tool or harness work ran: the main
// conversation's wall time less its user and idle time, or the sum of that
// over every subagent run, which may overlap one another.
type Part struct {
	ActiveMS   int64        `json:"active_ms"`
	Tokens     trace.Tokens `json:"tokens"`
	Cost       *float64     `json:"cost"`
	ModelCalls int          `json:"model_calls"`
	ToolCalls  int          `json:"tool_calls"`
	ToolErrors int          `json:"tool_errors"`
}

// ToolStat sums the calls of one tool across the session.
type ToolStat struct {
	Name       string `json:"name"`
	Calls      int    `json:"calls"`
	Errors     int    `json:"errors"`
	DurationMS int64  `json:"duration_ms"`
	// AgentCalls is the subagents' part of Calls.
	AgentCalls int `json:"agent_calls"`
}

// SearchStats counts the csl searches (csl_search, csl_semantic_search,
// csl_hybrid_search) by how they came back.
type SearchStats struct {
	Calls int `json:"calls"`
	// Zero found nothing.
	Zero int `json:"zero"`
	// Truncated hit the limit or a cap: more results existed.
	Truncated int `json:"truncated"`
	// ZeroResultHint came back with csl's account of an empty search.
	ZeroResultHint int `json:"zero_result_hint"`
	// Relaxed dropped query terms that matched no file and searched the
	// rest.
	Relaxed int `json:"relaxed"`
	// Errors failed, such as on a query that did not parse.
	Errors int `json:"errors"`
	// Unread came back with a result the transcript keeps only the head
	// of (a subagent's) or saved to a file: a cap or a hint past the head
	// is not counted.
	Unread int `json:"unread"`
}

// ReadStats counts the Read calls by what they pulled into the context.
type ReadStats struct {
	Calls  int `json:"calls"`
	Errors int `json:"errors"`
	// Unranged read the whole file: the tool's own result says it started
	// at line 1 and returned every line, or, when the transcript does not
	// hold it, the call gave no offset, limit or page range.
	Unranged int `json:"unranged"`
	// Large are the unranged reads that returned LargeLines lines or more.
	Large      int `json:"large"`
	LargeLines int `json:"large_lines"`
	// Lines is what the sized reads returned. Unsized are the reads whose
	// line count the transcript does not hold: a subagent's transcript
	// keeps only the head of a result.
	Lines   int `json:"lines"`
	Unsized int `json:"unsized"`
	// Largest is the read that returned the most lines, null when no read
	// was sized.
	Largest *ReadCall `json:"largest"`
}

// ReadCall is one Read call.
type ReadCall struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	// TotalLines is the file's length when the tool reported it.
	TotalLines int `json:"total_lines,omitempty"`
	// Ranged is set when the read was not of the whole file (Unranged).
	Ranged  bool   `json:"ranged"`
	AgentID string `json:"agent_id,omitempty"`
}

// HintStats counts the belt hints the session was shown.
type HintStats struct {
	Shown int `json:"shown"`
	// ByHint has one row per hint id, most shown first.
	ByHint []HintCount `json:"by_hint"`
}

// HintCount is how often one hint fired.
type HintCount struct {
	Hint  string `json:"hint"`
	Count int    `json:"count"`
}

// AgentStats is the session's subagents: their runs from the trace, and
// the lifecycle counts from the belt ledger.
type AgentStats struct {
	Count int `json:"count"`
	// Runs are the subagents with a recorded run, costliest first.
	Runs []AgentRun `json:"runs"`
	// Ledger is null when the events service did not answer.
	Ledger *Ledger `json:"ledger"`
}

// AgentRun is one subagent's run.
type AgentRun struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Name       string       `json:"name,omitempty"`
	Model      string       `json:"model,omitempty"`
	DurationMS int64        `json:"duration_ms"`
	Tokens     trace.Tokens `json:"tokens"`
	Cost       *float64     `json:"cost"`
	ModelCalls int          `json:"model_calls"`
	ToolCalls  int          `json:"tool_calls"`
	ToolErrors int          `json:"tool_errors"`
}

// Ledger counts the session's belt ledger entries by event.
type Ledger struct {
	SubagentStart    int `json:"subagent_start"`
	SubagentStop     int `json:"subagent_stop"`
	PermissionDenied int `json:"permission_denied"`
	PreCompact       int `json:"pre_compact"`
	StopFailure      int `json:"stop_failure"`
}
