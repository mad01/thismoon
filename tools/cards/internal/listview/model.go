// Package listview is the Bubble Tea model for list mode. The screen is one
// header row, the cards in a grid of one to three columns, and one footer row
// with the filter, the counts and the latest message. Each card sits behind
// a two-column gutter: a dim focus bar runs down the cursor card and a green
// mark bar down each marked card, in every card style.
//
// The model does no I/O of its own. Input arrives as input.LineMsg and
// input.EndMsg, which input.Read produces from a reader, and an action key
// hands each target's action to the Starter and Emitter in Config. A bad
// input line or a rejected message never stops the model: it is counted and
// shown in the footer.
//
// Space marks, so the filter is one word: typed text goes into it as it
// comes, and a paste goes in with its spaces dropped. The one exception is a
// letter key: while a marked card the filter shows has an action for the
// letter typed, the letter runs that action on the marked cards instead.
// Enter acts on the cursor card when nothing is marked; a letter never does,
// so typing a filter word cannot fire an action by accident. While cards are
// marked, the footer names the keys they offer.
package listview

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/frame"
	"github.com/mad01/thismoon/tools/cards/internal/input"
	"github.com/mad01/thismoon/tools/cards/internal/liststore"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// Starter starts a run action's argv without waiting for it to finish.
type Starter interface {
	Start(argv []string) error
}

// Emitter reports an emit action on the card with id.
type Emitter interface {
	Emit(action, id string) error
}

// errNoEmitter is the footer error for an emit action with no Emitter.
var errNoEmitter = errors.New("emit needs --out FILE")

// Config is what New needs.
type Config struct {
	// Starter starts run actions.
	Starter Starter
	// Emitter writes emit actions. Nil means there is nowhere to emit to,
	// and an emit action fails with a footer error.
	Emitter Emitter
	// Style sets the cards apart. The zero value is render.CardNone.
	Style render.CardStyle
	// Newest is where a card with a new id goes. The zero value is
	// order.Bottom. With order.Top, the cursor stays on the newest card
	// until the first key or paste.
	Newest order.Newest
	// EscQuits makes Esc quit, as Ctrl+C does, when it has nothing to clear:
	// no marks, hidden ones included, and an empty filter. Each Esc before
	// that still clears the marks, then the filter.
	EscQuits bool
	// WrapLast gives a card's last line a second row when it is wider than
	// the card's text, broken at a space, as render.Cells.WrapLast does. A
	// card whose lines fit draws as without it.
	WrapLast bool
}

// Model is the list-mode state. Construct with New.
type Model struct {
	store   *liststore.Store
	starter Starter
	emitter Emitter
	style   render.CardStyle
	// escQuits is Config.EscQuits.
	escQuits bool
	// wrapLast is Config.WrapLast.
	wrapLast bool
	// cells counts cells the way the renderer does; see render.Cells.
	cells  render.Cells
	query  string
	width  int
	height int
	// top is the first grid row on screen.
	top    int
	status frame.Status
}

// New returns a Model with no cards, waiting for its first WindowSizeMsg.
func New(cfg Config) *Model {
	return &Model{
		store:    liststore.NewAt(cfg.Newest),
		starter:  cfg.Starter,
		emitter:  cfg.Emitter,
		style:    cfg.Style,
		escQuits: cfg.EscQuits,
		wrapLast: cfg.WrapLast,
	}
}

// Init starts nothing: input arrives from Read through the program.
func (m *Model) Init() tea.Cmd { return nil }

// Update applies one message and returns the model, with tea.Quit on a key
// that quits.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.ModeReportMsg:
		m.cells = m.cells.Report(msg.Mode, msg.Value)
	case tea.KeyPressMsg:
		if m.quits(msg) {
			return m, tea.Quit
		}
		m.store.Touch()
		m.key(msg)
	case tea.PasteMsg:
		m.store.Touch()
		m.paste(msg.Content)
	case input.LineMsg:
		m.input(msg)
	case input.EndMsg:
		m.status.End(msg.Err)
	}
	m.scroll()
	return m, nil
}

// input applies one decoded line to the store.
func (m *Model) input(msg input.LineMsg) {
	err := msg.Err
	if err == nil {
		err = m.store.Apply(msg.Message)
	}
	if err != nil {
		m.status.Fail(fmt.Errorf("line %d: %w", msg.Line, err))
	}
}

func (m *Model) key(k tea.KeyPressMsg) {
	switch k.Keystroke() {
	case "up":
		m.moveRows(-1)
	case "down":
		m.moveRows(1)
	case "left":
		m.store.MoveCursor(-1)
	case "right":
		m.store.MoveCursor(1)
	case "pgup":
		m.moveRows(-m.pageRows())
	case "pgdown":
		m.moveRows(m.pageRows())
	case "space":
		m.store.ToggleMark()
		m.store.MoveCursor(1)
	case "enter":
		m.enter()
	case "esc":
		m.escape()
	default:
		if m.letter(k) {
			return
		}
		if q, ok := frame.EditQuery(m.query, k); ok {
			m.setQuery(q)
		}
	}
}

func (m *Model) setQuery(q string) {
	m.query = q
	m.store.SetQuery(q)
}

// paste appends pasted text to the filter. Its control characters, line
// breaks among them, become spaces, and then every space is dropped, since
// the filter is one word.
func (m *Model) paste(text string) {
	m.setQuery(m.query + strings.Join(strings.Fields(protocol.Sanitize(text)), ""))
}

// quits reports whether key k quits: Ctrl+C always, and Esc with EscQuits
// set once there is nothing for escape to clear.
func (m *Model) quits(k tea.KeyPressMsg) bool {
	switch k.Keystroke() {
	case "ctrl+c":
		return true
	case "esc":
		return m.escQuits && !m.store.HasMarks() && m.query == ""
	default:
		return false
	}
}

// escape clears the marks when there are any, hidden ones included, else the
// filter.
func (m *Model) escape() {
	if m.store.HasMarks() {
		m.store.ClearMarks()
		return
	}
	m.setQuery("")
}

// moveRows moves the cursor n grid rows down, or up for a negative n,
// keeping its column. Past the first row it stops on the first row; past the
// last it stops on the last card, or stays put when already on the last row.
func (m *Model) moveRows(n int) {
	cur := m.store.Cursor()
	if cur < 0 || n == 0 {
		return
	}
	cols := columns(m.width)
	last := m.store.VisibleLen() - 1
	target := cur + n*cols
	switch {
	case target < 0:
		target = cur % cols
	case target > last && last/cols > cur/cols:
		target = last
	case target > last:
		target = cur
	}
	m.store.MoveCursor(target - cur)
}

// enter runs the enter action of every target card, then clears the marks
// and says in the footer what happened.
func (m *Model) enter() {
	targets := m.store.Targets()
	if len(targets) == 0 {
		return
	}
	var r result
	for _, c := range targets {
		r.add(m.act(c.Enter, c.ID))
	}
	m.store.ClearMarks()
	m.status.Note(r.summary("an action"), r.failed > 0)
}

// letter runs the action of the letter key k types on every marked card the
// filter shows, in screen order, when at least one of them has one. Then it
// clears the marks and says in the footer what happened, named by the key
// and the first such card's label. It reports whether it acted; when it did
// not, the key types into the filter as usual.
func (m *Model) letter(k tea.KeyPressMsg) bool {
	key, ok := frame.Typed(k)
	if !ok || !m.store.HasMarks() {
		return false
	}
	marked := m.store.Marked()
	first := slices.IndexFunc(marked, func(c protocol.Card) bool { return c.Letters[key] != nil })
	if first < 0 {
		return false
	}
	var r result
	for _, c := range marked {
		r.add(m.act(c.Letters[key], c.ID))
	}
	m.store.ClearMarks()
	name := key
	if label := m.label(marked[first].Letters[key]); label != "" {
		name += " " + label
	}
	m.status.Note(name+": "+r.summary(key), r.failed > 0)
	return true
}

// outcome is what one card's action did.
type outcome int

const (
	skipped outcome = iota
	ran
	emitted
)

// act runs action a of the card with id and reports what it did, or why it
// failed. A nil a is skipped: the card has no action for the key.
func (m *Model) act(a *protocol.Action, id string) (outcome, error) {
	switch {
	case a == nil:
		return skipped, nil
	case len(a.Run) > 0:
		return ran, m.starter.Start(a.Run)
	case m.emitter == nil:
		return emitted, errNoEmitter
	default:
		return emitted, m.emitter.Emit(a.Emit, id)
	}
}

// result tallies the outcomes of one action key.
type result struct {
	ran, emitted, skipped, failed int
	lastErr                       error
}

func (r *result) add(o outcome, err error) {
	switch {
	case err != nil:
		r.failed++
		r.lastErr = err
	case o == ran:
		r.ran++
	case o == emitted:
		r.emitted++
	default:
		r.skipped++
	}
}

// summary reads like "ran 2, emitted 1, 1 without an action", where missing
// names what a skipped card lacks.
func (r result) summary(missing string) string {
	var parts []string
	if r.ran > 0 {
		parts = append(parts, fmt.Sprintf("ran %d", r.ran))
	}
	if r.emitted > 0 {
		parts = append(parts, fmt.Sprintf("emitted %d", r.emitted))
	}
	if r.skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d without %s", r.skipped, missing))
	}
	if r.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed: %v", r.failed, r.lastErr))
	}
	return strings.Join(parts, ", ")
}
