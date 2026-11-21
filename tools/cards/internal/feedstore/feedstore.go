// Package feedstore holds the state of feed mode: the header, the entries in
// arrival order with the newest last, and the filter. It does no I/O and
// reads no clock: the TUI passes each message with the time its line
// arrived.
//
// A card upserts by id, so a producer that sends an event again (a tail -F
// that starts a trimmed file over) does not show it twice: the entry with
// that id is replaced where it stands. A card with no id always appends. The
// store keeps the newest entries up to its limit and drops the oldest.
//
// An entry whose card has a scope shows only while the filter selects its
// scope, as package match says; the others make up the default set. The
// filter is parsed each time it is used, against the scopes the entries
// have then, so an entry that brings or takes away a scope changes what
// the filter selects.
package feedstore

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/mad01/thismoon/tools/cards/internal/match"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// DefaultMax is how many entries feed mode keeps unless told otherwise.
const DefaultMax = 1000

// Entry is one card in the feed.
type Entry struct {
	// Seq numbers the entries in arrival order, from 1. A replaced entry
	// keeps its number, so Seq rises with the position in the feed.
	Seq  uint64
	Card protocol.Card
	// Arrived is when the entry's first card arrived.
	Arrived time.Time
}

// Time returns when the entry's event happened: the card's At, or Arrived
// when the card has none.
func (e Entry) Time() time.Time {
	if e.Card.At.IsZero() {
		return e.Arrived
	}
	return e.Card.At
}

// Store is the feed-mode state. Construct with New.
type Store struct {
	limit   int
	header  protocol.Line
	entries []Entry
	// seq is the Seq of the newest entry ever added.
	seq uint64
	// query is the filter as typed.
	query string
}

// New returns an empty Store that keeps the newest limit entries; a limit
// below 1 keeps one.
func New(limit int) *Store { return &Store{limit: max(limit, 1)} }

// Apply updates the store with one decoded message whose line arrived at
// arrived. A card replaces the entry with its id in place, keeping that
// entry's Seq and Arrived, or appends a new entry; a card with an empty id
// always appends. A snapshot replaces every entry with its cards, under the
// same rule; Clear drops every entry and keeps the header.
func (s *Store) Apply(m protocol.Message, arrived time.Time) error {
	switch m := m.(type) {
	case protocol.Header:
		s.header = m.Text
	case protocol.Card:
		s.upsert(m, arrived)
	case protocol.Remove:
		s.entries = slices.DeleteFunc(s.entries, func(e Entry) bool { return e.Card.ID == m.ID })
	case protocol.Snapshot:
		s.entries = nil
		for _, c := range m.Cards {
			s.upsert(c, arrived)
		}
	case protocol.Clear:
		s.entries = nil
	default:
		return fmt.Errorf("feedstore: unsupported message %T", m)
	}
	return nil
}

// upsert replaces the entry with c's id, or appends c as a new entry and
// drops the oldest entries past the limit.
func (s *Store) upsert(c protocol.Card, arrived time.Time) {
	if c.ID != "" {
		i := slices.IndexFunc(s.entries, func(e Entry) bool { return e.Card.ID == c.ID })
		if i >= 0 {
			s.entries[i].Card = c
			return
		}
	}
	s.seq++
	s.entries = append(s.entries, Entry{Seq: s.seq, Card: c, Arrived: arrived})
	if over := len(s.entries) - s.limit; over > 0 {
		// Slicing off the front is cheap; append moves the entries to a
		// fresh array once the old one is full, which frees the dropped
		// ones.
		s.entries = s.entries[over:]
	}
}

// Header returns the header line; nil when none was sent.
func (s *Store) Header() protocol.Line { return s.header }

// SetQuery filters the entries by query, as package match parses it, on
// their cards' scope and match text. An empty query clears the filter,
// which shows the default set.
func (s *Store) SetQuery(query string) { s.query = query }

// Visible returns the entries that match the filter, oldest first.
func (s *Store) Visible() []Entry {
	q := s.parsed()
	var out []Entry
	for _, e := range s.entries {
		if q.Match(e.Card.Scope, e.Card.MatchText()) {
			out = append(out, e)
		}
	}
	return out
}

// Scopes returns the scopes the filter selects, sorted, or nil while it
// shows the default set.
func (s *Store) Scopes() []string { return s.parsed().Scopes() }

// Len returns how many entries the store holds, hidden ones included.
func (s *Store) Len() int { return len(s.entries) }

// ScopeLen returns how many entries the shown set holds: the entries in the
// scopes the filter selects, or the default set while it selects none, with
// the entries the rest of the filter hides.
func (s *Store) ScopeLen() int {
	q := s.parsed()
	n := 0
	for _, e := range s.entries {
		if q.Selects(e.Card.Scope) {
			n++
		}
	}
	return n
}

// parsed returns the filter parsed against the scopes the entries have.
func (s *Store) parsed() match.Query { return match.Parse(s.query, s.scopes) }

// scopes yields the scope of every entry that has one.
func (s *Store) scopes(yield func(string) bool) {
	for _, e := range s.entries {
		if e.Card.Scope != "" && !yield(e.Card.Scope) {
			return
		}
	}
}

// Holds reports whether the store holds the entry numbered seq, hidden or
// not. Its cost follows the log of the entries, as Seq rises with position.
func (s *Store) Holds(seq uint64) bool {
	_, ok := slices.BinarySearchFunc(s.entries, seq, func(e Entry, seq uint64) int {
		return cmp.Compare(e.Seq, seq)
	})
	return ok
}
