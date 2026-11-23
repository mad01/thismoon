package feedview

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// wide is a last line too wide for an entry in a pane 40 cells wide: 31
// cells of text, or 27 inside a box.
const wide = "alice wants to merge the change into main"

// newWrapModel returns a model sized width by height that keeps 100
// entries, set apart in style, newest at newest, with Config.WrapLast set to
// wrap.
func newWrapModel(width, height int, style render.CardStyle, newest order.Newest,
	wrap bool,
) *Model {
	m := New(Config{
		Max:      100,
		Now:      func() time.Time { return arrived },
		Location: zone,
		Style:    style,
		Newest:   newest,
		WrapLast: wrap,
	})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func assertBody(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := body(m)[:len(want)]; !slices.Equal(got, want) {
		t.Errorf("body =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestWrapLastDrawsTwoRows(t *testing.T) {
	// Width 40: the text area is 31 cells, 27 inside a box.
	rule := "         " + strings.Repeat("─", 31)
	top, bottom := "  ╭"+strings.Repeat("─", 36)+"╮", "  ╰"+strings.Repeat("─", 36)+"╯"
	inside := func(s string) string { return "  │ " + s + strings.Repeat(" ", 34-len(s)) + " │" }
	cases := []struct {
		style render.CardStyle
		want  []string
	}{
		{render.CardNone, []string{
			"  10:15  one", "         alice wants to merge the change", "         into main",
			"  10:15  two",
		}},
		{render.CardSpace, []string{
			"  10:15  one", "         alice wants to merge the change", "         into main", "",
			"  10:15  two",
		}},
		{render.CardLine, []string{
			"  10:15  one", "         alice wants to merge the change", "         into main", rule,
			"  10:15  two", rule,
		}},
		{render.CardBox, []string{
			top, inside("10:15  one"), inside("       alice wants to merge the"),
			inside("       change into main"), bottom,
			top, inside("10:15  two"), bottom,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.style.String(), func(t *testing.T) {
			m := newWrapModel(40, 12, tc.style, order.Bottom, true)
			feed(m, card("a", "", "one", wide), card("b", "", "two"))
			want := append(slices.Clone(tc.want), make([]string, 10-len(tc.want))...)
			assertBody(t, m, want...)
		})
	}
}

func TestWrapLastOffCutsTheLine(t *testing.T) {
	m := newWrapModel(40, 6, render.CardNone, order.Bottom, false)
	feed(m, card("a", "", "one", wide), card("b", "", "two"))
	assertBody(t, m, "  10:15  one", "         alice wants to merge the chang…", "  10:15  two", "")
}

func TestWrapLastLeavesFittingEntriesAlone(t *testing.T) {
	entries := []string{card("a", "", "one", "bob wants review"), card("b", "", "two")}
	for _, style := range []render.CardStyle{
		render.CardNone, render.CardSpace, render.CardLine, render.CardBox,
	} {
		for _, width := range []int{30, 40, 80, 120} {
			on := newWrapModel(width, 8, style, order.Bottom, true)
			off := newWrapModel(width, 8, style, order.Bottom, false)
			feed(on, entries...)
			feed(off, entries...)
			if got, want := on.View().Content, off.View().Content; got != want {
				t.Errorf("%v width %d: with --wrap-last\n%s\nwant as without\n%s",
					style, width, ansi.Strip(got), ansi.Strip(want))
			}
		}
	}
}

func TestWrapLastFollowShowsTheWrappedRow(t *testing.T) {
	// A body of 3 rows. The newest entry, at the bottom, takes 3 rows of
	// text and its rule: following shows its text, not the row above it.
	m := newWrapModel(40, 5, render.CardLine, order.Bottom, true)
	feed(m, card("e0", "", "event-00"), card("b", "", "two", wide))
	newest := []string{
		"  10:15  two",
		"         alice wants to merge the change",
		"         into main",
	}
	assertBody(t, m, newest...)

	press(m, keyUp)
	assertBody(t, m, "  10:15  event-00", "         "+strings.Repeat("─", 31), "  10:15  two")
	press(m, keyDown)
	assertBody(t, m, newest...)
	press(m, keyEnd)
	assertBody(t, m, newest...)
}

func TestWrapLastTopShowsTheWrappedText(t *testing.T) {
	// A body of 4 rows. The newest entry, on top in a box, takes 3 rows of
	// text and its border, so it does not fit: its text goes on the first
	// row, not the top of its box.
	m := newWrapModel(40, 6, render.CardBox, order.Top, true)
	feed(m, card("a", "", "old"), card("b", "", "two", wide))
	inside := func(s string) string { return "  │ " + s + strings.Repeat(" ", 34-len(s)) + " │" }
	assertBody(t, m, inside("10:15  two"), inside("       alice wants to merge the"),
		inside("       change into main"), "  ╰"+strings.Repeat("─", 36)+"╯")
}

func TestWrapLastFollowsResize(t *testing.T) {
	// wide wraps at width 40 and fits at 60, where the text area is 51 cells.
	m := newWrapModel(40, 10, render.CardNone, order.Bottom, true)
	feed(m, card("e0", "", "event-00"), card("a", "", "one", wide), card("b", "", "two"))
	wrapped := []string{"event-00", "one", "alice wants to merge the change", "into main", "two"}
	for _, step := range []struct {
		width int
		want  []string
	}{
		{40, wrapped},
		{60, []string{"event-00", "one", wide, "two"}},
		{40, wrapped},
	} {
		m.Update(tea.WindowSizeMsg{Width: step.width, Height: 10})
		if got := texts(m); !slices.Equal(got, step.want) {
			t.Errorf("width %d: texts = %q, want %q", step.width, got, step.want)
		}
	}

	// Pinned to the second entry in a body of 3 rows, a resize keeps that
	// entry on top whether it wraps or not.
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})
	press(m, keyHome, keyDown)
	for _, step := range []struct {
		width int
		want  []string
	}{
		{40, []string{"one", "alice wants to merge the change", "into main"}},
		{60, []string{"one", wide, "two"}},
		{40, []string{"one", "alice wants to merge the change", "into main"}},
	} {
		m.Update(tea.WindowSizeMsg{Width: step.width, Height: 5})
		if got := texts(m); !slices.Equal(got, step.want) {
			t.Errorf("pinned, width %d: texts = %q, want %q", step.width, got, step.want)
		}
	}
}
