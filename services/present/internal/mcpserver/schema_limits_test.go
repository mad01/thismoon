package mcpserver

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/limits"
)

// schemaNumber is a number standing on its own in the schema text, not the
// digit of a name like h3.
var schemaNumber = regexp.MustCompile(`\b\d+\b`)

// TestSchemaTextMatchesLimits holds every number in the create tool's
// schema text to its source. A number the limits table owns is written
// from the table here and must appear as written; a number another source
// owns is pinned to that source or named below with it. Once those phrases
// are taken out, no number may be left, so a number added to the text
// without a source fails this test.
func TestSchemaTextMatchesLimits(t *testing.T) {
	n := limits.Number
	fromTable := []string{
		fmt.Sprintf("session is 1 to %d of a-z, 0-9, and -", n(limits.TerminalSession)),
		fmt.Sprintf("rows %d to %d", n(limits.TerminalRowsMin), n(limits.TerminalRowsMax)),
		fmt.Sprintf("a ribbon takes at most %d named series", n(limits.RibbonSeries)),
		fmt.Sprintf(
			"a diagram at most %d nodes and %d groups, groups nested at most %d levels deep",
			n(limits.DiagramNodes),
			n(limits.DiagramGroups),
			n(limits.DiagramDepth),
		),
	}
	fromElsewhere := []string{
		// present.MaxImageBytes, the cap on an image file brought in from
		// this machine.
		fmt.Sprintf("at most %d MiB", present.MaxImageBytes>>20),
		// render.validateBlocks: a columns block has two or three columns.
		"columns {cols: 2 or 3 arrays of blocks}",
		// app.js DOTS_MAX: the deck view swaps the dots for a bar on a long
		// deck.
		"dots give way to a bar above 24 slides",
		// The skill's deck rules: guidance on slide size, not a lint rule.
		"3 to 5 short items or two sentences per slide",
		"5 to 12 slides",
		// app.js MODULE_COLORS: a module node's color indexes four border
		// slots.
		"color: 0 to 3 picks one of four border slots",
	}
	var text strings.Builder
	typ := reflect.TypeOf(createInput{})
	for _, name := range []string{"Content", "Deck", "Graph"} {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("createInput has no %s field", name)
		}
		text.WriteString(f.Tag.Get("jsonschema") + "\n")
	}
	rest := text.String()
	for _, phrase := range append(fromTable, fromElsewhere...) {
		if !strings.Contains(rest, phrase) {
			t.Errorf("schema text lacks %q", phrase)
		}
		rest = strings.ReplaceAll(rest, phrase, "")
	}
	for _, loc := range schemaNumber.FindAllStringIndex(rest, -1) {
		from, to := max(loc[0]-40, 0), min(loc[1]+20, len(rest))
		t.Errorf("number %s has no source: ...%s...", rest[loc[0]:loc[1]], rest[from:to])
	}
}
