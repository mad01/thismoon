package feedview

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/feedstore"
	"github.com/mad01/thismoon/tools/cards/internal/frame"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

const (
	// clockLayout draws an entry's time, in the time.Format layout.
	clockLayout = "15:04"
	clockGap    = "  "
	// clockWidth is the time column: the time and the gap after it.
	clockWidth = len(clockLayout) + len(clockGap)
	// textColumn is where an entry's text starts when the style takes no
	// cells beside it: after the frame indent and the time column.
	textColumn = len(frame.Indent) + clockWidth
)

// noClock fills the time column on an entry's later lines.
var noClock = strings.Repeat(" ", len(clockLayout))

// layout stacks the visible entries into screen rows, each entry the rows
// its style draws for its lines: the oldest on top, or the newest when
// newest is order.Top. The gap after an entry counts as its rows, so an
// entry's first row is the top of its card.
type layout struct {
	// entries holds the entries in screen order, top first.
	entries []feedstore.Entry
	style   render.CardStyle
	newest  order.Newest
	// starts holds the first row of each entry. Every entry has at least
	// one line, so it rises strictly.
	starts []int
	// rows is how many rows all the entries take.
	rows int
}

// newLayout stacks entries, oldest first as the store returns them, with
// the newest at the newest end.
func newLayout(entries []feedstore.Entry, style render.CardStyle, newest order.Newest) layout {
	if newest == order.Top {
		entries = slices.Clone(entries)
		slices.Reverse(entries)
	}
	starts := make([]int, len(entries))
	rows := 0
	for i, e := range entries {
		if i > 0 {
			rows += style.Gap()
		}
		starts[i] = rows
		rows += style.Rows(len(e.Card.Lines))
	}
	return layout{entries: entries, style: style, newest: newest, starts: starts, rows: rows}
}

// height returns the rows entry i takes, the gap after it included.
func (l layout) height(i int) int {
	if i+1 < len(l.entries) {
		return l.starts[i+1] - l.starts[i]
	}
	return l.rows - l.starts[i]
}

// topRange returns the lowest and the highest top row a screen of body
// rows takes. Neither shows blank rows past the oldest entry while others
// are off screen, and neither cuts the newest entry's text off for its rule
// or box edge. The newest end is where following keeps the screen.
func (l layout) topRange(body int) (low, high int) {
	if l.newest == order.Top {
		low = l.firstTop(body)
		return low, max(l.rows-body, low)
	}
	return 0, l.lastTop(body)
}

// followTop returns the top row that keeps the newest entry in view.
func (l layout) followTop(body int) int {
	low, high := l.topRange(body)
	if l.newest == order.Top {
		return low
	}
	return high
}

// lastTop returns the top row that puts the last row on the last of body
// rows, or 0 when every row fits. When the newest entry, at the bottom,
// does not fit in body rows, its last row of text goes there instead, so
// its text shows rather than only its rule or the bottom of its box.
func (l layout) lastTop(body int) int {
	end := l.rows
	if last := len(l.entries) - 1; last >= 0 && l.height(last) > body {
		end = l.starts[last] + l.style.TextEnd(len(l.entries[last].Card.Lines))
	}
	return max(end-body, 0)
}

// firstTop returns the top row that puts the first row on the first of body
// rows: 0, unless the newest entry, on top, does not fit in body rows. Then
// its first row of text goes there instead, so its text shows rather than
// only the top of its box.
func (l layout) firstTop(body int) int {
	if len(l.entries) > 0 && l.style.Rows(len(l.entries[0].Card.Lines)) > body {
		return l.style.TextStart()
	}
	return 0
}

// entryAt returns the index of the entry that holds row, which must be
// below rows; a row past the end gives the last entry.
func (l layout) entryAt(row int) int {
	i, found := slices.BinarySearch(l.starts, row)
	if found {
		return i
	}
	return i - 1
}

// firstFrom returns the index of the first entry on screen that is the one
// numbered seq or below it: numbered seq or later while the newest entry is
// at the bottom, seq or earlier while it is on top. It returns len(entries)
// when there is none.
func (l layout) firstFrom(seq uint64) int {
	i, _ := slices.BinarySearchFunc(l.entries, seq, func(e feedstore.Entry, seq uint64) int {
		if l.newest == order.Top {
			return cmp.Compare(seq, e.Seq)
		}
		return cmp.Compare(e.Seq, seq)
	})
	return i
}

// prevStart returns the first row of the entry above row: the one that
// holds row-1, so a top inside an entry goes to that entry's first row.
func (l layout) prevStart(row int) int {
	if row <= 0 {
		return 0
	}
	return l.starts[l.entryAt(row-1)]
}

// nextStart returns the first row of the entry after the one that holds
// row, or row when that entry is the last.
func (l layout) nextStart(row int) int {
	if l.rows == 0 {
		return 0
	}
	if i := l.entryAt(row) + 1; i < len(l.entries) {
		return l.starts[i]
	}
	return row
}

// bodyHeight is the screen rows between the header and the footer.
func (m *Model) bodyHeight() int { return max(m.height-2, 0) }

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
	l := newLayout(m.store.Visible(), m.style, m.newest)
	rows := make([]string, 0, m.height)
	rows = append(rows, frame.Header(m.cells, m.store.Header(), m.width))
	rows = append(rows, m.body(l)...)
	rows = append(rows, m.footerRow(len(l.entries)))
	rows = rows[:min(len(rows), m.height)]
	for i, r := range rows {
		rows[i] = m.cells.Cut(r, m.width)
	}
	return strings.Join(rows, "\n")
}

// body draws exactly bodyHeight rows: the entry rows from the top row on,
// then blank rows when the entries do not fill the screen.
func (m *Model) body(l layout) []string {
	height := m.bodyHeight()
	rows := make([]string, 0, height)
	if len(l.entries) == 0 {
		rows = append(rows, frame.Empty(m.store.ScopeLen(), m.status.Ended()))
	} else {
		top := m.top(l)
		first := l.entryAt(top)
		for i := first; i < len(l.entries) && len(rows) < height; i++ {
			entry := m.entryRows(l.entries[i])
			for len(entry) < l.height(i) {
				entry = append(entry, "")
			}
			if i == first {
				entry = entry[top-l.starts[i]:]
			}
			rows = append(rows, entry...)
		}
	}
	rows = rows[:min(len(rows), height)]
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// entryRows draws e in the style, after the frame indent: the dim time and
// the first line, then each later line under the first line's text. A box
// goes around the time column too; a rule runs under the text only. The
// card's tone and span colors apply as in list mode.
func (m *Model) entryRows(e feedstore.Entry) []string {
	bounds := render.CardBounds{
		Width: m.width - len(frame.Indent),
		Lead:  clockWidth,
		Lines: len(e.Card.Lines),
	}
	textWidth := m.style.TextWidth(bounds.Width) - clockWidth
	text := make([]string, len(e.Card.Lines))
	for i, l := range e.Card.Lines {
		clock := noClock
		if i == 0 {
			clock = render.Paint(e.Time().In(m.loc).Format(clockLayout), protocol.ColorDim, false)
		}
		text[i] = clock + clockGap + m.cells.Line(l, e.Card.Tone, textWidth)
	}
	rows, _ := m.style.Draw(m.cells, text, bounds)
	for i, r := range rows {
		rows[i] = frame.Indent + r
	}
	return rows
}

// footerRow draws the footer with the scopes the filter selects, the shown
// count and the count of the set it shows, and the follow state, which
// names the key that follows again.
func (m *Model) footerRow(shown int) string {
	state := "following"
	if !m.following {
		_, name := m.followKey()
		state = render.Paint("scrolled, "+name+" follows", protocol.ColorYellow, false)
	}
	counts := frame.Counts(m.store.Scopes(), shown, m.store.ScopeLen())
	return frame.Footer(m.query, m.status, counts, state)
}
