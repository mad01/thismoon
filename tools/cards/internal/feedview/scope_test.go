package feedview

import (
	"fmt"
	"strings"
	"testing"
)

// scopedCard returns a card line with id and one line of text, in scope.
func scopedCard(id, scope, text string) string {
	return fmt.Sprintf(`{"type":"card","id":%q,"lines":[%q],"scope":%q}`, id, text, scope)
}

func TestScopePrefixFilters(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m,
		card("a", "", "deploy finished"),
		scopedCard("b", "debug", "cache warm"),
		scopedCard("c", "debug", "cache miss for alice"),
	)
	assertTexts(t, m, "deploy finished")
	assertFooter(t, m, ">   1/1  following")

	typeText(m, "de:")
	assertTexts(t, m, "cache warm", "cache miss for alice")
	assertFooter(t, m, "> de:  debug  2/2  following")

	typeText(m, " miss alice")
	assertTexts(t, m, "cache miss for alice")
	assertFooter(t, m, "> de: miss alice  debug  1/2  following")

	press(m, keyEsc)
	assertTexts(t, m, "deploy finished")
	if f := footer(m); strings.Contains(f, "debug") {
		t.Errorf("footer = %q after Esc, want the default set and no scope", f)
	}
}

func TestScopePrefixIsTextUntilItsScopeArrives(t *testing.T) {
	m := newModel(60, 8, 100)
	feed(m, card("a", "", "de: note"), card("b", "", "deploy finished"))
	typeText(m, "de:")
	assertTexts(t, m, "de: note")
	assertFooter(t, m, "> de:  1/2")

	feed(m, scopedCard("c", "debug", "cache warm"))
	assertTexts(t, m, "cache warm")
	assertFooter(t, m, "> de:  debug  1/1")

	feed(m, `{"type":"remove","id":"c"}`)
	assertTexts(t, m, "de: note")
	assertFooter(t, m, "> de:  1/2")
}
