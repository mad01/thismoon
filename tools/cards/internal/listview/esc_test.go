package listview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// newEscModel returns a model sized 80 by 20 with a fake starter, in list
// mode's default card style, with Esc quitting as escQuits says, holding
// three cards.
func newEscModel(escQuits bool) *Model {
	m := New(Config{Starter: &fakeStarter{}, Style: render.CardSpace, EscQuits: escQuits})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	feed(m, cardLine("a", "fix login"), cardLine("b", "docs typo"), cardLine("c", "fix logout"))
	return m
}

// isQuit reports whether cmd is tea.Quit.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func withNothing(*Model) {}

func withFilter(m *Model) { typeText(m, "fix") }

func withMark(m *Model) { press(m, keySpace) }

// withMarkThenFilter marks a card the filter then shows.
func withMarkThenFilter(m *Model) {
	typeText(m, "fix")
	press(m, keySpace)
}

// withHiddenMarkThenFilter marks a card the filter then hides.
func withHiddenMarkThenFilter(m *Model) {
	press(m, keySpace)
	typeText(m, "docs")
}

func TestEscQuits(t *testing.T) {
	const presses = 4
	cases := []struct {
		name     string
		escQuits bool
		setup    func(m *Model)
		// quitsOn is the Esc press, from 1, that quits; 0 is none.
		quitsOn int
	}{
		{"nothing to clear", true, withNothing, 1},
		{"a filter", true, withFilter, 2},
		{"a mark", true, withMark, 2},
		{"marks then a filter", true, withMarkThenFilter, 3},
		{"a hidden mark then a filter", true, withHiddenMarkThenFilter, 3},
		{"no flag, nothing to clear", false, withNothing, 0},
		{"no flag, marks then a filter", false, withMarkThenFilter, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newEscModel(tc.escQuits)
			tc.setup(m)
			for n := 1; n <= presses; n++ {
				_, cmd := m.Update(keyEsc)
				if got, want := isQuit(cmd), n == tc.quitsOn; got != want {
					t.Fatalf("esc %d quit = %v, want %v (footer %q)", n, got, want, footer(m))
				}
				if n == tc.quitsOn {
					return
				}
			}
		})
	}
}

func TestEscQuitsClearsMarksThenFilterFirst(t *testing.T) {
	m := newEscModel(true)
	withMarkThenFilter(m)

	press(m, keyEsc)
	if f := footer(m); !strings.Contains(f, "> fix  2/3  0 marked") {
		t.Errorf("footer after one esc = %q, want the marks cleared and the filter kept", f)
	}
	press(m, keyEsc)
	if f := footer(m); !strings.Contains(f, ">   3/3  0 marked") {
		t.Errorf("footer after two escs = %q, want the filter cleared", f)
	}
}
