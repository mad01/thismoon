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
	// OriginAgent is a message another agent session sent.
	OriginAgent TurnOrigin = "agent"
	// OriginContinuation is the summary the harness opens a session with
	// once it continues after compaction.
	OriginContinuation TurnOrigin = "continuation"
	// OriginNone is a turn without prompt text.
	OriginNone TurnOrigin = "none"
)

// originHeads maps the head the harness gives a prompt it wrote itself to
// the origin it marks, in the order PromptOrigin tries them.
var originHeads = []struct {
	head   string
	origin TurnOrigin
}{
	{"<task-notification>", OriginTask},
	{"Another Claude session sent a message:", OriginTeammate},
	{"<teammate-message", OriginTeammate},
	{"<agent-message", OriginAgent},
	{"<cross-session-message", OriginAgent},
	{"This session is being continued from a previous conversation", OriginContinuation},
}

// PromptOrigin classifies a prompt by its head: the harness wraps a task
// notification, a teammate's or another agent's message and the summary
// after compaction in a head of its own, and anything else is a person's.
// A prompt without text has OriginNone.
func PromptOrigin(prompt string) TurnOrigin {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return OriginNone
	}
	for _, h := range originHeads {
		if strings.HasPrefix(text, h.head) {
			return h.origin
		}
	}
	return OriginPerson
}
