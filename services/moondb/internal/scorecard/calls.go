package scorecard

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Call is one tool call read from transcript lines: what it asked for, and
// its result as the transcript keeps it. A session trace holds the call's
// span; a Call holds what the span leaves out.
type Call struct {
	ID   string
	Name string
	// AgentID is the subagent that made the call, "" for the main
	// conversation.
	AgentID string
	// At is the time of the line that asked for the call.
	At    time.Time
	Input json.RawMessage
	// Answered is set once a result came. Result is its text, the text
	// blocks joined by newlines and an image as [image].
	Answered bool
	Result   string
	// Cut is set when the transcript keeps only the head of the result, as
	// a subagent's transcript does, or a preview of an output the harness
	// saved to a file.
	Cut     bool
	IsError bool
	// Lines and TotalLines are what a file read returned and the file's
	// length, and StartLine the first line it returned, from the tool's own
	// result; -1 when it did not say.
	Lines      int
	TotalLines int
	StartLine  int
}

// unknownLines marks a line count the transcript does not hold.
const unknownLines = -1

// persistedPrefix starts a result the harness saved to a file, leaving a
// preview in the transcript.
const persistedPrefix = "<persisted-output>"

// cutHead is the marker a subagent's transcript puts after the head it
// keeps of a text (internal/agentfile).
var cutHead = regexp.MustCompile(`\.\.\.\[\+\d+ bytes\]$`)

// The transcript shape a Call relies on: an assistant line asks for tools
// with tool_use blocks {id, name, input}; a user line answers with
// tool_result blocks {tool_use_id, is_error, content}, and, when it answers
// one call, carries the tool's own result as toolUseResult. A file read's
// toolUseResult has file.numLines and file.totalLines.
type callLine struct {
	Type          string          `json:"type"`
	Timestamp     string          `json:"timestamp"`
	IsSidechain   bool            `json:"isSidechain"`
	Message       *callMessage    `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type callMessage struct {
	Content json.RawMessage `json:"content"`
}

type callBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
	Text      string          `json:"text"`
}

// fileResult is the part of a file read's toolUseResult a Call reads.
type fileResult struct {
	File *struct {
		NumLines   *int `json:"numLines"`
		TotalLines *int `json:"totalLines"`
		StartLine  *int `json:"startLine"`
	} `json:"file"`
}

// ParseCalls reads the tool calls in transcript lines, in the order they
// were asked for. agentID names the subagent whose agent transcript the
// lines are; "" reads a turn's lines, which skips the lines a subagent
// wrote into the main transcript, as a turn's trace does. A line that does
// not parse is read past: a scorecard counts what it can.
func ParseCalls(chunk []byte, agentID string) []Call {
	var calls []Call
	byID := map[string]int{}
	for _, raw := range bytes.Split(chunk, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var l callLine
		if json.Unmarshal(raw, &l) != nil || l.Message == nil {
			continue
		}
		if l.IsSidechain && agentID == "" {
			continue
		}
		blocks := decodeBlocks(l.Message.Content)
		switch l.Type {
		case "assistant":
			at, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
			calls = addUses(calls, byID, blocks, agentID, at)
		case "user":
			addResults(calls, byID, blocks, l.ToolUseResult)
		}
	}
	return calls
}

// addUses appends a Call per tool_use block not seen before.
func addUses(calls []Call, byID map[string]int, blocks []callBlock, agentID string,
	at time.Time,
) []Call {
	for _, b := range blocks {
		if b.Type != "tool_use" || b.ID == "" {
			continue
		}
		if _, seen := byID[b.ID]; seen {
			continue
		}
		byID[b.ID] = len(calls)
		calls = append(calls, Call{
			ID: b.ID, Name: b.Name, AgentID: agentID, At: at, Input: b.Input,
			Lines: unknownLines, TotalLines: unknownLines, StartLine: unknownLines,
		})
	}
	return calls
}

// addResults fills in the calls the tool_result blocks answer. The
// toolUseResult belongs to the line's one result; a line answering several
// calls leaves their line counts unknown.
func addResults(calls []Call, byID map[string]int, blocks []callBlock, own json.RawMessage) {
	var results []callBlock
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			results = append(results, b)
		}
	}
	for _, b := range results {
		i, ok := byID[b.ToolUseID]
		if !ok {
			continue
		}
		c := &calls[i]
		c.Answered, c.IsError = true, b.IsError
		c.Result = resultText(b.Content)
		c.Cut = cutHead.MatchString(c.Result) ||
			strings.HasPrefix(strings.TrimSpace(c.Result), persistedPrefix)
		if len(results) == 1 {
			c.Lines, c.TotalLines, c.StartLine = fileLines(own)
		}
	}
}

// fileLines reads a file read's line counts and first line from its
// toolUseResult, each unknownLines when it does not say.
func fileLines(raw json.RawMessage) (lines, total, start int) {
	var r fileResult
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &r) != nil || r.File == nil {
		return unknownLines, unknownLines, unknownLines
	}
	orUnknown := func(n *int) int {
		if n == nil {
			return unknownLines
		}
		return *n
	}
	return orUnknown(r.File.NumLines), orUnknown(r.File.TotalLines), orUnknown(r.File.StartLine)
}

// decodeBlocks decodes a message's content when it is an array of blocks.
func decodeBlocks(content json.RawMessage) []callBlock {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || content[0] != '[' {
		return nil
	}
	var blocks []callBlock
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	return blocks
}

// resultText flattens a tool result's content: a string as it is, the text
// blocks of an array joined by newlines, any other block as [type].
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []string
	for _, b := range decodeBlocks(raw) {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "":
		default:
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// toolName is a tool's own name: an MCP tool's name after its server
// prefix (mcp__csl__csl_search is csl_search), any other name as it is.
func toolName(name string) string {
	if !strings.HasPrefix(name, "mcp__") {
		return name
	}
	return name[strings.LastIndex(name, "__")+2:]
}
