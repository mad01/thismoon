package trace

import (
	"bytes"
	"encoding/json"
	"time"
)

// The pi session file shape a trace relies on (pi's docs/session-format.md
// and docs/message-types.md, format version 3). Each line is one entry with
// a type, an id and an ISO timestamp, written when the entry is appended.
// These entries make the timeline:
//
//	message     role assistant: model, usage, stopReason, and content[]
//	            with toolCall blocks {id, name}. pi writes a message once,
//	            complete, so the entry id is the message's id.
//	            role user: a prompt.
//	            role toolResult: the answer to one tool call, by
//	            toolCallId, with isError.
//	usage       model work outside the conversation, such as cache_warm,
//	            with its own provider, model and usage
//	compaction, branch_summary
//	            the summary pi had the model write, with its usage and no
//	            model: it is priced at the model last seen before it
//
// Every other entry (the header, model and thinking changes, labels,
// custom entries, system messages) is read past. pi writes no line when
// the loop stops, so a pi turn has no Stop marker.
type piEntry struct {
	Type      string     `json:"type"`
	Timestamp string     `json:"timestamp"`
	ID        string     `json:"id"`
	Model     string     `json:"model"`
	ModelID   string     `json:"modelId"`
	Message   *piMessage `json:"message"`
	Usage     *piUsage   `json:"usage"`
}

type piMessage struct {
	Role       string          `json:"role"`
	Model      string          `json:"model"`
	Content    json.RawMessage `json:"content"`
	Usage      *piUsage        `json:"usage"`
	StopReason string          `json:"stopReason"`
	ToolCallID string          `json:"toolCallId"`
	IsError    bool            `json:"isError"`
}

// piUsage is a message's or entry's usage. Input is the uncached input;
// reasoning is already counted in output, and cacheWrite1h is the part of
// cacheWrite written with one-hour retention.
type piUsage struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cacheRead"`
	CacheWrite   int64 `json:"cacheWrite"`
	CacheWrite1h int64 `json:"cacheWrite1h"`
	Reasoning    int64 `json:"reasoning"`
}

type piBlock struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Entry types and message roles the pi timeline reads.
const (
	piTypeMessage       = "message"
	piTypeUsage         = "usage"
	piTypeCompaction    = "compaction"
	piTypeBranchSummary = "branch_summary"
	piTypeModelChange   = "model_change"
	piRoleAssistant     = "assistant"
	piRoleUser          = "user"
	piRoleToolResult    = "toolResult"
)

// piStopReasons maps pi's stop reasons onto the ones finishReasons knows.
// A run that stopped, failed or was aborted waits for new input; toolUse
// goes on by itself; a reason not listed says nothing.
var piStopReasons = map[string]string{
	"stop":    "end_turn",
	"error":   "end_turn",
	"aborted": "end_turn",
	"toolUse": "tool_use",
	"length":  "max_tokens",
}

// piParser reads pi lines in chunk order, remembering the model last seen
// so a summary entry, which names none, is priced at it.
type piParser struct {
	model string
}

// line decodes one pi line, with parseLine's contract: ok is false for a
// line that is valid but not on the timeline, err is set for a broken one.
func (p *piParser) line(raw []byte, line int, _ bool) (ev event, ok bool, err error) {
	var e piEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return event{}, false, err
	}
	switch {
	case e.Type == piTypeModelChange && e.ModelID != "":
		p.model = e.ModelID
		return event{}, false, nil
	case e.Type == piTypeMessage && e.Message != nil:
	case e.Type == piTypeUsage, e.Type == piTypeCompaction, e.Type == piTypeBranchSummary:
		if e.Usage == nil {
			return event{}, false, nil
		}
	default:
		return event{}, false, nil
	}
	at, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		return event{}, false, err
	}
	ev = event{at: at, line: line}
	if e.Message == nil {
		return p.summary(ev, e), true, nil
	}
	return p.message(ev, e)
}

// summary is the event of a usage, compaction or branch_summary entry:
// model work the turn paid for, outside the tool loop.
func (p *piParser) summary(ev event, e piEntry) event {
	model := e.Model
	if model == "" {
		model = p.model
	}
	ev.kind = evAssistant
	ev.msgID = e.ID
	ev.call = piCall(model, e.Usage)
	return ev
}

// message is the event of a message entry; ok is false for a role the
// timeline reads past.
func (p *piParser) message(ev event, e piEntry) (event, bool, error) {
	m := e.Message
	switch m.Role {
	case piRoleAssistant:
		if m.Model != "" {
			p.model = m.Model
		}
		ev.kind = evAssistant
		ev.msgID = e.ID
		ev.call = piCall(m.Model, m.Usage)
		ev.uses = piToolUses(m.Content)
		ev.stopReason = piStopReasons[m.StopReason]
	case piRoleUser:
		ev.kind = evUser
	case piRoleToolResult:
		ev.kind = evUser
		if m.ToolCallID != "" {
			ev.results = []toolResult{{id: m.ToolCallID, isError: m.IsError}}
		}
	default:
		return event{}, false, nil
	}
	return ev, true, nil
}

// piCall reads a pi usage into the call it billed. pi's cacheWrite holds
// both durations, so the 5-minute part is what the 1-hour part leaves.
func piCall(model string, u *piUsage) Call {
	c := Call{Model: model}
	if u == nil {
		return c
	}
	c.Tokens = Tokens{
		Input:        u.Input,
		CacheWrite5m: u.CacheWrite - u.CacheWrite1h,
		CacheWrite1h: u.CacheWrite1h,
		CacheRead:    u.CacheRead,
		Output:       u.Output,
		Thinking:     u.Reasoning,
	}
	return c
}

func piToolUses(content json.RawMessage) []toolUse {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || content[0] != '[' {
		return nil
	}
	var blocks []piBlock
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var out []toolUse
	for _, b := range blocks {
		if b.Type == "toolCall" && b.ID != "" {
			out = append(out, toolUse{id: b.ID, name: b.Name})
		}
	}
	return out
}
