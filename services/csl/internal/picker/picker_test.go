package picker

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

const (
	testWidth   = 80
	testHeight  = 24
	testTimeout = 2 * time.Second
)

// bottomItemRow is the row the best match is drawn on: prompt and count lines
// sit below it.
const bottomItemRow = testHeight - 3

// countRow is the row holding the matched/total count.
const countRow = testHeight - 2

func testItems() []Item {
	return []Item{
		{{Text: "alpha", Color: Default}},
		{{Text: "bravo", Color: Cyan}},
		{{Text: "charlie", Color: Blue}},
	}
}

// outcome is what pick returned, carried off the picker's goroutine.
type outcome struct {
	idx int
	err error
}

func newScreen(t *testing.T) tcell.SimulationScreen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	s.SetSize(testWidth, testHeight)
	// Finalizing unblocks a picker still polling for events, so a failed test
	// cannot leave the goroutine behind.
	t.Cleanup(s.Fini)
	return s
}

func startPicker(t *testing.T, items []Item) (tcell.SimulationScreen, <-chan outcome) {
	t.Helper()
	s := newScreen(t)
	done := make(chan outcome, 1)
	go func() {
		idx, err := pick(s, items)
		done <- outcome{idx: idx, err: err}
	}()
	return s, done
}

func waitOutcome(t *testing.T, done <-chan outcome) outcome {
	t.Helper()
	select {
	case o := <-done:
		return o
	case <-time.After(testTimeout):
		t.Fatal("picker did not return before the timeout")
		return outcome{}
	}
}

// waitRune polls a cell until it holds want, because the picker redraws on its
// own goroutine. It returns the style so callers can assert on colors.
func waitRune(t *testing.T, s tcell.SimulationScreen, x, y int, want rune) tcell.Style {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for {
		r, _, style, _ := s.GetContent(x, y)
		if r == want {
			return style
		}
		if time.Now().After(deadline) {
			t.Fatalf("cell (%d,%d) holds %q, want %q", x, y, r, want)
			return style
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func typeQuery(s tcell.SimulationScreen, query string) {
	for _, r := range query {
		s.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

func TestPickQueryReturnsOriginalIndex(t *testing.T) {
	s, done := startPicker(t, testItems())
	typeQuery(s, "ch")
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	got := waitOutcome(t, done)
	if got.err != nil {
		t.Fatalf("pick returned error %v, want nil", got.err)
	}
	if got.idx != 2 {
		t.Errorf("pick returned index %d, want 2 (charlie)", got.idx)
	}
}

func TestPickEmptyQueryReturnsFirstItem(t *testing.T) {
	s, done := startPicker(t, testItems())
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	got := waitOutcome(t, done)
	if got.err != nil {
		t.Fatalf("pick returned error %v, want nil", got.err)
	}
	if got.idx != 0 {
		t.Errorf("pick returned index %d, want 0", got.idx)
	}
}

func TestPickCursorUpSelectsSecondItem(t *testing.T) {
	s, done := startPicker(t, testItems())
	s.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	got := waitOutcome(t, done)
	if got.err != nil {
		t.Fatalf("pick returned error %v, want nil", got.err)
	}
	if got.idx != 1 {
		t.Errorf("pick returned index %d, want 1 (bravo)", got.idx)
	}
}

func TestPickAbort(t *testing.T) {
	tests := []struct {
		name string
		key  tcell.Key
		mod  tcell.ModMask
	}{
		{name: "escape", key: tcell.KeyEscape, mod: tcell.ModNone},
		{name: "ctrl-c", key: tcell.KeyCtrlC, mod: tcell.ModCtrl},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, done := startPicker(t, testItems())
			s.InjectKey(tt.key, 0, tt.mod)

			got := waitOutcome(t, done)
			if !errors.Is(got.err, ErrAbort) {
				t.Fatalf("pick returned error %v, want ErrAbort", got.err)
			}
			if got.idx != -1 {
				t.Errorf("pick returned index %d, want -1", got.idx)
			}
		})
	}
}

func TestPickNoItemsAborts(t *testing.T) {
	s := newScreen(t)
	idx, err := pick(s, nil)
	if !errors.Is(err, ErrAbort) {
		t.Fatalf("pick returned error %v, want ErrAbort", err)
	}
	if idx != -1 {
		t.Errorf("pick returned index %d, want -1", idx)
	}
}

func TestPickBackspaceRestoresMatches(t *testing.T) {
	s, done := startPicker(t, testItems())
	typeQuery(s, "zz")
	// Nothing matches "zz", so the count line proves the query landed before
	// the backspaces undo it.
	waitRune(t, s, 2, countRow, '0')

	s.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	s.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	waitRune(t, s, 2, countRow, '3')
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	got := waitOutcome(t, done)
	if got.err != nil {
		t.Fatalf("pick returned error %v, want nil", got.err)
	}
	if got.idx != 0 {
		t.Errorf("pick returned index %d, want 0", got.idx)
	}
}

func TestPickSegmentColors(t *testing.T) {
	items := []Item{{
		{Text: "alpha", Color: Default},
		{Text: " ", Color: Default},
		{Text: "beta", Color: Cyan},
	}}
	// Column of the 'b' in "beta": two marker columns plus "alpha ".
	const betaColumn = 2 + len("alpha ")

	tests := []struct {
		name    string
		noColor string
		want    tcell.Color
	}{
		{name: "colored", noColor: "", want: tcell.ColorAqua},
		{name: "no color", noColor: "1", want: tcell.ColorDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			s, _ := startPicker(t, items)

			style := waitRune(t, s, betaColumn, bottomItemRow, 'b')
			fg, _, _ := style.Decompose()
			if fg != tt.want {
				t.Errorf("foreground is %v, want %v", fg, tt.want)
			}
		})
	}
}

func TestItemString(t *testing.T) {
	tests := []struct {
		name string
		item Item
		want string
	}{
		{name: "empty", item: Item{}, want: ""},
		{name: "single segment", item: Item{{Text: "alpha"}}, want: "alpha"},
		{
			name: "joined segments",
			item: Item{
				{Text: "csl", Color: Cyan},
				{Text: " ", Color: Default},
				{Text: "mad01", Color: Dim},
			},
			want: "csl mad01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchedRunes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		plain string
		pos   [2]int
		want  []bool
	}{
		{
			name:  "no match range",
			query: "ab",
			plain: "abc",
			pos:   [2]int{-1, -1},
			want:  []bool{false, false, false},
		},
		{
			name:  "prefix",
			query: "ab",
			plain: "abc",
			pos:   [2]int{0, 1},
			want:  []bool{true, true, false},
		},
		{
			name:  "case insensitive",
			query: "AB",
			plain: "abc",
			pos:   [2]int{0, 1},
			want:  []bool{true, true, false},
		},
		{
			name:  "outside range ignored",
			query: "c",
			plain: "abc",
			pos:   [2]int{0, 1},
			want:  []bool{false, false, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchedRunes([]rune(tt.query), tt.plain, tt.pos)
			if len(got) != len(tt.want) {
				t.Fatalf("matchedRunes returned %d flags, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("rune %d matched = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTrimWord(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "empty", query: "", want: ""},
		{name: "single word", query: "alpha", want: ""},
		{name: "two words", query: "alpha bravo", want: "alpha "},
		{name: "trailing spaces", query: "alpha bravo  ", want: "alpha "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(trimWord([]rune(tt.query))); got != tt.want {
				t.Errorf("trimWord(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

// TestScrollKeepsCursorVisibleAfterResize pins the scroll window: moving
// past the first screenful slides the window, and a resize that drops rows
// re-clamps it so the cursor row is still drawn (and Enter returns what the
// user sees).
func TestScrollKeepsCursorVisibleAfterResize(t *testing.T) {
	items := make([]Item, 10)
	for i := range items {
		items[i] = Item{{Text: fmt.Sprintf("item-%02d", i)}}
	}
	s := newScreen(t)
	s.SetSize(testWidth, 8) // 6 item rows
	p := newPicker(s, items)
	for range 8 {
		p.moveCursor(1)
	}
	if p.cursor != 8 || p.offset != 3 {
		t.Fatalf("after 8 moves cursor=%d offset=%d, want 8 and 3", p.cursor, p.offset)
	}

	s.SetSize(testWidth, 5) // 3 item rows: the cursor is now off screen
	p.scroll()
	if rows := p.itemRows(); p.cursor < p.offset || p.cursor >= p.offset+rows {
		t.Fatalf("after shrink cursor=%d offset=%d rows=%d: cursor not in window",
			p.cursor, p.offset, rows)
	}
	p.draw()
	// The cursor marker sits on the row of the selected item; with the
	// cursor at the top of a 3-row window it is the top item row (y=0).
	if r, _, _, _ := s.GetContent(0, 0); r != '>' {
		t.Errorf("cursor marker not drawn at the top row after resize; got %q", r)
	}
}

// TestPickResizeEventReclamps drives the same case through the event loop.
func TestPickResizeEventReclamps(t *testing.T) {
	items := make([]Item, 10)
	for i := range items {
		items[i] = Item{{Text: fmt.Sprintf("item-%02d", i)}}
	}
	s, done := startPicker(t, items)
	waitRune(t, s, textColumn, testHeight-3, 'i')
	for range 8 {
		s.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	}
	s.SetSize(testWidth, 5)
	if err := s.PostEvent(tcell.NewEventResize(testWidth, 5)); err != nil {
		t.Fatalf("post resize: %v", err)
	}
	// The selected line (item-08) must be drawn somewhere on screen.
	waitRune(t, s, textColumn+6, 0, '8')
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	if o := waitOutcome(t, done); o.err != nil || o.idx != 8 {
		t.Fatalf("pick = %d, %v; want 8", o.idx, o.err)
	}
}
