package store

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// maxBodyBytes caps one message so a single record stays a sane JSONL line.
const maxBodyBytes = 64 * 1024

// previewRunes is how much of the last message the channel list shows.
const previewRunes = 160

// nameRE is the channel-name grammar: a lowercase slug that survives a URL
// path segment unescaped, so one agent can hand the name to another and have
// it work as-is in the API, the CLI, and the web page. Underscore is
// deliberately excluded — channel ids start with "ch_", so ids and names can
// never collide and either one resolves a channel.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

// Channel is a named conversation several sessions post into. The name is the
// handle: an agent opens a channel and passes the name (or the id) to another
// agent, which is the whole point of the service.
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Topic    string `json:"topic,omitempty"`
	OpenedBy string `json:"opened_by,omitempty"`
	// UpdatedAt is when the channel record itself last changed (opened or
	// closed), not when it last saw a message — that is the newest-wins key
	// for the JSONL log. Last activity is derived from the messages.
	UpdatedAt time.Time  `json:"updated_at"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	CloseNote string     `json:"close_note,omitempty"`
}

// Closed reports whether the conversation has been wrapped up. A closed
// channel takes no more messages and never blocks a reader waiting for one.
func (c Channel) Closed() bool { return c.ClosedAt != nil }

// Message is one immutable turn in a channel. ChannelID plus Seq is its
// identity; Seq is also the cursor a reader advances, so "everything after
// what I already saw" is a single integer comparison.
type Message struct {
	ChannelID string    `json:"channel_id"`
	Seq       int64     `json:"seq"`
	From      string    `json:"from"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Summary is a channel plus the read-model the list view needs: how much has
// been said, the cursor to resume from, who has spoken, and a preview of the
// last line. All of it is derived from the messages, never stored.
type Summary struct {
	Channel
	Messages     int        `json:"messages"`
	Cursor       int64      `json:"cursor"`
	Participants []string   `json:"participants"`
	LastFrom     string     `json:"last_from,omitempty"`
	LastBody     string     `json:"last_body,omitempty"`
	LastAt       *time.Time `json:"last_at,omitempty"`
}

// NormalizeName lowercases and trims a channel name and checks it against the
// slug grammar. An empty name is valid and reported as empty: the caller mints
// a fallback name for it.
func NormalizeName(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return "", nil
	}
	if !nameRE.MatchString(n) {
		return "", fmt.Errorf(
			"invalid channel name %q: use 1-64 chars of a-z, 0-9, dot, or dash, starting with a letter or digit",
			name,
		)
	}
	return n, nil
}

// checkFrom rejects an unattributed message: in a channel several sessions
// post into, a message nobody signed cannot be answered.
func checkFrom(from string) (string, error) {
	f := strings.TrimSpace(from)
	if f == "" {
		return "", errors.New("from is required — name the session posting")
	}
	if utf8.RuneCountInString(f) > 64 {
		return "", errors.New("from must be at most 64 characters")
	}
	return f, nil
}

// checkBody rejects an empty or oversized message body.
func checkBody(body string) (string, error) {
	b := strings.TrimRight(body, "\n")
	if strings.TrimSpace(b) == "" {
		return "", errors.New("body is required")
	}
	if len(b) > maxBodyBytes {
		return "", fmt.Errorf("body is %d bytes, over the %d limit", len(b), maxBodyBytes)
	}
	return b, nil
}

// preview shortens a message body for the channel list: one line, clipped on a
// rune boundary so multi-byte text never breaks mid-character.
func preview(body string) string {
	s := strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(s) <= previewRunes {
		return s
	}
	return string([]rune(s)[:previewRunes]) + "…"
}

// participants lists every distinct sender in order of first appearance, so
// the list view can show who is on a channel without a membership record.
func participants(msgs []Message) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range msgs {
		if seen[m.From] {
			continue
		}
		seen[m.From] = true
		out = append(out, m.From)
	}
	return out
}

// summarize folds a channel and its messages into the list read-model.
func summarize(c Channel, msgs []Message) Summary {
	s := Summary{
		Channel:      c,
		Messages:     len(msgs),
		Cursor:       int64(len(msgs)),
		Participants: participants(msgs),
	}
	if len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		s.LastFrom = last.From
		s.LastBody = preview(last.Body)
		at := last.CreatedAt
		s.LastAt = &at
	}
	return s
}

// activityAt is the time a channel last saw anything happen, used to sort the
// list newest-first: the last message, or the open time on a silent channel.
func (s Summary) activityAt() time.Time {
	if s.LastAt != nil {
		return *s.LastAt
	}
	return s.CreatedAt
}
