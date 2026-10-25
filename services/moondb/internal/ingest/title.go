package ingest

import (
	"bytes"
	"cmp"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
)

// Transcript line types that name a session.
const (
	// lineAgentName carries agentName, the name the person gave the
	// session; it wins over a generated title.
	lineAgentName = "agent-name"
	// lineAITitle carries aiTitle, the title the harness generated.
	lineAITitle = "ai-title"
)

// titleEntry is the part of a transcript line that names the session.
type titleEntry struct {
	Type      string `json:"type"`
	AgentName string `json:"agentName"`
	AITitle   string `json:"aiTitle"`
}

// transcriptTitle is the title a turn's transcript lines give the session:
// the last name the person gave it, else the last title the harness
// generated, else "". Lines that mention neither type are not decoded.
func transcriptTitle(lines []byte) string {
	if !bytes.Contains(lines, []byte(lineAgentName)) &&
		!bytes.Contains(lines, []byte(lineAITitle)) {
		return ""
	}
	var name, generated string
	for raw := range bytes.Lines(lines) {
		if !bytes.Contains(raw, []byte(lineAgentName)) &&
			!bytes.Contains(raw, []byte(lineAITitle)) {
			continue
		}
		var e titleEntry
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		switch e.Type {
		case lineAgentName:
			name = cmp.Or(strings.TrimSpace(e.AgentName), name)
		case lineAITitle:
			generated = cmp.Or(strings.TrimSpace(e.AITitle), generated)
		}
	}
	return cmp.Or(name, generated)
}

// markup matches an XML-like tag: the harness wraps a teammate message, a
// slash command or a task notification in one, and a title reads better
// without it.
var markup = regexp.MustCompile(`</?[A-Za-z][\w:-]*(\s[^<>]*)?/?>`)

// titleLine is the first line of a prompt that holds text once its tags
// are gone, with runs of space collapsed, cut to
// moondb.SessionPromptLineRunes with an ellipsis.
func titleLine(prompt string) string {
	for line := range strings.Lines(prompt) {
		text := strings.Join(strings.Fields(markup.ReplaceAllString(line, " ")), " ")
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) <= moondb.SessionPromptLineRunes {
			return text
		}
		return string([]rune(text)[:moondb.SessionPromptLineRunes]) + "…"
	}
	return ""
}
