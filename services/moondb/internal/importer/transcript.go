package importer

import (
	"bytes"
	"encoding/json"
	"time"
)

// The transcript shape this package relies on. A transcript is JSON lines,
// one entry per line; only the fields named here are read.
//
// Layout under the projects directory:
//
//	<project>/<sessionId>.jsonl                          the main transcript
//	<project>/<sessionId>/subagents/agent-<agentId>.jsonl one subagent's entries
//	<project>/<sessionId>/subagents/agent-<agentId>.meta.json   agentType
//
// Every entry: type, sessionId, timestamp, and on most the entrypoint
// (cli, sdk-cli, ...) the harness was started by. The types read:
//
//	user, assistant   uuid, cwd, gitBranch, isSidechain, isMeta, agentId
//	                  (subagent entries), promptId (user entries, the same id
//	                  the prompt hook calls prompt_id; a subagent's entries
//	                  carry the promptId of the main turn they run under),
//	                  message.id, message.model, message.content
//	message.content   a string (a typed prompt) or an array of blocks:
//	                  text{text}, tool_use{id, name, input},
//	                  tool_result{tool_use_id, is_error}
//	toolUseResult     on the user entry that answers a tool call:
//	                  Edit: filePath, oldString, newString, replaceAll,
//	                  originalFile (null for files past about 10 KB),
//	                  structuredPatch, userModified.
//	                  Write: type (create|update), filePath, content,
//	                  originalFile, structuredPatch, userModified.
//	                  MultiEdit: filePath, edits, originalFile or
//	                  originalFileContents, structuredPatch.
//	                  A failed call: a string, with is_error on the block.
//	agent-name        agentName: the name the person gave the session
//	ai-title          aiTitle: the title the harness generated
//	file-history-snapshot  snapshot.trackedFileBackups{<path>: backup}
//	file-history-delta     trackingPath, backup
//	backup            backupFileName (null: the file did not exist),
//	                  backupTime, realParentDir
//
// The other types the harness writes are known and skipped (ignoredTypes);
// any type outside both lists is counted as unknown, so a new shape shows
// up in the import counts instead of vanishing.

// Entry types the importer reads.
const (
	typeUser      = "user"
	typeAssistant = "assistant"
	typeAgentName = "agent-name"
	typeAITitle   = "ai-title"
	typeSnapshot  = "file-history-snapshot"
	typeDelta     = "file-history-delta"
)

// ignoredTypes are entry types the harness writes that carry nothing the
// store records.
var ignoredTypes = map[string]bool{
	"attachment": true, "system": true, "last-prompt": true, "atis-latch": true,
	"mode": true, "permission-mode": true, "api-request": true, "api-request-blob": true,
	"api-request-shape": true, "pr-link": true, "queue-operation": true, "cost-state": true,
	"agent-setting": true, "fork-context-ref": true, "continued-in": true,
}

// File tools whose calls become edits.
const (
	toolEdit     = "Edit"
	toolWrite    = "Write"
	toolMulti    = "MultiEdit"
	toolNotebook = "NotebookEdit"
	toolBash     = "Bash"
)

func fileTool(name string) bool {
	switch name {
	case toolEdit, toolWrite, toolMulti, toolNotebook:
		return true
	}
	return false
}

// maxLineBytes bounds one transcript line. Real lines stay under a few
// MiB; a longer one is skipped and counted, never buffered whole.
const maxLineBytes = 64 << 20

// entry is one transcript line, decoded one level deep. The nested parts
// stay raw until the entry's type says how to read them, so a line whose
// message is shaped differently (api-request-blob) still decodes.
type entry struct {
	Type          string          `json:"type"`
	UUID          string          `json:"uuid"`
	SessionID     string          `json:"sessionId"`
	Timestamp     string          `json:"timestamp"`
	Cwd           string          `json:"cwd"`
	GitBranch     string          `json:"gitBranch"`
	Entrypoint    string          `json:"entrypoint"`
	IsSidechain   bool            `json:"isSidechain"`
	IsMeta        bool            `json:"isMeta"`
	AgentID       string          `json:"agentId"`
	PromptID      string          `json:"promptId"`
	Message       json.RawMessage `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	AgentName     string          `json:"agentName"`
	AITitle       string          `json:"aiTitle"`
	Snapshot      json.RawMessage `json:"snapshot"`
	TrackingPath  string          `json:"trackingPath"`
	Backup        json.RawMessage `json:"backup"`
}

// at is the entry's time, zero when it has none or an unreadable one.
func (e entry) at() time.Time {
	t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// message is the message of a user or assistant entry.
type message struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

// block is one content block. Input is kept raw for the tool_use record;
// a tool_result's own content is never decoded.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

// content is a message's content: the text of a string content, or the
// blocks of an array. ok is false for content that is neither.
type content struct {
	text   string
	isText bool
	blocks []block
}

func (e entry) content() (message, content, bool) {
	var m message
	if err := json.Unmarshal(e.Message, &m); err != nil {
		return message{}, content{}, false
	}
	c := bytes.TrimSpace(m.Content)
	switch {
	case len(c) > 0 && c[0] == '"':
		var s string
		if err := json.Unmarshal(c, &s); err != nil {
			return m, content{}, false
		}
		return m, content{text: s, isText: true}, true
	case len(c) > 0 && c[0] == '[':
		var bs []block
		if err := json.Unmarshal(c, &bs); err != nil {
			return m, content{}, false
		}
		return m, content{blocks: bs}, true
	}
	return m, content{}, false
}

// prompt returns what a person typed when the content is a prompt: a
// string, or text blocks with no tool result among them.
func (c content) prompt() (string, bool) {
	if c.isText {
		return c.text, true
	}
	var text []string
	for _, b := range c.blocks {
		switch b.Type {
		case "tool_result":
			return "", false
		case "text":
			text = append(text, b.Text)
		}
	}
	if len(text) == 0 {
		return "", false
	}
	var buf bytes.Buffer
	for i, t := range text {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(t)
	}
	return buf.String(), true
}

// backup is one file-history version as the transcript lists it.
type backup struct {
	BackupFileName *string `json:"backupFileName"`
	BackupTime     string  `json:"backupTime"`
	RealParentDir  string  `json:"realParentDir"`
}

// snapshot is a file-history-snapshot's payload.
type snapshot struct {
	TrackedFileBackups map[string]backup `json:"trackedFileBackups"`
}
