package store

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
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
	// Conventions is the opener's ground rules for the conversation — tag
	// vocabulary, expected message shapes, whatever the participants should
	// agree on. It lives on the channel rather than in the first message so a
	// session joining at a non-zero cursor still sees it.
	Conventions string `json:"conventions,omitempty"`
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
	ChannelID string `json:"channel_id"`
	Seq       int64  `json:"seq"`
	From      string `json:"from"`
	// To addresses the message to one agent by name. Empty means everyone.
	// With several sessions on a channel an unaddressed obligation belongs to
	// nobody in particular, so a question meant for a specific agent should
	// carry its name — that is what routes it into AwaitingReplyBy.
	To   string `json:"to,omitempty"`
	Body string `json:"body"`
	// Kind classifies the message's intent from a small closed set (see
	// checkKind); empty is a plain message. It is what lets a reader answer
	// "which of these are open questions" without parsing prose.
	Kind string `json:"kind,omitempty"`
	// ReplyTo is the seq of the message this one answers, 0 when it stands
	// alone. Seq is arrival order and ReplyTo is causal order; the two are
	// independent, which is what keeps an interleaved transcript followable.
	ReplyTo int64 `json:"reply_to,omitempty"`
	// ReplyNeeded marks that the sender expects an answer. A later message
	// naming this one in ReplyTo settles it; until then the channel reports
	// the seq in AwaitingReply.
	ReplyNeeded bool      `json:"reply_needed,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Summary is a channel plus the read-model the list view needs: how much has
// been said, the cursor to resume from, who has spoken, and a preview of the
// last line. All of it is derived from the messages, never stored.
type Summary struct {
	Channel
	Messages     int      `json:"messages"`
	Cursor       int64    `json:"cursor"`
	Participants []string `json:"participants"`
	// Members is the roster: the opener plus everyone who joined and has not
	// left. Distinct from Participants (who has spoken) — a member may be
	// silently reading, and a participant may never have joined.
	Members       []string `json:"members,omitempty"`
	AwaitingReply []int64  `json:"awaiting_reply,omitempty"`
	// AwaitingReplyBy groups the open obligations by the agent they are
	// addressed to. Unaddressed ones appear only in AwaitingReply — with
	// several agents on a channel they belong to whoever picks them up.
	AwaitingReplyBy map[string][]int64 `json:"awaiting_reply_by,omitempty"`
	// AwaitingReplyOffRoster lists the AwaitingReplyBy keys that are not in
	// Members — open obligations addressed to a name nobody currently answers
	// to. Advisory only: a pre-join handoff, a typo'd name, and an obligation
	// stranded by a leave all look identical here, and only the first resolves
	// itself.
	AwaitingReplyOffRoster []string   `json:"awaiting_reply_off_roster,omitempty"`
	LastFrom               string     `json:"last_from,omitempty"`
	LastBody               string     `json:"last_body,omitempty"`
	LastAt                 *time.Time `json:"last_at,omitempty"`
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

// kinds is the closed set of message intents. It is server-owned and small on
// purpose: these are the classifications cross-channel tooling can rely on,
// and channel-specific vocabulary belongs in the body or the channel's
// conventions, not in new kinds.
var kinds = map[string]bool{
	"task":     true,
	"result":   true,
	"question": true,
	"answer":   true,
	"ack":      true,
	"note":     true,
	"join":     true,
	"leave":    true,
}

// checkKind rejects a kind outside the closed set. Empty is valid: a plain
// message needs no classification.
func checkKind(kind string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" || kinds[k] {
		return k, nil
	}
	return "", fmt.Errorf(
		"invalid kind %q: use task, result, question, answer, ack, note, join, or leave", kind,
	)
}

// checkTo bounds an addressee name. Empty is valid — an unaddressed message is
// for everyone. The name is not checked against the roster on purpose: the
// normal handoff posts a task addressed to an agent before that agent has
// joined.
func checkTo(to string) (string, error) {
	t := strings.TrimSpace(to)
	if utf8.RuneCountInString(t) > 64 {
		return "", errors.New("to must be at most 64 characters")
	}
	return t, nil
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

// members derives the roster from the transcript: the opener, plus everyone
// whose latest join/leave message is a join. Like participants it is never
// stored, so there is no membership record to keep in sync — a join or leave
// is just a message, and the roster is what the messages say it is.
func members(c Channel, msgs []Message) []string {
	present := map[string]bool{}
	seen := map[string]bool{}
	var order []string
	add := func(who string) {
		if who == "" {
			return
		}
		if !seen[who] {
			seen[who] = true
			order = append(order, who)
		}
		present[who] = true
	}
	add(c.OpenedBy)
	for _, m := range msgs {
		switch m.Kind {
		case "join":
			add(m.From)
		case "leave":
			present[m.From] = false
		}
	}
	var out []string
	for _, who := range order {
		if present[who] {
			out = append(out, who)
		}
	}
	return out
}

// answeredSeqs collects every seq some later message has named in ReplyTo —
// the settled side of the obligation ledger.
func answeredSeqs(msgs []Message) map[int64]bool {
	answered := map[int64]bool{}
	for _, m := range msgs {
		if m.ReplyTo > 0 {
			answered[m.ReplyTo] = true
		}
	}
	return answered
}

// awaitingReply lists the messages still owed an answer: every seq posted
// with ReplyNeeded that no later message has named in ReplyTo. Derived from
// the transcript on every read, never stored, so it cannot drift from it.
func awaitingReply(msgs []Message) []int64 {
	answered := answeredSeqs(msgs)
	var out []int64
	for _, m := range msgs {
		if m.ReplyNeeded && !answered[m.Seq] {
			out = append(out, m.Seq)
		}
	}
	return out
}

// awaitingReplyBy groups the open obligations by addressee, so each agent on a
// busy channel can answer "which of these are mine" without scanning the
// transcript. Unaddressed obligations are left out — they belong to whoever
// picks them up, and are still listed in awaitingReply.
func awaitingReplyBy(msgs []Message) map[string][]int64 {
	answered := answeredSeqs(msgs)
	out := map[string][]int64{}
	for _, m := range msgs {
		if m.ReplyNeeded && m.To != "" && !answered[m.Seq] {
			out[m.To] = append(out[m.To], m.Seq)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// awaitingReplyOffRoster lists the addressees in the obligation ledger that
// the roster does not contain. It exists because a misaddressed question is
// otherwise silent: it files under a key matching no member, the addressee's
// own awaitingReplyBy entry stays empty, and an agent settling only what is
// under its own name drops a question plainly meant for it. The signal is
// advisory and does not classify — a handoff addressed before the agent joins,
// a typo, and an obligation stranded by a leave have the same shape here, the
// server cannot tell them apart, and judging which is which belongs to a
// reader. Sorted so the list is stable across reads.
func awaitingReplyOffRoster(byName map[string][]int64, roster []string) []string {
	var out []string
	for who := range byName {
		if !slices.Contains(roster, who) {
			out = append(out, who)
		}
	}
	slices.Sort(out)
	return out
}

// summarize folds a channel and its messages into the list read-model.
func summarize(c Channel, msgs []Message) Summary {
	mem := members(c, msgs)
	by := awaitingReplyBy(msgs)
	s := Summary{
		Channel:                c,
		Messages:               len(msgs),
		Cursor:                 int64(len(msgs)),
		Participants:           participants(msgs),
		Members:                mem,
		AwaitingReply:          awaitingReply(msgs),
		AwaitingReplyBy:        by,
		AwaitingReplyOffRoster: awaitingReplyOffRoster(by, mem),
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
