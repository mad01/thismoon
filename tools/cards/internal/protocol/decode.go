package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Every error Decode returns wraps exactly one of these, so a caller can tell
// a malformed line from a well-formed one that breaks a protocol rule.
var (
	// ErrBadJSON marks a line that is not JSON or does not have the shape
	// the protocol defines (a number where a string belongs, say).
	ErrBadJSON = errors.New("protocol: bad JSON")
	// ErrUnknownType marks a message whose type is missing or not one of
	// header, card, remove, snapshot and clear.
	ErrUnknownType = errors.New("protocol: unknown message type")
	// ErrInvalid marks a well-formed message that breaks a protocol rule:
	// a line count outside 1 to 3, an unknown color, a malformed action, an
	// action key other than enter and a to z, a malformed scope.
	ErrInvalid = errors.New("protocol: invalid message")
)

const (
	// maxLines is the most lines a card may carry.
	maxLines = 3
	// maxScope is the most characters a scope name may have.
	maxScope = 32
)

// enterKey is the action key name for the enter key.
const enterKey = "enter"

// Decode parses one JSON line into a Message. Unknown fields are ignored, so
// a producer may send fields a later version understands. It never panics.
func Decode(line []byte) (Message, error) {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadJSON, err)
	}
	switch env.Type {
	case "header":
		return decodeAs(env.Type, line, &wireHeader{})
	case "card":
		return decodeAs(env.Type, line, &wireCard{})
	case "remove":
		return decodeAs(env.Type, line, &wireRemove{})
	case "snapshot":
		return decodeAs(env.Type, line, &wireSnapshot{})
	case "clear":
		return Clear{}, nil
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownType, env.Type)
}

// wireMessage is a message body as sent, before the protocol rules run.
type wireMessage interface {
	message() (Message, error)
}

// decodeAs unmarshals line into w and turns it into a Message, tagging a
// rule violation with the message type.
func decodeAs(typ string, line []byte, w wireMessage) (Message, error) {
	if err := json.Unmarshal(line, w); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrBadJSON, typ, err)
	}
	m, err := w.message()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, typ, err)
	}
	return m, nil
}

type wireHeader struct {
	// Text is a pointer so an omitted text differs from an empty one.
	Text *wireLine `json:"text"`
}

func (w *wireHeader) message() (Message, error) {
	if w.Text == nil {
		return nil, errors.New("text is required")
	}
	text, err := w.Text.line()
	if err != nil {
		return nil, fmt.Errorf("text: %w", err)
	}
	return Header{Text: text}, nil
}

type wireRemove struct {
	ID string `json:"id"`
}

func (w *wireRemove) message() (Message, error) {
	if w.ID == "" {
		return nil, errors.New("id is required")
	}
	return Remove{ID: w.ID}, nil
}

type wireSnapshot struct {
	Cards []wireCard `json:"cards"`
}

func (w *wireSnapshot) message() (Message, error) {
	cards := make([]Card, len(w.Cards))
	for i, wc := range w.Cards {
		c, err := wc.card()
		if err != nil {
			return nil, fmt.Errorf("card %d: %w", i+1, err)
		}
		cards[i] = c
	}
	return Snapshot{Cards: cards}, nil
}

type wireCard struct {
	ID      string                 `json:"id"`
	Lines   []wireLine             `json:"lines"`
	Tone    string                 `json:"tone"`
	Match   string                 `json:"match"`
	At      string                 `json:"at"`
	Scope   string                 `json:"scope"`
	Actions map[string]*wireAction `json:"actions"`
}

func (w *wireCard) message() (Message, error) { return w.card() }

func (w *wireCard) card() (Card, error) {
	if n := len(w.Lines); n < 1 || n > maxLines {
		return Card{}, fmt.Errorf("lines: got %d, want 1 to %d", n, maxLines)
	}
	lines := make([]Line, len(w.Lines))
	for i, wl := range w.Lines {
		l, err := wl.line()
		if err != nil {
			return Card{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		lines[i] = l
	}
	tone, err := parseColor(w.Tone)
	if err != nil {
		return Card{}, fmt.Errorf("tone: %w", err)
	}
	at, err := parseAt(w.At)
	if err != nil {
		return Card{}, fmt.Errorf("at: %w", err)
	}
	if err := checkScope(w.Scope); err != nil {
		return Card{}, fmt.Errorf("scope: %w", err)
	}
	enter, letters, err := actions(w.Actions)
	if err != nil {
		return Card{}, fmt.Errorf("actions: %w", err)
	}
	return Card{
		ID:      w.ID,
		Lines:   lines,
		Tone:    tone,
		Match:   Sanitize(w.Match),
		At:      at,
		Scope:   w.Scope,
		Enter:   enter,
		Letters: letters,
	}, nil
}

// actions validates a card's actions by key name: enter and the letters a
// to z. Keys are checked in sorted order, so a card with two bad keys always
// reports the same one. A key whose action is null has no action.
func actions(wire map[string]*wireAction) (enter *Action, letters map[string]*Action, err error) {
	for _, key := range slices.Sorted(maps.Keys(wire)) {
		if key != enterKey && !isLetter(key) {
			return nil, nil, fmt.Errorf("unknown key %q, want enter or a letter a to z", key)
		}
		a, err := wire[key].action()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", key, err)
		}
		switch {
		case a == nil:
		case key == enterKey:
			enter = a
		default:
			if letters == nil {
				letters = map[string]*Action{}
			}
			letters[key] = a
		}
	}
	return enter, letters, nil
}

// isLetter reports whether key names a letter key: one of "a" to "z".
func isLetter(key string) bool { return len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' }

type wireAction struct {
	// Pointers so an omitted field differs from an empty one.
	Run   *[]string `json:"run"`
	Emit  *string   `json:"emit"`
	Label string    `json:"label"`
}

// action validates w; a nil w is no action.
func (w *wireAction) action() (*Action, error) {
	if w == nil {
		return nil, nil
	}
	a := &Action{Label: Sanitize(w.Label)}
	switch {
	case w.Run != nil && w.Emit != nil:
		return nil, errors.New("set one of run and emit, not both")
	case w.Run != nil:
		argv := *w.Run
		if len(argv) == 0 || argv[0] == "" {
			return nil, errors.New("run: argv must start with a program name")
		}
		a.Run = argv
	case w.Emit != nil:
		if *w.Emit == "" {
			return nil, errors.New("emit: name must not be empty")
		}
		a.Emit = Sanitize(*w.Emit)
	default:
		return nil, errors.New("set one of run and emit")
	}
	return a, nil
}

// wireLine is a LINE as sent: a JSON string or an array of spans. A string
// becomes one span with no color.
type wireLine []wireSpan

type wireSpan struct {
	Text  string `json:"text"`
	Color string `json:"color"`
	Bold  bool   `json:"bold"`
}

// UnmarshalJSON accepts a JSON string or an array of spans. Anything else,
// null included, is a shape error and surfaces as ErrBadJSON.
func (l *wireLine) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*l = wireLine{{Text: s}}
		return nil
	}
	if len(b) > 0 && b[0] == '[' {
		var spans []wireSpan
		if err := json.Unmarshal(b, &spans); err != nil {
			return err
		}
		*l = spans
		return nil
	}
	return errors.New("a line must be a string or an array of spans")
}

func (l wireLine) line() (Line, error) {
	line := make(Line, len(l))
	for i, s := range l {
		c, err := parseColor(s.Color)
		if err != nil {
			return nil, fmt.Errorf("span %d: %w", i+1, err)
		}
		line[i] = Span{Text: Sanitize(s.Text), Color: c, Bold: s.Bold}
	}
	return line, nil
}

// parseColor maps an omitted or empty color to ColorDefault and rejects a
// name outside the protocol's list.
func parseColor(s string) (Color, error) {
	if s == "" {
		return ColorDefault, nil
	}
	switch c := Color(s); c {
	case ColorDefault, ColorRed, ColorGreen, ColorYellow, ColorBlue,
		ColorMagenta, ColorCyan, ColorWhite, ColorDim:
		return c, nil
	}
	return "", fmt.Errorf("unknown color %q", s)
}

// checkScope accepts an empty scope, the default set, or a name of 1 to
// maxScope characters that starts with a lowercase letter a to z and goes on
// with those, the digits 0 to 9 and hyphens. That keeps a scope one filter
// word with no colon in it, so the prefix before a colon can select it.
func checkScope(s string) error {
	for i, r := range s {
		if !scopeRune(r, i == 0) {
			return fmt.Errorf("%q: want a lowercase letter, then letters, digits and hyphens", s)
		}
	}
	if len(s) > maxScope {
		return fmt.Errorf("longer than %d characters", maxScope)
	}
	return nil
}

// scopeRune reports whether r may stand in a scope name: a lowercase letter
// anywhere, a digit or a hyphen anywhere but first.
func scopeRune(r rune, first bool) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case first:
		return false
	}
	return r >= '0' && r <= '9' || r == '-'
}

// parseAt parses an RFC3339 timestamp; an omitted one is the zero time.
func parseAt(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// Sanitize replaces each control character in s with a space: C0
// (U+0000 to U+001F, which holds ESC, CR, LF and tab), DEL (U+007F) and C1
// (U+0080 to U+009F, which holds the one-byte CSI). That is exactly the set
// unicode.IsControl reports, and it is what a terminal escape needs to start.
// Decode applies it to every string it keeps that a mode draws; a UI applies
// it to other text it draws that may carry producer input, such as an error
// message.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
