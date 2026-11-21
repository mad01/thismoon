package listview

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	keyLeft      = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyRight     = tea.KeyPressMsg{Code: tea.KeyRight}
	keyPgUp      = tea.KeyPressMsg{Code: tea.KeyPgUp}
	keyPgDown    = tea.KeyPressMsg{Code: tea.KeyPgDown}
	keySpace     = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyEnter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyCtrlC     = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// fakeStarter records each argv instead of starting a process.
type fakeStarter struct {
	started [][]string
	err     error
}

func (f *fakeStarter) Start(argv []string) error {
	f.started = append(f.started, argv)
	return f.err
}

// newModel returns a model sized width by height with a fake starter, in
// list mode's default card style.
func newModel(width, height int) (*Model, *fakeStarter) {
	return newStyledModel(width, height, render.CardSpace)
}

// newStyledModel returns a model sized width by height with a fake starter,
// in style.
func newStyledModel(width, height int, style render.CardStyle) (*Model, *fakeStarter) {
	starter := &fakeStarter{}
	m := New(Config{Starter: starter, Style: style})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m, starter
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

func footer(m *Model) string {
	rows := screen(m)
	return rows[len(rows)-1]
}

// cursorCard returns the first word of the cursor card's first text row: the
// first word after a focus bar, which a space or the mark bar follows, a box
// side aside.
func cursorCard(t *testing.T, m *Model) string {
	t.Helper()
	for _, row := range screen(m) {
		if word := focusedWord(row); word != "" {
			return word
		}
	}
	t.Fatalf("no focus bar on screen:\n%s", strings.Join(screen(m), "\n"))
	return ""
}

// focusedWord returns the first word after the focus bar on a stripped row,
// or "" when the row has no focus bar or it starts a box edge.
func focusedWord(row string) string {
	runes := []rune(row)
	for i := 0; i+1 < len(runes); i++ {
		if string(runes[i]) != bar || (runes[i+1] != ' ' && string(runes[i+1]) != bar) {
			continue
		}
		words := strings.Fields(strings.TrimPrefix(string(runes[i+2:]), "│"))
		if len(words) == 0 || strings.HasPrefix(words[0], "╭") ||
			strings.HasPrefix(words[0], "╰") {
			return ""
		}
		return words[0]
	}
	return ""
}

func cardLine(id string, lines ...string) string {
	quoted := make([]string, len(lines))
	for i, l := range lines {
		quoted[i] = fmt.Sprintf("%q", l)
	}
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%s]}`, id, strings.Join(quoted, ","))
}

func runCard(id string, argv ...string) string {
	run, err := json.Marshal(argv)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%q],"actions":{"enter":{"run":%s}}}`,
		id, id, run)
}

func emitCard(id, action string) string {
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%q],"actions":{"enter":{"emit":%q}}}`,
		id, id, action)
}

// cards returns n single-line cards named card-0 and up.
func cards(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = cardLine(fmt.Sprintf("c%d", i), fmt.Sprintf("card-%d", i))
	}
	return out
}

func TestHeaderRow(t *testing.T) {
	m, _ := newModel(80, 10)
	feed(m, `{"type":"header","text":"Open pull requests"}`, cardLine("a", "acme/api#12"))

	if got := screen(m)[0]; got != "  Open pull requests" {
		t.Errorf("header row = %q, want the header indented to the card text", got)
	}
	raw := strings.Split(m.View().Content, "\n")[0]
	if !strings.Contains(raw, "\x1b[1m") {
		t.Errorf("header row = %q, want a plain header drawn bold", raw)
	}
}

func TestLongLineIsCut(t *testing.T) {
	m, _ := newModel(30, 6)
	feed(m, cardLine("a", "acme/api#12 fix the login redirect loop for alice"))

	row := screen(m)[1]
	if !strings.HasSuffix(row, "…") || ansi.StringWidth(row) != 30 {
		t.Errorf("card row = %q (%d cells), want it cut to 30 cells ending in …",
			row, ansi.StringWidth(row))
	}
}

func TestNoRowWiderThanThePane(t *testing.T) {
	long := strings.Repeat("acme/api#12 日本語 alice ", 20)
	for _, width := range []int{1, 2, 3, 10, 59, 61, 125, 130, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, _ := newModel(width, 12)
			feed(m, fmt.Sprintf(`{"type":"header","text":%q}`, long),
				cardLine("a", long, long, long), cardLine("b", long), cardLine("c", "x"),
				cardLine("d", long, "y"), "not json "+long)
			typeText(m, "a")
			rows := screen(m)
			if len(rows) != 12 {
				t.Errorf("drew %d rows, want the pane height 12", len(rows))
			}
			for i, row := range rows {
				if w := ansi.StringWidth(row); w > width {
					t.Errorf("row %d is %d cells wide, want at most %d: %q", i, w, width, row)
				}
			}
		})
	}
}

func TestCursorAndMarkGutters(t *testing.T) {
	m, _ := newModel(80, 10)
	feed(m, cardLine("a", "card-a", "alice"), cardLine("b", "card-b"))
	rows := screen(m)
	if rows[1] != "▌ card-a" || rows[2] != "▌ alice" || rows[4] != "  card-b" {
		t.Errorf("rows = %q, want the focus bar on both lines of card-a only", rows[1:5])
	}

	press(m, keySpace)
	rows = screen(m)
	if rows[1] != " ▌card-a" || rows[2] != " ▌alice" || rows[4] != "▌ card-b" {
		t.Errorf("rows after space = %q, want the mark bar down card-a and the focus bar on card-b",
			rows[1:5])
	}
	if f := footer(m); !strings.Contains(f, "1 marked") {
		t.Errorf("footer = %q, want 1 marked", f)
	}
}

func TestColumnsFollowWidth(t *testing.T) {
	cases := []struct {
		width     int
		wantFirst string
	}{
		{width: 130, wantFirst: "card-0 card-1"},
		{width: 80, wantFirst: "card-0"},
		{width: 200, wantFirst: "card-0 card-1 card-2"},
	}
	m, _ := newModel(130, 10)
	feed(m, cards(4)...)
	for _, tc := range cases {
		m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 10})
		first := strings.Join(strings.Fields(strings.ReplaceAll(screen(m)[1], bar, "")), " ")
		if first != tc.wantFirst {
			t.Errorf("width %d: first grid row = %q, want %q", tc.width, first, tc.wantFirst)
		}
	}
}

func TestGridNavigation(t *testing.T) {
	// Width 130 gives two columns: [0 1] [2 3] [4].
	m, _ := newModel(130, 20)
	feed(m, cards(5)...)
	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{keyRight, "card-1"},
		{keyDown, "card-3"},
		{keyDown, "card-4"}, // the last row is short: down lands on its last card
		{keyDown, "card-4"},
		{keyLeft, "card-3"},
		{keyUp, "card-1"},
		{keyUp, "card-1"},
		{keyLeft, "card-0"},
		{keyLeft, "card-0"},
		{keyPgDown, "card-4"},
		{keyPgUp, "card-0"},
	}
	for i, s := range steps {
		press(m, s.key)
		if got := cursorCard(t, m); got != s.want {
			t.Fatalf("step %d (%s): cursor on %s, want %s", i+1, s.key, got, s.want)
		}
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	// Height 9 leaves 7 body rows: four single-line cards and their gaps.
	m, _ := newModel(80, 9)
	feed(m, cards(20)...)
	for i := 1; i <= 12; i++ {
		press(m, keyDown)
		if got, want := cursorCard(t, m), fmt.Sprintf("card-%d", i); got != want {
			t.Fatalf("after %d downs: cursor on %s, want %s", i, got, want)
		}
	}
	if body := strings.Join(screen(m), "\n"); strings.Contains(body, "card-0\n") {
		t.Errorf("screen still shows card-0 with the cursor on card-12:\n%s", body)
	}

	press(m, keyPgUp)
	if got := cursorCard(t, m); got != "card-8" {
		t.Errorf("after pgup: cursor on %s, want card-8, one screen of four cards up", got)
	}

	// A taller pane fills with the cards above instead of blank rows.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 60})
	if rows := screen(m); rows[1] != "  card-0" {
		t.Errorf("first card row after growing = %q, want card-0", rows[1])
	}
}

func TestEnterRunsEachTargetAndClearsMarks(t *testing.T) {
	m, starter := newModel(80, 20)
	feed(m, runCard("a", "open", "acme/api#12"), runCard("b", "open", "acme/api#13"),
		runCard("c", "open", "acme/web#7"))
	press(m, keySpace, keyRight, keySpace, keyEnter)

	want := [][]string{{"open", "acme/api#12"}, {"open", "acme/web#7"}}
	if !slices.EqualFunc(starter.started, want, slices.Equal) {
		t.Errorf("started %q, want %q", starter.started, want)
	}
	f := footer(m)
	if !strings.Contains(f, "0 marked") || !strings.Contains(f, "ran 2") {
		t.Errorf("footer = %q, want 0 marked and ran 2", f)
	}
	if body := strings.Join(screen(m)[1:], "\n"); strings.Contains(body, " "+bar+"acme") {
		t.Errorf("screen still shows a mark:\n%s", body)
	}

	press(m, keyEnter)
	if got := starter.started[len(starter.started)-1]; !slices.Equal(got, want[1]) {
		t.Errorf("enter with no marks started %q, want the cursor card's %q", got, want[1])
	}
}

func TestEnterSummary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.jsonl")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	starter := &fakeStarter{}
	m := New(Config{Starter: starter, Emitter: NewJSONEmitter(f)})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	feed(m, runCard("a", "open"), emitCard("b", "approve"), runCard("c", "open"),
		cardLine("d", "no action"))
	press(m, keySpace, keySpace, keySpace, keySpace, keyEnter)

	if got := footer(m); !strings.HasSuffix(got, "ran 2, emitted 1, 1 without an action") {
		t.Errorf("footer = %q, want it to end in the summary", got)
	}
}

func TestEmitAppendsToOutFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(out, []byte("{\"earlier\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := New(Config{Starter: &fakeStarter{}, Emitter: NewJSONEmitter(f)})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	feed(m, emitCard("acme/api#12", "approve"))
	press(m, keyEnter)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"earlier\":true}\n{\"action\":\"approve\",\"id\":\"acme/api#12\"}\n"
	if string(got) != want {
		t.Errorf("out file = %q, want %q", got, want)
	}
	if got := footer(m); !strings.HasSuffix(got, "emitted 1") {
		t.Errorf("footer = %q, want emitted 1", got)
	}
}

func TestActionFailuresShowInFooter(t *testing.T) {
	t.Run("emit without an out file", func(t *testing.T) {
		m, _ := newModel(80, 10)
		feed(m, emitCard("a", "approve"))
		press(m, keyEnter)
		if f := footer(m); !strings.HasSuffix(f, "1 failed: "+errNoEmitter.Error()) {
			t.Errorf("footer = %q, want the missing --out error", f)
		}
	})
	t.Run("start failure", func(t *testing.T) {
		m, starter := newModel(100, 10)
		starter.err = errors.New("start \"open\": no such file")
		feed(m, runCard("a", "open"))
		press(m, keyEnter)
		if f := footer(m); !strings.HasSuffix(f, "1 failed: start \"open\": no such file") {
			t.Errorf("footer = %q, want the start error", f)
		}
	})
	t.Run("error text is sanitized", func(t *testing.T) {
		m, starter := newModel(100, 10)
		starter.err = errors.New("start \x1b]0;pwned\x07: no such file")
		feed(m, runCard("a", "open"))
		press(m, keyEnter)
		if raw := m.View().Content; strings.Contains(raw, "\x1b]0;") {
			t.Errorf("view = %q, want the escape in the error replaced", raw)
		}
	})
}

func TestEscClearsMarksThenFilter(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, cardLine("a", "fix login"), cardLine("b", "docs typo"), cardLine("c", "fix logout"))
	typeText(m, "fix")
	press(m, keySpace)
	if f := footer(m); !strings.Contains(f, "> fix  2/3  1 marked") {
		t.Fatalf("footer = %q, want the filter, 2/3 and 1 marked", f)
	}

	press(m, keyEsc)
	if f := footer(m); !strings.Contains(f, "> fix  2/3  0 marked") {
		t.Errorf("footer after one esc = %q, want the marks cleared and the filter kept", f)
	}
	press(m, keyEsc)
	if f := footer(m); !strings.Contains(f, ">   3/3  0 marked") {
		t.Errorf("footer after two escs = %q, want the filter cleared", f)
	}
}

func TestFilterTyping(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, cardLine("a", "acme/api#12 login"), cardLine("b", "acme/api#13 docs"))
	typeText(m, "lgnx")
	if f := footer(m); !strings.Contains(f, "> lgnx  0/2") {
		t.Errorf("footer = %q, want no card to match lgnx", f)
	}
	press(m, keyBackspace)
	if f := footer(m); !strings.Contains(f, "> lgn  1/2") {
		t.Errorf("footer = %q, want backspace to leave lgn matching one card", f)
	}
	if got := cursorCard(t, m); got != "acme/api#12" {
		t.Errorf("cursor on %s, want the login card", got)
	}
}

func TestBadInputKeepsGoing(t *testing.T) {
	m, _ := newModel(120, 20)
	feed(m, cardLine("a", "card-a"), "", "{not json", `{"type":"card","lines":["no id"]}`,
		"   ", cardLine("b", "card-b"))

	f := footer(m)
	for _, want := range []string{
		"2/2", "input ended", "2 errors", "line 4: liststore: card id is required",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("footer = %q, want %q", f, want)
		}
	}

	feed(m, cardLine("c", "card-c"))
	if f := footer(m); !strings.Contains(f, "3/3") {
		t.Errorf("footer = %q, want the next card applied after the errors", f)
	}
}

func TestCtrlCQuits(t *testing.T) {
	m, _ := newModel(80, 10)
	_, cmd := m.Update(keyCtrlC)
	if cmd == nil {
		t.Fatal("ctrl+c returned no command, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c command gave %T, want tea.QuitMsg", cmd())
	}
}

func TestManySingleCardsStayFast(t *testing.T) {
	// Each line used to refilter and lay out every card before it, so this
	// took over ten seconds. Now a line costs about one screen of work; the
	// bound is generous enough for the race detector on a slow machine. The
	// cursor sits a few pages down, so scrolling checks the rows around it.
	const n, bound = 40_000, 4 * time.Second
	m, _ := newModel(30, 8)
	feed(m, cards(50)...)
	press(m, keyPgDown, keyPgDown, keySpace)
	lines := strings.Join(cards(n), "\n")
	start := time.Now()
	feed(m, lines)
	if took := time.Since(start); took > bound {
		t.Errorf("%d single-card lines took %v, want under %v", n, took, bound)
	}
	if f := footer(m); !strings.Contains(f, fmt.Sprintf("%d/%d  1 marked", n, n)) {
		t.Errorf("footer = %q, want every card and the mark kept", f)
	}
}

func TestEscClearsHiddenMarks(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, cardLine("a", "fix login"), cardLine("b", "docs typo"))
	press(m, keySpace)
	typeText(m, "docs")
	if f := footer(m); !strings.Contains(f, "> docs  1/2  0 marked") {
		t.Fatalf("footer = %q, want the marked card hidden by the filter", f)
	}

	press(m, keyEsc)
	if f := footer(m); !strings.Contains(f, "> docs  1/2") {
		t.Errorf("footer after esc = %q, want the filter kept while a hidden mark is cleared", f)
	}
	press(m, keyBackspace, keyBackspace, keyBackspace, keyBackspace)
	if f := footer(m); !strings.Contains(f, ">   2/2  0 marked") {
		t.Errorf("footer = %q, want the hidden mark gone once the filter is cleared", f)
	}
}

func TestPasteGoesIntoTheFilterAsOneWord(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, cardLine("a", "acme/api#12 login"), cardLine("b", "acme/api#13 docs"))

	m.Update(tea.PasteMsg{Content: "12 l\ng\x1bn"})
	if f := footer(m); !strings.Contains(f, "> 12lgn  1/2") {
		t.Errorf("footer = %q, want the paste in the filter with its spaces dropped", f)
	}
	if got := cursorCard(t, m); got != "acme/api#12" {
		t.Errorf("cursor on %s, want the login card", got)
	}
}

func TestIDsStayAsSent(t *testing.T) {
	var out strings.Builder
	m := New(Config{Starter: &fakeStarter{}, Emitter: NewJSONEmitter(&out)})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	feed(m, emitCard("a\tb", "approve"), emitCard("a\nb", "approve"))
	if f := footer(m); !strings.Contains(f, "2/2") {
		t.Errorf("footer = %q, want ids that differ only in a control character kept apart", f)
	}

	press(m, keyEnter)
	if want := `{"action":"approve","id":"a\tb"}` + "\n"; out.String() != want {
		t.Errorf("emitted %q, want %q", out.String(), want)
	}
}

func TestEmojiFitsTheGridByTheRendererMethod(t *testing.T) {
	// A ZWJ family is 6 cells by wcwidth and 2 by grapheme width; a flag is
	// 2 either way. The renderer counts by wcwidth until the terminal
	// reports mode 2027.
	const family, flag = "👨\u200d👩\u200d👧", "🇸🇪"
	title := "fix " + family + " login " + flag + " " + strings.Repeat(family+flag, 30)
	// The zero report is about no mode, so it leaves wcwidth on.
	for _, report := range []tea.ModeReportMsg{
		{}, {Mode: ansi.ModeUnicodeCore, Value: ansi.ModeSet},
	} {
		cells := render.Cells{}.Report(report.Mode, report.Value)
		for _, style := range allStyles {
			for _, width := range []int{20, 61, 120, 181} {
				m, _ := newStyledModel(width, 12, style)
				m.Update(report)
				feed(m, cardLine("a", title, flag+family), cardLine("b", "right-b"),
					cardLine("c", "right-c "+title))
				for i, row := range strings.Split(m.View().Content, "\n") {
					if w := cells.Width(row); w > width {
						t.Errorf("method %d %v width %d: row %d = %q is %d cells wide",
							cells.Method, style, width, i, ansi.Strip(row), w)
					}
				}
				// Two or three columns: card b sits right of the cut card a.
				if columns(width) > 1 && !slices.ContainsFunc(screen(m), func(row string) bool {
					return strings.Contains(row, "…") && strings.Contains(row, "right-b")
				}) {
					t.Errorf("method %d %v width %d: no row holds cut card a and card b:\n%s",
						cells.Method, style, width, strings.Join(screen(m), "\n"))
				}
			}
		}
	}
}
