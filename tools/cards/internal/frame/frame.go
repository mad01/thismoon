// Package frame draws the rows every cards mode puts around its body: the
// header row above it and the footer row below it. The footer starts with
// the filter prompt and the counts, which Counts draws after the scopes the
// filter selects. Typed says what text a key types, EditQuery applies a key
// to the filter, and the footer ends in the input status that Status keeps:
// whether the input ended, how many lines failed, and the latest message.
package frame

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

const (
	// Indent starts the header and footer rows, so they line up with a
	// body that starts its rows with the same indent.
	Indent = "  "
	// gap separates two parts of the footer.
	gap = "  "
)

// Header draws the header row: h after Indent, cut to width cells as c
// counts them, and bold when it has no styling of its own.
func Header(c render.Cells, h protocol.Line, width int) string {
	if render.Plain(h) {
		h = render.Bold(h)
	}
	return Indent + c.Line(h, protocol.ColorDefault, width-len(Indent))
}

// Footer draws the footer row: the filter prompt with query, then the
// mode's own parts, then the status. The caller cuts it to the pane width.
func Footer(query string, s Status, parts ...string) string {
	all := []string{render.Paint(">", protocol.ColorCyan, true) + " " + query}
	all = append(all, parts...)
	all = append(all, s.parts()...)
	return Indent + strings.Join(all, gap)
}

// Counts draws the footer part with the shown and total counts, as in
// "2/5". While the filter selects scopes, their names, in the prompt's
// color, come first, so the user sees which set the counts are of.
func Counts(scopes []string, shown, total int) string {
	counts := fmt.Sprintf("%d/%d", shown, total)
	if len(scopes) == 0 {
		return counts
	}
	return render.Paint(strings.Join(scopes, ", "), protocol.ColorCyan, true) + gap + counts
}

// Empty draws the body row shown while no card is: total is how many cards
// the mode holds, the filter hiding them all when it is above zero.
func Empty(total int, ended bool) string {
	text := "waiting for cards"
	switch {
	case total > 0:
		text = "no cards match the filter"
	case ended:
		text = "no cards"
	}
	return Indent + render.Paint(text, protocol.ColorDim, false)
}

// EditQuery applies key k to the filter query q. Backspace deletes the last
// rune, and a key that types text, as Typed says, appends it. The footer
// draws the query, so a control character in the key's text (a C1 code
// point can arrive as one) is appended as a space. ok is false when k does
// not edit the query.
func EditQuery(q string, k tea.KeyPressMsg) (edited string, ok bool) {
	if k.Keystroke() == "backspace" {
		_, size := utf8.DecodeLastRuneInString(q)
		return q[:len(q)-size], true
	}
	if text, ok := Typed(k); ok {
		return q + protocol.Sanitize(text), true
	}
	return q, false
}

// Typed returns the text key k types: its text, when it has some and
// neither ctrl nor alt is held. Shift and Caps Lock change the text, so a
// capital letter types as one. The text is as the terminal sent it, control
// characters included. ok is false when k types nothing.
func Typed(k tea.KeyPressMsg) (text string, ok bool) {
	if k.Text == "" || k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
		return "", false
	}
	return k.Text, true
}

// Status is the input status the footer reports. The zero Status is ready
// to use: input still coming, no errors, no message.
type Status struct {
	// note is the latest message; noteIsErr marks an error.
	note      string
	noteIsErr bool
	// errs counts the input lines and messages that failed.
	errs  int
	ended bool
}

// Fail counts err and shows it as the latest message.
func (s *Status) Fail(err error) {
	s.errs++
	s.Note(err.Error(), true)
}

// Note sets the latest message, drawn red when isErr is set. An error can
// quote producer input, so the text is sanitized before it is drawn.
func (s *Status) Note(text string, isErr bool) {
	s.note, s.noteIsErr = protocol.Sanitize(text), isErr
}

// End records that the input is done. A non-nil err, a failed read, also
// counts as a failure.
func (s *Status) End(err error) {
	s.ended = true
	if err != nil {
		s.Fail(fmt.Errorf("input: %w", err))
	}
}

// Ended reports whether the input is done.
func (s Status) Ended() bool { return s.ended }

// parts returns the footer parts for the status: the input state, the
// error count and the latest message, each only when there is one.
func (s Status) parts() []string {
	var parts []string
	if s.ended {
		parts = append(parts, render.Paint("input ended", protocol.ColorDim, false))
	}
	if s.errs > 0 {
		parts = append(parts, render.Paint(count(s.errs, "error"), protocol.ColorRed, false))
	}
	if s.note != "" {
		c := protocol.ColorDefault
		if s.noteIsErr {
			c = protocol.ColorRed
		}
		parts = append(parts, render.Paint(s.note, c, false))
	}
	return parts
}

// count reads like "1 error" or "3 errors".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
