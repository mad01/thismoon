package listview

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// SGR codes the gutter bars and the box border must use, and the hues
// they must not.
const (
	sgrDim   = "\x1b[2m"
	sgrGreen = "\x1b[32m"
	sgrCyan  = "\x1b[36m"
)

var allStyles = []render.CardStyle{
	render.CardNone, render.CardSpace, render.CardLine, render.CardBox,
}

// splitGutter splits a stripped one-column row into its two gutter columns
// and the rest.
func splitGutter(row string) (gutter, text string) {
	runes := []rune(row)
	if len(runes) < gutterWidth {
		return row, ""
	}
	return string(runes[:gutterWidth]), string(runes[gutterWidth:])
}

func TestCardStyleRows(t *testing.T) {
	// Width 30 is one column: a card is 28 cells after the gutter.
	rule := "  " + strings.Repeat("─", 28)
	top, bottom := "╭"+strings.Repeat("─", 26)+"╮", "╰"+strings.Repeat("─", 26)+"╯"
	inside := func(s string) string { return "│ " + s + strings.Repeat(" ", 24-len(s)) + " │" }
	cases := []struct {
		style render.CardStyle
		want  []string
	}{
		{render.CardNone, []string{"▌ card-a", "▌ alice", "  card-b", "  bob"}},
		{render.CardSpace, []string{"▌ card-a", "▌ alice", "", "  card-b", "  bob"}},
		{render.CardLine, []string{"▌ card-a", "▌ alice", rule, "  card-b", "  bob", rule}},
		{render.CardBox, []string{
			"▌ " + top, "▌ " + inside("card-a"), "▌ " + inside("alice"), "▌ " + bottom,
			"  " + top, "  " + inside("card-b"), "  " + inside("bob"), "  " + bottom,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.style.String(), func(t *testing.T) {
			m, _ := newStyledModel(30, 12, tc.style)
			feed(m, cardLine("a", "card-a", "alice"), cardLine("b", "card-b", "bob"))
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

func TestBoxFitsItsColumn(t *testing.T) {
	long := strings.Repeat("acme/api#12 日本語 alice ", 8)
	cases := []struct {
		width, cols int
	}{
		{width: 80, cols: 1},
		{width: 130, cols: 2},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.width), func(t *testing.T) {
			m, _ := newStyledModel(tc.width, 20, render.CardBox)
			feed(m, cardLine("a", long, "alice"), cardLine("b", "card-b"), cardLine("c", long))
			rows := screen(m)
			for i, row := range rows {
				if w := ansi.StringWidth(row); w > tc.width {
					t.Errorf("row %d is %d cells wide, want at most %d: %q", i, w, tc.width, row)
				}
			}
			// The first grid row: a box per column, each ending on its column's
			// last cell, its text cut inside the border.
			cell := (tc.width - columnGap*(tc.cols-1)) / tc.cols
			for _, corners := range []struct {
				row         int
				left, right string
			}{{1, "╭", "╮"}, {4, "╰", "╯"}} {
				row := rows[corners.row]
				if n := strings.Count(row, corners.left); n != tc.cols {
					t.Errorf("row %d = %q has %d %s corners, want %d",
						corners.row, row, n, corners.left, tc.cols)
				}
				end := strings.Index(row, corners.right) + len(corners.right)
				if w := ansi.StringWidth(row[:end]); w != cell {
					t.Errorf("row %d: first box ends at cell %d, want the column width %d",
						corners.row, w, cell)
				}
			}
			if row := rows[2]; !strings.Contains(row, "…") {
				t.Errorf("first text row = %q, want the long line cut with …", row)
			}
		})
	}
}

func TestBarsShowFocusAndMarksInEveryStyle(t *testing.T) {
	// card-a is focused and marked, card-b neither, card-c only marked. The
	// bars run down each card's own rows: its lines, and its box when it
	// has one.
	focusAndMark := sgrDim + bar + "\x1b[m" + sgrGreen + bar
	cases := []struct {
		style               render.CardStyle
		focusRows, markRows int
	}{
		{style: render.CardNone, focusRows: 2, markRows: 1},
		{style: render.CardSpace, focusRows: 2, markRows: 1},
		{style: render.CardLine, focusRows: 2, markRows: 1},
		{style: render.CardBox, focusRows: 4, markRows: 3},
	}
	for _, tc := range cases {
		t.Run(tc.style.String(), func(t *testing.T) {
			m, _ := newStyledModel(40, 16, tc.style)
			feed(m, cardLine("a", "card-a", "alice"), cardLine("b", "card-b", "bob"),
				cardLine("c", "card-c"))
			press(m, keyDown, keyDown, keySpace, keyUp, keyUp, keySpace, keyLeft)

			raw := strings.Split(m.View().Content, "\n")
			for i := 1; i <= tc.focusRows; i++ {
				if !strings.HasPrefix(raw[i], focusAndMark) {
					t.Errorf("row %d = %q, want a dim focus bar, then a green mark bar", i, raw[i])
				}
			}
			if !strings.Contains(raw[1]+raw[2], "\x1b[1mcard-a") {
				t.Errorf("rows = %q, want the cursor card's first line bold", raw[1:3])
			}
			var focus, markOnly int
			for _, r := range screen(m) {
				switch g, _ := splitGutter(r); g {
				case bar + bar:
					focus++
				case " " + bar:
					markOnly++
				}
			}
			if focus != tc.focusRows || markOnly != tc.markRows {
				t.Errorf("%d rows with both bars and %d with the mark bar only, want %d and %d",
					focus, markOnly, tc.focusRows, tc.markRows)
			}
			body := strings.Join(raw[1:len(raw)-1], "\n")
			if strings.Contains(body, sgrCyan) {
				t.Errorf("body draws cyan, want focus shown by the dim bar only:\n%q", body)
			}
			if tc.style != render.CardBox {
				return
			}
			for _, edge := range []string{"╭", "│", "╰"} {
				if strings.Contains(body, sgrGreen+edge) || strings.Count(body, sgrDim+edge) < 3 {
					t.Errorf("box edge %s is not dim on every card:\n%q", edge, body)
				}
			}
		})
	}
}

func TestBoxScrollKeepsCursorCardWhole(t *testing.T) {
	// Height 11 leaves 9 body rows: two boxed two-line cards of 4 rows each.
	m, _ := newStyledModel(40, 11, render.CardBox)
	var lines []string
	for i := range 10 {
		lines = append(lines, cardLine(fmt.Sprint(i), fmt.Sprintf("card-%d", i), "alice"))
	}
	feed(m, lines...)
	check := func(step string, want string) {
		t.Helper()
		if got := cursorCard(t, m); got != want {
			t.Fatalf("%s: cursor on %s, want %s", step, got, want)
		}
		var focused []string
		for _, r := range screen(m) {
			if g, text := splitGutter(r); strings.HasPrefix(g, bar) {
				focused = append(focused, text)
			}
		}
		if len(focused) != 4 || !strings.HasPrefix(focused[0], "╭") ||
			!strings.HasPrefix(focused[3], "╰") {
			t.Fatalf("%s: focused rows = %q, want the whole box of %s", step, focused, want)
		}
	}
	for i := 1; i < 10; i++ {
		press(m, keyDown)
		check(fmt.Sprintf("after %d downs", i), fmt.Sprintf("card-%d", i))
	}
	press(m, keyPgUp)
	check("after pgup", "card-7")
	for i := 6; i >= 0; i-- {
		press(m, keyUp)
		check(fmt.Sprintf("up to card-%d", i), fmt.Sprintf("card-%d", i))
	}
}

func TestEveryStyleFitsThePane(t *testing.T) {
	long := strings.Repeat("acme/api#12 日本語 alice ", 20)
	for _, style := range allStyles {
		for _, width := range []int{1, 2, 3, 5, 10, 59, 61, 125, 130, 200} {
			m, _ := newStyledModel(width, 12, style)
			feed(m, cardLine("a", long, long, long), cardLine("b", long), cardLine("c", "x"),
				cardLine("d", long, "y"))
			press(m, keySpace)
			rows := screen(m)
			if len(rows) != 12 {
				t.Errorf("%v width %d: drew %d rows, want 12", style, width, len(rows))
			}
			for i, row := range rows {
				if w := ansi.StringWidth(row); w > width {
					t.Errorf("%v width %d: row %d is %d cells wide: %q", style, width, i, w, row)
				}
			}
		}
	}
}
