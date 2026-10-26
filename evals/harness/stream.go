package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Transcript is what one `claude -p --output-format stream-json` run
// reported: the session, every tool call in order, and the result event.
// HasResult is false when the run ended before claude wrote a result, as a
// timeout or a crash does; the other result fields are then zero.
type Transcript struct {
	SessionID         string
	Model             string
	ClaudeVersion     string
	ToolCalls         []ToolCall
	HasResult         bool
	Result            string
	Subtype           string
	IsError           bool
	CostUSD           float64
	DurationMS        int64
	NumTurns          int
	Usage             Usage
	PermissionDenials int
	BadLines          int
}

// ToolCall is one tool_use block. Input is the call's input as compact JSON
// with sorted keys, so graders can match it with a regex.
type ToolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

// Usage is the result event's token accounting.
type Usage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
}

// Total is every token the run sent or received, cached or not.
func (u Usage) Total() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens
}

// ToolCounts returns how many times each tool was called.
func (t Transcript) ToolCounts() map[string]int {
	out := map[string]int{}
	for _, c := range t.ToolCalls {
		out[c.Name]++
	}
	return out
}

// streamEvent is the subset of a stream-json line the runner reads. Message
// stays raw because a user event's content may be a plain string.
type streamEvent struct {
	Type              string            `json:"type"`
	Subtype           string            `json:"subtype"`
	SessionID         string            `json:"session_id"`
	Model             string            `json:"model"`
	Version           string            `json:"claude_code_version"`
	Message           json.RawMessage   `json:"message"`
	Result            string            `json:"result"`
	IsError           bool              `json:"is_error"`
	TotalCostUSD      float64           `json:"total_cost_usd"`
	DurationMS        int64             `json:"duration_ms"`
	NumTurns          int               `json:"num_turns"`
	Usage             Usage             `json:"usage"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
}

type assistantMessage struct {
	Content []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

// parseStream reads a stream-json transcript. A line that is not valid JSON
// is counted in BadLines and skipped, so a run killed mid-line still yields
// the calls it made before that.
func parseStream(r io.Reader) (Transcript, error) {
	var t Transcript
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			t.apply(line)
		}
		if errors.Is(err, io.EOF) {
			return t, nil
		}
		if err != nil {
			return t, err
		}
	}
}

func (t *Transcript) apply(line []byte) {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		t.BadLines++
		return
	}
	switch {
	case ev.Type == "system" && ev.Subtype == "init":
		t.SessionID, t.Model, t.ClaudeVersion = ev.SessionID, ev.Model, ev.Version
	case ev.Type == "assistant":
		var msg assistantMessage
		if err := json.Unmarshal(ev.Message, &msg); err != nil {
			t.BadLines++
			return
		}
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				t.ToolCalls = append(t.ToolCalls,
					ToolCall{Name: block.Name, Input: canonicalJSON(block.Input)})
			}
		}
	case ev.Type == "result":
		t.HasResult = true
		t.Result, t.Subtype, t.IsError = ev.Result, ev.Subtype, ev.IsError
		t.CostUSD, t.DurationMS, t.NumTurns = ev.TotalCostUSD, ev.DurationMS, ev.NumTurns
		t.Usage = ev.Usage
		t.PermissionDenials = len(ev.PermissionDenials)
		if ev.SessionID != "" {
			t.SessionID = ev.SessionID
		}
	}
}

// canonicalJSON re-encodes raw with sorted keys and no HTML escaping, so a
// grader regex sees the same text whatever order the model wrote the keys
// in and "&&" stays "&&". Input that does not decode is returned as is.
func canonicalJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return string(raw)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
