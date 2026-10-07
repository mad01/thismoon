package render

import (
	"errors"
	"strings"
	"testing"
)

const storedPNG = "/img/3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f.png"

// An image renders as a webkit figure: the img with its alt and lazy
// loading, and the caption under it as fixated prose (MAD-375).
func TestRenderDocImageBlock(t *testing.T) {
	out := renderBlocks(t, Block{
		T: "image", Src: "https://example.com/a.png", Alt: `Error rate "peaking" <at> 14:15`,
		Caption: "The dashboard at **14:15**.",
	})
	for _, want := range []string{
		"<wk-figure>",
		`<img src="https://example.com/a.png" alt="Error rate &#34;peaking&#34; &lt;at&gt; 14:15" loading="lazy">`,
		`<wk-figcaption data-fixation>The dashboard at <strong>14:15</strong>.</wk-figcaption>`,
		"</wk-figure>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	out = renderBlocks(t, Block{T: "image", Src: storedPNG, Alt: "a"})
	if !strings.Contains(out, `<img src="`+storedPNG+`" alt="a" loading="lazy">`) {
		t.Errorf("stored path not rendered as given: %s", out)
	}
	if strings.Contains(out, "wk-figcaption") {
		t.Errorf("image without caption rendered one: %s", out)
	}
	// The caption is the figure's only fixated text: title, heading, caption.
	out = renderBlocks(t, Block{T: "image", Src: storedPNG, Alt: "a", Caption: "c"})
	if n := strings.Count(out, "data-fixation"); n != 3 {
		t.Errorf("data-fixation count = %d, want 3 (the image itself is skipped)", n)
	}
}

// An image goes inside a column and inside a details block alike.
func TestRenderDocImageNested(t *testing.T) {
	img := Block{T: "image", Src: storedPNG, Alt: "a", Caption: "c"}
	out := renderBlocks(
		t,
		Block{T: "columns", Columns: [][]Block{{img}, {{T: "p", Text: "beside it"}}}},
		Block{T: "details", Summary: "More", Blocks: []Block{img}},
	)
	if n := strings.Count(out, "<wk-figure>"); n != 2 {
		t.Errorf("figures = %d, want 2: %s", n, out)
	}
}

// Alt and caption are prose and normalise like any other text field.
func TestRenderDocImageNormalizes(t *testing.T) {
	out := renderBlocks(t, Block{T: "image", Src: storedPNG, Alt: "A → B & C", Caption: "x ≈ y"})
	for _, want := range []string{`alt="A to B and C"`, "x approximately y"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestRenderDocImageValidation(t *testing.T) {
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{"no alt", Block{T: "image", Src: storedPNG}, "alt is required"},
		{"blank alt", Block{T: "image", Src: storedPNG, Alt: "  "}, "alt is required"},
		{"no src", Block{T: "image", Alt: "a"}, "src must be"},
		{"file path", Block{T: "image", Src: "/Users/me/shot.png", Alt: "a"}, "src must be"},
		{"home path", Block{T: "image", Src: "~/shot.png", Alt: "a"}, "src must be"},
		{"relative", Block{T: "image", Src: "shot.png", Alt: "a"}, "src must be"},
		{"file url", Block{T: "image", Src: "file:///tmp/shot.png", Alt: "a"}, "src must be"},
		{"data url", Block{T: "image", Src: "data:image/png;base64,AAAA", Alt: "a"}, "src must be"},
		{"javascript", Block{T: "image", Src: "javascript:alert(1)", Alt: "a"}, "src must be"},
		{"no host", Block{T: "image", Src: "https:///a.png", Alt: "a"}, "src must be"},
		{
			"stored svg",
			Block{T: "image", Src: strings.TrimSuffix(storedPNG, "png") + "svg", Alt: "a"},
			"src must be",
		},
		{"short hash", Block{T: "image", Src: "/img/abc.png", Alt: "a"}, "src must be"},
		{"traversal", Block{T: "image", Src: "/img/../pages/x.png", Alt: "a"}, "src must be"},
		{"other prefix", Block{T: "image", Src: "/assets/a.png", Alt: "a"}, "src must be"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RenderDoc(
				Doc{Sections: []Section{{Heading: "S", Blocks: []Block{c.block}}}},
				"T",
			)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
	for _, src := range []string{
		"https://example.com/a.png", "http://example.com/a?x=1&y=2", "HTTPS://example.com/a.png",
		storedPNG, strings.TrimSuffix(storedPNG, "png") + "jpg", strings.TrimSuffix(storedPNG, "png") + "jpeg",
		strings.TrimSuffix(storedPNG, "png") + "gif", strings.TrimSuffix(storedPNG, "png") + "webp",
	} {
		if _, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: []Block{{T: "image", Src: src, Alt: "a"}}}}}, "T"); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

// The image fields ride through the canonical JSON the store keeps.
func TestCompileKeepsImage(t *testing.T) {
	src := `{"sections":[{"h":"S","blocks":[{"t":"image","src":"` + storedPNG + `","alt":"a","caption":"c"}]}]}`
	c, err := Compile([]byte(src), "T")
	if err != nil {
		t.Fatal(err)
	}
	if string(c.JSON) != src {
		t.Errorf("canonical JSON = %s", c.JSON)
	}
}

// EachBlock visits every block, the nested ones included, in document
// order, and the first error stops it.
func TestDocEachBlock(t *testing.T) {
	d := Doc{Sections: []Section{
		{Heading: "S", Blocks: []Block{
			{T: "p", Text: "1"},
			{
				T:       "columns",
				Columns: [][]Block{{{T: "p", Text: "2"}}, {{T: "image", Src: "x", Alt: "3"}}},
			},
		}},
		{
			Heading: "U",
			Blocks:  []Block{{T: "details", Summary: "d", Blocks: []Block{{T: "p", Text: "4"}}}},
		},
	}}
	var seen []string
	err := d.EachBlock(func(b *Block) error {
		seen = append(seen, b.T)
		if b.T == "image" {
			b.Src = "rewritten"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(seen, " "); got != "p columns p image details p" {
		t.Errorf("visited %q", got)
	}
	if d.Sections[0].Blocks[1].Columns[1][0].Src != "rewritten" {
		t.Error("EachBlock did not hand out the Doc's own blocks")
	}
	stop := errors.New("stop")
	n := 0
	err = d.EachBlock(func(b *Block) error {
		n++
		if b.T == "columns" {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || n != 2 {
		t.Errorf("err = %v after %d blocks, want stop after 2", err, n)
	}
}

// TestRenderImageFrame writes frame as given on the figure, so the
// stylesheet can drop the border on false, and nothing when it is unset.
func TestRenderImageFrame(t *testing.T) {
	no := false
	out := renderBlocks(t,
		Block{T: "image", Src: "https://example.com/a.png", Alt: "a", Frame: &no},
		Block{T: "image", Src: "https://example.com/b.png", Alt: "b"},
	)
	if !strings.Contains(out, `<wk-figure data-frame="false">`) {
		t.Errorf("output lacks the frameless figure:\n%s", out)
	}
	if n := strings.Count(out, "<wk-figure>"); n != 1 {
		t.Errorf("plain figures = %d, want 1", n)
	}
}
