package render

import (
	"encoding/json"
	"strings"
	"testing"
)

// A section's id is accepted and ignored since MAD-377 removed the badge:
// a stored doc.json or deck.json, and an MCP caller, may still carry one,
// and neither rendition shows it. The heading and the TOC entry are the
// bare heading text in the brief and in the deck alike.
func TestSectionIDAcceptedAndIgnored(t *testing.T) {
	src := `{"sections":[
		{"h":"First Item","id":"R001","blocks":[{"t":"p","text":"one"}]},
		{"h":"Second Item","id":"R002","blocks":[{"t":"p","text":"two"}]}]}`
	var doc Doc
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("a Doc with section ids must still parse: %v", err)
	}

	renditions := []struct {
		name   string
		render func(Doc, string) (string, error)
	}{
		{"brief", RenderDoc},
		{"deck", RenderDeck},
	}
	for _, r := range renditions {
		t.Run(r.name, func(t *testing.T) {
			out, err := r.render(doc, "Test Page")
			if err != nil {
				t.Fatal(err)
			}
			for _, gone := range []string{"wk-section-id", "R001", "R002"} {
				if strings.Contains(out, gone) {
					t.Errorf("%s rendition shows %q:\n%s", r.name, gone, out)
				}
			}
			for _, want := range []string{
				`<wk-section-heading data-fixation>First Item</wk-section-heading>`,
				`<a href="#first-item">First Item</a>`,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("%s rendition missing %q in:\n%s", r.name, want, out)
				}
			}
		})
	}
}
