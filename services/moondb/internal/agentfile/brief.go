package agentfile

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// The text the harness adds around a message: the envelope of a message
// one agent of a team sends another, which holds a teammate's spawn brief
// and each later message its lead sends it, and the boilerplate a fork's
// directive follows. An envelope's attributes are quoted, so a '>' inside
// one does not end its tag, and its body ends at the first closing tag,
// so two envelopes in one line stay two. Its teammate_id names the
// sender.
var (
	teammateEnvelope = regexp.MustCompile(
		`(?s)<teammate-message\b((?:[^>"]|"[^"]*")*)>(.*?)</teammate-message>`,
	)
	attribute       = regexp.MustCompile(`([\w-]+)="([^"]*)"`)
	forkBoilerplate = regexp.MustCompile(`(?s)\A<fork-boilerplate>.*?</fork-boilerplate>`)
)

// harnessHeads open the text of a user line the harness wrote that
// store.PromptOrigin takes for a person's: a reminder, a local command's
// output and an interrupt marker. None is a message to the agent.
var harnessHeads = []string{
	"<system-reminder>", "<local-command-stdout>", "<local-command-stderr>",
	"<bash-stdout>", "<bash-stderr>", "[Request interrupted",
}

// Message is a later message: one that reached the agent after its spawn
// brief, as the transcript line that delivered it shows it.
type Message struct {
	// At is the time of the line that delivered it.
	At time.Time
	// From is the teammate_id of its envelope; "" for a line with none.
	From string
	// Text is its body without the envelope, cut to moondb.MaxBriefBytes.
	Text string
}

// envelope is one teammate envelope: who sent it and its body.
type envelope struct{ from, body string }

// briefOf is the spawn brief a first user line gives, the text of its
// message (userText) without the harness's text around it, cut to
// moondb.MaxBriefBytes, and the messages the line delivers after it: a
// line that opens with envelopes gives the first one's body as the brief,
// and the rest arrived with it. "" when the line holds no text.
func briefOf(l fields) (string, []envelope) {
	text := userText(l)
	var more []envelope
	if envs := envelopes(text); len(envs) > 0 {
		text, more = envs[0].body, envs[1:]
	}
	text = strings.TrimSpace(forkBoilerplate.ReplaceAllString(text, ""))
	return cut(text, moondb.MaxBriefBytes), more
}

// messagesOf lists the messages a user line after the first delivers: one
// per envelope its text opens with, else the whole text of a line a
// person typed. A tool result, a line the harness marks as its own (meta,
// a compaction summary) or opens with a head of its own (harnessHeads,
// store.PromptOrigin), and a line without text deliver none.
func messagesOf(l fields) []envelope {
	if harnessLine(l) {
		return nil
	}
	text := userText(l)
	if envs := envelopes(text); len(envs) > 0 {
		return envs
	}
	if text == "" || !typed(text) {
		return nil
	}
	return []envelope{{body: text}}
}

// harnessLine reports whether the harness marks l as a line it wrote for
// itself: one it adds to the model's context, or the summary that
// resumes a compacted transcript.
func harnessLine(l fields) bool {
	for _, key := range []string{"isMeta", "isCompactSummary", "isVisibleInTranscriptOnly"} {
		if string(l[key]) == "true" {
			return true
		}
	}
	return false
}

// typed reports whether text with no envelope at its head is a message
// someone sent the agent: a person's line or a relayed teammate's, not a
// line the harness opens with a head of its own.
func typed(text string) bool {
	switch store.PromptOrigin(text) {
	case store.OriginPerson:
		return !slices.ContainsFunc(harnessHeads, func(h string) bool {
			return strings.HasPrefix(text, h)
		})
	case store.OriginTeammate:
		return true
	}
	return false
}

// envelopes lists the teammate envelopes in text, when text opens with
// one; none otherwise.
func envelopes(text string) []envelope {
	if !strings.HasPrefix(text, "<teammate-message") {
		return nil
	}
	var out []envelope
	for _, m := range teammateEnvelope.FindAllStringSubmatch(text, -1) {
		out = append(out, envelope{from: sender(m[1]), body: strings.TrimSpace(m[2])})
	}
	return out
}

// sender is the teammate_id among an envelope's attributes, "" when it
// has none. Each value is read whole, so one that quotes another
// attribute does not stand in for it.
func sender(attrs string) string {
	for _, a := range attribute.FindAllStringSubmatch(attrs, -1) {
		if a[1] == "teammate_id" {
			return a[2]
		}
	}
	return ""
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
