package trace

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The Codex rollout shape a trace relies on, as Codex CLI 0.163 writes it
// to $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<thread id>.jsonl in
// the paginated history mode. Each line is {timestamp, type, payload},
// written when Codex appends it. These lines make the timeline:
//
//	response_item     message role user: the prompt, and the environment
//	                  context Codex sends beside it
//	                  function_call, custom_tool_call: a tool call {call_id,
//	                  name}, written before the usage of the response that
//	                  asked for it
//	                  function_call_output, custom_tool_call_output: the
//	                  answer to one tool call, by call_id
//	token_usage_record
//	                  one model response: response_id and its usage, written
//	                  after the response's items
//	event_msg         task_complete: the turn's loop stopped
//	                  item_completed: a CommandExecution's status and exit
//	                  code, when Codex writes one
//	turn_context      the model the turn runs, which no usage names
//
// Every other line (session_meta, world_state, developer messages,
// reasoning, assistant messages, the other events) is read past. A
// response's time is the stretch up to its usage line, so the usage line
// alone stands for the model call, and it carries the tool calls the
// response asked for.
type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Name       string          `json:"name"`
	CallID     string          `json:"call_id"`
	Output     json.RawMessage `json:"output"`
	Model      string          `json:"model"`
	ResponseID string          `json:"response_id"`
	Usage      *codexUsage     `json:"usage"`
	Item       *codexItem      `json:"item"`
}

// codexUsage is one response's usage. Input counts every input token,
// the cached and the cache-written ones too; reasoning is already counted
// in output.
type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	CachedRead int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
}

type codexItem struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code"`
}

// Line types, payload types and roles the Codex timeline reads.
const (
	codexTypeResponseItem = "response_item"
	codexTypeUsage        = "token_usage_record"
	codexTypeEvent        = "event_msg"
	codexTypeTurnContext  = "turn_context"
	codexTypeSessionMeta  = "session_meta"

	codexItemMessage          = "message"
	codexItemCall             = "function_call"
	codexItemCustomCall       = "custom_tool_call"
	codexItemCallOutput       = "function_call_output"
	codexItemCustomCallOutput = "custom_tool_call_output"
	codexEventTaskComplete    = "task_complete"
	codexEventItemCompleted   = "item_completed"
	codexItemCommand          = "CommandExecution"
	codexStatusFailed         = "failed"
	codexRoleUser             = "user"
)

// codexLineTypes are the line types only a Codex rollout writes.
var codexLineTypes = map[string]bool{
	codexTypeResponseItem: true,
	codexTypeUsage:        true,
	codexTypeEvent:        true,
	codexTypeTurnContext:  true,
	codexTypeSessionMeta:  true,
}

// codexExitCode finds the exit code in the header Codex puts on a command's
// output for the model.
var codexExitCode = regexp.MustCompile(`(?m)^Process exited with code (-?\d+)$`)

// codexUnsupported starts the output Codex answers a call to a tool it
// does not offer with.
const codexUnsupported = "unsupported call: "

// codexParser reads Codex lines in chunk order. It keeps the model the
// turn runs, the tool calls asked since the last usage line, and the calls
// whose command Codex reported failed.
type codexParser struct {
	model   string
	pending []toolUse
	failed  map[string]bool
}

// line decodes one Codex line, with parseLine's contract: ok is false for
// a line that is valid but not on the timeline, err is set for a broken
// one.
func (p *codexParser) line(raw []byte, line int, _ bool) (ev event, ok bool, err error) {
	var l codexLine
	if err := json.Unmarshal(raw, &l); err != nil {
		return event{}, false, err
	}
	var pl codexPayload
	if err := json.Unmarshal(l.Payload, &pl); err != nil {
		return event{}, false, err
	}
	kind, onTimeline := p.read(l.Type, pl)
	if !onTimeline {
		return event{}, false, nil
	}
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return event{}, false, err
	}
	ev = event{at: at, line: line, kind: kind}
	switch kind {
	case evAssistant:
		ev.msgID = pl.ResponseID
		ev.call = codexCall(p.model, pl.Usage)
		ev.uses = p.pending
		ev.stopReason = "end_turn"
		if len(p.pending) > 0 {
			ev.stopReason = "tool_use"
		}
		p.pending = nil
	case evUser:
		if pl.CallID != "" {
			ev.results = []toolResult{{id: pl.CallID, isError: p.isError(pl)}}
		}
	}
	return ev, true, nil
}

// read takes what the parser keeps from a line and says which event the
// line is, false for one the timeline reads past.
func (p *codexParser) read(typ string, pl codexPayload) (eventKind, bool) {
	switch typ {
	case codexTypeTurnContext:
		if pl.Model != "" {
			p.model = pl.Model
		}
	case codexTypeUsage:
		return evAssistant, pl.Usage != nil
	case codexTypeEvent:
		return p.event(pl)
	case codexTypeResponseItem:
		return p.item(pl)
	}
	return 0, false
}

func (p *codexParser) event(pl codexPayload) (eventKind, bool) {
	switch pl.Type {
	case codexEventTaskComplete:
		return evStop, true
	case codexEventItemCompleted:
		it := pl.Item
		if it != nil && it.Type == codexItemCommand && it.ID != "" &&
			(it.Status == codexStatusFailed || it.ExitCode != nil && *it.ExitCode != 0) {
			if p.failed == nil {
				p.failed = map[string]bool{}
			}
			p.failed[it.ID] = true
		}
	}
	return 0, false
}

func (p *codexParser) item(pl codexPayload) (eventKind, bool) {
	switch pl.Type {
	case codexItemMessage:
		return evUser, pl.Role == codexRoleUser
	case codexItemCall, codexItemCustomCall:
		if pl.CallID != "" {
			p.pending = append(p.pending, toolUse{id: pl.CallID, name: pl.Name})
		}
	case codexItemCallOutput, codexItemCustomCallOutput:
		return evUser, true
	}
	return 0, false
}

// isError reports whether a tool call's output says it failed: Codex
// reported its command failed, the output's header names a nonzero exit
// code, or Codex does not offer the tool. Any other failure, such as a
// patch that did not apply, reads as a success.
func (p *codexParser) isError(pl codexPayload) bool {
	if p.failed[pl.CallID] {
		return true
	}
	var out string
	if json.Unmarshal(pl.Output, &out) != nil {
		return false
	}
	if strings.HasPrefix(out, codexUnsupported) {
		return true
	}
	m := codexExitCode.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	code, err := strconv.Atoi(m[1])
	return err == nil && code != 0
}

// codexCall reads a Codex usage into the call it billed. The uncached
// input is what the cached and cache-written tokens leave of the input;
// Codex reports one cache write, priced as the 5-minute one.
func codexCall(model string, u *codexUsage) Call {
	c := Call{Model: model}
	if u == nil {
		return c
	}
	c.Tokens = Tokens{
		Input:        max(u.Input-u.CachedRead-u.CacheWrite, 0),
		CacheWrite5m: u.CacheWrite,
		CacheRead:    u.CachedRead,
		Output:       u.Output,
		Thinking:     u.Reasoning,
	}
	return c
}

// isCodexLine reports whether a line with this type and payload is one
// only a Codex rollout writes.
func isCodexLine(typ string, payload json.RawMessage) bool {
	return codexLineTypes[typ] && len(bytes.TrimSpace(payload)) > 0
}
