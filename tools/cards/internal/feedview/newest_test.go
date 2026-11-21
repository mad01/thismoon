package feedview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// newTopModel returns a model sized width by height that keeps max entries,
// newest on top, set apart in style.
func newTopModel(width, height, max int, style render.CardStyle) *Model {
	m := New(Config{
		Max:      max,
		Now:      func() time.Time { return arrived },
		Location: zone,
		Style:    style,
		Newest:   order.Top,
	})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func assertFooter(t *testing.T, m *Model, want string) {
	t.Helper()
	if f := footer(m); !strings.Contains(f, want) {
		t.Errorf("footer = %q, want %q in it", f, want)
	}
}

func TestTopFollowKeepsNewestOnTop(t *testing.T) {
	m := newTopModel(40, 6, 100, render.CardNone) // a body of 4 rows
	for i, line := range events(0, 10) {
		feed(m, line)
		if rows, want := texts(m), fmt.Sprintf("event-%02d", i); rows[0] != want {
			t.Fatalf("after %d entries the first row = %q, want the newest, %s", i+1, rows[0], want)
		}
	}
	assertTexts(t, m, "event-09", "event-08", "event-07", "event-06")
	assertFooter(t, m, "10/10  following")
}

func TestTopDownPinsWhileEntriesArriveAndHomeFollows(t *testing.T) {
	m := newTopModel(40, 6, 100, render.CardNone)
	feed(m, events(0, 10)...)

	press(m, keyDown)
	assertTexts(t, m, "event-08", "event-07", "event-06", "event-05")
	assertFooter(t, m, "scrolled, Home follows")

	feed(m, events(10, 3)...)
	assertTexts(t, m, "event-08", "event-07", "event-06", "event-05")

	press(m, keyHome)
	assertTexts(t, m, "event-12", "event-11", "event-10", "event-09")
	assertFooter(t, m, "13/13  following")
}

func TestTopUpStopsFollowingAtTheTop(t *testing.T) {
	m := newTopModel(40, 6, 100, render.CardNone)
	feed(m, events(0, 10)...)

	press(m, keyUp)
	assertTexts(t, m, "event-09", "event-08", "event-07", "event-06")
	assertFooter(t, m, "scrolled, Home follows")
	feed(m, events(10, 1)...)
	assertTexts(t, m, "event-09", "event-08", "event-07", "event-06")
}

func TestTopScrollKeys(t *testing.T) {
	m := newTopModel(40, 6, 100, render.CardNone)
	feed(m, events(0, 10)...)

	press(m, keyEnd)
	assertTexts(t, m, "event-03", "event-02", "event-01", "event-00")
	press(m, keyUp)
	assertTexts(t, m, "event-04", "event-03", "event-02", "event-01")
	press(m, keyPgUp)
	assertTexts(t, m, "event-08", "event-07", "event-06", "event-05")
	press(m, keyPgUp, keyUp)
	assertTexts(t, m, "event-09", "event-08", "event-07", "event-06")
	press(m, keyPgDown)
	assertTexts(t, m, "event-05", "event-04", "event-03", "event-02")
	press(m, keyDown, keyDown, keyDown)
	assertTexts(t, m, "event-03", "event-02", "event-01", "event-00")
	assertFooter(t, m, "scrolled, Home follows")
}

func TestTopMaxDropsTheOldestFromTheBottom(t *testing.T) {
	m := newTopModel(40, 6, 6, render.CardNone)
	feed(m, events(0, 6)...)
	press(m, keyDown)
	assertTexts(t, m, "event-04", "event-03", "event-02", "event-01")

	feed(m, events(6, 1)...) // the limit drops event-00, below the screen
	assertTexts(t, m, "event-04", "event-03", "event-02", "event-01")
	assertFooter(t, m, "6/6")

	feed(m, events(7, 3)...)
	press(m, keyEnd)
	assertTexts(t, m, "event-07", "event-06", "event-05", "event-04")
	press(m, keyHome)
	assertTexts(t, m, "event-09", "event-08", "event-07", "event-06")
	assertFooter(t, m, "6/6  following")
}

func TestTopRepeatedIDKeepsItsPlace(t *testing.T) {
	m := newTopModel(60, 8, 100, render.CardNone)
	feed(m,
		card("a", "", "acme/api#12 opened"),
		card("b", "", "acme/api#13 opened"),
		card("a", "", "acme/api#12 merged"),
	)

	assertTexts(t, m, "acme/api#13 opened", "acme/api#12 merged")
	assertFooter(t, m, "2/2")
}

func TestTopFilter(t *testing.T) {
	m := newTopModel(60, 8, 100, render.CardNone)
	feed(m,
		card("a", "", "login failed for alice"),
		card("b", "", "deploy finished"),
		card("c", "", "login ok for alice"),
	)

	typeText(m, "lgn")
	assertTexts(t, m, "login ok for alice", "login failed for alice")
	press(m, keyEsc)
	assertTexts(t, m, "login ok for alice", "deploy finished", "login failed for alice")
}

func TestTopEveryStylePinsAndFollows(t *testing.T) {
	for _, style := range []render.CardStyle{
		render.CardNone, render.CardSpace, render.CardLine, render.CardBox,
	} {
		t.Run(style.String(), func(t *testing.T) {
			// Twenty entries fill more than the body of 10 rows in every style.
			m := newTopModel(40, 12, 100, style)
			feed(m, events(0, 20)...)
			if row := firstText(body(m)); !strings.Contains(row, "event-19") {
				t.Fatalf("first text row = %q, want the newest, event-19\n%s", row,
					strings.Join(body(m), "\n"))
			}

			press(m, keyDown)
			pinned := body(m)
			if row := firstText(pinned); !strings.Contains(row, "event-18") {
				t.Errorf("first text row after down = %q, want event-18", row)
			}
			feed(m, events(20, 2)...)
			if rows := body(m); !slices.Equal(rows, pinned) {
				t.Errorf("pinned body =\n%s\nwant it kept\n%s", strings.Join(rows, "\n"),
					strings.Join(pinned, "\n"))
			}

			press(m, keyHome)
			if row := firstText(body(m)); !strings.Contains(row, "event-21") {
				t.Errorf("first text row after home = %q, want the newest, event-21", row)
			}
		})
	}
}

// firstText returns the first body row that holds an entry's text, not a
// box edge or a rule.
func firstText(rows []string) string {
	for _, r := range rows {
		if strings.Contains(r, "event-") {
			return r
		}
	}
	return ""
}

func TestTopShortBodyShowsNewestText(t *testing.T) {
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
		// A tall entry shows its first rows of text, the same in every style.
		{render.CardNone, 3, []string{"one", "two", "three"}, []string{"one"}},
		{render.CardSpace, 3, []string{"one", "two", "three"}, []string{"one"}},
		{render.CardLine, 3, []string{"one", "two", "three"}, []string{"one"}},
		{render.CardBox, 3, []string{"one", "two", "three"}, []string{"one"}},
		{render.CardLine, 4, []string{"one", "two"}, []string{"one", "two"}},
		{render.CardBox, 5, []string{"one", "two", "three"}, []string{"one", "two", "three"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%v height %d %d lines", tc.style, tc.height, len(tc.lines)),
			func(t *testing.T) {
				m := newTopModel(30, tc.height, 100, tc.style)
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

func TestTopPinFallingBackToTheTopEntryStaysPut(t *testing.T) {
	// A body of 4 rows. Boxed, y is 5 rows tall, so while it is on top the
	// lowest top skips its top border.
	m := newTopModel(40, 6, 100, render.CardBox)
	feed(m, card("y", "", "y-1", "y-2", "y-3"), card("x", "", "x-1"))
	press(m, keyUp) // pins x, on top
	feed(m, `{"type":"remove","id":"x"}`)
	pinned := body(m)
	if !strings.Contains(pinned[0], "y-1") {
		t.Fatalf("body after the remove =\n%s\nwant y's first text row on top",
			strings.Join(pinned, "\n"))
	}

	feed(m, card("z", "", "z-1")) // short, so the lowest top is row 0 again
	if rows := body(m); !slices.Equal(rows, pinned) {
		t.Errorf("body after a short entry on top =\n%s\nwant it kept\n%s",
			strings.Join(rows, "\n"), strings.Join(pinned, "\n"))
	}
	assertFooter(t, m, "scrolled, Home follows")
}

func TestTopPinHiddenByTheFilterComesBack(t *testing.T) {
	m := newTopModel(40, 6, 100, render.CardBox)
	feed(m, card("y", "", "y-1", "y-2", "y-3"), card("x", "", "x-1"))
	press(m, keyUp) // pins x, on top
	pinned := body(m)

	typeText(m, "y") // hides x, so y shows on top
	press(m, keyEsc)
	if rows := body(m); !slices.Equal(rows, pinned) {
		t.Errorf("body after the filter is cleared =\n%s\nwant the pinned x back\n%s",
			strings.Join(rows, "\n"), strings.Join(pinned, "\n"))
	}
}

// newModelAt returns a model sized 40 by 6, a body of 4 rows, that keeps
// 100 entries with the newest at newest.
func newModelAt(newest order.Newest) *Model {
	if newest == order.Top {
		return newTopModel(40, 6, 100, render.CardNone)
	}
	return newModel(40, 6, 100)
}

func TestScrollWithNoEntryOnScreenChangesNothing(t *testing.T) {
	cases := []struct {
		newest order.Newest
		// want is the entry rows while following ten entries.
		want []string
	}{
		{order.Bottom, []string{"event-06", "event-07", "event-08", "event-09"}},
		{order.Top, []string{"event-09", "event-08", "event-07", "event-06"}},
	}
	for _, tc := range cases {
		t.Run(tc.newest.String()+" empty feed", func(t *testing.T) {
			m := newModelAt(tc.newest)
			press(m, keyUp, keyPgDown)
			assertFooter(t, m, "0/0  following")
			feed(m, events(0, 10)...)
			assertTexts(t, m, tc.want...)
			assertFooter(t, m, "10/10  following")
		})
		t.Run(tc.newest.String()+" filter hides every entry", func(t *testing.T) {
			m := newModelAt(tc.newest)
			feed(m, events(0, 6)...)
			typeText(m, "zzz")
			press(m, keyUp, keyPgDown)
			feed(m, events(6, 4)...)
			press(m, keyEsc)
			assertTexts(t, m, tc.want...)
			assertFooter(t, m, "10/10  following")
		})
		t.Run(tc.newest.String()+" pinned", func(t *testing.T) {
			m := newModelAt(tc.newest)
			feed(m, events(0, 10)...)
			press(m, keyUp)
			pinned := texts(m)
			typeText(m, "zzz")
			press(m, keyDown)
			press(m, keyEsc)
			assertTexts(t, m, pinned...)
			assertFooter(t, m, "scrolled")
		})
	}
}
