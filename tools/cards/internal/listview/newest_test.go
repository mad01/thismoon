package listview

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// newTopModel returns a model sized width by height with a fake starter, in
// list mode's default card style, that puts a card with a new id on top.
func newTopModel(width, height int) *Model {
	m := New(Config{Starter: &fakeStarter{}, Style: render.CardSpace, Newest: order.Top})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func TestTopInsertKeepsCursorAndMarks(t *testing.T) {
	m := newTopModel(80, 20)
	feed(m, cardLine("a", "card-a"), cardLine("b", "card-b"))
	if rows := screen(m); rows[1] != "▌ card-b" || rows[3] != "  card-a" {
		t.Fatalf("rows = %q, want card-b on top with the cursor, before any key", rows[1:4])
	}

	press(m, keySpace) // mark card-b; the cursor moves on to card-a
	feed(m, cardLine("c", "card-c"))
	rows := screen(m)
	if rows[1] != "  card-c" || rows[3] != " ▌card-b" || rows[5] != "▌ card-a" {
		t.Errorf("rows = %q, want card-c on top, the mark on card-b, the cursor on card-a",
			rows[1:6])
	}
	if f := footer(m); !strings.Contains(f, "3/3  1 marked") {
		t.Errorf("footer = %q, want 3/3 and 1 marked", f)
	}
}

func TestTopReplaceAndSnapshotKeepTheirOrder(t *testing.T) {
	m := newTopModel(80, 20)
	feed(m, cardLine("a", "card-a"), cardLine("b", "card-b"), cardLine("a", "card-a2"))
	if rows := screen(m); rows[1] != "▌ card-b" || rows[3] != "  card-a2" {
		t.Errorf("rows = %q, want card-a replaced in place under card-b", rows[1:4])
	}

	press(m, keyDown)
	snapshot := `{"type":"snapshot","cards":[` +
		`{"id":"x","lines":["card-x"]},{"id":"y","lines":["card-y"]}]}`
	feed(m, snapshot)
	// The cursor card is gone, so the cursor keeps its index, as at the bottom.
	if rows := screen(m); rows[1] != "  card-x" || rows[3] != "▌ card-y" {
		t.Errorf("rows = %q, want the snapshot in its own order", rows[1:4])
	}
}

func TestTopGridReadsLeftToRightTopToBottom(t *testing.T) {
	// Width 130 gives two columns: [4 3] [2 1] [0].
	m := newTopModel(130, 20)
	feed(m, cards(5)...)
	rows := screen(m)
	for i, want := range map[int]string{1: "card-4 card-3", 3: "card-2 card-1", 5: "card-0"} {
		if got := strings.Join(strings.Fields(strings.ReplaceAll(rows[i], bar, "")), " "); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
	if got := cursorCard(t, m); got != "card-4" {
		t.Errorf("cursor on %s, want card-4, the newest", got)
	}
}

func TestTopScrollKeepsCursorVisible(t *testing.T) {
	// Height 9 leaves 7 body rows: four single-line cards and their gaps.
	m := newTopModel(80, 9)
	feed(m, cards(20)...)
	if got := cursorCard(t, m); got != "card-19" {
		t.Fatalf("cursor on %s, want card-19, the newest", got)
	}
	press(m, keyPgDown, keyPgDown, keyPgDown, keyPgDown, keyPgDown)
	if got := cursorCard(t, m); got != "card-0" {
		t.Fatalf("after five pgdown: cursor on %s, want card-0 kept on screen", got)
	}
	press(m, keyPgUp)
	if got := cursorCard(t, m); got != "card-4" {
		t.Errorf("after pgup: cursor on %s, want card-4, one screen of four cards up", got)
	}
}

func TestTopStreamedCardsKeepCursorOnNewestUntilAKey(t *testing.T) {
	m := newTopModel(80, 9) // four cards on screen
	for i, line := range cards(10) {
		feed(m, line)
		if got, want := cursorCard(t, m), fmt.Sprintf("card-%d", i); got != want {
			t.Fatalf("after %d cards: cursor on %s, want the newest, %s", i+1, got, want)
		}
	}

	press(m, keyDown)
	feed(m, cardLine("c10", "card-10"), cardLine("c11", "card-11"))
	if got := cursorCard(t, m); got != "card-8" {
		t.Errorf("cursor on %s after down and two new cards, want card-8 kept", got)
	}
}

func TestTopAnyKeyKeepsCursorOnItsCard(t *testing.T) {
	cases := []struct {
		name string
		act  func(m *Model)
		// want is the cursor card after a new card arrives on top.
		want string
	}{
		{"up", func(m *Model) { press(m, keyUp) }, "card-2"},
		{"down", func(m *Model) { press(m, keyDown) }, "card-1"},
		{"space", func(m *Model) { press(m, keySpace) }, "card-1"},
		{"enter", func(m *Model) { press(m, keyEnter) }, "card-2"},
		{"esc", func(m *Model) { press(m, keyEsc) }, "card-2"},
		{"letter", func(m *Model) { typeText(m, "c") }, "card-2"},
		{"paste", func(m *Model) { m.Update(tea.PasteMsg{Content: "card"}) }, "card-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTopModel(80, 20)
			feed(m, cards(3)...)
			tc.act(m)
			feed(m, cardLine("c3", "card-3"))
			if got := cursorCard(t, m); got != tc.want {
				t.Errorf("cursor on %s, want %s kept", got, tc.want)
			}
		})
	}
}

func TestBottomStreamedCardsKeepCursorOnFirst(t *testing.T) {
	m, _ := newModel(80, 20)
	for i, line := range cards(5) {
		feed(m, line)
		if got := cursorCard(t, m); got != "card-0" {
			t.Fatalf("after %d cards: cursor on %s, want card-0, the first", i+1, got)
		}
	}
}
