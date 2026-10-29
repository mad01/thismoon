package agentfile

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
)

// The text the harness adds around a brief: the wrapper of the message a
// teammate is spawned with, which holds the brief, and the boilerplate a
// fork's directive follows.
var (
	teammateWrap    = regexp.MustCompile(`(?s)\A<teammate-message\b[^>]*>(.*)</teammate-message>\z`)
	forkBoilerplate = regexp.MustCompile(`(?s)\A<fork-boilerplate>.*?</fork-boilerplate>`)
)

// briefOf is the brief a user line gives: its message's text (messageText)
// without the harness's own text around it, cut to moondb.MaxBriefBytes.
// "" when the line holds no text.
func briefOf(l fields) string {
	var msg fields
	if json.Unmarshal(l["message"], &msg) != nil || msg == nil {
		return ""
	}
	text := strings.TrimSpace(messageText(msg["content"]))
	if m := teammateWrap.FindStringSubmatch(text); m != nil {
		text = strings.TrimSpace(m[1])
	}
	text = strings.TrimSpace(forkBoilerplate.ReplaceAllString(text, ""))
	return cut(text, moondb.MaxBriefBytes)
}

// messageText is a message content's text: a string as it is, an array's
// text blocks joined by a blank line. A fork's first line holds its
// spawn's tool result beside the text, and the result is left out.
func messageText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []fields
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.str("type") == "text" {
			parts = append(parts, b.str("text"))
		}
	}
	return strings.Join(parts, "\n\n")
}
