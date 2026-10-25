package record

import (
	"encoding/json"
	"fmt"
	"io"
)

// fields is a JSON object decoded one level deep, so a field of the wrong
// type reads as absent instead of failing the whole payload.
type fields map[string]json.RawMessage

// str returns the string at key, "" when it is absent or not a string.
func (f fields) str(key string) string {
	s, _ := f.text(key)
	return s
}

// text returns the string at key and whether the value is a JSON string,
// which tells an empty string from a null or a missing field.
func (f fields) text(key string) (string, bool) {
	raw := f[key]
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// obj returns the object at key, nil when it is absent or not an object.
func (f fields) obj(key string) fields {
	var o fields
	if err := json.Unmarshal(f[key], &o); err != nil {
		return nil
	}
	return o
}

// flag returns the boolean at key, false when it is absent or not a bool.
func (f fields) flag(key string) bool {
	var b bool
	_ = json.Unmarshal(f[key], &b)
	return b
}

// array returns the raw value at key when it is a JSON array.
func (f fields) array(key string) json.RawMessage {
	raw := f[key]
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	return raw
}

// payload is the part of a hook payload Run reads. Every event carries the
// session, prompt, transcript and cwd; inside a subagent also the agent.
type payload struct {
	event          string
	sessionID      string
	promptID       string
	transcriptPath string
	cwd            string
	agentID        string
	agentType      string
	// UserPromptSubmit.
	prompt string
	title  string
	// PostToolUse. input is tool_input verbatim; inputFields and response
	// are tool_input and tool_response decoded one level deep.
	tool        string
	toolUseID   string
	input       json.RawMessage
	inputFields fields
	response    fields
	// Stop.
	lastMessage string
}

// decodePayload reads one hook payload. Only a payload that is not a JSON
// object is an error; a field of the wrong type reads as empty.
func decodePayload(in io.Reader) (payload, error) {
	var f fields
	if err := json.NewDecoder(in).Decode(&f); err != nil {
		return payload{}, fmt.Errorf("record: decode payload: %w", err)
	}
	p := payload{
		event:          f.str("hook_event_name"),
		sessionID:      f.str("session_id"),
		promptID:       f.str("prompt_id"),
		transcriptPath: f.str("transcript_path"),
		cwd:            f.str("cwd"),
		agentID:        f.str("agent_id"),
		agentType:      f.str("agent_type"),
		prompt:         f.str("prompt"),
		title:          f.str("session_title"),
		tool:           f.str("tool_name"),
		toolUseID:      f.str("tool_use_id"),
		inputFields:    f.obj("tool_input"),
		response:       f.obj("tool_response"),
		lastMessage:    f.str("last_assistant_message"),
	}
	if p.inputFields != nil {
		p.input = f["tool_input"]
	}
	return p, nil
}
