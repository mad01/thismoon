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

var (
	keyX      = tea.KeyPressMsg{Code: 'x', Text: "x"}
	keyShiftX = tea.KeyPressMsg{Code: 'x', ShiftedCode: 'X', Text: "X", Mod: tea.ModShift}
)

// actionsCard returns a card line with id, one line of text and the actions
// object given as raw JSON.
func actionsCard(id, text, actions string) string {
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%q],"actions":%s}`, id, text, actions)
}

// muteCard returns a card whose x key runs mute with its id, labeled
// ignore.
func muteCard(id, text string) string {
	return actionsCard(id, text,
		fmt.Sprintf(`{"x":{"run":["/usr/local/bin/mute",%q],"label":"ignore"}}`, id))
}

func TestLetterRunsOnMarkedShownCardsThatHaveIt(t *testing.T) {
	m, starter := newModel(80, 30)
	feed(m, muteCard("acme/api#12", "login-a"), cardLine("acme/api#13", "login-b"),
		muteCard("acme/web#7", "docs-c"), muteCard("acme/web#8", "login-d"),
		muteCard("acme/web#9", "login-e"))
	// Mark d first, then a, b and c; e stays unmarked. The filter then hides
	// c, so the marked cards it shows are a, b and d, and the cursor sits on
	// d.
	press(m, keyDown, keyDown, keyDown, keySpace, keyUp, keyUp, keyUp, keyUp,
		keySpace, keySpace, keySpace)
	typeText(m, "log")
	if f := footer(m); !strings.Contains(f, "> log  4/5  3 marked") {
		t.Fatalf("footer = %q, want the filter, 4/5 and 3 marked", f)
	}

	press(m, keyX)
	want := [][]string{
		{"/usr/local/bin/mute", "acme/api#12"},
		{"/usr/local/bin/mute", "acme/web#8"},
	}
	if !slices.EqualFunc(starter.started, want, slices.Equal) {
		t.Errorf("started %q, want %q: the marked shown cards with x, in screen order",
			starter.started, want)
	}
	f := footer(m)
	if !strings.Contains(f, "> log  4/5  0 marked") {
		t.Errorf("footer = %q, want the filter kept and the marks cleared", f)
	}
	if !strings.HasSuffix(f, "x ignore: ran 2, 1 without x") {
		t.Errorf("footer = %q, want it to end in the x summary", f)
	}

	press(m, keyBackspace, keyBackspace, keyBackspace)
	if f := footer(m); !strings.Contains(f, ">   5/5  0 marked") {
		t.Errorf("footer = %q, want the hidden mark on c cleared too", f)
	}
}

func TestLetterTypesWhenNoMarkedShownCardHasIt(t *testing.T) {
	cases := []struct {
		name string
		// setup marks cards and types a filter before x is pressed.
		setup     func(m *Model)
		key       tea.KeyPressMsg
		wantQuery string
	}{
		{
			name:      "no marks, the cursor card has x",
			setup:     func(*Model) {},
			key:       keyX,
			wantQuery: "x",
		},
		{
			name:      "the marked cards lack x",
			setup:     func(m *Model) { press(m, keyDown, keySpace) },
			key:       keyX,
			wantQuery: "x",
		},
		{
			name: "the marked card with x is hidden",
			setup: func(m *Model) {
				press(m, keySpace)
				typeText(m, "doc")
			},
			key:       keyX,
			wantQuery: "docx",
		},
		{
			name:      "a capital X types",
			setup:     func(m *Model) { press(m, keySpace) },
			key:       keyShiftX,
			wantQuery: "X",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, starter := newModel(80, 20)
			feed(m, muteCard("acme/api#12", "login-a"), cardLine("acme/api#13", "docs-b"))
			tc.setup(m)
			press(m, tc.key)
			if len(starter.started) > 0 {
				t.Errorf("started %q, want nothing", starter.started)
			}
			if f := footer(m); !strings.Contains(f, "> "+tc.wantQuery+" ") {
				t.Errorf("footer = %q, want the filter %q", f, tc.wantQuery)
			}
		})
	}
}

func TestEnterIgnoresLetterActions(t *testing.T) {
	m, starter := newModel(80, 20)
	feed(m, actionsCard(
		"acme/api#12",
		"login-a",
		`{"enter":{"run":["open","acme/api#12"],"label":"open"},"x":{"run":["mute","acme/api#12"]}}`,
	),
		muteCard("acme/api#13", "login-b"))

	press(m, keyEnter)
	want := [][]string{{"open", "acme/api#12"}}
	if !slices.EqualFunc(starter.started, want, slices.Equal) {
		t.Errorf("enter with no marks started %q, want the cursor card's enter action %q",
			starter.started, want)
	}
	if f := footer(m); !strings.HasSuffix(f, "ran 1") {
		t.Errorf("footer = %q, want the enter summary unchanged", f)
	}

	press(m, keySpace, keySpace, keyEnter)
	want = append(want, []string{"open", "acme/api#12"})
	if !slices.EqualFunc(starter.started, want, slices.Equal) {
		t.Errorf("enter on both marked cards started %q, want only the enter action %q",
			starter.started, want)
	}
	if f := footer(m); !strings.HasSuffix(f, "ran 1, 1 without an action") {
		t.Errorf("footer = %q, want the card with only x counted without an action", f)
	}
}

func TestLetterEmitsWithTheOutFile(t *testing.T) {
	var out strings.Builder
	m := New(Config{Starter: &fakeStarter{}, Emitter: NewJSONEmitter(&out)})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	feed(m, actionsCard("acme/api#12", "login-a", `{"a":{"emit":"approve"}}`))
	press(m, keySpace)
	typeText(m, "a")

	if want := `{"action":"approve","id":"acme/api#12"}` + "\n"; out.String() != want {
		t.Errorf("emitted %q, want %q", out.String(), want)
	}
	if f := footer(m); !strings.HasSuffix(f, "a: emitted 1") {
		t.Errorf("footer = %q, want the summary named by the key alone with no label", f)
	}
}

func TestFooterHintNamesTheMarkedCardsKeys(t *testing.T) {
	m, _ := newModel(120, 20)
	feed(
		m,
		actionsCard(
			"acme/api#12",
			"login-a",
			`{"enter":{"run":["open","acme/api#12"],"label":"open"},"x":{"run":["mute"],"label":"ignore"}}`,
		),
		actionsCard(
			"acme/api#13",
			"login-b",
			`{"enter":{"emit":"review"},"a":{"run":["approve"]}}`,
		),
		actionsCard("acme/web#7", "login-c", `{"x":{"emit":"mute","label":"mute"}}`),
		cardLine("acme/web#8", "login-d"),
	)

	if f := footer(m); strings.Contains(f, "enter") {
		t.Errorf("footer with no marks = %q, want no key hint", f)
	}
	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{keySpace, "1 marked  enter open  x ignore"},
		{keySpace, "2 marked  enter open  a run  x ignore"},
		// c's x label differs; the first marked card with x names it.
		{keySpace, "3 marked  enter open  a run  x ignore"},
	}
	for i, s := range steps {
		press(m, s.key)
		if f := footer(m); !strings.Contains(f, s.want) {
			t.Errorf("step %d: footer = %q, want %q", i+1, f, s.want)
		}
	}

	// Only d is marked: it has no action, so there is nothing to hint.
	press(m, keyEsc, keySpace)
	if f := footer(m); !strings.Contains(f, "1 marked  input ended") {
		t.Errorf("footer with only d marked = %q, want no key hint", f)
	}
}

func TestFooterHintNamesAnUnlabeledEmit(t *testing.T) {
	m, _ := newModel(120, 20)
	feed(m, actionsCard("a", "login-a", `{"enter":{"emit":"review"}}`))
	press(m, keySpace)
	if f := footer(m); !strings.Contains(f, "1 marked  enter emit") {
		t.Errorf("footer = %q, want an unlabeled emit named emit", f)
	}
}

func TestFooterHintCutsLongLabels(t *testing.T) {
	label := "ignore-this-pull-request-for-good"
	m, _ := newModel(120, 20)
	feed(m, actionsCard("a", "login-a", fmt.Sprintf(`{"x":{"run":["mute"],"label":%q}}`, label)))
	press(m, keySpace)

	want := "x " + label[:labelWidth-1] + render.Ellipsis
	if f := footer(m); !strings.Contains(f, "1 marked  "+want+"  ") {
		t.Errorf("footer = %q, want the label cut to %d cells: %q", f, labelWidth, want)
	}
}

func TestFooterHintIsCutToThePane(t *testing.T) {
	// A ZWJ family is 6 cells by wcwidth and 2 by grapheme width.
	const family = "👨‍👩‍👧"
	actions := fmt.Sprintf(`{"enter":{"run":["open"],"label":"open"},`+
		`"x":{"run":["mute"],"label":%q},"y":{"emit":"y","label":%q}}`,
		family+"ignore", family+family+family)
	for _, report := range []tea.ModeReportMsg{
		{}, {Mode: ansi.ModeUnicodeCore, Value: ansi.ModeSet},
	} {
		cells := render.Cells{}.Report(report.Mode, report.Value)
		for _, width := range []int{20, 30, 45} {
			m, _ := newModel(width, 10)
			m.Update(report)
			feed(m, actionsCard("a", "login-a", actions))
			press(m, keySpace)

			rows := strings.Split(m.View().Content, "\n")
			raw := rows[len(rows)-1]
			if w := cells.Width(raw); w > width {
				t.Errorf("method %d width %d: footer %q is %d cells wide",
					cells.Method, width, ansi.Strip(raw), w)
			}
			if f := footer(m); !strings.HasPrefix(f, "  >   1/1  1 marked") ||
				!strings.HasSuffix(f, render.Ellipsis) {
				t.Errorf("method %d width %d: footer = %q, want the hint cut short with %s",
					cells.Method, width, f, render.Ellipsis)
			}
		}
	}
}
