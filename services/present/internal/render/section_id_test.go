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
