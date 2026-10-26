package store

import "strings"

// TurnOrigin is what opened a turn: a person typing, or the harness
// starting the loop again for a message or a notification.
type TurnOrigin string

const (
	// OriginPerson is a prompt a person typed, a slash command included.
	OriginPerson TurnOrigin = "person"
	// OriginTask is a background task's notification that it finished.
	OriginTask TurnOrigin = "task"
	// OriginTeammate is a message a teammate of the session sent.
	OriginTeammate TurnOrigin = "teammate"
	// OriginSubagent is a message a subagent, or another session, sent.
	OriginSubagent TurnOrigin = "subagent"
	// OriginContinuation is the summary the harness opens a session with
	// once it continues after compaction.
	OriginContinuation TurnOrigin = "continuation"
	// OriginNone is a turn without prompt text.
	OriginNone TurnOrigin = "none"
)

// relayHead is the line the harness puts before a message it relays from
// another session. The wrapper after it says who sent the message.
const relayHead = "Another Claude session sent a message:"

// originHeads maps the head the harness gives a prompt it wrote itself to
// the origin it marks, in the order PromptOrigin tries them.
var originHeads = []struct {
	head   string
	origin TurnOrigin
}{
	{"<task-notification>", OriginTask},
	{"<teammate-message", OriginTeammate},
	{"<agent-message", OriginSubagent},
	{"<cross-session-message", OriginSubagent},
	{"This session is being continued from a previous conversation", OriginContinuation},
}

// PromptOrigin classifies a prompt by its head: the harness wraps a task
// notification, a teammate's or a subagent's message and the summary
// after compaction in a head of its own, and anything else is a person's.
// A relayed message is classified by the wrapper after relayHead, and is
// a teammate's when no known wrapper follows. A prompt without text has
// OriginNone.
func PromptOrigin(prompt string) TurnOrigin {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return OriginNone
	}
	if rest, ok := strings.CutPrefix(text, relayHead); ok {
		if o := headOrigin(strings.TrimSpace(rest)); o != OriginPerson {
			return o
		}
		return OriginTeammate
	}
	return headOrigin(text)
}

// headOrigin is the origin text's head marks, OriginPerson for none.
func headOrigin(text string) TurnOrigin {
	for _, h := range originHeads {
		if strings.HasPrefix(text, h.head) {
			return h.origin
		}
	}
	return OriginPerson
}
