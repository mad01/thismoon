package scorecard

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The fixture lines follow the shape Claude Code writes, as read from local
// sessions: an assistant line holds tool_use blocks {id, name, input}; a
// user line answers one call with a tool_result block whose content is an
// array of text blocks or a plain string, and a file read's user line
// carries the tool's own result as toolUseResult.file. A subagent's agent
// transcript (internal/agentfile) keeps each result as a string cut to its
// head, followed by ...[+N bytes].

// t0 anchors the fixture times.
var t0 = time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)

// at is the fixture time s seconds after t0, as a transcript writes it.
func at(s int) string {
	return t0.Add(time.Duration(s) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

// use is an assistant line asking for one tool call.
func use(sec int, id, name, input string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"isSidechain":false,`+
		`"message":{"id":"msg_%s","model":"claude-opus-5-5","role":"assistant",`+
		`"content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}],`+
		`"usage":{"input_tokens":2,"cache_read_input_tokens":90000,"output_tokens":120}}}`,
		at(sec), id, id, name, input)
}

// result is a user line answering one call. content is the tool_result's
// content as JSON; own, when set, is its toolUseResult.
func result(sec int, id, content string, isError bool, own string) string {
	line := fmt.Sprintf(`{"type":"user","timestamp":%q,"isSidechain":false,"promptId":"p1",`+
		`"message":{"role":"user","content":[{"tool_use_id":%q,"type":"tool_result",`+
		`"content":%s,"is_error":%t}]}`, at(sec), id, content, isError)
	if own != "" {
		line += `,"toolUseResult":` + own
	}
	return line + "}"
}

// agentUse and agentResult are a subagent's lines as its agent transcript
// keeps them: sidechain lines, the result cut to a string.
func agentUse(sec int, id, name, input string) string {
	return fmt.Sprintf(`{"type":"assistant","isSidechain":true,"timestamp":%q,`+
		`"message":{"content":[{"id":%q,"input":%s,"name":%q,"type":"tool_use"}],`+
		`"id":"msg_%s","model":"claude-opus-5-5","role":"assistant","stop_reason":"tool_use",`+
		`"usage":{"input_tokens":2,"output_tokens":40}}}`, at(sec), id, input, name, id)
}

func agentResult(sec int, id, text string) string {
	return fmt.Sprintf(`{"type":"user","isSidechain":true,"promptId":"p1","timestamp":%q,`+
		`"message":{"content":[{"content":%s,"tool_use_id":%q,"type":"tool_result"}],`+
		`"role":"user"}}`, at(sec), str(text), id)
}

// texts is a tool_result content of text blocks.
func texts(text string) string {
	b, _ := json.Marshal([]map[string]string{{"type": "text", "text": text}})
	return string(b)
}

// str is a JSON string.
func str(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// fileOwn is a file read's toolUseResult.
func fileOwn(path string, lines, start, total int) string {
	return fmt.Sprintf(`{"type":"text","file":{"filePath":%q,"content":"...",`+
		`"numLines":%d,"startLine":%d,"totalLines":%d}}`, path, lines, start, total)
}

// chunk joins lines into a transcript chunk.
func chunk(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// searchInput is a csl_search input.
const searchInput = `{"query":"retry|backoff","repo":"acme/repo$","limit":20}`
