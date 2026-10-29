package agentfile

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
)

// The text the harness adds around a message: the envelope of a message
// one agent of a team sends another, which holds a teammate's spawn brief
// and each later message its lead sends it, and the boilerplate a fork's
// directive follows. An envelope's attributes are quoted, so a '>' inside
// one does not end its tag, and its body ends at the first closing tag,
// so two envelopes in one line stay two.
var (
	teammateEnvelope = regexp.MustCompile(
		`(?s)<teammate-message\b(?:[^>"]|"[^"]*")*>(.*?)</teammate-message>`,
	)
	forkBoilerplate = regexp.MustCompile(`(?s)\A<fork-boilerplate>.*?</fork-boilerplate>`)
)

// briefOf is the spawn brief a first user line gives, the text of its
// message (userText) without the harness's text around it, cut to
// moondb.MaxBriefBytes, and how many more messages the line delivers: a
// line that opens with envelopes gives the first one's body as the brief,
// and the rest arrived after it. "" when the line holds no text.
func briefOf(l fields) (brief string, more int) {
	text := userText(l)
	if bodies := envelopes(text); len(bodies) > 0 {
		text, more = bodies[0], len(bodies)-1
	}
	text = strings.TrimSpace(forkBoilerplate.ReplaceAllString(text, ""))
	return cut(text, moondb.MaxBriefBytes), more
}

// laterMessages counts the messages a user line after the first delivers:
// the envelopes its text opens with. A tool result, a reminder the harness
// adds and a summary of a compacted transcript are none.
func laterMessages(l fields) int { return len(envelopes(userText(l))) }

// envelopes lists the bodies of the teammate envelopes in text, when text
// opens with one; none otherwise.
func envelopes(text string) []string {
	if !strings.HasPrefix(text, "<teammate-message") {
		return nil
	}
	var bodies []string
	for _, m := range teammateEnvelope.FindAllStringSubmatch(text, -1) {
		bodies = append(bodies, strings.TrimSpace(m[1]))
	}
	return bodies
}

// userText is the text of a user line's message (messageText), trimmed.
func userText(l fields) string {
	var msg fields
	if json.Unmarshal(l["message"], &msg) != nil || msg == nil {
		return ""
	}
	return strings.TrimSpace(messageText(msg["content"]))
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
