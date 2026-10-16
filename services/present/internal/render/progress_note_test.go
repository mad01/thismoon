package render

import (
	"strings"
	"testing"
)

// A progress note follows the label inside the existing label element; a
// block without one renders as before, and a note anywhere else is refused.
func TestProgressNoteFollowsTheLabel(t *testing.T) {
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{
			"label and note",
			Block{T: "progress", Percent: 60, Label: "Index", Note: "12 of 20"},
			`<wk-progress-label>Index: 12 of 20</wk-progress-label>`,
		},
		{
			"note alone",
			Block{T: "progress", Percent: 60, Note: "12 of 20"},
			`<wk-progress-label>12 of 20</wk-progress-label>`,
		},
		{
			"label alone",
			Block{T: "progress", Percent: 60, Label: "Index"},
			"</wk-progress-bar>\n  <wk-progress-label>Index</wk-progress-label>\n</wk-progress>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := RenderDoc(
				Doc{Sections: []Section{{Heading: "S", Blocks: []Block{tc.block}}}},
				"T",
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("rendered progress lacks %q:\n%s", tc.want, out)
			}
		})
	}
	_, err := RenderDoc(
		Doc{Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x", Note: "n"}}}}},
		"T",
	)
	if err == nil || !strings.Contains(err.Error(), "note is a progress field") {
		t.Errorf("note on a p block: err = %v, want it refused", err)
	}
}
