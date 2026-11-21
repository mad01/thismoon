// Package liststore holds the state of list mode: the header, the cards in
// producer order, the filter, the cursor and the marks. It does no I/O: the
// TUI feeds it decoded messages and key presses and reads back what to draw.
//
// A card with a new id goes at the store's newest end: below the others in a
// Store from New, above them in one from NewAt(order.Top). A snapshot keeps
// its own order at either end.
//
// The cursor and the marks follow card ids, so they survive every update.
// When the cursor's card goes away or the filter hides it, the cursor moves
// to the card now at the same visible index (clamped to the last one), and
// it is unset only while no card is visible. The one exception is a Store
// from NewAt(order.Top) before Touch: until the user presses a key, its
// cursor goes to the first visible card, the newest, after every update, so
// cards that stream in one at a time leave it on the newest.
//
// A mark on an id that no longer exists is dropped; a mark on a card the
// filter hides is kept but not reported, so an action only ever targets
// cards the user can see.
//
// A card with a scope shows only while the filter selects its scope, as
// package match says; the others make up the default set. The filter can
// select only the scopes the cards have, so when the first card of a scope
// comes or the last one goes, the store parses the filter again and
// matches every card. Every other card or remove matches only that card.
package liststore

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/mad01/thismoon/tools/cards/internal/match"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// ErrMissingID is returned by Apply for a card without an id, alone or in a
// snapshot: list mode keys every card by id.
var ErrMissingID = errors.New("liststore: card id is required")

// Store is the list-mode state. Construct with New.
type Store struct {
	header protocol.Line
	// newest is the end of Visible that a card with a new id goes to.
	newest order.Newest
	// cards holds every card so that a new one appends: in screen order
	// when newest is order.Bottom, bottom first when it is order.Top. pos
	// maps each id to its index there.
	cards []entry
	pos   map[string]int
	// raw is the filter as typed, and query is raw parsed against the
	// scopes the cards have.
	raw   string
	query match.Query
	// scopes counts the cards in each scope the cards have, the default set
	// left out.
	scopes map[string]int
	// visible holds the indexes into cards of the cards that match query, in
	// rising order; slot maps an index into Visible to one into visible.
	visible []int
	// cursor indexes Visible; -1 exactly when visible is empty.
	cursor int
	// untouched is true from NewAt(order.Top) until Touch. While it is, every
	// update puts the cursor on the first visible card, the newest.
	untouched bool
	// marks holds marked ids; every one is a key of pos.
	marks map[string]bool
}

// entry is one card with its match text prepared, so a new query or another
// card never redoes that work.
type entry struct {
	card protocol.Card
	text match.Text
}

// New returns an empty Store that puts a card with a new id below the
// others: NewAt(order.Bottom).
func New() *Store { return NewAt(order.Bottom) }

// NewAt returns an empty Store with no header, no filter and no cursor, that
// puts a card with a new id at the newest end of Visible. With order.Top,
// the cursor stays on the newest card until Touch.
func NewAt(newest order.Newest) *Store {
	return &Store{
		newest:    newest,
		pos:       map[string]int{},
		scopes:    map[string]int{},
		marks:     map[string]bool{},
		cursor:    -1,
		untouched: newest == order.Top,
	}
}

// Apply updates the store with one decoded message. A card upserts by id: an
// existing id keeps its position, a new one goes at the newest end. A
// snapshot replaces every card in its own order, at either end; Clear drops
// every card and keeps the header. A rejected message leaves the store
// unchanged.
//
// A card or a remove costs about the same however many cards the store
// holds: only that card is matched against the filter, unless it brings
// the first card of a scope or takes the last one away.
func (s *Store) Apply(m protocol.Message) error {
	switch m := m.(type) {
	case protocol.Header:
		s.header = m.Text
	case protocol.Card:
		if m.ID == "" {
			return ErrMissingID
		}
		s.keepCursor(func() { s.upsert(m) })
	case protocol.Remove:
		s.keepCursor(func() { s.remove(m.ID) })
	case protocol.Snapshot:
		if slices.ContainsFunc(m.Cards, func(c protocol.Card) bool { return c.ID == "" }) {
			return ErrMissingID
		}
		s.keepCursor(func() { s.replace(m.Cards) })
	case protocol.Clear:
		s.keepCursor(func() { s.replace(nil) })
	default:
		return fmt.Errorf("liststore: unsupported message %T", m)
	}
	return nil
}

// Header returns the header line; nil when none was sent.
func (s *Store) Header() protocol.Line { return s.header }

// SetQuery filters the cards by query, as package match parses it against
// the scopes the cards have. An empty query clears the filter, which shows
// the default set.
func (s *Store) SetQuery(query string) {
	s.raw = query
	s.keepCursor(s.reparse)
}

// Scopes returns the scopes the filter selects, sorted, or nil while it
// shows the default set.
func (s *Store) Scopes() []string { return s.query.Scopes() }

// Visible returns the cards that match the filter, in screen order: the
// producer's order, with each card that had a new id at the newest end.
func (s *Store) Visible() []protocol.Card {
	if len(s.visible) == 0 {
		return nil
	}
	out := make([]protocol.Card, len(s.visible))
	for k := range out {
		out[k] = s.VisibleCard(k)
	}
	return out
}

// VisibleLen returns how many cards the filter shows: len(Visible()),
// without building it.
func (s *Store) VisibleLen() int { return len(s.visible) }

// VisibleCard returns the card at index i of Visible, without building the
// rest. i must be in range.
func (s *Store) VisibleCard(i int) protocol.Card { return s.cards[s.visible[s.slot(i)]].card }

// Len returns how many cards the store holds, hidden ones included.
func (s *Store) Len() int { return len(s.cards) }

// ScopeLen returns how many cards the shown set holds: the cards in the
// scopes the filter selects, or the default set while it selects none,
// with the cards the rest of the filter hides. Its cost follows the
// scopes, not the cards.
func (s *Store) ScopeLen() int {
	if selected := s.query.Scopes(); selected != nil {
		n := 0
		for _, scope := range selected {
			n += s.scopes[scope]
		}
		return n
	}
	n := len(s.cards)
	for _, count := range s.scopes {
		n -= count
	}
	return n
}

// Cursor returns the cursor's index into Visible, or -1 when no card is
// visible.
func (s *Store) Cursor() int { return s.cursor }

// Touch records that the user pressed a key. From then on the cursor stays
// on its card as cards come in above it, so an action never lands on a card
// the user did not pick. Before it, a Store from NewAt(order.Top) puts the
// cursor on the newest card after every update. A Store from New has its
// cursor stay on its card from the start, so Touch changes nothing there.
func (s *Store) Touch() { s.untouched = false }

// MoveCursor moves the cursor delta places through the visible cards,
// stopping at the first and the last.
func (s *Store) MoveCursor(delta int) {
	if s.cursor < 0 {
		return
	}
	s.cursor = min(max(s.cursor+delta, 0), len(s.visible)-1)
}

// ToggleMark marks the cursor's card, or unmarks it when it is marked. It
// does nothing when no card is visible.
func (s *Store) ToggleMark() {
	if s.cursor < 0 {
		return
	}
	id := s.VisibleCard(s.cursor).ID
	if s.marks[id] {
		delete(s.marks, id)
		return
	}
	s.marks[id] = true
}

// IsMarked reports whether the card with id is marked.
func (s *Store) IsMarked(id string) bool { return s.marks[id] }

// HasMarks reports whether any card is marked, hidden ones included.
func (s *Store) HasMarks() bool { return len(s.marks) > 0 }

// MarkedLen returns how many marked cards the filter shows: len(Marked()),
// without building it.
func (s *Store) MarkedLen() int {
	n := 0
	for id := range s.marks {
		if _, ok := s.visibleIndex(id); ok {
			n++
		}
	}
	return n
}

// Marked returns the marked cards the filter shows, in visible order, or
// nil when there are none. Its cost follows the marks, not the cards, so a
// view can call it on every frame.
func (s *Store) Marked() []protocol.Card {
	var shown []int
	for id := range s.marks {
		if k, ok := s.visibleIndex(id); ok {
			shown = append(shown, k)
		}
	}
	if len(shown) == 0 {
		return nil
	}
	slices.Sort(shown)
	out := make([]protocol.Card, len(shown))
	for j, k := range shown {
		out[j] = s.VisibleCard(k)
	}
	return out
}

// Targets returns the cards an action applies to: the marked cards in
// visible order, or the cursor's card when none is marked, or nil when no
// card is visible.
func (s *Store) Targets() []protocol.Card {
	if marked := s.Marked(); len(marked) > 0 {
		return marked
	}
	if s.cursor < 0 {
		return nil
	}
	return []protocol.Card{s.VisibleCard(s.cursor)}
}

// ClearMarks unmarks every card, hidden ones included.
func (s *Store) ClearMarks() { clear(s.marks) }

// keepCursor runs edit, which keeps the marks and visible in step with the
// cards and the query, then puts the cursor back on its card when that card
// is still visible, else at its old index clamped to the new list. While
// the store is untouched, it puts the cursor on the first visible card.
func (s *Store) keepCursor(edit func()) {
	prevID, prevIndex := "", max(s.cursor, 0)
	if s.cursor >= 0 {
		prevID = s.VisibleCard(s.cursor).ID
	}

	edit()

	switch i, ok := s.visibleIndex(prevID); {
	case len(s.visible) == 0:
		s.cursor = -1
	case s.untouched:
		s.cursor = 0
	case ok:
		s.cursor = i
	default:
		s.cursor = min(prevIndex, len(s.visible)-1)
	}
}

// visibleIndex returns the index into Visible of the card with id; ok is
// false when no card has id or the filter hides it.
func (s *Store) visibleIndex(id string) (int, bool) {
	i, ok := s.pos[id]
	if !ok {
		return 0, false
	}
	k, ok := slices.BinarySearch(s.visible, i)
	if !ok {
		return 0, false
	}
	return s.slot(k), true
}

// slot returns the index into visible of the card at index i of Visible,
// and the other way round: i itself when cards is in screen order, else i
// counted from the end.
func (s *Store) slot(i int) int {
	if s.newest == order.Top {
		return len(s.visible) - 1 - i
	}
	return i
}

// reparse parses the filter again against the scopes the cards have, then
// rebuilds visible.
func (s *Store) reparse() {
	s.query = match.Parse(s.raw, maps.Keys(s.scopes))
	s.refilter()
}

// refilter rebuilds visible from the cards and the query.
func (s *Store) refilter() {
	s.visible = s.visible[:0]
	for i, e := range s.cards {
		if s.query.MatchText(e.text) {
			s.visible = append(s.visible, i)
		}
	}
}

// newEntry returns c with its match text prepared.
func newEntry(c protocol.Card) entry {
	return entry{card: c, text: match.Prepare(c.Scope, c.MatchText())}
}

// upsert replaces the card with c's id, or appends c, and adds it to or drops
// it from visible as the query says. When that changes the scopes the cards
// have, it parses the filter again and matches every card instead.
func (s *Store) upsert(c protocol.Card) {
	e := newEntry(c)
	i, ok := s.pos[c.ID]
	if !ok {
		i = len(s.cards)
		s.pos[c.ID] = i
		s.cards = append(s.cards, e)
		if s.addScope(c.Scope) {
			s.reparse()
		} else if s.query.MatchText(e.text) {
			s.visible = append(s.visible, i)
		}
		return
	}
	prev := s.cards[i].card.Scope
	s.cards[i] = e
	if prev != c.Scope {
		dropped, added := s.dropScope(prev), s.addScope(c.Scope)
		if dropped || added {
			s.reparse()
			return
		}
	}
	shown := s.query.MatchText(e.text)
	switch k, wasShown := slices.BinarySearch(s.visible, i); {
	case shown && !wasShown:
		s.visible = slices.Insert(s.visible, k, i)
	case !shown && wasShown:
		s.visible = slices.Delete(s.visible, k, k+1)
	}
}

// remove drops the card with id and its mark, and shifts the indexes of the
// cards after it. When it was the last card in its scope, it parses the
// filter again and matches every card.
func (s *Store) remove(id string) {
	i, ok := s.pos[id]
	if !ok {
		return
	}
	scope := s.cards[i].card.Scope
	s.cards = slices.Delete(s.cards, i, i+1)
	delete(s.pos, id)
	delete(s.marks, id)
	for j := i; j < len(s.cards); j++ {
		s.pos[s.cards[j].card.ID] = j
	}
	if s.dropScope(scope) {
		s.reparse()
		return
	}
	k, wasShown := slices.BinarySearch(s.visible, i)
	if wasShown {
		s.visible = slices.Delete(s.visible, k, k+1)
	}
	for ; k < len(s.visible); k++ {
		s.visible[k]--
	}
}

// replace swaps every card for cards, shown in their own order, and drops
// the marks on ids that are gone. A repeated id keeps its first position and
// its last content, the same as a run of upserts at the bottom.
func (s *Store) replace(cards []protocol.Card) {
	s.cards = nil
	clear(s.pos)
	for _, c := range cards {
		if i, ok := s.pos[c.ID]; ok {
			s.cards[i] = newEntry(c)
			continue
		}
		s.pos[c.ID] = len(s.cards)
		s.cards = append(s.cards, newEntry(c))
	}
	if s.newest == order.Top {
		s.reverse()
	}
	clear(s.scopes)
	for _, e := range s.cards {
		s.addScope(e.card.Scope)
	}
	s.reparse()
	maps.DeleteFunc(s.marks, func(id string, _ bool) bool {
		_, ok := s.pos[id]
		return !ok
	})
}

// reverse turns the order of cards around and keeps pos in step, so cards
// built in screen order is held bottom first.
func (s *Store) reverse() {
	slices.Reverse(s.cards)
	for i, e := range s.cards {
		s.pos[e.card.ID] = i
	}
}

// addScope counts one more card in scope and reports whether it is the
// first, so the scopes the cards have changed. The default set is not
// counted.
func (s *Store) addScope(scope string) bool {
	if scope == "" {
		return false
	}
	s.scopes[scope]++
	return s.scopes[scope] == 1
}

// dropScope counts one card fewer in scope and reports whether it was the
// last, so the scopes the cards have changed.
func (s *Store) dropScope(scope string) bool {
	if scope == "" {
		return false
	}
	if s.scopes[scope]--; s.scopes[scope] > 0 {
		return false
	}
	delete(s.scopes, scope)
	return true
}
