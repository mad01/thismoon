package listview

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/frame"
	"github.com/mad01/thismoon/tools/cards/internal/liststore"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// The grid: a pane gets one column per columnWidth cells, from one up to
// maxColumns.
const (
	columnWidth = 60
	maxColumns  = 3
	// columnGap is the blank space between two grid columns.
	columnGap = 2
	// gutterWidth is the left edge of every card: the focus bar, then the
	// mark bar. It is as wide as frame.Indent, so the header and footer line
	// up with the card text.
	gutterWidth = 2
)

const (
	// bar draws the focus bar and the mark bar, each one gutter column.
	bar = "▌"
	gap = "  "
	// noGutter is the gutter of a row with neither bar.
	noGutter = "  "
)

// labelWidth is the most cells an action label takes in the footer, so one
// long label cannot push the other keys off the row.
const labelWidth = 16

// columns returns how many grid columns a pane width cells wide gets.
func columns(width int) int { return min(max(width/columnWidth, 1), maxColumns) }

// grid lays the visible cards out in rows of cols cards, in the store's
// order from left to right and top to bottom, set apart in style. It reads each card from the store when a row needs
// it, so its cost follows the rows it looks at, not the cards in the store.
type grid struct {
	store *liststore.Store
	cols  int
	style render.CardStyle
}

func newGrid(store *liststore.Store, width int, style render.CardStyle) grid {
	return grid{store: store, cols: columns(width), style: style}
}

// rows returns how many grid rows the visible cards fill.
func (g grid) rows() int { return (g.store.VisibleLen() + g.cols - 1) / g.cols }

// bounds returns the visible indexes of grid row r's cards: first to end,
// end not included.
func (g grid) bounds(r int) (first, end int) {
	first = r * g.cols
	return first, min(first+g.cols, g.store.VisibleLen())
}

// height returns grid row r's height: its tallest card's line count.
func (g grid) height(r int) int {
	h := 0
	first, end := g.bounds(r)
	for i := first; i < end; i++ {
		h = max(h, len(g.store.VisibleCard(i).Lines))
	}
	return h
}

// fits reports whether grid rows first through last fit in body screen
// rows, counting the style's gap between two grid rows. It stops at the
// first row that does not fit, so it looks at one screen of rows at most.
func (g grid) fits(first, last, body int) bool {
	n := 0
	for r := first; r <= last; r++ {
		if r > first {
			n += g.style.Gap()
		}
		if n += g.style.Rows(g.height(r)); n > body {
			return false
		}
	}
	return true
}

// bodyHeight is the screen rows between the header and the footer.
func (m *Model) bodyHeight() int { return max(m.height-2, 0) }

// scroll moves top so the cursor's grid row is on screen. When the rows
// from top to the last one leave space below, it moves top back up to fill
// it, so a taller pane shows more cards.
func (m *Model) scroll() {
	cur := m.store.Cursor()
	if cur < 0 {
		m.top = 0
		return
	}
	g := newGrid(m.store, m.width, m.style)
	row, lastRow, body := cur/g.cols, g.rows()-1, m.bodyHeight()
	m.top = min(m.top, row)
	for m.top < row && !g.fits(m.top, row, body) {
		m.top++
	}
	for m.top > 0 && g.fits(m.top-1, lastRow, body) {
		m.top--
	}
}

// pageRows returns how many whole grid rows fit on screen from top, at
// least one.
func (m *Model) pageRows() int {
	g := newGrid(m.store, m.width, m.style)
	n := 1
	for m.top+n < g.rows() && g.fits(m.top, m.top+n, m.bodyHeight()) {
		n++
	}
	return n
}

// View draws the screen in the alternate screen buffer.
func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}

// screen draws the header, the body and the footer, every row cut to the
// pane width. It is empty until the first WindowSizeMsg.
func (m *Model) screen() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	rows := make([]string, 0, m.height)
	rows = append(rows, frame.Header(m.cells, m.store.Header(), m.width))
	rows = append(rows, m.body()...)
	rows = append(rows, m.footerRow())
	rows = rows[:min(len(rows), m.height)]
	for i, r := range rows {
		rows[i] = m.cells.Cut(r, m.width)
	}
	return strings.Join(rows, "\n")
}

// body draws exactly bodyHeight rows: the grid rows from top, the last one
// cut short when it does not fit, then blank rows.
func (m *Model) body() []string {
	height := m.bodyHeight()
	rows := make([]string, 0, height)
	if m.store.VisibleLen() == 0 {
		rows = append(rows, frame.Empty(m.store.ScopeLen(), m.status.Ended()))
	} else {
		g := newGrid(m.store, m.width, m.style)
		for r := m.top; r < g.rows() && len(rows) < height; r++ {
			if r > m.top {
				rows = append(rows, make([]string, g.style.Gap())...)
			}
			rows = append(rows, m.gridRow(g, r)...)
		}
	}
	rows = rows[:min(len(rows), height)]
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// gridRow draws grid row r: the style's rows for a card as tall as the
// row's tallest, each card padded to its column so the next column lines up.
func (m *Model) gridRow(g grid, r int) []string {
	cellWidth := (m.width - columnGap*(g.cols-1)) / g.cols
	first, end := g.bounds(r)
	lines := g.height(r)
	rows := make([]string, g.style.Rows(lines))
	for i := first; i < end; i++ {
		c := m.store.VisibleCard(i)
		for k, cell := range m.cell(c, i == m.store.Cursor(), lines, cellWidth) {
			if i > first {
				rows[k] += gap
			}
			if i < end-1 {
				cell = m.cells.Pad(cell, cellWidth)
			}
			rows[k] += cell
		}
	}
	return rows
}

// cell draws card c in the style, with room for lines rows of text, gutter
// included, in width cells. The gutter bars run down every row of the card
// itself, its box included: the focus bar for the cursor card, the mark bar
// for a marked card. The cursor card's first line is bold.
func (m *Model) cell(c protocol.Card, isCursor bool, lines, width int) []string {
	bounds := render.CardBounds{Width: width - gutterWidth, Lines: lines}
	textWidth := m.style.TextWidth(bounds.Width)
	text := make([]string, len(c.Lines))
	for i, l := range c.Lines {
		if isCursor && i == 0 {
			l = render.Bold(l)
		}
		text[i] = m.cells.Line(l, c.Tone, textWidth)
	}
	rows, card := m.style.Draw(m.cells, text, bounds)
	bars := m.gutter(c.ID, isCursor)
	for i, row := range rows {
		switch {
		case i < card:
			rows[i] = bars + row
		case row != "":
			rows[i] = noGutter + row
		}
	}
	return rows
}

// gutter draws the gutter of a card's rows: the dim focus bar when it is
// the cursor card, then the green mark bar when it is marked.
func (m *Model) gutter(id string, isCursor bool) string {
	focus, mark := " ", " "
	if isCursor {
		focus = render.Paint(bar, protocol.ColorDim, false)
	}
	if m.store.IsMarked(id) {
		mark = render.Paint(bar, protocol.ColorGreen, false)
	}
	return focus + mark
}

// footerRow draws the footer with the scopes the filter selects, the
// visible count and the count of the set it shows, the marked count, and
// the keys the marked cards offer.
func (m *Model) footerRow() string {
	marked := m.store.Marked()
	parts := []string{
		frame.Counts(m.store.Scopes(), m.store.VisibleLen(), m.store.ScopeLen()),
		fmt.Sprintf("%d marked", len(marked)),
	}
	return frame.Footer(m.query, m.status, append(parts, m.keyHints(marked)...)...)
}

// keyHints names each key the marked cards offer, enter first and then the
// letters in order, as the key and the label of the first card in marked
// that has an action for it, or run or emit when that action has no label.
// It is empty when marked is.
func (m *Model) keyHints(marked []protocol.Card) []string {
	var hints []string
	if i := slices.IndexFunc(marked, func(c protocol.Card) bool { return c.Enter != nil }); i >= 0 {
		hints = append(hints, m.keyHint("enter", marked[i].Enter))
	}
	letters := map[string]*protocol.Action{}
	for _, c := range marked {
		for key, a := range c.Letters {
			if _, ok := letters[key]; !ok {
				letters[key] = a
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(letters)) {
		hints = append(hints, m.keyHint(key, letters[key]))
	}
	return hints
}

// keyHint draws key in bold and then what a does: its label, or run or emit.
func (m *Model) keyHint(key string, a *protocol.Action) string {
	name := m.label(a)
	switch {
	case name != "":
	case len(a.Run) > 0:
		name = "run"
	default:
		name = "emit"
	}
	return render.Paint(key, protocol.ColorDefault, true) + " " + name
}

// label returns a's label cut to labelWidth cells, the way the renderer
// counts them; "" when a has none.
func (m *Model) label(a *protocol.Action) string { return m.cells.Cut(a.Label, labelWidth) }
