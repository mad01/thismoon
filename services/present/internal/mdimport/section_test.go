package mdimport

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

func TestConvertSectionMapsOneSection(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want render.Section
	}{
		{
			name: "level-2 heading with body",
			src:  "## Results\n\nShipped **it**.\n\n### Detail\n\n- one\n- two\n",
			want: render.Section{Heading: "Results", Blocks: []render.Block{
				{T: "p", Text: "Shipped **it**."},
				{T: "h3", Text: "Detail"},
				{T: "list", Items: []string{"one", "two"}},
			}},
		},
		{
			name: "level-1 heading names it too",
			src:  "# Results\n\ntext\n",
			want: render.Section{
				Heading: "Results",
				Blocks:  []render.Block{{T: "p", Text: "text"}},
			},
		},
		{
			name: "heading alone",
			src:  "## Empty\n",
			want: render.Section{Heading: "Empty", Blocks: []render.Block{}},
		},
		{
			name: "a mermaid flowchart stays code",
			src:  "## Flow\n\n```mermaid\ngraph TD\n  A --> B\n```\n",
			want: render.Section{Heading: "Flow", Blocks: []render.Block{
				{T: "code", Lang: "mermaid", Text: "graph TD\n  A --> B"},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertSection([]byte(tc.src))
			if err != nil {
				t.Fatalf("ConvertSection: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ConvertSection =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestConvertSectionRefusesWhatIsNotOneSection(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want error
	}{
		{"no heading", "just text\n", ErrNoHeading},
		{"text before the heading", "intro\n\n## Results\n", ErrNoHeading},
		{"a level-3 heading first", "### Detail\n\ntext\n", ErrNoHeading},
		{"an empty heading", "##\n\ntext\n", ErrNoHeading},
		{"two sections", "## One\n\na\n\n## Two\n\nb\n", ErrManySections},
		{"a later level-1 heading", "## One\n\na\n\n# Two\n", ErrManySections},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ConvertSection([]byte(tc.src)); !errors.Is(err, tc.want) {
				t.Errorf("ConvertSection(%q): err = %v, want %v", tc.src, err, tc.want)
			}
		})
	}
}
