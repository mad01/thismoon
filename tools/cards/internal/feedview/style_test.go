package feedview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// newStyledModel returns a model sized width by height that keeps 100
// entries, set apart in style.
func newStyledModel(width, height int, style render.CardStyle) *Model {
	m := New(Config{
		Max:      100,
		Now:      func() time.Time { return arrived },
		Location: zone,
		Style:    style,
	})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func TestCardStyleRows(t *testing.T) {
	// Width 30: the text area is 21 cells, a box 28 cells after the indent.
	rule := "         " + strings.Repeat("─", 21)
	top, bottom := "  ╭"+strings.Repeat("─", 26)+"╮", "  ╰"+strings.Repeat("─", 26)+"╯"
	inside := func(s string) string { return "  │ " + s + strings.Repeat(" ", 24-len(s)) + " │" }
	cases := []struct {
		style render.CardStyle
		want  []string
	}{
		{render.CardNone, []string{"  10:15  one", "         one more", "  10:15  two"}},
		{render.CardSpace, []string{"  10:15  one", "         one more", "", "  10:15  two"}},
		{render.CardLine, []string{
			"  10:15  one", "         one more", rule, "  10:15  two", rule,
		}},
		{render.CardBox, []string{
			top, inside("10:15  one"), inside("       one more"), bottom,
			top, inside("10:15  two"), bottom,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.style.String(), func(t *testing.T) {
			m := newStyledModel(30, 10, tc.style)
			feed(m, card("a", "", "one", "one more"), card("b", "", "two"))
			want := append(slices.Clone(tc.want), make([]string, 8-len(tc.want))...)
			if got := body(m); !slices.Equal(got, want) {
				t.Errorf("body =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// lastDrawn returns the index of the last body row that is not blank.
func lastDrawn(rows []string) int {
	i := len(rows) - 1
	for i > 0 && rows[i] == "" {
		i--
	}
	return i
}

func TestLineStyleFollowsNewest(t *testing.T) {
	// A body of 4 rows holds two one-line entries and their rules.
	m := newStyledModel(40, 6, render.CardLine)
	rule := "         " + strings.Repeat("─", 31)
	for i, line := range events(0, 6) {
		feed(m, line)
		rows := body(m)
		last := lastDrawn(rows)
		want := fmt.Sprintf("  10:15  event-%02d", i)
		if rows[last] != rule || rows[last-1] != want {
			t.Fatalf("after %d entries body = %q, want %s and its rule last", i+1, rows, want)
		}
	}

	press(m, keyUp)
	feed(m, events(6, 2)...)
	want := []string{"  10:15  event-03", rule, "  10:15  event-04", rule}
	if rows := body(m); !slices.Equal(rows, want) {
		t.Errorf("pinned body = %q, want %q", rows, want)
	}
	press(m, keyEnd)
	if rows := body(m); rows[2] != "  10:15  event-07" || rows[3] != rule {
		t.Errorf("body after end = %q, want the newest entry and its rule at the bottom", rows)
	}
}

func TestBoxStyleFollowsNewest(t *testing.T) {
	// A body of 6 rows holds two boxed one-line entries.
	m := newStyledModel(40, 8, render.CardBox)
	boxed := func(rows []string, last int, event string) bool {
		return last >= 2 && strings.HasPrefix(rows[last-2], "  ╭") &&
			strings.Contains(rows[last-1], "│ 10:15  "+event) &&
			strings.HasPrefix(rows[last], "  ╰")
	}
	for i, line := range events(0, 6) {
		feed(m, line)
		rows := body(m)
		if event := fmt.Sprintf("event-%02d", i); !boxed(rows, lastDrawn(rows), event) {
			t.Fatalf("after %d entries body =\n%s\nwant %s boxed last", i+1,
				strings.Join(rows, "\n"), event)
		}
	}
	for i, row := range screen(m) {
		if w := ansi.StringWidth(row); w != 40 && i > 0 && i < 7 {
			t.Errorf("row %d = %q is %d cells wide, want the box as wide as the pane", i, row, w)
		}
	}

	press(m, keyUp)
	feed(m, events(6, 2)...)
	if rows := body(m); !boxed(rows, 2, "event-03") || !boxed(rows, 5, "event-04") {
		t.Errorf("pinned body =\n%s\nwant event-03 and event-04 kept", strings.Join(rows, "\n"))
	}
	press(m, keyEnd)
	if rows := body(m); !boxed(rows, 5, "event-07") {
		t.Errorf("body after end =\n%s\nwant event-07 boxed at the bottom",
			strings.Join(rows, "\n"))
	}
}

func TestEveryStyleFitsTheWidth(t *testing.T) {
	long := "acme/api#12 fix the login redirect loop for alice 日本語"
	for _, style := range []render.CardStyle{
		render.CardNone, render.CardSpace, render.CardLine, render.CardBox,
	} {
		for _, width := range []int{1, 4, 9, 12, 30} {
			m := newStyledModel(width, 8, style)
			feed(m, card("a", "", long, long), card("b", "", "x"))
			for i, row := range screen(m) {
				if w := ansi.StringWidth(row); w > width {
					t.Errorf("%v width %d: row %d = %q is %d cells wide", style, width, i, row, w)
				}
			}
		}
	}
}

func TestShortBodyShowsNewestText(t *testing.T) {
	cases := []struct {
		style  render.CardStyle
		height int
		lines  []string
		// want is each body row's text, the time column and box sides
		// trimmed.
		want []string
	}{
		{render.CardNone, 3, []string{"newest"}, []string{"newest"}},
		{render.CardSpace, 3, []string{"newest"}, []string{"newest"}},
		{render.CardLine, 3, []string{"newest"}, []string{"newest"}},
		{render.CardBox, 3, []string{"newest"}, []string{"newest"}},
		// A tall entry shows its last rows of text, the same in every style.
		{render.CardNone, 3, []string{"one", "two", "three"}, []string{"three"}},
		{render.CardLine, 3, []string{"one", "two", "three"}, []string{"three"}},
		{render.CardBox, 3, []string{"one", "two", "three"}, []string{"three"}},
		{render.CardLine, 4, []string{"one", "two"}, []string{"one", "two"}},
		{render.CardBox, 5, []string{"one", "two", "three"}, []string{"one", "two", "three"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%v height %d %d lines", tc.style, tc.height, len(tc.lines)),
			func(t *testing.T) {
				m := newStyledModel(30, tc.height, tc.style)
				feed(m, events(0, 3)...)
				feed(m, card("newest", "", tc.lines...))
				var got []string
				for _, row := range body(m) {
					row = strings.Trim(row, " │")
					row = strings.TrimPrefix(row, "10:15")
					got = append(got, strings.TrimSpace(row))
				}
				if !slices.Equal(got, tc.want) {
					t.Errorf("body = %q, want %q", body(m), tc.want)
				}
			})
	}
}
