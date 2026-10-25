package store

import "testing"

// TestOfferTitle: a name outranks a generated title, within one rank the
// first title stays, an empty candidate changes nothing, and an empty
// source, offered or stored, ranks as generated.
func TestOfferTitle(t *testing.T) {
	tests := []struct {
		name        string
		held        string
		heldSource  TitleSource
		candidate   string
		src         TitleSource
		want        string
		wantSource  TitleSource
		wantChanged bool
	}{
		{"fills an empty title", "", "", "a", TitleGenerated, "a", TitleGenerated, true},
		{"empty source fills as generated", "", "", "a", "", "a", TitleGenerated, true},
		{"empty candidate", "a", TitleGenerated, "", TitleNamed, "a", TitleGenerated, false},
		{
			"first generated stays",
			"a",
			TitleGenerated,
			"b",
			TitleGenerated,
			"a",
			TitleGenerated,
			false,
		},
		{"name outranks generated", "a", TitleGenerated, "b", TitleNamed, "b", TitleNamed, true},
		{"first name stays", "a", TitleNamed, "b", TitleNamed, "a", TitleNamed, false},
		{
			"generated never replaces a name",
			"a",
			TitleNamed,
			"b",
			TitleGenerated,
			"a",
			TitleNamed,
			false,
		},
		{
			"stored without a source ranks generated",
			"a",
			"",
			"b",
			TitleNamed,
			"b",
			TitleNamed,
			true,
		},
		{"stored without a source stays", "a", "", "b", TitleGenerated, "a", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, source := tc.held, tc.heldSource
			changed := OfferTitle(&title, &source, tc.candidate, tc.src)
			if title != tc.want || source != tc.wantSource || changed != tc.wantChanged {
				t.Errorf("OfferTitle = %q from %q, changed %v; want %q from %q, changed %v",
					title, source, changed, tc.want, tc.wantSource, tc.wantChanged)
			}
		})
	}
}
