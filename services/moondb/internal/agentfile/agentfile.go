// Package agentfile reads one subagent's transcript file, and the meta file
// the harness writes beside it, into an agent transcript: the agent's
// timeline lines with each text, thinking block, tool input and tool
// result cut to a head (moondb.AgentHeadBytes), capped at
// moondb.MaxAgentTranscriptBytes. The usage, ids, names, times, promptId
// and cwd of every kept line stay as the harness wrote them, so
// internal/trace reads an agent transcript as it reads a turn's.
//
// `moondb record` reads one when a SubagentStop hook fires and `moondb
// import` one per subagent file of a session it imports, so both ship the
// same record.
package agentfile

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/jsonl"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// maxLineBytes bounds one transcript line read. A longer one is skipped,
// as the importer skips it.
const maxLineBytes = 64 << 20

// Capture is one subagent as its files tell it.
type Capture struct {
	// ID is the agentId its lines carry, or the one in the file name
	// (agent-<id>.jsonl) when no line carries one.
	ID string
	// Type and Name follow Kind.
	Type        string
	Name        string
	Description string
	// Model is the first model an assistant line names, else the meta
	// file's model.
	Model string
	// TurnIDs are the promptIds of the agent's lines, first seen first:
	// the main turns it ran under.
	TurnIDs   []string
	Cwd       string
	GitBranch string
	// StartedAt and LastAt are the earliest and latest time a timeline
	// line carries, the lines the cap cut among them.
	StartedAt time.Time
	LastAt    time.Time
	// Brief is the spawn brief, the prompt the agent was spawned with: the
	// text of the file's first user line (briefOf), whole up to
	// moondb.MaxBriefBytes. "" when that line holds no text.
	Brief string
	// Messages are the later messages, oldest first: the ones the file
	// shows reaching the agent after its spawn brief, such as a lead's
	// follow-ups to a teammate (messagesOf), a parent's SendMessage queued
	// to a running subagent (queuedOf), and any envelope after the first
	// in the brief's own line. The brief holds none of them.
	Messages []Message
	// Transcript is the agent transcript, newline-terminated lines.
	Transcript []byte
	Lines      store.AgentLines

	// sawUser is set once the first user line has given the brief.
	sawUser bool
	// response is the message id of the latest model response read, and
	// calls holds the ids of the tool calls each response made, by its id.
	response string
	calls    map[string][]string
}

// ErrNoLines is returned for a file with no timeline line in it.
var ErrNoLines = errors.New("agentfile: no transcript lines")

// Read reads the transcript at path and the meta file beside it. hookType
// is the agent_type a hook payload named, "" when there is none. When ctx
// ends before the file does, or a read fails after a timeline line, Read
// keeps what it read and marks the capture partial; only a file it cannot
// open or read a timeline line from is an error.
func Read(ctx context.Context, path, hookType string) (Capture, error) {
	f, err := os.Open(path)
	if err != nil {
		return Capture{}, err
	}
	defer f.Close()
	c, err := readLines(ctx, f)
	if err != nil {
		return Capture{}, err
	}
	c.ID = cmp.Or(c.ID, IDFromPath(path))
	m := ReadMeta(MetaPath(path))
	c.Type, c.Name = Kind(m, hookType)
	c.Description = m.Description
	c.Model = cmp.Or(c.Model, m.Model)
	return c, nil
}

// readLines reads r's timeline lines into a capture, slimmed and capped.
func readLines(ctx context.Context, r io.Reader) (Capture, error) {
	var c Capture
	var kept [][]byte
	_, err := jsonl.Scan(r, maxLineBytes, func(line []byte, _ int64) bool {
		if ctx.Err() != nil {
			c.Lines.Partial = true
			return false
		}
		c.Lines.Read++
		if out, ok := c.add(line); ok {
			kept = append(kept, out)
		}
		return true
	})
	c.placeMessages()
	switch {
	case err != nil && len(kept) == 0:
		return Capture{}, err
	case err != nil:
		c.Lines.Partial = true
	case len(kept) == 0:
		return Capture{}, ErrNoLines
	}
	c.Transcript, c.Lines.Cut = capHead(kept, moondb.MaxAgentTranscriptBytes)
	c.Lines.Kept = len(kept) - c.Lines.Cut
	return c, nil
}

// IDFromPath is the agent id in a transcript file name, agent-<id>.jsonl.
func IDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return strings.TrimPrefix(base, "agent-")
}

// add notes what line says about the agent and returns its slimmed form,
// false for a line that is not on the timeline or does not decode.
func (c *Capture) add(line []byte) ([]byte, bool) {
	var l fields
	if json.Unmarshal(line, &l) != nil {
		return nil, false
	}
	typ := l.str("type")
	switch typ {
	case "user":
		c.noteUser(l)
	case "assistant":
		c.noteResponse(l)
	case "attachment":
		c.noteMessages(l, queuedOf(l))
	}
	if !timelineTypes[typ] {
		return nil, false
	}
	slim, model := slimLine(l)
	c.note(l, model)
	out, err := json.Marshal(slim)
	return out, err == nil
}

// noteUser takes the spawn brief from the first user line and the
// messages every user line delivers after it.
func (c *Capture) noteUser(l fields) {
	if c.sawUser {
		c.noteMessages(l, messagesOf(l))
		return
	}
	c.sawUser = true
	var envs []envelope
	c.Brief, envs = briefOf(l)
	c.noteMessages(l, envs)
}

// noteMessages adds the messages envs line l delivered, at its time and
// after the latest model response read.
func (c *Capture) noteMessages(l fields, envs []envelope) {
	at := lineTime(l)
	for _, e := range envs {
		c.Messages = append(c.Messages, Message{
			At: at, From: e.from, Text: cut(e.body, moondb.MaxBriefBytes), response: c.response,
		})
	}
}

// noteResponse notes the model response an assistant line belongs to and
// the tool calls the line holds. The lines of one response share its
// message id, and tool results can come between them.
func (c *Capture) noteResponse(l fields) {
	id, calls := responseOf(l)
	c.response = id
	if id == "" || len(calls) == 0 {
		return
	}
	if c.calls == nil {
		c.calls = map[string][]string{}
	}
	c.calls[id] = append(c.calls[id], calls...)
}

// placeMessages gives each message the tool calls of the model response
// it arrived after, once the whole file is read: a line of that response
// can follow the message's line.
func (c *Capture) placeMessages() {
	for i, m := range c.Messages {
		c.Messages[i].AfterCalls = c.calls[m.response]
	}
}

// AgentMessages is c's later messages as the Agent record keeps them,
// each one's text put in a chunk by add, which returns the chunk's key.
func (c Capture) AgentMessages(add func([]byte) string) []store.AgentMessage {
	var out []store.AgentMessage
	for _, m := range c.Messages {
		out = append(out, store.AgentMessage{
			At: m.At, From: m.From, Chunk: add([]byte(m.Text)), AfterCalls: m.AfterCalls,
		})
	}
	return out
}

// lineTime is the time a line carries, zero when it carries none.
func lineTime(l fields) time.Time {
	at, err := time.Parse(time.RFC3339Nano, l.str("timestamp"))
	if err != nil {
		return time.Time{}
	}
	return at
}

// note folds one timeline line's ids, place, time and model into c.
func (c *Capture) note(l fields, model string) {
	c.ID = cmp.Or(c.ID, l.str("agentId"))
	if p := l.str("promptId"); p != "" && !slices.Contains(c.TurnIDs, p) {
		c.TurnIDs = append(c.TurnIDs, p)
	}
	c.Cwd = cmp.Or(c.Cwd, l.str("cwd"))
	c.GitBranch = cmp.Or(c.GitBranch, l.str("gitBranch"))
	if c.Model == "" && model != syntheticModel {
		c.Model = model
	}
	at := lineTime(l)
	if at.IsZero() {
		return
	}
	if c.StartedAt.IsZero() || at.Before(c.StartedAt) {
		c.StartedAt = at
	}
	if at.After(c.LastAt) {
		c.LastAt = at
	}
}

// syntheticModel is the model the harness names on a message it wrote
// itself, with no model call behind it.
const syntheticModel = "<synthetic>"

// capHead joins lines, dropping whole lines from the head until the rest
// fits in limit bytes, and returns how many it dropped. The last line is
// kept even when it alone is over the limit.
func capHead(lines [][]byte, limit int) ([]byte, int) {
	size, start := 0, len(lines)
	for start > 0 && (start == len(lines) || size+len(lines[start-1])+1 <= limit) {
		start--
		size += len(lines[start]) + 1
	}
	out := make([]byte, 0, size)
	for _, l := range lines[start:] {
		out = append(append(out, l...), '\n')
	}
	return out, start
}
