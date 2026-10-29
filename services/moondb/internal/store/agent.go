package store

import (
	"cmp"
	"slices"
	"time"
)

// InternalAgent is the Type of an agent with nothing to tell its kind by:
// no spawned agent type, no agent_type and no name. The harness ran it for
// itself.
const InternalAgent = "internal"

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
	// kind the harness did not say, or InternalAgent for an agent of the
	// harness itself.
	Type string `json:"type"`
	// Name is the name it was spawned under, when it had one other than
	// its type. A named spawn's hooks report the name as agent_type.
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// BriefChunk holds its spawn brief: the prompt the conversation that
	// spawned it gave it, the text of its transcript's first user line.
	// Empty on a record stored before briefs were kept, until an import
	// fills it.
	BriefChunk string `json:"brief_chunk,omitempty"`
	// LaterMessages counts the messages its transcript shows reaching it
	// after the spawn brief, such as a lead's follow-ups to a teammate.
	// The brief holds none of them, so a count above zero says the brief
	// may be out of date. Zero on a record stored before they were
	// counted, until an import fills the brief.
	LaterMessages int `json:"later_messages,omitempty"`
	// Messages are those later messages, oldest first, each one's text a
	// chunk. A record stored before they were kept has none, or fewer
	// than LaterMessages, until an import fills them.
	Messages []AgentMessage `json:"messages,omitempty"`
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

// AgentMessage is a later message: one that reached an Agent after its
// spawn brief, such as a lead's follow-up to a teammate or a line the
// person typed into the agent's pane. Its text sits in a chunk, so the
// record names it and no edit copies it.
type AgentMessage struct {
	// At is the time of the transcript line that delivered it.
	At time.Time `json:"at"`
	// From is the teammate_id of the envelope it came in; empty for a
	// line with no envelope.
	From string `json:"from,omitempty"`
	// Chunk holds its text, without the envelope.
	Chunk string `json:"chunk"`
}

// Label names an agent for a person: its name, else its type.
func (a Agent) Label() string { return cmp.Or(a.Name, a.Type, "subagent") }

// IsMainSegment reports whether the record stands for a main segment, not
// an Agent: a stretch of the main conversation the harness ran under an
// agent id of its own and reported as a subagent stop. Its agent
// transcript file never exists, so the record is an InternalAgent with no
// line read and no agent transcript. record no longer spools one and
// ingest drops one, so this hides only records stored before that.
func (a Agent) IsMainSegment() bool {
	return a.Type == InternalAgent && a.Lines.Read == 0 && a.TranscriptChunk == ""
}

// RanIn reports whether the agent ran under the main turn turnID.
func (a Agent) RanIn(turnID string) bool { return slices.Contains(a.TurnIDs, turnID) }

// LastMessage is the latest of a's later messages that arrived at or
// before t: the last word its parent gave it before it acted at t. false
// when none had arrived by then, so the spawn brief was the last.
func (a Agent) LastMessage(t time.Time) (AgentMessage, bool) {
	var last AgentMessage
	found := false
	for _, m := range a.Messages {
		if !m.At.After(t) && (!found || !m.At.Before(last.At)) {
			last, found = m, true
		}
	}
	return last, found
}

// NeedsFill reports whether an import can still fill what a's record
// lacks: its spawn brief, or later messages it counted but did not keep.
// The record must have read lines of a transcript file to take them from
// and have ended at or after since. An InternalAgent has no transcript
// file, and a record with no line read never read one, so an import gives
// neither anything.
func NeedsFill(a Agent, since time.Time) bool {
	lacks := a.BriefChunk == "" || a.LaterMessages > len(a.Messages)
	return lacks && a.Type != InternalAgent && a.Lines.Read > 0 && !a.EndedAt.Before(since)
}

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
