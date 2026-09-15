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
	"slices"
	"sort"
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
	// The terminal answers on the same tty tcell is about to take over, so
	// the band is settled before the screen exists.
	band := terminalBand()
	s, err := tcell.NewScreen()
	if err != nil {
		return -1, fmt.Errorf("picker: new screen: %w", err)
	}
	if err := s.Init(); err != nil {
		return -1, fmt.Errorf("picker: init screen: %w", err)
	}
	defer s.Fini()
	return pick(s, items, band)
}

// pick runs the picker on an already initialized screen. Pick owns the real
// terminal; this seam lets tests drive a simulation screen instead.
func pick(s tcell.Screen, items []Item, band tcell.Color) (int, error) {
	if len(items) == 0 {
		return -1, ErrAbort
	}
	p := newPicker(s, items, band)
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
	pos     int      // edit cursor in query, 0..len(query)
	terms   []string // query split into fzf-style terms, cached per refilter
	matched []match
	cursor  int         // index into matched; 0 is the line nearest the prompt
	offset  int         // index into matched of the bottom-most drawn line
	noColor bool        // honour NO_COLOR, read once so draws stay cheap
	band    tcell.Color // the selected row's background, see terminalBand
}

// match is one item every query term matched, with where each term hit.
type match struct {
	idx  int
	pos  [][2]int // one matched range per term, in term order
	rank int      // sum of the item's positions in the per-term rankings
}

func newPicker(s tcell.Screen, items []Item, band tcell.Color) *picker {
	plain := make([]string, len(items))
	for i, it := range items {
		plain[i] = it.String()
	}
	p := &picker{
		screen:  s,
		items:   items,
		plain:   plain,
		noColor: os.Getenv("NO_COLOR") != "",
		band:    band,
	}
	p.refilter()
	return p
}

// refilter recomputes the matched set for the current query and puts the
// cursor back on the best match.
func (p *picker) refilter() {
	p.terms = strings.Fields(string(p.query))
	p.matched = findAll(p.terms, p.plain)
	p.cursor = 0
	p.offset = 0
}

// findAll returns the items every term fuzzy-matches, best first. Each term
// is matched on its own, so "system:foo bar" finds lines holding both in any
// order, the way fzf's extended search does. The matcher keeps its score
// private, so ranking sums each item's position in the per-term result lists;
// with a single term that is exactly the matcher's own order.
func findAll(terms []string, plain []string) []match {
	if len(terms) == 0 {
		// matching.FindAll indexes the query's first rune, so it panics on an
		// empty one; an empty query means "everything, original order" anyway.
		all := make([]match, len(plain))
		for i := range plain {
			all[i] = match{idx: i}
		}
		return all
	}
	var found map[int]*match
	for i, term := range terms {
		hits := matching.FindAll(term, plain, matching.WithMode(matching.ModeSmart))
		found = keep(found, hits, i == 0)
	}
	res := make([]match, 0, len(found))
	for _, m := range found {
		res = append(res, *m)
	}
	sort.Slice(res, func(a, b int) bool {
		if res[a].rank == res[b].rank {
			return res[a].idx < res[b].idx
		}
		return res[a].rank < res[b].rank
	})
	return res
}

// keep narrows found to the items hits also contains, recording where the
// term matched and how far down the term's ranking the item sat. The first
// term seeds the set instead of narrowing it.
func keep(found map[int]*match, hits []matching.Matched, first bool) map[int]*match {
	next := make(map[int]*match, len(hits))
	for r, h := range hits {
		m, ok := found[h.Idx]
		if first {
			m, ok = &match{idx: h.Idx}, true
		}
		if !ok {
			continue
		}
		m.pos = append(m.pos, h.Pos)
		m.rank += r
		next[h.Idx] = m
	}
	return next
}

// handleKey applies one key event. It reports the chosen original index and
// true once the picker should close.
func (p *picker) handleKey(ev *tcell.EventKey) (int, bool, error) {
	switch ev.Key() {
	case tcell.KeyEnter:
		if len(p.matched) == 0 {
			return -1, false, nil
		}
		return p.matched[p.cursor].idx, true, nil
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return -1, true, ErrAbort
	case tcell.KeyUp, tcell.KeyCtrlP, tcell.KeyCtrlK:
		p.moveCursor(1)
	case tcell.KeyDown, tcell.KeyCtrlN, tcell.KeyCtrlJ:
		p.moveCursor(-1)
	default:
		p.editQuery(ev)
	}
	return -1, false, nil
}

// editQuery applies the readline-style editing keys a shell prompt has:
// cursor motion by rune and word, deletion on either side of the cursor,
// and insertion at the cursor. Ctrl-K stays "move up", as in fzf, so there
// is no kill-to-end.
func (p *picker) editQuery(ev *tcell.EventKey) {
	alt := ev.Modifiers()&tcell.ModAlt != 0
	switch ev.Key() {
	case tcell.KeyLeft, tcell.KeyCtrlB:
		p.movePos(-1, alt)
	case tcell.KeyRight, tcell.KeyCtrlF:
		p.movePos(1, alt)
	case tcell.KeyHome, tcell.KeyCtrlA:
		p.pos = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		p.pos = len(p.query)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		p.cut(p.pos-1, p.pos)
	case tcell.KeyDelete, tcell.KeyCtrlD:
		p.cut(p.pos, p.pos+1)
	case tcell.KeyCtrlU:
		p.cut(0, p.pos)
	case tcell.KeyCtrlW:
		p.cut(wordStart(p.query, p.pos), p.pos)
	case tcell.KeyRune:
		p.typeRune(ev.Rune(), alt)
	}
}

// typeRune inserts r at the cursor. With Alt held, b and f are the readline
// word motions instead, which is how terminals deliver Alt-B and Alt-F.
func (p *picker) typeRune(r rune, alt bool) {
	if alt {
		switch r {
		case 'b':
			p.movePos(-1, true)
		case 'f':
			p.movePos(1, true)
		}
		return
	}
	q := slices.Insert(slices.Clone(p.query), p.pos, r)
	p.pos++
	p.setQuery(q)
}

// movePos moves the edit cursor one rune, or one word when byWord is set,
// in the direction's sign. It never leaves the query.
func (p *picker) movePos(dir int, byWord bool) {
	switch {
	case byWord && dir < 0:
		p.pos = wordStart(p.query, p.pos)
	case byWord:
		p.pos = wordEnd(p.query, p.pos)
	default:
		p.pos = min(max(p.pos+dir, 0), len(p.query))
	}
}

// cut removes query[from:to], clamped to the query, and keeps the edit
// cursor on the same rune it sat on. A cursor inside the cut range lands at
// its start.
func (p *picker) cut(from, to int) {
	from, to = max(from, 0), min(to, len(p.query))
	if from >= to {
		return
	}
	switch {
	case p.pos >= to:
		p.pos -= to - from
	case p.pos > from:
		p.pos = from
	}
	p.setQuery(slices.Delete(slices.Clone(p.query), from, to))
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

// wordStart is where the word before pos begins: spaces are skipped first,
// then the word, the way Ctrl-W and Alt-B walk back in a shell.
func wordStart(q []rune, pos int) int {
	i := min(pos, len(q))
	for i > 0 && q[i-1] == ' ' {
		i--
	}
	for i > 0 && q[i-1] != ' ' {
		i--
	}
	return i
}

// wordEnd is where the word after pos ends: spaces are skipped first, then
// the word, the way Alt-F walks forward in a shell.
func wordEnd(q []rune, pos int) int {
	i := min(max(pos, 0), len(q))
	for i < len(q) && q[i] == ' ' {
		i++
	}
	for i < len(q) && q[i] != ' ' {
		i++
	}
	return i
}
