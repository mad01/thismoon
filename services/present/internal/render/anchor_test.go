package render

import (
	"encoding/json"
	"regexp"
	"testing"
)

var reSectionID = regexp.MustCompile(`<wk-section id="([^"]*)"`)

// TestSectionAnchorIsTheRenderedID pins the contract section patches and the
// changed-section marker rely on: the id a heading renders under equals
// SectionAnchor of the heading as sent and of the heading as the compiled
// Doc stores it. Headings with repeated operators broke this while name
// normalization took one pass.
func TestSectionAnchorIsTheRenderedID(t *testing.T) {
	cases := []struct{ heading, want string }{
		{"A - - B", "a-minus-minus-b"},
		{"x & & y", "x-and-and-y"},
		{"C + + D", "c-plus-plus-d"},
		{"a = = b", "a-equals-equals-b"},
		{"- - start", "minus-minus-start"},
		{"Plan → Build → Ship", "plan-to-build-to-ship"},
		{"In ← Out ↔ Both", "in-from-out-between-both"},
		{"Café au lait", "caf-au-lait"},
		{"C++ tips", "c-tips"},
		{"Don't panic", "dont-panic"},
		{"Release 1.2.3", "release-123"},
		{"snake_case name", "snake-case-name"},
		{"JIRA board", "jira-board"},
		{"设计 概览", ""}, // no ASCII letter or digit: addressed by index only
	}
	for _, c := range cases {
		t.Run(c.heading, func(t *testing.T) {
			stored := compileOneSection(t, c.heading)
			rendered := reSectionID.FindStringSubmatch(stored.html)
			if rendered == nil {
				t.Fatalf("no wk-section id in:\n%s", stored.html)
			}
			if rendered[1] != c.want {
				t.Errorf("rendered id = %q, want %q", rendered[1], c.want)
			}
			if got := SectionAnchor(c.heading); got != c.want {
				t.Errorf("SectionAnchor(heading as sent) = %q, want %q", got, c.want)
			}
			if got := SectionAnchor(stored.heading); got != c.want {
				t.Errorf("SectionAnchor(heading as stored, %q) = %q, want %q",
					stored.heading, got, c.want)
			}
			if again := NormalizeNames(stored.heading); again != stored.heading {
				t.Errorf("normalizing the stored heading again changed it: %q to %q",
					stored.heading, again)
			}
		})
	}
}

type compiledSection struct{ html, heading string }

// compileOneSection compiles a Doc holding one section with heading and
// returns the brief HTML and the heading its canonical JSON stores.
func compileOneSection(t *testing.T, heading string) compiledSection {
	t.Helper()
	doc := Doc{Sections: []Section{{Heading: heading, Blocks: []Block{{T: "p", Text: "x"}}}}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(raw, "T")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var stored Doc
	if err := json.Unmarshal(c.JSON, &stored); err != nil {
		t.Fatalf("decode the canonical JSON: %v", err)
	}
	return compiledSection{html: c.HTML, heading: stored.Sections[0].Heading}
}
