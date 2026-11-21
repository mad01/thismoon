// Package protocol decodes the cards input protocol: JSON Lines on stdin, one
// message per line, told apart by the type field. Decode turns one line into
// a typed Message and enforces the protocol rules, so the rest of the tool
// only sees valid values.
//
// Card text comes from untrusted sources (pull request titles, commit
// messages). Decode replaces every control character in the strings it keeps
// with a space, so no producer can inject a terminal escape sequence. Two
// fields are never drawn and pass through verbatim: an id, which is a key
// that an emit action writes back as sent, and a run action's argv, which is
// executed.
//
// A card's actions are keyed by the key that fires them: enter, or one of
// the letter keys a to z. A key name outside that set is a rule violation,
// unlike an unknown field, so a mistyped key does not leave a card silently
// without its action.
package protocol

import (
	"strings"
	"time"
)

// Message is one decoded protocol line. Its concrete type is Header, Card,
// Remove, Snapshot or Clear.
type Message interface{ isMessage() }

// Header sets the line drawn above the cards.
type Header struct {
	Text Line
}

// Card is one card: up to three lines of text plus optional styling, match
// text, timestamp and actions. A card with the id of an existing card
// replaces it.
type Card struct {
	// ID keys the card for upserts, removes, the cursor and marks. It is
	// kept as sent, control characters included, so two ids stay apart and
	// an emit action reports the id the producer sent; it is never drawn.
	// Decode accepts an empty ID; list mode rejects it.
	ID string
	// Lines holds 1 to 3 lines.
	Lines []Line
	// Tone colors the whole card. Never empty: an omitted tone is
	// ColorDefault.
	Tone Color
	// Match is the text the filter runs against, as sent. Empty means the
	// filter uses the line texts; MatchText resolves that.
	Match string
	// At is when the card's event happened; zero when omitted.
	At time.Time
	// Scope names the set the card belongs to, such as "ignored": a short
	// lowercase name the filter selects by prefix. Empty means the default
	// set, the one the empty filter shows.
	Scope string
	// Enter is the action for the enter key; nil when none.
	Enter *Action
	// Letters maps a letter key, "a" to "z", to its action; nil when the
	// card has none. It never holds a nil action.
	Letters map[string]*Action
}

// Remove drops the card with ID.
type Remove struct {
	ID string
}

// Snapshot replaces every card at once, in the snapshot's order.
type Snapshot struct {
	Cards []Card
}

// Clear drops every card.
type Clear struct{}

func (Header) isMessage()   {}
func (Card) isMessage()     {}
func (Remove) isMessage()   {}
func (Snapshot) isMessage() {}
func (Clear) isMessage()    {}

// Line is one line of text as styled spans. A plain-string line decodes to a
// single span in ColorDefault.
type Line []Span

// Span is a run of text with one style.
type Span struct {
	Text string
	// Color is never empty: an omitted color is ColorDefault.
	Color Color
	Bold  bool
}

// Color names one of the 16 terminal colors, so the user's terminal theme
// decides the actual shade. ColorDim is the faint attribute, not a hue.
type Color string

// The colors the protocol accepts.
const (
	ColorDefault Color = "default"
	ColorRed     Color = "red"
	ColorGreen   Color = "green"
	ColorYellow  Color = "yellow"
	ColorBlue    Color = "blue"
	ColorMagenta Color = "magenta"
	ColorCyan    Color = "cyan"
	ColorWhite   Color = "white"
	ColorDim     Color = "dim"
)

// Action is what a key does on a card. Exactly one of Run and Emit is set:
// Run, a non-empty argv to execute, or Emit, an event name to report.
type Action struct {
	Run  []string
	Emit string
	// Label names the action where list mode draws its key; empty when the
	// producer sent none.
	Label string
}

// Text returns the line's span texts joined without a separator.
func (l Line) Text() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

// MatchText returns the text the filter runs against: Match when set, else
// every line's text joined with spaces.
func (c Card) MatchText() string {
	if c.Match != "" {
		return c.Match
	}
	texts := make([]string, len(c.Lines))
	for i, l := range c.Lines {
		texts[i] = l.Text()
	}
	return strings.Join(texts, " ")
}
