package agentfile

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
)

// fields is a JSON object decoded one level deep.
type fields map[string]json.RawMessage

// str returns the string at key, "" when it is absent or not a string.
func (f fields) str(key string) string {
	raw := f[key]
	if len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// timelineTypes are the line types an agent transcript keeps: the ones a
// trace reads (internal/trace). Attachments, request bodies and the other
// bookkeeping lines are dropped whole.
var timelineTypes = map[string]bool{
	"user": true, "assistant": true, "api-request": true, "system": true,
}

// lineFields are the top-level fields a kept line keeps. Everything else,
// the tool's own result object (toolUseResult) and the request bodies
// among it, is dropped.
var lineFields = []string{
	"type", "subtype", "uuid", "parentUuid", "timestamp", "sessionId", "agentId",
	"promptId", "cwd", "gitBranch", "isSidechain", "isMeta", "requestId", "id",
}

// messageFields are the message fields a kept line keeps beside its
// content. usage stays whole: a trace prices it.
var messageFields = []string{"id", "role", "model", "stop_reason", "usage"}

// blockFields are the content block fields kept as they are: the ones a
// trace reads to pair a tool call with its result.
var blockFields = []string{"type", "id", "name", "tool_use_id", "is_error"}

// slimLine keeps l's lineFields, a system line's content cut to its head,
// and its message slimmed. It returns the message's model beside it.
func slimLine(l fields) (fields, string) {
	out := pick(l, lineFields)
	if c, ok := l["content"]; ok {
		out["content"] = slimContent(c)
	}
	var msg fields
	if json.Unmarshal(l["message"], &msg) != nil || msg == nil {
		return out, ""
	}
	m := pick(msg, messageFields)
	if c, ok := msg["content"]; ok {
		m["content"] = slimContent(c)
	}
	out["message"] = marshal(m)
	return out, msg.str("model")
}

// slimContent cuts a message's content: a string to its head, an array
// block by block.
func slimContent(raw json.RawMessage) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return marshal(head(s))
	}
	var blocks []fields
	if json.Unmarshal(raw, &blocks) != nil {
		return json.RawMessage(`null`)
	}
	out := make([]fields, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, slimBlock(b))
	}
	return marshal(out)
}

// slimBlock keeps a block's blockFields and the head of what it says: a
// text or thinking block's text, a tool call's input, a tool result's
// content flattened to text. A thinking block's signature is dropped.
func slimBlock(b fields) fields {
	out := pick(b, blockFields)
	for _, key := range []string{"text", "thinking"} {
		if _, ok := b[key]; ok {
			out[key] = marshal(head(b.str(key)))
		}
	}
	if in, ok := b["input"]; ok {
		out["input"] = slimInput(in)
	}
	if c, ok := b["content"]; ok {
		out["content"] = marshal(head(resultText(c)))
	}
	return out
}

// slimInput keeps a tool call's input as it is when it fits in the head,
// and otherwise the head of its JSON text, as a string.
func slimInput(raw json.RawMessage) json.RawMessage {
	if len(raw) <= moondb.AgentHeadBytes {
		return raw
	}
	return marshal(head(string(raw)))
}

// resultText flattens a tool result's content to text: a string as it is,
// an array's text blocks joined, each image as [image].
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []fields
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.str("type") {
		case "text":
			parts = append(parts, b.str("text"))
		case "":
		default:
			parts = append(parts, "["+b.str("type")+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// head cuts s to moondb.AgentHeadBytes at a rune boundary and says how
// many bytes it cut.
func head(s string) string {
	n := moondb.AgentHeadBytes
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return fmt.Sprintf("%s...[+%d bytes]", s[:n], len(s)-n)
}

// pick copies the fields of f named in keys.
func pick(f fields, keys []string) fields {
	out := make(fields, len(keys)+2)
	for _, k := range keys {
		if v, ok := f[k]; ok {
			out[k] = v
		}
	}
	return out
}

// marshal encodes v, which is always a string, a fields or a slice of
// them, so it cannot fail.
func marshal(v any) json.RawMessage {
	out, _ := json.Marshal(v)
	return out
}
