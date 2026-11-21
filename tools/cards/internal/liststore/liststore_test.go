package liststore

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/cards/internal/match"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// step is one thing the TUI does to the store.
type step func(*Store) error

func card(id, text string) protocol.Card {
	return protocol.Card{
		ID:    id,
		Lines: []protocol.Line{{{Text: text, Color: protocol.ColorDefault}}},
		Tone:  protocol.ColorDefault,
	}
}

// add sends one card per id, each with its id as text.
func add(ids ...string) step {
	return func(s *Store) error {
		for _, id := range ids {
			if err := s.Apply(card(id, id)); err != nil {
				return err
			}
		}
		return nil
	}
}

func addText(id, text string) step {
	return func(s *Store) error { return s.Apply(card(id, text)) }
}

// scopedCard returns a card with id and text in scope.
func scopedCard(id, text, scope string) protocol.Card {
	c := card(id, text)
	c.Scope = scope
	return c
}

func addScoped(id, text, scope string) step {
	return func(s *Store) error { return s.Apply(scopedCard(id, text, scope)) }
}

func snapshot(ids ...string) step {
	cards := make([]protocol.Card, len(ids))
	for i, id := range ids {
		cards[i] = card(id, id)
	}
	return func(s *Store) error { return s.Apply(protocol.Snapshot{Cards: cards}) }
}

func remove(id string) step {
	return func(s *Store) error { return s.Apply(protocol.Remove{ID: id}) }
}

func clearCards() step {
	return func(s *Store) error { return s.Apply(protocol.Clear{}) }
}

func query(q string) step {
	return func(s *Store) error { s.SetQuery(q); return nil }
}

func move(delta int) step {
	return func(s *Store) error { s.MoveCursor(delta); return nil }
}

func mark() step {
	return func(s *Store) error { s.ToggleMark(); return nil }
}

func clearMarks() step {
	return func(s *Store) error { s.ClearMarks(); return nil }
}

func touch() step {
	return func(s *Store) error { s.Touch(); return nil }
}

// prs is three pull request cards; "fix" matches a and c.
var prs = []step{
	addText("a", "acme/api#12 fix login alice"),
	addText("b", "acme/api#13 docs typo bob"),
	addText("c", "acme/web#7 fix logout alice"),
}

func then(base []step, more ...step) []step { return append(slices.Clone(base), more...) }

func ids(cards []protocol.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.ID
	}
	return out
}

func cursorID(s *Store) string {
	if i := s.Cursor(); i >= 0 {
		return s.Visible()[i].ID
	}
	return ""
}

func TestStore(t *testing.T) {
	cases := []struct {
		name        string
		steps       []step
		wantVisible []string
		// wantCursor is the cursor card's id; empty means no cursor.
		wantCursor  string
		wantMarked  []string
		wantTargets []string
	}{
		{
			name:  "empty store ignores cursor and mark keys",
			steps: []step{move(1), mark()},
		},
		{
			name:        "first card takes the cursor",
			steps:       []step{add("a")},
			wantVisible: []string{"a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "new ids append",
			steps:       []step{add("a", "b", "c")},
			wantVisible: []string{"a", "b", "c"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "upsert keeps position",
			steps:       []step{add("a", "b", "c"), add("a")},
			wantVisible: []string{"a", "b", "c"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "upsert keeps the cursor on its card",
			steps:       []step{add("a", "b", "c"), move(1), add("b", "a")},
			wantVisible: []string{"a", "b", "c"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "remove drops by id",
			steps:       []step{add("a", "b", "c"), remove("b")},
			wantVisible: []string{"a", "c"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "removing an unknown id changes nothing",
			steps:       []step{add("a", "b"), remove("z")},
			wantVisible: []string{"a", "b"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "removed cursor card moves the cursor to the same index",
			steps:       []step{add("a", "b", "c"), move(1), remove("b")},
			wantVisible: []string{"a", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "removed last cursor card clamps to the new last",
			steps:       []step{add("a", "b", "c"), move(2), remove("c")},
			wantVisible: []string{"a", "b"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:  "removing every card unsets the cursor",
			steps: []step{add("a"), remove("a")},
		},
		{
			name:        "a card after an empty list takes the cursor",
			steps:       []step{add("a"), remove("a"), add("b")},
			wantVisible: []string{"b"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:  "clear drops every card and mark",
			steps: []step{add("a", "b"), mark(), clearCards()},
		},
		{
			name:        "snapshot replaces every card in its own order",
			steps:       []step{add("a", "b", "c"), snapshot("c", "d", "a")},
			wantVisible: []string{"c", "d", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "snapshot keeps cursor and marks by id",
			steps:       []step{add("a", "b", "c"), mark(), move(2), snapshot("c", "a", "d")},
			wantVisible: []string{"c", "a", "d"}, wantCursor: "c",
			wantMarked: []string{"a"}, wantTargets: []string{"a"},
		},
		{
			name:        "snapshot without the cursor card keeps its index",
			steps:       []step{add("a", "b", "c"), move(1), snapshot("a", "c", "d")},
			wantVisible: []string{"a", "c", "d"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "snapshot repeating an id keeps its first position",
			steps:       []step{snapshot("a", "b", "a")},
			wantVisible: []string{"a", "b"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:  "empty snapshot unsets the cursor",
			steps: []step{add("a"), snapshot()},
		},
		{
			name:        "marks on removed ids are dropped",
			steps:       []step{add("a", "b"), move(1), mark(), remove("b"), add("b")},
			wantVisible: []string{"a", "b"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "marks on ids a snapshot leaves out are dropped",
			steps:       []step{add("a", "b"), mark(), snapshot("b"), snapshot("a", "b")},
			wantVisible: []string{"a", "b"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "filter keeps producer order",
			steps:       then(prs, query("fix")),
			wantVisible: []string{"a", "c"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "filter keeps the cursor on a matching card",
			steps:       then(prs, move(2), query("fix")),
			wantVisible: []string{"a", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "filter hiding the cursor card keeps its index",
			steps:       then(prs, move(1), query("fix")),
			wantVisible: []string{"a", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "cursor moves stop at the last visible card",
			steps:       then(prs, query("fix"), move(5)),
			wantVisible: []string{"a", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "cursor moves stop at the first visible card",
			steps:       then(prs, query("fix"), move(1), move(-9)),
			wantVisible: []string{"a", "c"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:  "filter matching nothing unsets the cursor",
			steps: then(prs, query("zzz"), mark()),
		},
		{
			name:        "clearing the filter keeps the cursor card",
			steps:       then(prs, query("fix"), move(1), query("")),
			wantVisible: []string{"a", "b", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "filter applies to cards that arrive later",
			steps:       then(prs, query("alice"), addText("d", "acme/api#14 bump deps alice")),
			wantVisible: []string{"a", "c", "d"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "update that stops matching hides the card",
			steps:       then(prs, query("fix"), addText("a", "acme/api#12 docs only")),
			wantVisible: []string{"c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "hidden marks are kept but not reported",
			steps:       then(prs, move(1), mark(), query("fix")),
			wantVisible: []string{"a", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
		{
			name:        "hidden marks come back with the filter cleared",
			steps:       then(prs, move(1), mark(), query("fix"), query("")),
			wantVisible: []string{"a", "b", "c"}, wantCursor: "c",
			wantMarked: []string{"b"}, wantTargets: []string{"b"},
		},
		{
			name:        "targets are the marked cards in visible order",
			steps:       []step{add("a", "b", "c"), move(2), mark(), move(-2), mark()},
			wantVisible: []string{"a", "b", "c"}, wantCursor: "a",
			wantMarked: []string{"a", "c"}, wantTargets: []string{"a", "c"},
		},
		{
			name:        "targets without marks are the cursor card",
			steps:       []step{add("a", "b", "c"), move(1)},
			wantVisible: []string{"a", "b", "c"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "toggling twice unmarks",
			steps:       []step{add("a"), mark(), mark()},
			wantVisible: []string{"a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "clear marks unmarks every card",
			steps:       []step{add("a", "b"), mark(), move(1), mark(), clearMarks()},
			wantVisible: []string{"a", "b"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "clear marks includes hidden cards",
			steps:       then(prs, move(1), mark(), query("fix"), clearMarks(), query("")),
			wantVisible: []string{"a", "b", "c"}, wantCursor: "c", wantTargets: []string{"c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			for i, st := range tc.steps {
				if err := st(s); err != nil {
					t.Fatalf("step %d: %v", i+1, err)
				}
			}
			if got := ids(s.Visible()); !slices.Equal(got, tc.wantVisible) {
				t.Errorf("Visible() = %q, want %q", got, tc.wantVisible)
			}
			if got := cursorID(s); got != tc.wantCursor {
				t.Errorf("cursor card = %q, want %q", got, tc.wantCursor)
			}
			if got := ids(s.Marked()); !slices.Equal(got, tc.wantMarked) {
				t.Errorf("Marked() = %q, want %q", got, tc.wantMarked)
			}
			if got := ids(s.Targets()); !slices.Equal(got, tc.wantTargets) {
				t.Errorf("Targets() = %q, want %q", got, tc.wantTargets)
			}
		})
	}
}

func TestStoreNewestTop(t *testing.T) {
	cases := []struct {
		name        string
		steps       []step
		wantVisible []string
		// wantCursor is the cursor card's id; empty means no cursor.
		wantCursor  string
		wantMarked  []string
		wantTargets []string
	}{
		{
			name:        "new ids go on top and the cursor stays on its card",
			steps:       []step{add("a", "b", "c")},
			wantVisible: []string{"c", "b", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "a card above the cursor keeps the cursor on its card",
			steps:       []step{add("a", "b"), move(-1), add("c")},
			wantVisible: []string{"c", "b", "a"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "a card above a mark keeps the mark on its card",
			steps:       []step{add("a", "b"), mark(), add("c")},
			wantVisible: []string{"c", "b", "a"}, wantCursor: "a",
			wantMarked: []string{"a"}, wantTargets: []string{"a"},
		},
		{
			name:        "upsert keeps position",
			steps:       []step{add("a", "b", "c"), add("a", "c")},
			wantVisible: []string{"c", "b", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "removed cursor card moves the cursor to the same index",
			steps:       []step{add("a", "b", "c"), move(-1), remove("b")},
			wantVisible: []string{"c", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "a card after an empty list takes the cursor",
			steps:       []step{add("a"), remove("a"), add("b")},
			wantVisible: []string{"b"}, wantCursor: "b", wantTargets: []string{"b"},
		},
		{
			name:        "snapshot keeps the producer's order",
			steps:       []step{add("a", "b", "c"), snapshot("c", "d", "a")},
			wantVisible: []string{"c", "d", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "snapshot keeps cursor and marks by id",
			steps:       []step{add("a", "b", "c"), mark(), move(-2), snapshot("c", "a", "d")},
			wantVisible: []string{"c", "a", "d"}, wantCursor: "c",
			wantMarked: []string{"a"}, wantTargets: []string{"a"},
		},
		{
			name:        "snapshot repeating an id keeps its first position",
			steps:       []step{snapshot("a", "b", "a")},
			wantVisible: []string{"a", "b"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "a card after a snapshot goes on top",
			steps:       []step{snapshot("a", "b"), add("c"), add("b")},
			wantVisible: []string{"c", "a", "b"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "filter applies to cards that arrive later",
			steps:       then(prs, query("alice"), addText("d", "acme/api#14 bump deps alice")),
			wantVisible: []string{"d", "c", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "a new card the filter hides stays hidden",
			steps:       then(prs, query("fix"), addText("d", "acme/api#14 bump deps")),
			wantVisible: []string{"c", "a"}, wantCursor: "a", wantTargets: []string{"a"},
		},
		{
			name:        "targets are the marked cards in screen order",
			steps:       []step{add("a", "b", "c"), mark(), move(-2), mark()},
			wantVisible: []string{"c", "b", "a"}, wantCursor: "c",
			wantMarked: []string{"c", "a"}, wantTargets: []string{"c", "a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// These cases follow the cursor after the user's first key;
			// TestNewestTopUntouched covers the cursor before it.
			s := NewAt(order.Top)
			s.Touch()
			for i, st := range tc.steps {
				if err := st(s); err != nil {
					t.Fatalf("step %d: %v", i+1, err)
				}
			}
			if got := ids(s.Visible()); !slices.Equal(got, tc.wantVisible) {
				t.Errorf("Visible() = %q, want %q", got, tc.wantVisible)
			}
			if got := cursorID(s); got != tc.wantCursor {
				t.Errorf("cursor card = %q, want %q", got, tc.wantCursor)
			}
			if got := ids(s.Marked()); !slices.Equal(got, tc.wantMarked) {
				t.Errorf("Marked() = %q, want %q", got, tc.wantMarked)
			}
			if got := ids(s.Targets()); !slices.Equal(got, tc.wantTargets) {
				t.Errorf("Targets() = %q, want %q", got, tc.wantTargets)
			}
		})
	}
}

func TestNewestTopUntouched(t *testing.T) {
	cases := []struct {
		name   string
		newest order.Newest
		steps  []step
		// wantCursor is the cursor card's id.
		wantCursor string
	}{
		{
			name:   "streamed cards put the cursor on the newest",
			newest: order.Top, steps: []step{add("a"), add("b"), add("c")},
			wantCursor: "c",
		},
		{
			name:   "an older card sent again leaves the cursor on the newest",
			newest: order.Top, steps: []step{add("a", "b", "c"), add("a")},
			wantCursor: "c",
		},
		{
			name:   "a removed newest card puts the cursor on the next newest",
			newest: order.Top, steps: []step{add("a", "b", "c"), remove("c")},
			wantCursor: "b",
		},
		{
			name:   "a snapshot puts the cursor on its first card",
			newest: order.Top, steps: []step{add("a", "b"), snapshot("x", "y")},
			wantCursor: "x",
		},
		{
			name:   "a card after Touch leaves the cursor on its card",
			newest: order.Top, steps: []step{add("a", "b"), touch(), add("c")},
			wantCursor: "b",
		},
		{
			name:   "a card after Touch and a move leaves the cursor on its card",
			newest: order.Top, steps: []step{add("a", "b", "c"), touch(), move(1), add("d")},
			wantCursor: "b",
		},
		{
			name:   "at the bottom streamed cards leave the cursor on the first",
			newest: order.Bottom, steps: []step{add("a"), add("b"), add("c")},
			wantCursor: "a",
		},
		{
			name:   "at the bottom a snapshot keeps the cursor on its card",
			newest: order.Bottom, steps: []step{add("a", "b"), snapshot("b", "a")},
			wantCursor: "a",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewAt(tc.newest)
			for i, st := range tc.steps {
				if err := st(s); err != nil {
					t.Fatalf("step %d: %v", i+1, err)
				}
			}
			if got := cursorID(s); got != tc.wantCursor {
				t.Errorf("cursor card = %q, want %q", got, tc.wantCursor)
			}
		})
	}
}

func TestUpsertReplacesContentInPlaceAtTop(t *testing.T) {
	s := NewAt(order.Top)
	for _, c := range []protocol.Card{card("a", "old title"), card("b", "other"), card("a", "new title")} {
		if err := s.Apply(c); err != nil {
			t.Fatalf("Apply(%q): %v", c.ID, err)
		}
	}
	if got := ids(s.Visible()); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("Visible() = %q, want b above a, a kept in place", got)
	}
	if got := s.VisibleCard(1).Lines[0].Text(); got != "new title" {
		t.Errorf("card a text = %q, want %q", got, "new title")
	}
}

func TestUpsertReplacesContent(t *testing.T) {
	s := New()
	for _, c := range []protocol.Card{card("a", "old title"), card("b", "other"), card("a", "new title")} {
		if err := s.Apply(c); err != nil {
			t.Fatalf("Apply(%q): %v", c.ID, err)
		}
	}
	if got := s.Visible()[0].Lines[0].Text(); got != "new title" {
		t.Errorf("card a text = %q, want %q", got, "new title")
	}
}

func TestFilterUsesMatchField(t *testing.T) {
	c := card("a", "Fix login redirect")
	c.Match = "alice"
	s := New()
	if err := s.Apply(c); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for q, want := range map[string]int{"alice": 1, "login": 0} {
		s.SetQuery(q)
		if got := len(s.Visible()); got != want {
			t.Errorf("SetQuery(%q): %d visible, want %d", q, got, want)
		}
	}
}

func TestApplyRejectsMissingID(t *testing.T) {
	cases := []struct {
		name string
		msg  protocol.Message
	}{
		{"card", card("", "no id")},
		{"snapshot", protocol.Snapshot{Cards: []protocol.Card{card("x", "x"), card("", "no id")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			if err := add("a", "b")(s); err != nil {
				t.Fatalf("setup: %v", err)
			}
			s.MoveCursor(1)
			s.ToggleMark()
			if err := s.Apply(tc.msg); !errors.Is(err, ErrMissingID) {
				t.Fatalf("Apply() error = %v, want %v", err, ErrMissingID)
			}
			if got := ids(s.Visible()); !slices.Equal(got, []string{"a", "b"}) {
				t.Errorf("Visible() = %q after a rejected message, want it unchanged", got)
			}
			if got := ids(s.Marked()); !slices.Equal(got, []string{"b"}) {
				t.Errorf("Marked() = %q after a rejected message, want it unchanged", got)
			}
		})
	}
}

func TestHeader(t *testing.T) {
	s := New()
	if s.Header() != nil {
		t.Errorf("Header() = %v on a new store, want nil", s.Header())
	}
	want := protocol.Line{{Text: "Open pull requests", Color: protocol.ColorDefault, Bold: true}}
	steps := []step{
		func(s *Store) error { return s.Apply(protocol.Header{Text: want}) },
		add("a"),
		clearCards(),
	}
	for i, st := range steps {
		if err := st(s); err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
	}
	if got := s.Header(); !slices.Equal(got, want) {
		t.Errorf("Header() = %v after clear, want %v", got, want)
	}
}

func TestLenCountsHiddenCards(t *testing.T) {
	s := New()
	for i, st := range then(prs, query("fix"), remove("a")) {
		if err := st(s); err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
	}
	if got, visible := s.Len(), len(s.Visible()); got != 2 || visible != 1 {
		t.Errorf("Len() = %d with %d visible, want 2 with 1 visible", got, visible)
	}
}

func TestApplyRejectsNilMessage(t *testing.T) {
	if err := New().Apply(nil); err == nil {
		t.Error("Apply(nil) error = nil, want an error")
	}
}

func TestVisibleTracksEveryChange(t *testing.T) {
	for _, newest := range []order.Newest{order.Bottom, order.Top} {
		t.Run(newest.String(), func(t *testing.T) { visibleTracksEveryChange(t, newest) })
	}
}

func visibleTracksEveryChange(t *testing.T, newest order.Newest) {
	// A model of the contract: cards in producer order, upsert by id, a new
	// id at the newest end unless a snapshot sends it, and Visible the ones
	// the query matches. Seeded, so a failure repeats.
	// A card's scope selects it as package match says, against the scopes
	// the model's cards have.
	type modelCard struct{ id, text, scope string }
	var model []modelCard
	upsert := func(c modelCard, at order.Newest) {
		if i := slices.IndexFunc(model, func(m modelCard) bool { return m.id == c.id }); i >= 0 {
			model[i] = c
			return
		}
		if at == order.Top {
			model = slices.Insert(model, 0, c)
			return
		}
		model = append(model, c)
	}
	present := func(yield func(string) bool) {
		for _, c := range model {
			if c.scope != "" && !yield(c.scope) {
				return
			}
		}
	}
	idSet := []string{"a", "b", "c", "d", "e", "f"}
	texts := []string{"fix login", "docs typo", "fix logout", "bump deps", "i: note"}
	scopes := []string{"", "", "ignored", "inbox"}
	queries := []string{"", "fix", "lgn", "docs", "zzz", "i:", "ig:fix", "in:", "i:lgn", "docs:"}
	rng := rand.New(rand.NewPCG(1, 2))
	pick := func(from []string) string { return from[rng.IntN(len(from))] }

	// held is whether the cursor stays on its card as cards come in: from the
	// start at the bottom, from the first key on top. A query comes from a
	// key, so it touches the store first, as the view does.
	s, q, held := NewAt(newest), "", newest == order.Bottom
	for step := range 2000 {
		switch rng.IntN(10) {
		case 0:
			s.Touch()
			held = true
			q = pick(queries)
			s.SetQuery(q)
		case 1:
			id := pick(idSet)
			model = slices.DeleteFunc(model, func(c modelCard) bool { return c.id == id })
			if err := s.Apply(protocol.Remove{ID: id}); err != nil {
				t.Fatal(err)
			}
		case 2:
			model = nil
			var cards []protocol.Card
			for range rng.IntN(4) {
				c := modelCard{pick(idSet), pick(texts), pick(scopes)}
				upsert(c, order.Bottom)
				cards = append(cards, scopedCard(c.id, c.text, c.scope))
			}
			if err := s.Apply(protocol.Snapshot{Cards: cards}); err != nil {
				t.Fatal(err)
			}
		default:
			c, prev := modelCard{pick(idSet), pick(texts), pick(scopes)}, cursorID(s)
			upsert(c, newest)
			shownBefore := slices.Contains(ids(s.Visible()), prev)
			if err := s.Apply(scopedCard(c.id, c.text, c.scope)); err != nil {
				t.Fatal(err)
			}
			// A card can change the scopes the filter selects and so hide
			// the cursor card; only a card that leaves it shown must leave
			// the cursor on it.
			stillShown := slices.Contains(ids(s.Visible()), prev)
			if held && shownBefore && stillShown && c.id != prev && cursorID(s) != prev {
				t.Fatalf("step %d: card %s moved the cursor from %s to %s", step, c.id, prev,
					cursorID(s))
			}
		}
		if !held && s.Cursor() > 0 {
			t.Fatalf("step %d: untouched cursor %d, want 0, the newest card", step, s.Cursor())
		}

		var want []string
		wantSet := 0
		parsed := match.Parse(q, present)
		for _, c := range model {
			if parsed.Selects(c.scope) {
				wantSet++
			}
			if parsed.Match(c.scope, c.text) {
				want = append(want, c.id)
			}
		}
		if got := ids(s.Visible()); !slices.Equal(got, want) {
			t.Fatalf("step %d, query %q: Visible() = %q, want %q", step, q, got, want)
		}
		if got := s.ScopeLen(); got != wantSet {
			t.Fatalf("step %d, query %q: ScopeLen() = %d, want %d", step, q, got, wantSet)
		}
		if got, want := s.Scopes(), parsed.Scopes(); !slices.Equal(got, want) {
			t.Fatalf("step %d, query %q: Scopes() = %q, want %q", step, q, got, want)
		}
		if cur := s.Cursor(); (cur < 0) != (len(want) == 0) || cur >= len(want) {
			t.Fatalf("step %d: cursor %d with %d visible", step, cur, len(want))
		}
	}
}

func TestManySingleCardsApplyFast(t *testing.T) {
	// Each card used to refilter every card before it, so this took tens of
	// seconds. Now each costs about the same; the bound is generous enough
	// for the race detector on a slow machine.
	// A card on top costs the same as one at the bottom, and a card in a
	// scope that is already present costs the same as one in the default
	// set: only the first card of a scope matches every card again.
	const n, bound = 20_000, 5 * time.Second
	for _, newest := range []order.Newest{order.Bottom, order.Top} {
		for _, q := range []string{"card", "i:card"} {
			s := NewAt(newest)
			s.SetQuery(q)
			start := time.Now()
			for i := range n {
				c := card(fmt.Sprintf("c%d", i), fmt.Sprintf("card %d", i))
				if i%2 == 1 {
					c.Scope = "ignored"
				}
				if err := s.Apply(c); err != nil {
					t.Fatal(err)
				}
			}
			if took := time.Since(start); took > bound {
				t.Errorf("%v %q: %d single-card applies took %v, want under %v", newest, q, n,
					took, bound)
			}
			if got, shown := s.Len(), s.VisibleLen(); got != n || shown != n/2 {
				t.Errorf("%v %q: Len() = %d with %d shown, want %d with %d shown", newest, q,
					got, shown, n, n/2)
			}
		}
	}
}

func TestManyScopedRemovesApplyFast(t *testing.T) {
	// Removing a card whose scope keeps other cards matches no other card.
	const n, bound = 20_000, 5 * time.Second
	cards := make([]protocol.Card, n)
	for i := range cards {
		cards[i] = scopedCard(fmt.Sprintf("c%d", i), fmt.Sprintf("card %d", i), "ignored")
	}
	s := New()
	if err := s.Apply(protocol.Snapshot{Cards: cards}); err != nil {
		t.Fatal(err)
	}
	s.SetQuery("i:card")
	start := time.Now()
	for i := n - 1; i > 0; i-- {
		if err := s.Apply(protocol.Remove{ID: fmt.Sprintf("c%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took > bound {
		t.Errorf("%d scoped removes took %v, want under %v", n-1, took, bound)
	}
	if got := ids(s.Visible()); !slices.Equal(got, []string{"c0"}) {
		t.Errorf("Visible() = %q, want the one card left", got)
	}
}

func TestScopes(t *testing.T) {
	// a and c are in the default set, b and d are ignored, e is in the inbox.
	cards := []step{
		addText("a", "acme/api#12 fix login i: note"),
		addScoped("b", "acme/api#13 docs typo", "ignored"),
		addText("c", "acme/web#7 fix logout"),
		addScoped("d", "acme/web#8 fix deps", "ignored"),
		addScoped("e", "acme/web#9 fix ci", "inbox"),
	}
	cases := []struct {
		name        string
		steps       []step
		wantVisible []string
		// wantScopes is what the filter selects; nil is the default set.
		wantScopes   []string
		wantScopeLen int
		wantMarked   []string
	}{
		{
			name:        "scoped cards are hidden by default",
			steps:       cards,
			wantVisible: []string{"a", "c"}, wantScopeLen: 2,
		},
		{
			name:        "a scope prefix shows only that scope",
			steps:       then(cards, query("ig:")),
			wantVisible: []string{"b", "d"}, wantScopes: []string{"ignored"}, wantScopeLen: 2,
		},
		{
			name:        "the rest of the word filters within the scope",
			steps:       then(cards, query("ignored:fix")),
			wantVisible: []string{"d"}, wantScopes: []string{"ignored"}, wantScopeLen: 2,
		},
		{
			name:        "a prefix of two scopes shows both",
			steps:       then(cards, query("i:")),
			wantVisible: []string{"b", "d", "e"}, wantScopes: []string{"ignored", "inbox"},
			wantScopeLen: 3,
		},
		{
			name:        "a prefix of no scope is text in the default set",
			steps:       then(cards, query("fix:")),
			wantVisible: []string{"a"}, wantScopeLen: 2,
		},
		{
			name:        "clearing the filter brings the default set back",
			steps:       then(cards, query("i:"), query("")),
			wantVisible: []string{"a", "c"}, wantScopeLen: 2,
		},
		{
			name: "the first card of a scope makes the prefix select it",
			steps: []step{
				addText("a", "acme/api#12 fix login i: note"), query("i:"),
				addScoped("b", "acme/api#13 docs typo", "ignored"),
			},
			wantVisible: []string{"b"}, wantScopes: []string{"ignored"}, wantScopeLen: 1,
		},
		{
			name:        "the last card of a scope going makes the prefix text again",
			steps:       then(cards, query("i:"), remove("b"), remove("d"), remove("e")),
			wantVisible: []string{"a"}, wantScopeLen: 2,
		},
		{
			name: "a card moving out of the last scope card makes the prefix text again",
			steps: []step{
				addText("a", "acme/api#12 fix login i: note"),
				addScoped("b", "acme/api#13 docs typo", "ignored"), query("i:"),
				addText("b", "acme/api#13 docs typo"),
			},
			wantVisible: []string{"a"}, wantScopeLen: 2,
		},
		{
			name:        "a card moving into a present scope shows in it",
			steps:       then(cards, query("i:"), addScoped("a", "acme/api#12 fix login", "inbox")),
			wantVisible: []string{"a", "b", "d", "e"}, wantScopes: []string{"ignored", "inbox"},
			wantScopeLen: 4,
		},
		{
			name: "a snapshot changes the scopes present",
			steps: then(cards, query("i:"), func(s *Store) error {
				return s.Apply(protocol.Snapshot{Cards: []protocol.Card{
					card("x", "x i: y"), scopedCard("y", "y", "muted"),
				}})
			}),
			wantVisible: []string{"x"}, wantScopeLen: 1,
		},
		{
			name:        "marks in the default set are kept but hidden under a scope",
			steps:       then(cards, mark(), query("i:")),
			wantVisible: []string{"b", "d", "e"}, wantScopes: []string{"ignored", "inbox"},
			wantScopeLen: 3,
		},
		{
			name:        "marks in the default set come back with it",
			steps:       then(cards, mark(), query("i:"), query("")),
			wantVisible: []string{"a", "c"}, wantScopeLen: 2, wantMarked: []string{"a"},
		},
		{
			name:        "a mark in a scope shows under its prefix",
			steps:       then(cards, query("ig:"), move(1), mark()),
			wantVisible: []string{"b", "d"}, wantScopes: []string{"ignored"}, wantScopeLen: 2,
			wantMarked: []string{"d"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			for i, st := range tc.steps {
				if err := st(s); err != nil {
					t.Fatalf("step %d: %v", i+1, err)
				}
			}
			if got := ids(s.Visible()); !slices.Equal(got, tc.wantVisible) {
				t.Errorf("Visible() = %q, want %q", got, tc.wantVisible)
			}
			if got := s.Scopes(); !slices.Equal(got, tc.wantScopes) {
				t.Errorf("Scopes() = %q, want %q", got, tc.wantScopes)
			}
			if got := s.ScopeLen(); got != tc.wantScopeLen {
				t.Errorf("ScopeLen() = %d, want %d", got, tc.wantScopeLen)
			}
			if got := ids(s.Marked()); !slices.Equal(got, tc.wantMarked) {
				t.Errorf("Marked() = %q, want %q", got, tc.wantMarked)
			}
		})
	}
}
