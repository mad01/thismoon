// Package feedview is the Bubble Tea model for feed mode: cards as a log of
// entries in arrival order, newest at the bottom, or on top when
// Config.Newest is order.Top. The screen is one header row, the entries,
// and one footer row with the filter, the counts, the follow state and the
// latest message.
//
// While following, the newest entry stays in view as entries arrive. A
// scroll key stops following and pins the screen to the entry at its top,
// so new entries and dropped old ones leave the view where it is. With no
// entry on screen there is nothing to pin, and a scroll key changes nothing.
// The key for the newest end resumes following: End with the newest at the
// bottom, Home with it on top.
//
// The model does no I/O of its own. Input arrives as input.LineMsg and
// input.EndMsg, which input.Read produces from a reader. A bad input line
// never stops the model: it is counted and shown in the footer. Feed mode
// has no marks and runs no actions.
package feedview

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/feedstore"
	"github.com/mad01/thismoon/tools/cards/internal/frame"
	"github.com/mad01/thismoon/tools/cards/internal/input"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// Config is what New needs.
type Config struct {
	// Max is how many entries the feed keeps; the oldest go first.
	Max int
	// Now returns the time an input line arrives, which an entry shows when
	// its card has no at. Nil means time.Now.
	Now func() time.Time
	// Location is the time zone entry times are drawn in. Nil means
	// time.Local.
	Location *time.Location
	// Style sets the entries apart. The zero value is render.CardNone.
	Style render.CardStyle
	// Newest is the end the newest entry shows at. The zero value is
	// order.Bottom.
	Newest order.Newest
}

// Model is the feed-mode state. Construct with New.
type Model struct {
	store  *feedstore.Store
	now    func() time.Time
	loc    *time.Location
	style  render.CardStyle
	newest order.Newest
	// cells counts cells the way the renderer does; see render.Cells.
	cells  render.Cells
	status frame.Status
	query  string
	width  int
	height int
	// following keeps the newest entry in view. While it is false, pin is
	// the top of the screen.
	following bool
	pin       position
}

// position is a screen top that stays put as entries come and go: row rows
// into the entry numbered seq. When that entry is gone or hidden, the top is
// the first visible entry below it on screen.
type position struct {
	seq uint64
	row int
}

// New returns a following Model with no entries, waiting for its first
// WindowSizeMsg.
func New(cfg Config) *Model {
	m := &Model{
		store:     feedstore.New(cfg.Max),
		now:       cfg.Now,
		loc:       cfg.Location,
		style:     cfg.Style,
		newest:    cfg.Newest,
		following: true,
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.loc == nil {
		m.loc = time.Local
	}
	return m
}

// Init starts nothing: input arrives from input.Read through the program.
func (m *Model) Init() tea.Cmd { return nil }

// Update applies one message and returns the model, with tea.Quit on Ctrl+C.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.ModeReportMsg:
		m.cells = m.cells.Report(msg.Mode, msg.Value)
	case tea.KeyPressMsg:
		if msg.Keystroke() == "ctrl+c" {
			return m, tea.Quit
		}
		m.key(msg)
	case tea.PasteMsg:
		// Control characters, line breaks among them, become spaces.
		m.setQuery(m.query + protocol.Sanitize(msg.Content))
	case input.LineMsg:
		m.input(msg)
	case input.EndMsg:
		m.status.End(msg.Err)
	}
	m.repin()
	return m, nil
}

// input applies one decoded line to the store, stamped with the time it
// arrived.
func (m *Model) input(msg input.LineMsg) {
	err := msg.Err
	if err == nil {
		err = m.store.Apply(msg.Message, m.now())
	}
	if err != nil {
		m.status.Fail(fmt.Errorf("line %d: %w", msg.Line, err))
	}
}

func (m *Model) key(k tea.KeyPressMsg) {
	follow, _ := m.followKey()
	switch key := k.Keystroke(); key {
	case follow:
		// First, because it is End or Home, which scroll otherwise.
		m.following = true
	case "up", "down", "pgup", "pgdown", "home", "end":
		m.scroll(key)
	case "esc":
		m.setQuery("")
	case "enter":
		// Feed mode has no actions.
	default:
		if q, ok := frame.EditQuery(m.query, k); ok {
			m.setQuery(q)
		}
	}
}

func (m *Model) setQuery(q string) {
	m.query = q
	m.store.SetQuery(q)
}

// followKey returns the key that resumes following, the one for the newest
// end, as Keystroke names it and as the footer does.
func (m *Model) followKey() (key, name string) {
	if m.newest == order.Top {
		return "home", "Home"
	}
	return "end", "End"
}

// scroll moves the screen for a scroll key and stops following: up and down
// by one entry, pgup and pgdown by one screen, home to the first row and end
// to the last. With no entry on screen it changes nothing.
func (m *Model) scroll(key string) {
	l := newLayout(m.store.Visible(), m.style, m.newest)
	top := m.top(l)
	switch key {
	case "up":
		top = l.prevStart(top)
	case "down":
		top = l.nextStart(top)
	case "pgup":
		top -= m.bodyHeight()
	case "pgdown":
		top += m.bodyHeight()
	case "home":
		top = 0
	case "end":
		top = l.rows
	}
	m.pinTo(l, top)
}

// top returns the first row of l on screen: while following, the row that
// keeps the newest entry in view, else the pinned row. It stays in the
// layout's top range, so no blank rows show past the oldest entry while
// others are off screen.
func (m *Model) top(l layout) int {
	body := m.bodyHeight()
	if m.following {
		return l.followTop(body)
	}
	low, high := l.topRange(body)
	i := l.firstFrom(m.pin.seq)
	if i == len(l.entries) {
		return high
	}
	row := l.starts[i]
	if l.entries[i].Seq == m.pin.seq {
		row += min(m.pin.row, l.height(i)-1)
	}
	return min(max(row, low), high)
}

// pinTo stops following and pins row of l, clamped to the layout's top
// range, to the top of the screen. With no entry in l there is nothing to
// pin, so it leaves following and the pin as they are: a pin there would
// show later entries from the oldest end.
func (m *Model) pinTo(l layout, row int) {
	if l.rows == 0 {
		return
	}
	m.following = false
	low, high := l.topRange(m.bodyHeight())
	row = max(min(row, high), low)
	i := l.entryAt(row)
	m.pin = position{seq: l.entries[i].Seq, row: row - l.starts[i]}
}

// repin moves a pin off an entry the store no longer holds when the screen
// falls back to the limit at the newest end: onto the entry at the top of
// the screen, at the row the screen shows. Falling back afresh after an
// entry arrives at that end would move the view.
//
// With the newest on top, that is when the entry the screen falls back to
// is the first on screen. Its row can rest on the lowest top, which a tall
// newest entry raises past its top border; a short entry arriving above
// would then show that border and move the view by a row. With the newest
// at the bottom, it is when that entry starts past the highest top, or
// there is none: an entry arriving below raises the highest top and would
// move the view down to it.
func (m *Model) repin() {
	if m.following || m.store.Holds(m.pin.seq) {
		return
	}
	l := newLayout(m.store.Visible(), m.style, m.newest)
	if len(l.entries) == 0 {
		return
	}
	i := l.firstFrom(m.pin.seq)
	if m.newest == order.Top && i == 0 || m.newest == order.Bottom && m.pastHighest(l, i) {
		m.pinTo(l, m.top(l))
	}
}

// pastHighest reports whether entry i of l, or the end of l when i is
// len(l.entries), starts past the highest top, so the screen top rests on
// that limit and not on the entry.
func (m *Model) pastHighest(l layout, i int) bool {
	if i == len(l.entries) {
		return true
	}
	_, high := l.topRange(m.bodyHeight())
	return l.starts[i] > high
}
