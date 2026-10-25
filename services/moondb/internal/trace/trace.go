// Package trace says what took time in a turn and what it cost. It reads
// the raw transcript lines a Turn keeps (its TranscriptChunk) and builds a
// Trace: one span for the turn with a child span per model call, per tool
// call and per subagent, plus the turn's time split by what was running.
// The tree is two levels deep, so a TurnTrace holds the turn span and its
// children side by side.
//
// Everything here is a pure function of a turn's records, its transcript
// lines and a price table. Nothing reads the store or the clock, so serve
// can compute a trace on read and cache it.
//
// Vocabulary (CONTEXT.md): a Trace is that account of one turn or one
// session; a Span is one timed piece of it.
package trace

import (
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Kind says what a span timed.
type Kind string

const (
	// KindTurn is the whole turn, from the prompt to Stop.
	KindTurn Kind = "turn"
	// KindModel is one assistant message: from the event before it to its
	// last line, which is the model generating it.
	KindModel Kind = "model"
	// KindTool is one tool call: from the assistant line that asked for it
	// to the user line that carried its result.
	KindTool Kind = "tool"
	// KindSubagent is a subagent's recorded work in the turn, from its
	// first recorded edit to its last.
	KindSubagent Kind = "subagent"
)

// Span is one timed piece of a turn. The fields after End apply to the
// kinds their comments name and are empty otherwise.
type Span struct {
	Kind Kind `json:"kind"`
	// ID is the turn id, the message id, the tool_use id or the agent id.
	ID string `json:"id,omitempty"`
	// Name is "turn", the model id, the tool name or the agent type.
	Name       string    `json:"name"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	DurationMS int64     `json:"duration_ms"`

	// Tokens and Cost belong to model spans. Cost is in USD and nil when
	// the model has no price; it is never a guessed zero.
	Tokens *Tokens  `json:"tokens,omitempty"`
	Cost   *float64 `json:"cost,omitempty"`
	Speed  string   `json:"speed,omitempty"`

	// IsError, Unfinished and EditIDs belong to tool spans. Unfinished is
	// set when the turn's lines hold no result for the call; the span then
	// runs to the turn's end.
	IsError    bool     `json:"is_error,omitempty"`
	Unfinished bool     `json:"unfinished,omitempty"`
	EditIDs    []string `json:"edit_ids,omitempty"`

	// EditCount belongs to subagent spans.
	EditCount int `json:"edit_count,omitempty"`
}

// Tokens counts what a model call billed, by kind. It is the store's type,
// since a Turn keeps its totals; the name stays here for the trace's
// readers.
type Tokens = store.Tokens

// GapKind says what a stretch of a turn's time was spent on.
type GapKind string

const (
	// GapModel ends at an assistant line: the model was generating.
	GapModel GapKind = "model"
	// GapTool ends at a tool result: a tool was running.
	GapTool GapKind = "tool"
	// GapHarness ends at anything else: hooks, attachments, request setup
	// and the Stop hooks in the harness, and the stretch from the last
	// line to the turn's end when no Stop line came first.
	GapHarness GapKind = "harness"
	// GapUser is waiting for input: between one turn's Stop and the next
	// prompt, which is the session's, and inside a turn from the model
	// finishing to a new input under the same prompt id, such as a
	// teammate message, or from a Stop line to the turn's recorded end.
	GapUser GapKind = "user"
)

// Gap is a stretch of time with one kind. A turn's gaps cover it from
// start to end without overlap, so they add up to its wall time.
type Gap struct {
	Kind       GapKind   `json:"kind"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	DurationMS int64     `json:"duration_ms"`
}

// Totals sums a turn or a session. The *MS fields split WallMS by gap
// kind.
type Totals struct {
	WallMS    int64  `json:"wall_ms"`
	ModelMS   int64  `json:"model_ms"`
	ToolMS    int64  `json:"tool_ms"`
	HarnessMS int64  `json:"harness_ms"`
	UserMS    int64  `json:"user_ms"`
	Tokens    Tokens `json:"tokens"`
	// Usage splits Tokens by model and rate, the shape a Turn stores so
	// its cost can be priced again later.
	Usage []store.ModelUsage `json:"usage,omitempty"`
	// Cost is the USD sum of the priced model calls, nil when none is
	// priced. Unpriced names the models whose calls it leaves out.
	Cost       *float64 `json:"cost,omitempty"`
	Unpriced   []string `json:"unpriced,omitempty"`
	ModelCalls int      `json:"model_calls"`
	ToolCalls  int      `json:"tool_calls"`
	ToolErrors int      `json:"tool_errors"`
}

// Lines counts the transcript lines a trace read.
type Lines struct {
	// Read is every non-blank line.
	Read int `json:"read"`
	// Events are the timestamped user, assistant, API request and Stop
	// lines of the main conversation, the ones on the timeline.
	Events int `json:"events"`
	// Skipped lines did not parse, or had no usable timestamp.
	Skipped int `json:"skipped"`
	// CutHead is set when the lines start inside the turn: the first event
	// is an assistant line or a tool result, never the prompt a turn opens
	// on. A turn longer than the transcript cap keeps only its tail, cut at
	// a line boundary. The stretch from the turn's start to the first kept
	// line is then harness time, and the calls in the cut head add no
	// tokens or cost.
	CutHead bool `json:"cut_head,omitempty"`
}

// TurnTrace is the trace of one turn: the turn's span and, under it, the
// spans of its model calls, tool calls and subagents.
type TurnTrace struct {
	TurnID string `json:"turn_id"`
	Seq    int    `json:"seq"`
	// Prompt is the prompt's first line, set by the caller that has it.
	Prompt string `json:"prompt,omitempty"`
	Span   Span   `json:"span"`
	// Spans are the turn span's children by start time: one per model
	// call, tool call and subagent.
	Spans  []Span `json:"spans"`
	Gaps   []Gap  `json:"gaps"`
	Totals Totals `json:"totals"`
	Lines  Lines  `json:"lines"`
	// Reason says why the trace is empty, "" when it is not.
	Reason string `json:"reason,omitempty"`
}

// SessionTrace is the trace of every turn of a session, in order, with the
// user time between them.
type SessionTrace struct {
	SessionID string      `json:"session_id"`
	Turns     []TurnTrace `json:"turns"`
	// Gaps are the user gaps between one turn's end and the next one's
	// start.
	Gaps []Gap `json:"gaps"`
	// Totals sum the turns, with WallMS from the first turn's start to the
	// last one's end and UserMS the gaps between turns.
	Totals Totals `json:"totals"`
}

// Ranked is one span of a session trace with the turn it ran in.
type Ranked struct {
	Seq    int    `json:"seq"`
	TurnID string `json:"turn_id"`
	Prompt string `json:"prompt,omitempty"`
	Span   Span   `json:"span"`
}

// span returns a span of kind from start to end with its duration set.
func span(kind Kind, id, name string, start, end time.Time) Span {
	return Span{
		Kind: kind, ID: id, Name: name, Start: start, End: end,
		DurationMS: end.Sub(start).Milliseconds(),
	}
}
