package listview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// scopedMuteCard returns a card in scope whose x key runs program with its id,
// labeled label.
func scopedMuteCard(id, text, scope, program, label string) string {
	return fmt.Sprintf(
		`{"type":"card","id":%q,"lines":[%q],"scope":%q,"actions":{"x":{"run":[%q,%q],"label":%q}}}`,
		id,
		text,
		scope,
		program,
		id,
		label,
	)
}

// prList is a pull request list: one shown pull request that x ignores,
// and two ignored ones that x brings back.
var prList = []string{
	muteCard("acme/api#12", "acme/api#12 fix login"),
	scopedMuteCard("acme/api#13", "acme/api#13 docs typo", "ignored",
		"/usr/local/bin/unmute", "unignore"),
	scopedMuteCard("acme/web#7", "acme/web#7 fix logout", "ignored",
		"/usr/local/bin/unmute", "unignore"),
}

// shows reports whether a body row of m holds text.
func shows(m *Model, text string) bool {
	return slices.ContainsFunc(
		screen(m),
		func(row string) bool { return strings.Contains(row, text) },
	)
}

func TestScopedCardsHideUntilTheirPrefix(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, prList...)
	if f := footer(m); !strings.Contains(f, ">   1/1  0 marked") {
		t.Errorf("footer = %q, want the default set counted: 1/1", f)
	}
	if shows(m, "docs typo") || shows(m, "fix logout") {
		t.Errorf("screen =\n%s\nwant the ignored cards hidden", strings.Join(screen(m), "\n"))
	}

	typeText(m, "i:")
	if f := footer(m); !strings.Contains(f, "> i:  ignored  2/2  0 marked") {
		t.Errorf("footer = %q, want the scope named before its counts", f)
	}
	if shows(m, "fix login") || !shows(m, "docs typo") || !shows(m, "fix logout") {
		t.Errorf("screen =\n%s\nwant only the ignored cards", strings.Join(screen(m), "\n"))
	}

	typeText(m, "lgt")
	if f := footer(m); !strings.Contains(f, "> i:lgt  ignored  1/2  0 marked") {
		t.Errorf("footer = %q, want the rest of the word to filter within the scope", f)
	}
}

func TestSpaceThenXActsOnAScopedCard(t *testing.T) {
	m, starter := newModel(80, 20)
	feed(m, prList...)
	typeText(m, "ign:")
	press(m, keySpace, keyX)

	want := [][]string{{"/usr/local/bin/unmute", "acme/api#13"}}
	if !slices.EqualFunc(starter.started, want, slices.Equal) {
		t.Errorf("started %q, want %q: the marked ignored card's x action", starter.started, want)
	}
	f := footer(m)
	if !strings.Contains(f, "> ign:  ignored  2/2  0 marked") {
		t.Errorf("footer = %q, want the scope kept and the marks cleared", f)
	}
	if !strings.HasSuffix(f, "x unignore: ran 1") {
		t.Errorf("footer = %q, want it to end in the x summary", f)
	}
}

func TestEscClearsMarksThenTheScope(t *testing.T) {
	m, starter := newModel(80, 20)
	feed(m, prList...)
	press(m, keySpace) // marks acme/api#12, in the default set
	typeText(m, "i:")
	if f := footer(m); !strings.Contains(f, "> i:  ignored  2/2  0 marked") {
		t.Fatalf("footer = %q, want the default set's mark hidden under the scope", f)
	}
	press(m, keyX)
	if len(starter.started) > 0 {
		t.Fatalf("started %q, want nothing: x only acts on marked cards the filter shows",
			starter.started)
	}
	if f := footer(m); !strings.Contains(f, "> i:x  ignored  1/2") {
		t.Fatalf("footer = %q, want x typed into the filter", f)
	}

	press(m, keyBackspace, keySpace, keyEsc)
	if f := footer(m); !strings.Contains(f, "> i:  ignored  2/2  0 marked") {
		t.Errorf("footer = %q after the first Esc, want the marks cleared, the scope kept", f)
	}
	press(m, keyEsc)
	f := footer(m)
	if !strings.Contains(f, ">   1/1  0 marked") || strings.Contains(f, "ignored") {
		t.Errorf("footer = %q after the second Esc, want the default set back", f)
	}
	if !shows(m, "fix login") || shows(m, "docs typo") {
		t.Errorf("screen =\n%s\nwant the default set", strings.Join(screen(m), "\n"))
	}
}

func TestColonWordIsTextWithoutItsScope(t *testing.T) {
	m, _ := newModel(80, 20)
	feed(m, cardLine("a", "docs: fix typo"), cardLine("b", "fix login"), prList[1])
	typeText(m, "docs:")
	f := footer(m)
	if !strings.Contains(f, "> docs:  1/2") || strings.Contains(f, "ignored") {
		t.Errorf("footer = %q, want docs: to filter the default set as text", f)
	}
	if !shows(m, "docs: fix typo") || shows(m, "fix login") {
		t.Errorf("screen =\n%s\nwant the docs: title only", strings.Join(screen(m), "\n"))
	}
}
