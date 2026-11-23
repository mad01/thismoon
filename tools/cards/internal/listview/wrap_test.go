package listview

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/render"
)

const (
	// wide is a last line too wide for a card 28 cells wide, or 24 inside a
	// box: it breaks after "the" in both.
	wide = "alice wants to merge the change into main"
	// wider is a last line too wide for a card in two columns, 62 cells, or 58
	// inside a box: it breaks before "deployment" in both.
	wider = "alice wants to merge the change into main before the deployment"
)

// newWrapModel returns a model sized width by height with a fake starter, in
// style, with Config.WrapLast set to wrap.
func newWrapModel(width, height int, style render.CardStyle, wrap bool) *Model {
	m := New(Config{Starter: &fakeStarter{}, Style: style, WrapLast: wrap})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// barRows returns the rows a card with lines rows of text has beside its
// gutter bars in style: its lines, and a box's border.
func barRows(style render.CardStyle, lines int) int {
	if style == render.CardBox {
		return lines + 2
	}
	return lines
}

// focusedRows returns the text of each screen row with the focus bar.
func focusedRows(m *Model) []string {
	var out []string
	for _, r := range screen(m) {
		if g, text := splitGutter(r); strings.HasPrefix(g, bar) {
			out = append(out, text)
		}
	}
	return out
}

// cellOf returns the cell where s starts on a stripped row, or -1.
func cellOf(row, s string) int {
	i := strings.Index(row, s)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(row[:i])
}

func TestWrapLastDrawsTwoRows(t *testing.T) {
	// Width 30 is one column: a card is 28 cells after the gutter.
	rule := "  " + strings.Repeat("─", 28)
	top, bottom := "╭"+strings.Repeat("─", 26)+"╮", "╰"+strings.Repeat("─", 26)+"╯"
	inside := func(s string) string { return "│ " + s + strings.Repeat(" ", 24-len(s)) + " │" }
	cases := []struct {
		style render.CardStyle
		want  []string
	}{
		{render.CardNone, []string{
			"▌ card-a", "▌ alice wants to merge the", "▌ change into main", "  card-b", "  bob",
		}},
		{render.CardSpace, []string{
			"▌ card-a", "▌ alice wants to merge the", "▌ change into main", "", "  card-b", "  bob",
		}},
		{render.CardLine, []string{
			"▌ card-a", "▌ alice wants to merge the", "▌ change into main", rule,
			"  card-b", "  bob", rule,
		}},
		{render.CardBox, []string{
			"▌ " + top, "▌ " + inside("card-a"), "▌ " + inside("alice wants to merge the"),
			"▌ " + inside("change into main"), "▌ " + bottom,
			"  " + top, "  " + inside("card-b"), "  " + inside("bob"), "  " + bottom,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.style.String(), func(t *testing.T) {
			m := newWrapModel(30, 14, tc.style, true)
			feed(m, cardLine("a", "card-a", wide), cardLine("b", "card-b", "bob"))
			rows := screen(m)
			if got := rows[1 : 1+len(tc.want)]; !slices.Equal(got, tc.want) {
				t.Errorf("rows =\n%s\nwant\n%s",
					strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
			if rest := strings.Join(rows[1+len(tc.want):len(rows)-1], ""); rest != "" {
				t.Errorf("rows after the second card = %q, want blank", rest)
			}
		})
	}
}

func TestWrapLastBoldsBothRowsOfTheCursorLine(t *testing.T) {
	m := newWrapModel(30, 10, render.CardSpace, true)
	feed(m, cardLine("a", wide))
	raw := strings.Split(m.View().Content, "\n")
	for _, i := range []int{1, 2} {
		if !strings.Contains(raw[i], "\x1b[1m") {
			t.Errorf("row %d = %q, want the cursor card's one line bold on both rows", i, raw[i])
		}
	}
}

func TestWrapLastOffCutsTheLine(t *testing.T) {
	m := newWrapModel(30, 10, render.CardSpace, false)
	feed(m, cardLine("a", "card-a", wide), cardLine("b", "card-b", "bob"))
	want := []string{"▌ card-a", "▌ alice wants to merge the ch…", "", "  card-b", "  bob"}
	if got := screen(m)[1 : 1+len(want)]; !slices.Equal(got, want) {
		t.Errorf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestWrapLastLeavesFittingCardsAlone(t *testing.T) {
	cards := []string{
		cardLine("a", "card-a", "alice"), cardLine("b", "card-b", "bob wants a review"),
		cardLine("c", "x"), cardLine("d", "card-d", "dave"),
	}
	for _, style := range allStyles {
		for _, width := range []int{30, 61, 130, 200} {
			on := newWrapModel(width, 12, style, true)
			off := newWrapModel(width, 12, style, false)
			for _, m := range []*Model{on, off} {
				feed(m, cards...)
				press(m, keyRight, keySpace)
			}
			if got, want := on.View().Content, off.View().Content; got != want {
				t.Errorf("%v width %d: with --wrap-last\n%s\nwant as without\n%s",
					style, width, ansi.Strip(got), ansi.Strip(want))
			}
		}
	}
}

func TestWrapLastGridRowLinesUp(t *testing.T) {
	// Width 130 is two columns of 64 cells. card-a wraps and card-b does
	// not, so the first grid row is three rows of text tall in both columns.
	const second = 64 + columnGap + gutterWidth
	for _, style := range allStyles {
		t.Run(style.String(), func(t *testing.T) {
			m := newWrapModel(130, 20, style, true)
			feed(m, cardLine("a", "card-a", wider), cardLine("b", "card-b", "bob"),
				cardLine("c", "card-c", "carol"), cardLine("d", "card-d", "dave"))
			press(m, keySpace, keyLeft)
			rows := screen(m)
			inset := 0
			if style == render.CardBox {
				inset = 2
			}
			firstRow := 1 + style.TextStart()
			nextRow := 1 + style.Rows(3) + style.Gap() + style.TextStart()
			for _, c := range []struct {
				row       int
				text      string
				wantCell  int
				wantWhere string
			}{
				{firstRow, "card-a", gutterWidth + inset, "first column"},
				{firstRow, "card-b", second + inset, "second column"},
				{firstRow + 2, "deployment", gutterWidth + inset, "card-a's third row"},
				{nextRow, "card-c", gutterWidth + inset, "first column"},
				{nextRow, "card-d", second + inset, "second column"},
			} {
				if got := cellOf(rows[c.row], c.text); got != c.wantCell {
					t.Errorf("row %d = %q: %s at cell %d, want %d (%s)",
						c.row, rows[c.row], c.text, got, c.wantCell, c.wantWhere)
				}
			}
			if style == render.CardBox {
				if row := rows[style.Rows(3)]; strings.Count(row, "╰") != 2 {
					t.Errorf("row %d = %q, want both boxes to end on it", style.Rows(3), row)
				}
			}
			// card-a is focused and marked: both bars run down its own rows.
			if got, want := len(focusedRows(m)), barRows(style, 3); got != want {
				t.Errorf("%d rows with the focus bar, want %d", got, want)
			}

			// A filter that hides card-b and card-d puts card-c beside card-a.
			typeText(m, "l")
			rows = screen(m)
			if got := cellOf(rows[firstRow], "card-c"); got != second+inset {
				t.Errorf("filtered row %d = %q: card-c at cell %d, want %d",
					firstRow, rows[firstRow], got, second+inset)
			}
			if body := strings.Join(rows, "\n"); strings.Contains(body, "card-b") ||
				strings.Contains(body, "card-d") {
				t.Errorf("filter l shows a card it hides:\n%s", body)
			}
		})
	}
}

func TestWrapLastCursorAndPaging(t *testing.T) {
	// Height 10 leaves 8 body rows. Even cards wrap to three lines of text,
	// odd ones keep two.
	var cards []string
	for i := range 6 {
		last := "short"
		if i%2 == 0 {
			last = wider
		}
		cards = append(cards, cardLine(fmt.Sprint(i), fmt.Sprintf("card-%d", i), last))
	}
	for _, style := range allStyles {
		t.Run(style.String(), func(t *testing.T) {
			m := newWrapModel(40, 10, style, true)
			feed(m, cards...)
			check := func(step string, want int) {
				t.Helper()
				if got := cursorCard(t, m); got != fmt.Sprintf("card-%d", want) {
					t.Fatalf("%s: cursor on %s, want card-%d", step, got, want)
				}
				lines := 2
				if want%2 == 0 {
					lines = 3
				}
				if got := len(focusedRows(m)); got != barRows(style, lines) {
					t.Fatalf("%s: %d focused rows, want card-%d whole: %d\n%s", step, got, want,
						barRows(style, lines), strings.Join(screen(m), "\n"))
				}
			}
			check("start", 0)
			for i := 1; i < 6; i++ {
				press(m, keyDown)
				check(fmt.Sprintf("after %d downs", i), i)
			}
			for i := 4; i >= 0; i-- {
				press(m, keyUp)
				check(fmt.Sprintf("up to card-%d", i), i)
			}
			press(m, keyPgDown)
			check("after pgdown", pageAfter(style))
			press(m, keyPgUp)
			check("after pgup", 0)
		})
	}
}

// pageAfter returns the card Page Down moves to from card-0 in
// TestWrapLastCursorAndPaging: past every whole card on the first screen of
// 8 rows. A box takes 5 rows for card-0 and 4 for card-1, so only card-0
// fits; unwrapped, both would.
func pageAfter(style render.CardStyle) int {
	n, rows := 0, 0
	for i := 0; ; i++ {
		lines := 2
		if i%2 == 0 {
			lines = 3
		}
		if i > 0 {
			rows += style.Gap()
		}
		if rows += style.Rows(lines); rows > 8 {
			return max(n, 1)
		}
		n++
	}
}

func TestWrapLastFollowsResize(t *testing.T) {
	// The last line wraps at width 40 (38 cells of text) and fits at 60.
	// Height 7 leaves 5 body rows: with the cursor on card-b, card-a shows
	// above it only while card-a fits on two rows.
	const line = "alice wants to merge the change into main today"
	m := newWrapModel(40, 7, render.CardSpace, true)
	feed(m, cardLine("a", "card-a", line), cardLine("b", "card-b", "bob"))
	press(m, keyDown)
	wrapped := []string{"▌ card-b", "▌ bob", "", "", ""}
	fits := []string{"  card-a", "  " + line, "", "▌ card-b", "▌ bob"}
	for _, step := range []struct {
		width, height int
		want          []string
	}{
		{40, 7, wrapped},
		{60, 7, fits},
		{40, 7, wrapped},
		{40, 9, []string{
			"  card-a", "  alice wants to merge the change into", "  main today", "",
			"▌ card-b", "▌ bob", "",
		}},
	} {
		m.Update(tea.WindowSizeMsg{Width: step.width, Height: step.height})
		rows := screen(m)
		if got := trimRows(rows[1 : len(rows)-1]); !slices.Equal(got, step.want) {
			t.Errorf("%dx%d: body =\n%s\nwant\n%s", step.width, step.height,
				strings.Join(got, "\n"), strings.Join(step.want, "\n"))
		}
	}
}

// trimRows returns rows with trailing spaces trimmed.
func trimRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = strings.TrimRight(r, " ")
	}
	return out
}
