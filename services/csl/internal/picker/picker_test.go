package picker

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ktr0731/go-fuzzyfinder/matching"
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
		idx, err := pick(s, items, fallbackBand)
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
	idx, err := pick(s, nil, fallbackBand)
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

func TestWordBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		pos       int
		wantStart int
		wantEnd   int
	}{
		{name: "empty", query: "", pos: 0, wantStart: 0, wantEnd: 0},
		{name: "single word end", query: "alpha", pos: 5, wantStart: 0, wantEnd: 5},
		{name: "two words end", query: "alpha bravo", pos: 11, wantStart: 6, wantEnd: 11},
		{name: "trailing spaces", query: "alpha bravo  ", pos: 13, wantStart: 6, wantEnd: 13},
		{name: "mid word", query: "alpha bravo", pos: 8, wantStart: 6, wantEnd: 11},
		{name: "at word start", query: "alpha bravo", pos: 6, wantStart: 0, wantEnd: 11},
		{name: "leading spaces", query: "  alpha", pos: 0, wantStart: 0, wantEnd: 7},
		{name: "pos past end clamps", query: "ab", pos: 9, wantStart: 0, wantEnd: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := []rune(tt.query)
			if got := wordStart(q, tt.pos); got != tt.wantStart {
				t.Errorf("wordStart(%q, %d) = %d, want %d", tt.query, tt.pos, got, tt.wantStart)
			}
			if got := wordEnd(q, tt.pos); got != tt.wantEnd {
				t.Errorf("wordEnd(%q, %d) = %d, want %d", tt.query, tt.pos, got, tt.wantEnd)
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
	p := newPicker(s, items, fallbackBand)
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

// key is one keystroke a test feeds the picker, in the shape tcell delivers.
type key struct {
	k   tcell.Key
	r   rune
	mod tcell.ModMask
}

func typed(s string) []key {
	keys := make([]key, 0, len(s))
	for _, r := range s {
		keys = append(keys, key{k: tcell.KeyRune, r: r})
	}
	return keys
}

func ctrl(k tcell.Key) key { return key{k: k, mod: tcell.ModCtrl} }

func plain(k tcell.Key) key { return key{k: k} }

func altRune(r rune) key { return key{k: tcell.KeyRune, r: r, mod: tcell.ModAlt} }

// TestEditQuery pins the readline-style editing keys: what the query holds
// and where the edit cursor sits after each sequence.
func TestEditQuery(t *testing.T) {
	tests := []struct {
		name    string
		keys    []key
		want    string
		wantPos int
	}{
		{name: "typing appends", keys: typed("ab"), want: "ab", wantPos: 2},
		{
			name:    "ctrl-a then insert at start",
			keys:    append(typed("ab"), ctrl(tcell.KeyCtrlA), typed("x")[0]),
			want:    "xab",
			wantPos: 1,
		},
		{
			name:    "home end",
			keys:    append(typed("ab"), plain(tcell.KeyHome), plain(tcell.KeyEnd)),
			want:    "ab",
			wantPos: 2,
		},
		{
			name: "left stops at start",
			keys: append(
				typed("ab"),
				plain(tcell.KeyLeft),
				plain(tcell.KeyLeft),
				plain(tcell.KeyLeft),
			),
			want:    "ab",
			wantPos: 0,
		},
		{
			name: "ctrl-b ctrl-f move by rune",
			keys: append(
				typed("abc"),
				ctrl(tcell.KeyCtrlB),
				ctrl(tcell.KeyCtrlB),
				ctrl(tcell.KeyCtrlF),
			),
			want:    "abc",
			wantPos: 2,
		},
		{
			name:    "backspace before cursor",
			keys:    append(typed("abc"), plain(tcell.KeyLeft), plain(tcell.KeyBackspace2)),
			want:    "ac",
			wantPos: 1,
		},
		{
			name:    "backspace at start is a no-op",
			keys:    append(typed("ab"), ctrl(tcell.KeyCtrlA), plain(tcell.KeyBackspace2)),
			want:    "ab",
			wantPos: 0,
		},
		{
			name: "delete under cursor",
			keys: append(
				typed("abc"),
				plain(tcell.KeyLeft),
				plain(tcell.KeyLeft),
				plain(tcell.KeyDelete),
			),
			want:    "ac",
			wantPos: 1,
		},
		{
			name:    "ctrl-d at end is a no-op",
			keys:    append(typed("ab"), ctrl(tcell.KeyCtrlD)),
			want:    "ab",
			wantPos: 2,
		},
		{
			name:    "ctrl-u kills to start keeping the tail",
			keys:    append(typed("abcd"), plain(tcell.KeyLeft), ctrl(tcell.KeyCtrlU)),
			want:    "d",
			wantPos: 0,
		},
		{
			name:    "ctrl-w kills the word before the cursor",
			keys:    append(typed("alpha bravo  "), ctrl(tcell.KeyCtrlW)),
			want:    "alpha ",
			wantPos: 6,
		},
		{
			name: "ctrl-w mid line keeps the tail",
			keys: append(typed("alpha bravo"),
				plain(tcell.KeyLeft), plain(tcell.KeyLeft), ctrl(tcell.KeyCtrlW)),
			want:    "alpha vo",
			wantPos: 6,
		},
		{
			name:    "alt-b alt-f move by word",
			keys:    append(typed("alpha bravo"), altRune('b'), altRune('b'), altRune('f')),
			want:    "alpha bravo",
			wantPos: 5,
		},
		{
			name: "alt-arrows move by word",
			keys: append(
				typed("alpha bravo"),
				key{
					k:   tcell.KeyLeft,
					mod: tcell.ModAlt,
				},
				key{k: tcell.KeyRight, mod: tcell.ModAlt},
			),
			want:    "alpha bravo",
			wantPos: 11,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPicker(newScreen(t), testItems(), fallbackBand)
			for _, k := range tt.keys {
				if _, done, err := p.handleKey(tcell.NewEventKey(k.k, k.r, k.mod)); done ||
					err != nil {
					t.Fatalf("key %v closed the picker (err %v)", k, err)
				}
			}
			if got := string(p.query); got != tt.want {
				t.Errorf("query = %q, want %q", got, tt.want)
			}
			if p.pos != tt.wantPos {
				t.Errorf("pos = %d, want %d", p.pos, tt.wantPos)
			}
		})
	}
}

// TestPickInsertMidQueryRefilters drives an insertion at the start of the
// query through the event loop: the list narrows to what the whole query
// matches and the terminal cursor follows the edit cursor.
func TestPickInsertMidQueryRefilters(t *testing.T) {
	s, done := startPicker(t, testItems())
	// "ha" matches alpha and charlie; "pha" only alpha.
	typeQuery(s, "ha")
	waitRune(t, s, 2, countRow, '2')
	s.InjectKey(tcell.KeyCtrlA, 0, tcell.ModCtrl)
	s.InjectKey(tcell.KeyRune, 'p', tcell.ModNone)
	waitRune(t, s, 2, countRow, '1')
	if x, y, visible := s.GetCursor(); !visible || x != textColumn+1 || y != testHeight-1 {
		t.Errorf("terminal cursor at (%d,%d) visible=%v, want (%d,%d) visible",
			x, y, visible, textColumn+1, testHeight-1)
	}
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	got := waitOutcome(t, done)
	if got.err != nil || got.idx != 0 {
		t.Fatalf("pick = %d, %v; want 0 (alpha)", got.idx, got.err)
	}
}

func TestFindAll(t *testing.T) {
	plain := []string{
		"mad01/dotfiles @ /code/dotfiles",
		"mad01/rollroll  air-mustache-man  owner:mad01  system:rollroll @ /code/rollroll",
		"mad01/ralph  ralph-cli  owner:mad01  system:ralph @ /code/ralph",
		"mad01/brain  brain-cli  owner:mad01  system:brain @ /code/brain",
	}
	tests := []struct {
		name  string
		terms []string
		want  []int // matched item indexes, best first
	}{
		{name: "no terms keeps original order", terms: nil, want: []int{0, 1, 2, 3}},
		{name: "one term", terms: []string{"rollroll"}, want: []int{1}},
		{
			name:  "two terms both required",
			terms: []string{"system:roll", "mustache"},
			want:  []int{1},
		},
		{name: "terms in any order", terms: []string{"mustache", "system:roll"}, want: []int{1}},
		{name: "second term narrows", terms: []string{"cli", "brain"}, want: []int{3}},
		{name: "term nothing holds", terms: []string{"ralph", "zzz"}, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAll(tt.terms, plain)
			idxs := make([]int, len(got))
			for i, m := range got {
				idxs[i] = m.idx
				if len(m.pos) != len(tt.terms) {
					t.Errorf(
						"match %d carries %d ranges, want one per term (%d)",
						m.idx,
						len(m.pos),
						len(tt.terms),
					)
				}
			}
			if fmt.Sprint(idxs) != fmt.Sprint(tt.want) {
				t.Errorf("findAll(%q) = %v, want %v", tt.terms, idxs, tt.want)
			}
		})
	}
}

// TestFindAllSingleTermKeepsMatcherOrder pins that one term ranks exactly as
// go-fuzzyfinder would, so the multi-term path changes nothing for the
// common case.
func TestFindAllSingleTermKeepsMatcherOrder(t *testing.T) {
	plain := []string{"alpha", "bravo", "a-b-c", "abc", "cab"}
	got := findAll([]string{"ab"}, plain)
	want := matching.FindAll("ab", plain, matching.WithMode(matching.ModeSmart))
	if len(got) != len(want) {
		t.Fatalf("findAll returned %d matches, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].idx != want[i].Idx {
			t.Errorf("position %d holds item %d, want %d", i, got[i].idx, want[i].Idx)
		}
	}
}

// TestPickMultiWordQuery drives a space-separated query through the event
// loop: both words have to hit, in any order, and the merged highlight marks
// runes from each.
func TestPickMultiWordQuery(t *testing.T) {
	items := []Item{
		{{Text: "alpha one"}},
		{{Text: "bravo two"}},
		{{Text: "alpha two"}},
	}
	s, done := startPicker(t, items)
	typeQuery(s, "two alp")
	waitRune(t, s, 2, countRow, '1')
	// The one survivor sits on the bottom item row; "alp" and "two" are both
	// highlighted green, the untouched "ha " is not.
	for _, tt := range []struct {
		col  int
		r    rune
		want tcell.Color
	}{
		{col: textColumn, r: 'a', want: tcell.ColorGreen},
		{col: textColumn + 3, r: 'h', want: tcell.ColorDefault},
		{col: textColumn + 6, r: 't', want: tcell.ColorGreen},
	} {
		style := waitRune(t, s, tt.col, bottomItemRow, tt.r)
		if fg, _, _ := style.Decompose(); fg != tt.want {
			t.Errorf("%q foreground is %v, want %v", tt.r, fg, tt.want)
		}
	}
	s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	if o := waitOutcome(t, done); o.err != nil || o.idx != 2 {
		t.Fatalf("pick = %d, %v; want 2 (alpha two)", o.idx, o.err)
	}
}

// TestSelectedRowBand pins how the selected row is painted: the band runs
// the full width, dim text is lifted to the default foreground so it
// does not vanish into a band of its own color, colored segments keep their
// color, and other rows stay on the terminal's own background.
func TestSelectedRowBand(t *testing.T) {
	items := []Item{
		{{Text: "one", Color: Default}, {Text: " @ /p", Color: Dim}, {Text: "c", Color: Cyan}},
		{{Text: "two", Color: Default}, {Text: " @ /q", Color: Dim}},
	}
	// Columns of the '/' in " @ /p", the cyan 'c' after it, and a cell past the text.
	const dimCol, cyanCol, padCol = textColumn + 6, textColumn + 8, textColumn + 20
	band := tcell.NewRGBColor(84, 86, 101)
	t.Run("band", func(t *testing.T) {
		s := newScreen(t)
		p := newPicker(s, items, band)
		p.draw()
		assertCell(t, s, dimCol, bottomItemRow, '/', tcell.ColorDefault, band)
		assertCell(t, s, cyanCol, bottomItemRow, 'c', tcell.ColorAqua, band)
		assertCell(t, s, padCol, bottomItemRow, ' ', tcell.ColorDefault, band)
		assertCell(t, s, dimCol, bottomItemRow-1, '/', tcell.ColorGray, tcell.ColorDefault)
	})
	t.Run("no color is reverse video", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		s := newScreen(t)
		p := newPicker(s, items, band)
		p.draw()
		_, _, style, _ := s.GetContent(dimCol, bottomItemRow)
		if _, _, attr := style.Decompose(); attr&tcell.AttrReverse == 0 {
			t.Errorf("selected row attrs %v lack reverse", attr)
		}
		assertCell(t, s, dimCol, bottomItemRow, '/', tcell.ColorDefault, tcell.ColorDefault)
	})
}

func assertCell(
	t *testing.T,
	s tcell.SimulationScreen,
	x, y int,
	wantRune rune,
	wantFg, wantBg tcell.Color,
) {
	t.Helper()
	r, _, style, _ := s.GetContent(x, y)
	fg, bg, _ := style.Decompose()
	if r != wantRune || fg != wantFg || bg != wantBg {
		t.Errorf(
			"cell (%d,%d) = %q fg=%v bg=%v, want %q fg=%v bg=%v",
			x,
			y,
			r,
			fg,
			bg,
			wantRune,
			wantFg,
			wantBg,
		)
	}
}
