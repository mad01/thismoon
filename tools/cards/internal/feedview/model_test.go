package feedview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/input"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

var (
	keyUp        = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown      = tea.KeyPressMsg{Code: tea.KeyDown}
	keyPgUp      = tea.KeyPressMsg{Code: tea.KeyPgUp}
	keyPgDown    = tea.KeyPressMsg{Code: tea.KeyPgDown}
	keyHome      = tea.KeyPressMsg{Code: tea.KeyHome}
	keyEnd       = tea.KeyPressMsg{Code: tea.KeyEnd}
	keyEnter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyCtrlC     = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// zone is two hours east of UTC, so a test sees the time converted.
var zone = time.FixedZone("UTC+2", 2*60*60)

// arrived is when every input line arrives in these tests: 10:15 in zone.
var arrived = time.Date(2026, 11, 21, 8, 15, 0, 0, time.UTC)

// newModel returns a model sized width by height that keeps max entries.
func newModel(width, height, max int) *Model {
	m := New(Config{Max: max, Now: func() time.Time { return arrived }, Location: zone})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// feed sends lines through input.Read into m, as stdin would.
func feed(m *Model, lines ...string) {
	input.Read(strings.NewReader(strings.Join(lines, "\n")), func(msg tea.Msg) { m.Update(msg) })
}

func press(m *Model, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// screen returns the drawn rows with escape codes stripped.
func screen(m *Model) []string { return strings.Split(ansi.Strip(m.View().Content), "\n") }

// body returns the rows between the header and the footer, trailing spaces
// trimmed.
func body(m *Model) []string {
	rows := screen(m)
	out := make([]string, 0, len(rows))
	for _, r := range rows[1 : len(rows)-1] {
		out = append(out, strings.TrimRight(r, " "))
	}
	return out
}

func footer(m *Model) string {
	rows := screen(m)
	return rows[len(rows)-1]
}

// card returns a card line with id, the RFC3339 time at (none when empty)
// and lines.
func card(id, at string, lines ...string) string {
	quoted := make([]string, len(lines))
	for i, l := range lines {
		quoted[i] = fmt.Sprintf("%q", l)
	}
	atField := ""
	if at != "" {
		atField = fmt.Sprintf(`,"at":%q`, at)
	}
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%s]%s}`,
		id, strings.Join(quoted, ","), atField)
}

// events returns n single-line cards, event-00 and up, with ids e0 and up.
func events(from, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = card(fmt.Sprintf("e%d", from+i), "", fmt.Sprintf("event-%02d", from+i))
	}
	return out
}

// texts returns the entry text on each body row, the time column cut off.
func texts(m *Model) []string {
	var out []string
	for _, r := range body(m) {
		if len(r) > textColumn {
			out = append(out, r[textColumn:])
		}
	}
	return out
}

func assertTexts(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := texts(m); !slices.Equal(got, want) {
		t.Errorf("entry rows = %q, want %q\n%s", got, want, strings.Join(screen(m), "\n"))
	}
}

func TestTimeColumnFromAt(t *testing.T) {
	m := newModel(60, 6, 10)
	feed(m,
		card("a", "2026-11-21T09:30:00Z", "acme/api#12 merged by alice"),
		card("b", "2026-11-21T09:45:00-05:00", "acme/api#13 opened"),
	)

	rows := body(m)
	if rows[0] != "  11:30  acme/api#12 merged by alice" {
		t.Errorf("row 1 = %q, want at in the test zone as HH:MM", rows[0])
	}
	if rows[1] != "  16:45  acme/api#13 opened" {
		t.Errorf("row 2 = %q, want an at with an offset converted to the test zone", rows[1])
	}
	raw := strings.Split(m.View().Content, "\n")[1]
	if !strings.Contains(raw, ansi.Style{}.Faint().String()+"11:30") {
		t.Errorf("row 1 = %q, want the time drawn dim", raw)
	}
}

func TestTimeFallsBackToArrival(t *testing.T) {
	m := newModel(60, 6, 10)
	feed(m, card("a", "", "deploy finished"))

	if got := body(m)[0]; got != "  10:15  deploy finished" {
		t.Errorf("row = %q, want the arrival time when at is missing", got)
	}
}

func TestLaterLinesIndentUnderFirstLineText(t *testing.T) {
	m := newModel(60, 6, 10)
	feed(m, card("a", "", "acme/api#12 fix login", "alice · checks failing", "third"))

	rows := body(m)
	want := []string{
		"  10:15  acme/api#12 fix login",
		"         alice · checks failing",
		"         third",
	}
	if !slices.Equal(rows[:3], want) {
		t.Errorf("rows = %q, want %q", rows[:3], want)
	}
}

func TestEntriesAreDense(t *testing.T) {
	m := newModel(60, 6, 10)
	feed(m, card("a", "", "one", "one more"), card("b", "", "two"))

	assertTexts(t, m, "one", "one more", "two")
}

func TestFollowKeepsNewestVisible(t *testing.T) {
	m := newModel(40, 6, 100) // a body of 4 rows
	for i, line := range events(0, 10) {
		feed(m, line)
		rows := texts(m)
		if want := fmt.Sprintf("event-%02d", i); rows[len(rows)-1] != want {
			t.Fatalf("after %d entries the last row = %q, want the newest, %s", i+1,
				rows[len(rows)-1], want)
		}
	}
	assertTexts(t, m, "event-06", "event-07", "event-08", "event-09")
	if f := footer(m); !strings.Contains(f, "10/10  following") {
		t.Errorf("footer = %q, want 10/10 and following", f)
	}
}

func TestFollowShowsTallNewestEntryWhole(t *testing.T) {
	m := newModel(40, 6, 100)
	feed(m, events(0, 4)...)
	feed(m, card("big", "", "big-1", "big-2", "big-3"))

	assertTexts(t, m, "event-03", "big-1", "big-2", "big-3")
}

func TestUpStopsFollowingAndEndResumes(t *testing.T) {
	m := newModel(40, 6, 100)
	feed(m, events(0, 10)...)

	press(m, keyUp)
	assertTexts(t, m, "event-05", "event-06", "event-07", "event-08")
	if f := footer(m); !strings.Contains(f, "scrolled, End follows") {
		t.Errorf("footer = %q, want scrolled, End follows", f)
	}

	feed(m, events(10, 3)...)
	assertTexts(t, m, "event-05", "event-06", "event-07", "event-08")

	press(m, keyEnd)
	assertTexts(t, m, "event-09", "event-10", "event-11", "event-12")
	if f := footer(m); !strings.Contains(f, "13/13  following") {
		t.Errorf("footer = %q, want following again", f)
	}
}

func TestScrollKeys(t *testing.T) {
	m := newModel(40, 6, 100)
	feed(m, events(0, 10)...)

	press(m, keyHome)
	assertTexts(t, m, "event-00", "event-01", "event-02", "event-03")
	press(m, keyDown)
	assertTexts(t, m, "event-01", "event-02", "event-03", "event-04")
	press(m, keyPgDown)
	assertTexts(t, m, "event-05", "event-06", "event-07", "event-08")
	press(m, keyPgDown, keyDown)
	assertTexts(t, m, "event-06", "event-07", "event-08", "event-09")
	press(m, keyPgUp)
	assertTexts(t, m, "event-02", "event-03", "event-04", "event-05")
	press(m, keyUp, keyUp, keyUp, keyUp)
	assertTexts(t, m, "event-00", "event-01", "event-02", "event-03")
	if f := footer(m); !strings.Contains(f, "scrolled, End follows") {
		t.Errorf("footer = %q, want scrolled after the scroll keys", f)
	}
}

func TestDownStopsFollowingAtTheBottom(t *testing.T) {
	m := newModel(40, 6, 100)
	feed(m, events(0, 10)...)

	press(m, keyDown)
	assertTexts(t, m, "event-06", "event-07", "event-08", "event-09")
	feed(m, events(10, 1)...)
	assertTexts(t, m, "event-06", "event-07", "event-08", "event-09")
}

func TestUpGoesToTheStartOfAPartlyShownEntry(t *testing.T) {
	m := newModel(40, 6, 100)
	feed(m, card("big", "", "big-1", "big-2", "big-3"), card("a", "", "a"), card("b", "", "b"))

	// Following shows big-2 at the top; up shows big whole.
	assertTexts(t, m, "big-2", "big-3", "a", "b")
	press(m, keyUp)
	assertTexts(t, m, "big-1", "big-2", "big-3", "a")
}

func TestPinnedViewSurvivesDroppedEntries(t *testing.T) {
	m := newModel(40, 6, 6)
	feed(m, events(0, 6)...)
	press(m, keyUp)
	assertTexts(t, m, "event-01", "event-02", "event-03", "event-04")

	feed(m, events(6, 1)...) // the limit drops event-00
	assertTexts(t, m, "event-01", "event-02", "event-03", "event-04")
	if f := footer(m); !strings.Contains(f, "6/6") {
		t.Errorf("footer = %q, want 6/6 after the limit", f)
	}
}

func TestFilter(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m,
		card("a", "", "login failed for alice"),
		card("b", "", "deploy finished"),
		card("c", "", "login ok for alice"),
	)

	typeText(m, "lgn")
	assertTexts(t, m, "login failed for alice", "login ok for alice")
	if f := footer(m); !strings.Contains(f, "> lgn  2/3") {
		t.Errorf("footer = %q, want the query and 2/3", f)
	}
	typeText(m, "x")
	if f := footer(m); !strings.Contains(f, "> lgnx  0/3") {
		t.Errorf("footer = %q, want no entry to match lgnx", f)
	}
	if got := body(m)[0]; got != "  no cards match the filter" {
		t.Errorf("body = %q, want the no-match text", got)
	}
	press(m, keyBackspace)
	assertTexts(t, m, "login failed for alice", "login ok for alice")
	press(m, keyEsc)
	assertTexts(t, m, "login failed for alice", "deploy finished", "login ok for alice")
}

func TestRepeatedIDAddsNoRow(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m,
		card("a", "", "acme/api#12 opened"),
		card("b", "", "acme/api#13 opened"),
		card("a", "", "acme/api#12 opened"),
	)

	assertTexts(t, m, "acme/api#12 opened", "acme/api#13 opened")
	if f := footer(m); !strings.Contains(f, "2/2") {
		t.Errorf("footer = %q, want 2/2", f)
	}
}

func TestNoRowWiderThanTheWidth(t *testing.T) {
	for _, width := range []int{4, 12, 30} {
		m := newModel(width, 8, 100)
		feed(m,
			`{"type":"header","text":"Deploy events for every service in the acme org"}`,
			card("a", "", "acme/api#12 fix the login redirect loop for alice",
				"a second line that runs well past the edge", "and a third"),
			`not json`,
		)
		typeText(m, "a long filter query")
		for i, row := range screen(m) {
			if w := ansi.StringWidth(row); w > width {
				t.Errorf("width %d: row %d = %q is %d cells wide", width, i, row, w)
			}
		}
	}
}

func TestLongLineIsCut(t *testing.T) {
	m := newModel(30, 6, 100)
	feed(m, card("a", "", "acme/api#12 fix the login redirect loop for alice"))

	row := screen(m)[1]
	if !strings.HasSuffix(row, "…") || ansi.StringWidth(row) != 30 {
		t.Errorf("row = %q (%d cells), want it cut to 30 cells ending in …",
			row, ansi.StringWidth(row))
	}
}

func TestToneAndSpanColors(t *testing.T) {
	m := newModel(60, 6, 100)
	feed(m, `{"type":"card","id":"a","tone":"red","lines":[[`+
		`{"text":"failed","bold":true},{"text":" acme/api#12","color":"blue"}]]}`)

	raw := strings.Split(m.View().Content, "\n")[1]
	for _, want := range []string{"\x1b[1;31mfailed", "\x1b[34m acme/api#12"} {
		if !strings.Contains(raw, want) {
			t.Errorf("row = %q, want %q: the tone on the plain span, blue on its own",
				raw, want)
		}
	}
}

func TestBadInputKeepsGoing(t *testing.T) {
	m := newModel(80, 6, 100)
	feed(m, card("a", "", "one"), "not json", card("b", "", "two"))

	f := footer(m)
	for _, want := range []string{"2/2", "input ended", "1 error", "line 2: protocol: bad JSON"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer = %q, want %q", f, want)
		}
	}
	assertTexts(t, m, "one", "two")
}

func TestWaitingThenNoCards(t *testing.T) {
	m := newModel(60, 6, 100)
	if got := body(m)[0]; got != "  waiting for cards" {
		t.Errorf("body = %q, want the waiting text", got)
	}
	feed(m)
	if got := body(m)[0]; got != "  no cards" {
		t.Errorf("body after the input ended = %q, want no cards", got)
	}
}

func TestResizeReflows(t *testing.T) {
	m := newModel(60, 6, 100)
	feed(m, events(0, 10)...)
	feed(m, card("w", "", "acme/api#12 fix the login redirect loop"))

	m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	assertTexts(t, m, "event-05", "event-06", "event-07", "event-08", "event-09", "acme/api#12 fi…")
}

func TestEnterDoesNothingAndCtrlCQuits(t *testing.T) {
	m := newModel(60, 6, 100)
	feed(m, `{"type":"card","id":"a","lines":["one"],"actions":{"enter":{"run":["true"]}}}`)
	before := m.View().Content

	if _, cmd := m.Update(keyEnter); cmd != nil || m.View().Content != before {
		t.Errorf("enter changed the screen or returned a command")
	}
	_, cmd := m.Update(keyCtrlC)
	if cmd == nil {
		t.Fatal("ctrl+c returned no command, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c command = %T, want tea.QuitMsg", cmd())
	}
}

func TestSpaceTypesIntoTheFilter(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m, card("a", "", "login failed for alice"), card("b", "", "login ok for bob"))

	typeText(m, "lgn bob")
	assertTexts(t, m, "login ok for bob")
}

func TestPasteGoesIntoTheFilter(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m, card("a", "", "login failed for alice"), card("b", "", "login ok for bob"))

	m.Update(tea.PasteMsg{Content: "lgn\nbob"})
	assertTexts(t, m, "login ok for bob")
	if f := footer(m); !strings.Contains(f, "> lgn bob  1/2") {
		t.Errorf("footer = %q, want the paste in the filter with its newline as a space", f)
	}

	press(m, keyEsc)
	m.Update(tea.PasteMsg{Content: "\x1b[31mbob"})
	if raw := m.View().Content; strings.Contains(raw, "\x1b[31mbob") {
		t.Errorf("view = %q, want the escape in the paste replaced", raw)
	}
}

func TestEmojiFitsTheWidthByTheRendererMethod(t *testing.T) {
	// A ZWJ family is 6 cells by wcwidth and 2 by grapheme width; a flag is
	// 2 either way. The renderer counts by wcwidth until the terminal
	// reports mode 2027.
	const family, flag = "👨\u200d👩\u200d👧", "🇸🇪"
	title := "fix " + family + " login " + flag + " " + strings.Repeat(family+flag, 6)
	// The zero report is about no mode, so it leaves wcwidth on.
	for _, report := range []tea.ModeReportMsg{
		{}, {Mode: ansi.ModeUnicodeCore, Value: ansi.ModeReset},
	} {
		cells := render.Cells{}.Report(report.Mode, report.Value)
		name := fmt.Sprintf("method %d", cells.Method)
		for _, style := range []render.CardStyle{
			render.CardNone, render.CardSpace, render.CardLine, render.CardBox,
		} {
			for _, width := range []int{12, 20, 33, 40} {
				m := newStyledModel(width, 8, style)
				m.Update(report)
				feed(m, `{"type":"header","text":"`+title+`"}`, card("a", "", title, flag+family))
				for i, row := range strings.Split(m.View().Content, "\n") {
					if w := cells.Width(row); w > width {
						t.Errorf("%s %v width %d: row %d = %q is %d cells wide",
							name, style, width, i, ansi.Strip(row), w)
					}
				}
				if rows := screen(m); !strings.HasSuffix(strings.TrimRight(rows[0], " "), "…") {
					t.Errorf("%s %v width %d: header = %q, want it cut with an ellipsis",
						name, style, width, rows[0])
				}
			}
		}
	}
}

func TestPinFallingBackToTheNewestEntryStaysPut(t *testing.T) {
	// A body of 4 rows. Boxed, each entry is 3 rows tall. With x gone, n
	// starts on row 3, past the highest top, row 2, so the screen shows a's
	// bottom border above n.
	m := newStyledModel(40, 6, render.CardBox)
	feed(m, card("a", "", "a-1"), card("x", "", "x-1"), card("n", "", "n-1"))
	press(m, keyUp) // pins x
	feed(m, `{"type":"remove","id":"x"}`)
	pinned := body(m)
	if !strings.Contains(pinned[len(pinned)-2], "n-1") {
		t.Fatalf("body after the remove =\n%s\nwant n at the bottom", strings.Join(pinned, "\n"))
	}

	feed(m, card("z", "", "z-1")) // below n, so the highest top is row 5 again
	if rows := body(m); !slices.Equal(rows, pinned) {
		t.Errorf("body after a short entry below =\n%s\nwant it kept\n%s",
			strings.Join(rows, "\n"), strings.Join(pinned, "\n"))
	}
	assertFooter(t, m, "scrolled, End follows")
}

func TestPinFallingBackPastTheHighestTopStaysPut(t *testing.T) {
	m := newModel(40, 6, 100) // a body of 4 rows, one row per entry
	feed(m, events(0, 6)...)
	press(m, keyUp, keyDown) // pins event-02, on the highest top
	feed(m, `{"type":"remove","id":"e2"}`)
	pinned := texts(m)

	feed(m, events(6, 1)...)
	assertTexts(t, m, pinned...)
}

func TestPinFallingBackPastTheLastEntryStaysPut(t *testing.T) {
	// A body of 4 rows. Boxed, x is 5 rows tall, so pinning it shows its top
	// border and its three lines, and with x gone a fits with a blank row.
	m := newStyledModel(40, 6, render.CardBox)
	feed(m, card("a", "", "a-1"), card("x", "", "x-1", "x-2", "x-3"))
	press(m, keyUp, keyDown) // pins x
	feed(m, `{"type":"remove","id":"x"}`)
	pinned := body(m)

	feed(m, card("z", "", "z-1")) // its top border takes the blank row
	if rows := body(m); !slices.Equal(rows[:3], pinned[:3]) {
		t.Errorf("body after an entry below =\n%s\nwant a kept on top\n%s",
			strings.Join(rows, "\n"), strings.Join(pinned, "\n"))
	}
	assertFooter(t, m, "scrolled, End follows")
}
