package docedit_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/render"
)

func docJSON(t *testing.T, sections ...render.Section) []byte {
	t.Helper()
	raw, err := json.Marshal(render.Doc{Sections: sections})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDiffSectionsNamesChangedAddedRemoved(t *testing.T) {
	edited := section("Beta")
	edited.Blocks[0].Text = "rewritten"
	cases := []struct {
		name          string
		before, after []byte
		want          docedit.SectionDiff
	}{
		{
			name:   "change, add, remove",
			before: docJSON(t, section("Alpha"), section("Beta"), section("Gamma")),
			after:  docJSON(t, section("Alpha"), edited, section("Delta")),
			want: docedit.SectionDiff{
				Changed: []string{"beta"}, Added: []string{"delta"}, Removed: []string{"gamma"},
			},
		},
		{
			name:   "a move is not a change",
			before: docJSON(t, section("Alpha"), section("Beta")),
			after:  docJSON(t, section("Beta"), section("Alpha")),
			want: docedit.SectionDiff{
				Changed: []string{},
				Added:   []string{},
				Removed: []string{},
			},
		},
		{
			name:   "a rendition that appeared",
			before: nil,
			after:  docJSON(t, section("Slide")),
			want: docedit.SectionDiff{
				Changed: []string{},
				Added:   []string{"slide"},
				Removed: []string{},
			},
		},
		{
			// Once one Dup is gone the other has a unique anchor again, so it
			// reads as a new name and both indexed ones as removed.
			name:   "shared and empty anchors go by index",
			before: docJSON(t, section("Dup"), section("Dup")),
			after:  docJSON(t, section("Dup"), edited, section("日本語")),
			want: docedit.SectionDiff{
				Changed: []string{},
				Added:   []string{"dup", "beta", "index:2"},
				Removed: []string{"index:0", "index:1"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := docedit.DiffSections(tc.before, tc.after)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DiffSections = %+v, want %+v", got, tc.want)
			}
		})
	}
	if _, err := docedit.DiffSections([]byte(`{"sections":`), nil); err == nil {
		t.Error("a malformed Doc must be an error")
	}
}
