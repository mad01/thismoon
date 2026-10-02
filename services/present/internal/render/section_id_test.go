package render

import (
	"strings"
	"testing"
)

func TestSectionIDRender(t *testing.T) {
	doc := Doc{
		Summary: "Test with section IDs",
		Sections: []Section{
			{Heading: "First Item", ID: "A1", Blocks: []Block{{T: "p", Text: "Content one"}}},
			{Heading: "Second Item", ID: "A2", Blocks: []Block{{T: "p", Text: "Content two"}}},
			{Heading: "No ID Section", Blocks: []Block{{T: "p", Text: "No id here"}}},
		},
	}
	html, err := RenderDoc(doc, "Test Page")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(html, `<wk-section-id>A1</wk-section-id>First Item`) {
		t.Error("section heading should contain ID badge for A1")
	}
	if !strings.Contains(html, `<wk-section-id>A2</wk-section-id>Second Item`) {
		t.Error("section heading should contain ID badge for A2")
	}
	if strings.Contains(html, `<wk-section-id></wk-section-id>No ID Section`) {
		t.Error("section without ID should not render empty badge")
	}
	// TOC should also have ID badges
	if !strings.Contains(html, `<wk-section-id>A1</wk-section-id>First Item</a>`) {
		t.Error("TOC should contain ID badge for A1")
	}
}

// The deck never shows a section's id: a badge in a slide heading tells the
// room nothing. The brief keeps it by the heading and in the TOC, which
// TestSectionIDRender pins.
func TestSectionIDOmittedFromDeck(t *testing.T) {
	doc := Doc{
		Sections: []Section{
			{Heading: "First Item", ID: "A1", Blocks: []Block{{T: "p", Text: "Content one"}}},
			{Heading: "Second Item", ID: "A2", Blocks: []Block{{T: "p", Text: "Content two"}}},
		},
	}
	html, err := RenderDeck(doc, "Test Page")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<wk-section-id>") {
		t.Errorf("deck rendition carries a section id badge:\n%s", html)
	}
	if !strings.Contains(
		html,
		`<wk-section-heading data-fixation>First Item</wk-section-heading>`,
	) {
		t.Error("deck slide heading should be the bare heading")
	}
	if !strings.Contains(html, `<a href="#first-item">First Item</a>`) {
		t.Error("deck TOC entry should be the bare heading")
	}
}
