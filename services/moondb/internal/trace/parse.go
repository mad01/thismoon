package trace

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"
)

// The transcript shape a trace relies on. Each line is one JSON object
// with a type and an RFC 3339 timestamp. These lines make the timeline:
//
//	assistant    message.id, message.model, message.usage,
//	             message.stop_reason, and message.content[] with tool_use
//	             blocks {id, name}. A streamed message spans several lines
//	             with one message.id; its last line carries the final usage.
//	user         the prompt, or message.content[] with tool_result blocks
//	             {tool_use_id, is_error} when it answers tool calls
//	api-request  the harness about to send a request to the model
//	system       with subtype stop_hook_summary: the Stop hooks ran, so
//	             the loop stopped and waits for input
//
// Every other line (attachments, other system lines, bookkeeping) is read
// past. A line marked isSidechain belongs to a subagent, not the main
// conversation, and is read past in a turn's lines; an agent transcript
// holds nothing else. A user line's promptId names the main turn it came
// in under, which in an agent transcript says which turn the agent was
// working for.
type rawLine struct {
	Type        string      `json:"type"`
	Subtype     string      `json:"subtype"`
	Timestamp   string      `json:"timestamp"`
	IsSidechain bool        `json:"isSidechain"`
	PromptID    string      `json:"promptId"`
	Message     *rawMessage `json:"message"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *rawUsage       `json:"usage"`
	// StopReason is why the model stopped: tool_use while the loop goes
	// on, end_turn when it finished.
	StopReason string `json:"stop_reason"`
}

type rawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	OutputTokensDetails      *struct {
		ThinkingTokens int64 `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
	CacheCreation *struct {
		Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	Speed        string `json:"speed"`
	InferenceGeo string `json:"inference_geo"`
}

type rawBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
}

// eventKind is which kind of timeline line an event is.
type eventKind int

const (
	evAssistant eventKind = iota
	evUser
	evRequest
	evStop
)

// stopMarker is the subtype of the system line written when the Stop
// hooks ran.
const stopMarker = "stop_hook_summary"

// event is one line on the timeline.
type event struct {
	at time.Time
	// line is the line's position in the chunk: the order among equal
	// times, and which line of a streamed message came last.
	line int
	kind eventKind
	// promptID is the line's promptId, "" on a line without one.
	promptID string

	// assistant lines
	msgID      string
	call       Call
	uses       []toolUse
	stopReason string

	// user lines
	results []toolResult
}

type toolUse struct{ id, name string }

type toolResult struct {
	id      string
	isError bool
}

// parseLines reads a transcript chunk into its timeline events, sorted by
// time with the chunk order breaking ties. A line that does not parse is
// counted as skipped and never fails the whole chunk. A chunk over the size
// cap keeps its tail from a line boundary, so its head is missing rather
// than broken; cutHead reports it.
// sidechain keeps the lines a subagent wrote, for an agent transcript; a
// turn's lines read past them.
func parseLines(chunk []byte, sidechain bool) ([]event, Lines) {
	var counts Lines
	var events []event
	for i, raw := range bytes.Split(chunk, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		counts.Read++
		ev, ok, err := parseLine(raw, i, sidechain)
		switch {
		case err != nil:
			counts.Skipped++
		case ok:
			events = append(events, ev)
		}
	}
	slices.SortStableFunc(events, func(a, b event) int { return a.at.Compare(b.at) })
	counts.Events = len(events)
	counts.CutHead = cutHead(events)
	return events, counts
}

// cutHead reports whether the timeline starts inside the turn. A turn opens
// on its prompt, so a first event that is an assistant line or a tool
// result answers lines the chunk no longer holds: the size cap cut them.
func cutHead(events []event) bool {
	if len(events) == 0 {
		return false
	}
	first := events[0]
	return first.kind == evAssistant || len(first.results) > 0
}

// parseLine decodes one line. ok is false for a line that is valid but not
// on the timeline; err is set for one that is broken.
func parseLine(raw []byte, line int, sidechain bool) (ev event, ok bool, err error) {
	var l rawLine
	if err := json.Unmarshal(raw, &l); err != nil {
		return event{}, false, err
	}
	kind, onTimeline := timelineKind(l)
	if !onTimeline || l.IsSidechain && !sidechain {
		return event{}, false, nil
	}
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return event{}, false, err
	}
	ev = event{at: at, line: line, kind: kind, promptID: l.PromptID}
	if l.Message == nil {
		return ev, true, nil
	}
	blocks := contentBlocks(l.Message.Content)
	switch kind {
	case evAssistant:
		ev.msgID = l.Message.ID
		ev.call = usageCall(l.Message.Model, l.Message.Usage)
		ev.uses = toolUses(blocks)
		ev.stopReason = l.Message.StopReason
	case evUser:
		ev.results = toolResults(blocks)
	}
	return ev, true, nil
}

// timelineKind maps a line's type to its event kind; false for lines the
// timeline reads past.
func timelineKind(l rawLine) (eventKind, bool) {
	switch l.Type {
	case "assistant":
		return evAssistant, true
	case "user":
		return evUser, true
	case "api-request":
		return evRequest, true
	case "system":
		return evStop, l.Subtype == stopMarker
	}
	return 0, false
}

// contentBlocks decodes a message's content when it is an array of blocks.
// A plain string, or content that does not decode, has no blocks.
func contentBlocks(content json.RawMessage) []rawBlock {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || content[0] != '[' {
		return nil
	}
	var blocks []rawBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}
	return blocks
}

func toolUses(blocks []rawBlock) []toolUse {
	var out []toolUse
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			out = append(out, toolUse{id: b.ID, name: b.Name})
		}
	}
	return out
}

func toolResults(blocks []rawBlock) []toolResult {
	var out []toolResult
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			out = append(out, toolResult{id: b.ToolUseID, isError: b.IsError})
		}
	}
	return out
}

// usageCall reads a message's usage into the call it billed. Cache writes
// split by duration when the usage says how; otherwise they count as
// 5-minute writes, the default duration.
func usageCall(model string, u *rawUsage) Call {
	c := Call{Model: model}
	if u == nil {
		return c
	}
	c.Speed, c.Geo = u.Speed, u.InferenceGeo
	c.Tokens = Tokens{
		Input:        u.InputTokens,
		CacheWrite5m: u.CacheCreationInputTokens,
		CacheRead:    u.CacheReadInputTokens,
		Output:       u.OutputTokens,
	}
	if d := u.OutputTokensDetails; d != nil {
		c.Tokens.Thinking = d.ThinkingTokens
	}
	if cc := u.CacheCreation; cc != nil && cc.Ephemeral1h > 0 {
		total := max(u.CacheCreationInputTokens, cc.Ephemeral5m+cc.Ephemeral1h)
		c.Tokens.CacheWrite1h = cc.Ephemeral1h
		c.Tokens.CacheWrite5m = total - cc.Ephemeral1h
	}
	return c
}
