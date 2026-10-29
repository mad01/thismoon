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
	// LaterMessages counts the messages the file shows reaching the agent
	// after its spawn brief, such as a lead's follow-ups to a teammate:
	// the envelopes later user lines open with (laterMessages), and any
	// after the first in the brief's own line. The brief holds none of
	// them.
	LaterMessages int
	// Transcript is the agent transcript, newline-terminated lines.
	Transcript []byte
	Lines      store.AgentLines

	// sawUser is set once the first user line has given the brief.
	sawUser bool
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
	if !timelineTypes[typ] {
		return nil, false
	}
	if typ == "user" {
		c.noteUser(l)
	}
	slim, model := slimLine(l)
	c.note(l, model)
	out, err := json.Marshal(slim)
	return out, err == nil
}

// noteUser takes the spawn brief from the first user line and counts the
// messages every user line delivers after it.
func (c *Capture) noteUser(l fields) {
	if c.sawUser {
		c.LaterMessages += laterMessages(l)
		return
	}
	c.sawUser = true
	var more int
	c.Brief, more = briefOf(l)
	c.LaterMessages += more
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
	at, err := time.Parse(time.RFC3339Nano, l.str("timestamp"))
	if err != nil {
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
