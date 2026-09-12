// Package picker is a single-select fuzzy list picker for the terminal that
// draws each item as colored segments. The library it replaces here,
// go-fuzzyfinder, renders every item as plain runes and only understands ANSI
// color inside its preview pane, so a picker line cannot show a repo's name,
// catalog component, owner and path in different colors. This package reuses
// that library's matching subpackage, so filtering feels identical, and owns
// only the drawing.
package picker

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ktr0731/go-fuzzyfinder/matching"
)

// Color is a segment's foreground color.
type Color int

// The colors a segment can carry. Default leaves the terminal's own
// foreground alone, which keeps the picker readable on any theme.
const (
	Default Color = iota
	Cyan
	Magenta
	Blue
	Dim
)

// Segment is a run of text drawn in one color.
type Segment struct {
	Text  string
	Color Color
}

// Item is one pickable line, drawn as its segments in order.
type Item []Segment

// String returns the plain text the fuzzy matcher sees: the segment texts
// joined. Colors never take part in matching.
func (it Item) String() string {
	var b strings.Builder
	for _, seg := range it {
		b.WriteString(seg.Text)
	}
	return b.String()
}

// ErrAbort is returned when the user leaves the picker without choosing (Esc
// or Ctrl-C), or when there is nothing to pick. The returned index is -1.
var ErrAbort = errors.New("picker: aborted")

// Pick opens the picker on the terminal and returns the index into items of
// the chosen item, or ErrAbort if the user left without choosing.
func Pick(items []Item) (int, error) {
	if len(items) == 0 {
		// Decided before the terminal is touched, so nothing flashes the
		// alternate screen for a list with nothing on it.
		return -1, ErrAbort
	}
	s, err := tcell.NewScreen()
	if err != nil {
		return -1, fmt.Errorf("picker: new screen: %w", err)
	}
	if err := s.Init(); err != nil {
		return -1, fmt.Errorf("picker: init screen: %w", err)
	}
	defer s.Fini()
	return pick(s, items)
}

// pick runs the picker on an already initialized screen. Pick owns the real
// terminal; this seam lets tests drive a simulation screen instead.
func pick(s tcell.Screen, items []Item) (int, error) {
	if len(items) == 0 {
		return -1, ErrAbort
	}
	p := newPicker(s, items)
	p.draw()
	for {
		switch ev := s.PollEvent().(type) {
		case *tcell.EventResize:
			s.Sync()
			// The window may have lost rows; keep the cursor on screen so
			// Enter never picks a line the user cannot see.
			p.scroll()
			p.draw()
		case *tcell.EventKey:
			idx, done, err := p.handleKey(ev)
			if done {
				return idx, err
			}
			p.draw()
		case nil:
			// The screen was finalized underneath us; there is nothing left
			// to poll, so treat it as an abort rather than spinning.
			return -1, ErrAbort
		}
	}
}

// picker holds one picker session's state.
type picker struct {
	screen  tcell.Screen
	items   []Item
	plain   []string // items as the matcher sees them, cached per item
	query   []rune
	matched []matching.Matched
	cursor  int  // index into matched; 0 is the line nearest the prompt
	offset  int  // index into matched of the bottom-most drawn line
	noColor bool // honour NO_COLOR, read once so draws stay cheap
}

func newPicker(s tcell.Screen, items []Item) *picker {
	plain := make([]string, len(items))
	for i, it := range items {
		plain[i] = it.String()
	}
	p := &picker{
		screen:  s,
		items:   items,
		plain:   plain,
		noColor: os.Getenv("NO_COLOR") != "",
	}
	p.refilter()
	return p
}

// refilter recomputes the matched set for the current query and puts the
// cursor back on the best match.
func (p *picker) refilter() {
	if len(p.query) == 0 {
		// matching.FindAll indexes the query's first rune, so it panics on an
		// empty one; an empty query means "everything, original order" anyway.
		p.matched = make([]matching.Matched, len(p.items))
		for i := range p.items {
			p.matched[i] = matching.Matched{Idx: i, Pos: [2]int{-1, -1}}
		}
	} else {
		p.matched = matching.FindAll(string(p.query), p.plain, matching.WithMode(matching.ModeSmart))
	}
	p.cursor = 0
	p.offset = 0
}

// handleKey applies one key event. It reports the chosen original index and
// true once the picker should close.
func (p *picker) handleKey(ev *tcell.EventKey) (int, bool, error) {
	switch ev.Key() {
	case tcell.KeyEnter:
		if len(p.matched) == 0 {
			return -1, false, nil
		}
		return p.matched[p.cursor].Idx, true, nil
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return -1, true, ErrAbort
	case tcell.KeyUp, tcell.KeyCtrlP, tcell.KeyCtrlK:
		p.moveCursor(1)
	case tcell.KeyDown, tcell.KeyCtrlN, tcell.KeyCtrlJ:
		p.moveCursor(-1)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(p.query) > 0 {
			p.setQuery(p.query[:len(p.query)-1])
		}
	case tcell.KeyCtrlU:
		p.setQuery(nil)
	case tcell.KeyCtrlW:
		p.setQuery(trimWord(p.query))
	case tcell.KeyRune:
		p.setQuery(append(p.query, ev.Rune()))
	}
	return -1, false, nil
}

func (p *picker) setQuery(q []rune) {
	p.query = q
	p.refilter()
}

// moveCursor walks the matched list, positive being away from the prompt.
func (p *picker) moveCursor(delta int) {
	next := p.cursor + delta
	if next < 0 || next >= len(p.matched) {
		return
	}
	p.cursor = next
	p.scroll()
}

// scroll slides the drawn window so the cursor stays on screen.
func (p *picker) scroll() {
	rows := p.itemRows()
	if rows <= 0 {
		p.offset = p.cursor
		return
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+rows {
		p.offset = p.cursor - rows + 1
	}
}

// itemRows is how many item lines fit above the count and prompt lines.
func (p *picker) itemRows() int {
	_, h := p.screen.Size()
	if h < 3 {
		return 0
	}
	return h - 2
}

// trimWord drops the trailing word of a query, trailing spaces first, the way
// Ctrl-W does in a shell.
func trimWord(q []rune) []rune {
	i := len(q)
	for i > 0 && q[i-1] == ' ' {
		i--
	}
	for i > 0 && q[i-1] != ' ' {
		i--
	}
	return q[:i]
}
