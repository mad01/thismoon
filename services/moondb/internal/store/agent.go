package store

import (
	"cmp"
	"slices"
	"time"
)

// Agent is one subagent of a session, recorded when it stops: what kind it
// was, which main turns it ran under, when it ran, and its transcript lines
// cut to heads (an agent transcript), so its model and tool calls can be
// traced and priced like a turn's. A resumed agent stops again and its
// record is put again, whole.
type Agent struct {
	// ID is the harness's agent id (the hook's agent_id).
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	// Type is the kind of agent the harness ran: the spawned agent type
	// (general-purpose, Explore, ...), "subagent" for a named spawn whose
	// kind the harness did not say, or "internal" for an agent of the
	// harness itself.
	Type string `json:"type"`
	// Name is the name it was spawned under, when it had one other than
	// its type. A named spawn's hooks report the name as agent_type.
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// Model is the model of its first model call.
	Model string `json:"model,omitempty"`
	// TurnIDs are the main turns it ran under, in the order its lines name
	// them: the turn that spawned it first.
	TurnIDs []string `json:"turn_ids,omitempty"`
	// Cwd is the directory it worked in and Location where that sits in
	// git, placed as an edit's file is.
	Cwd      string   `json:"cwd,omitempty"`
	Location Location `json:"location"`
	// StartedAt is its first line's time. EndedAt is when it stopped, or
	// its last line's time when no stop was recorded (an import).
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	// TranscriptChunk holds its agent transcript.
	TranscriptChunk string     `json:"transcript_chunk,omitempty"`
	Lines           AgentLines `json:"lines"`
	// Totals is what its trace adds up to, computed by serve when the
	// record lands. It is nil until then, and on a record stored by an
	// older trace build until the totals backfill reaches it. Replace it
	// whole, never change it in place: a backend may share it.
	Totals *TurnTotals `json:"totals,omitempty"`
}

// AgentLines counts an agent's transcript lines.
type AgentLines struct {
	// Read is every line of its transcript file that was read.
	Read int `json:"read"`
	// Kept is the lines its agent transcript holds; the others were of a
	// kind a trace reads past, or did not parse.
	Kept int `json:"kept"`
	// Cut is the lines the size cap took off the head of the agent
	// transcript after the cut to heads. Their model calls add no tokens
	// or cost.
	Cut int `json:"cut,omitempty"`
	// Partial is set when the reader's budget ran out before the end of
	// the file, so the lines after Read are missing.
	Partial bool `json:"partial,omitempty"`
}

// Label names an agent for a person: its name, else its type.
func (a Agent) Label() string { return cmp.Or(a.Name, a.Type, "subagent") }

// RanIn reports whether the agent ran under the main turn turnID.
func (a Agent) RanIn(turnID string) bool { return slices.Contains(a.TurnIDs, turnID) }

// NeedsAgentTotals reports whether an agent lacks totals from trace
// version version: it has none, or older ones.
func NeedsAgentTotals(a Agent, version int) bool {
	return a.Totals == nil || a.Totals.Version < version
}

// SortAgents puts agents in the order a session lists them: by start,
// then by id.
func SortAgents(agents []Agent) {
	slices.SortFunc(agents, func(a, b Agent) int {
		return cmp.Or(a.StartedAt.Compare(b.StartedAt), cmp.Compare(a.ID, b.ID))
	})
}
