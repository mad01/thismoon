package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func renderBlocks(t *testing.T, blocks ...Block) string {
	t.Helper()
	out, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: blocks}}}, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	return out
}

func TestRenderDocColumnsBlock(t *testing.T) {
	out := renderBlocks(t, Block{T: "columns", Columns: [][]Block{
		{{T: "p", Text: "left"}, {T: "stat", Value: "41 min", Label: "outage"}},
		{{T: "chart", Kind: "bar", Series: []ChartSeries{{Points: []ChartPoint{{X: "a", Y: 1}}}}}},
		{{T: "list", Items: []string{"one"}}},
	}})
	for _, want := range []string{
		`<wk-columns cols="3">`,
		`<p data-fixation>left</p>`,
		`<wk-stat-value>41 min</wk-stat-value>`,
		`class="present-chart"`,
		`<li>one</li>`,
		`</wk-columns>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if n := strings.Count(out, "<wk-col>"); n != 3 {
		t.Errorf("wk-col count = %d, want 3", n)
	}
}

func TestRenderDocStatBlock(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "stat", Value: "41 min", Label: "checkout **outage**", Subtitle: "14:02 to 14:43"},
	)
	for _, want := range []string{
		`<wk-stat>`,
		`<wk-stat-value>41 min</wk-stat-value>`,
		`<wk-stat-label data-fixation>checkout <strong>outage</strong></wk-stat-label>`,
		`<wk-stat-sub data-fixation>14:02 to 14:43</wk-stat-sub>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	// The value is a figure: escaped, never fixated, no inline markdown.
	out = renderBlocks(t, Block{T: "stat", Value: "<3 **x**", Label: "l"})
	if !strings.Contains(out, `<wk-stat-value>&lt;3 **x**</wk-stat-value>`) {
		t.Errorf("stat value not escaped verbatim: %s", out)
	}
	if strings.Contains(out, "wk-stat-sub") {
		t.Errorf("stat without sub rendered a sub line: %s", out)
	}
}

func TestRenderDocQuoteBlock(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "quote", Text: "We never saw the resolver.", Cite: "On-call, retro"},
	)
	for _, want := range []string{
		"<blockquote>",
		`<p data-fixation>We never saw the resolver.</p>`,
		`<cite>On-call, retro</cite>`,
		"</blockquote>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if out := renderBlocks(t, Block{T: "quote", Text: "q"}); strings.Contains(out, "<cite>") {
		t.Errorf("quote without cite rendered one: %s", out)
	}
}

func TestRenderDocDetailsBlock(t *testing.T) {
	out := renderBlocks(t, Block{T: "details", Summary: "Full timeline", Blocks: []Block{
		{T: "list", Items: []string{"14:02 first 502s"}},
		{T: "callout", Severity: "ok", Text: "resolved"},
	}})
	for _, want := range []string{
		"<details>",
		`<summary data-fixation>Full timeline</summary>`,
		`<li>14:02 first 502s</li>`,
		`<wk-callout variant="ok" data-fixation>resolved</wk-callout>`,
		"</details>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if strings.Contains(out, "<details open") {
		t.Error("details must render closed")
	}
}

func TestRenderDocCalloutSeverities(t *testing.T) {
	for _, sev := range []string{"info", "warn", "ok", "error"} {
		out := renderBlocks(t, Block{T: "callout", Severity: sev, Text: "x"})
		if !strings.Contains(out, `<wk-callout variant="`+sev+`" data-fixation>`) {
			t.Errorf("sev %q: %s", sev, out)
		}
	}
}

// Name normalization reaches the blocks a container holds and the new
// text fields, and still leaves a nested code block alone.
func TestRenderDocNormalizesNestedBlocks(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "columns", Columns: [][]Block{
			{{T: "p", Text: "a & b"}, {T: "code", Text: "a & b"}},
			{{T: "stat", Value: "A → B", Label: "c & d", Subtitle: "e & f"}},
		}},
		Block{
			T:       "details",
			Summary: "g & h",
			Blocks:  []Block{{T: "quote", Text: "i & j", Cite: "k & l"}},
		},
	)
	for _, want := range []string{
		"a and b", "<code class=\"language-text\">a &amp; b</code>",
		"<wk-stat-value>A → B</wk-stat-value>", "c and d", "e and f", "g and h", "i and j", "k and l",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestRenderDocBlockValidation(t *testing.T) {
	one := []Block{{T: "p", Text: "x"}}
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{"one column", Block{T: "columns", Columns: [][]Block{one}}, "want 2 or 3 columns"},
		{
			"four columns",
			Block{T: "columns", Columns: [][]Block{one, one, one, one}},
			"want 2 or 3 columns",
		},
		{"no columns", Block{T: "columns"}, "want 2 or 3 columns"},
		{
			"nested columns",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "columns", Columns: [][]Block{one, one}}}},
			},
			"columns block: not allowed inside",
		},
		{
			"details in columns",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "details", Summary: "s", Blocks: one}}},
			},
			"details block: not allowed inside",
		},
		{
			"graph in columns",
			Block{T: "columns", Columns: [][]Block{one, {{T: "graph"}}}},
			"graph block: not allowed inside",
		},
		{"empty details", Block{T: "details", Summary: "s"}, "blocks is empty"},
		{"details without summary", Block{T: "details", Blocks: one}, "summary is required"},
		{
			"nested details",
			Block{
				T:       "details",
				Summary: "s",
				Blocks:  []Block{{T: "details", Summary: "t", Blocks: one}},
			},
			"details block: not allowed inside",
		},
		{
			"graph in details",
			Block{T: "details", Summary: "s", Blocks: []Block{{T: "graph"}}},
			"graph block: not allowed inside",
		},
		{
			"panel accent in a column",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "panel", Title: "P", Accent: "wg600"}}},
			},
			`unknown accent "wg600"`,
		},
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
}

// A container built in Go that skipped validation stops at the depth guard
// instead of recursing.
func TestRenderBlockAtDepthGuard(t *testing.T) {
	deep := Block{T: "columns", Columns: [][]Block{{{T: "p", Text: "x"}}, {{T: "p", Text: "y"}}}}
	if out := string(renderBlockAt(deep, 1)); !strings.Contains(
		out,
		"render error: columns block nested too deep",
	) {
		t.Errorf("container at depth 1: %s", out)
	}
	if out := string(renderBlockAt(Block{T: "p", Text: "x"}, 2)); !strings.Contains(
		out,
		"nested too deep",
	) {
		t.Errorf("block past max depth: %s", out)
	}
}

// cols is one wire key for two shapes: a table's header strings and a
// columns block's arrays of blocks. Both round-trip through the canonical
// JSON, and a table keeps the key where it was.
func TestBlockColsJSONRoundTrip(t *testing.T) {
	src := `{"sections":[{"h":"S","blocks":[` +
		`{"t":"table","cols":["A","B"],"rows":[["1","2"]]},` +
		`{"t":"columns","cols":[[{"t":"p","text":"l"}],[{"t":"stat","value":"1","label":"x"}]]}` +
		`]}]}`
	c, err := Compile([]byte(src), "T")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	blocks := c.Doc.Sections[0].Blocks
	if got := blocks[0].Cols; len(got) != 2 || got[0] != "A" {
		t.Errorf("table cols = %v", got)
	}
	if blocks[0].Columns != nil {
		t.Errorf("table decoded Columns: %v", blocks[0].Columns)
	}
	if got := blocks[1].Columns; len(got) != 2 || got[1][0].T != "stat" || got[1][0].Value != "1" {
		t.Errorf("columns = %+v", got)
	}
	if blocks[1].Cols != nil {
		t.Errorf("columns block decoded Cols: %v", blocks[1].Cols)
	}
	if !strings.Contains(string(c.JSON), `{"t":"table","cols":["A","B"],"rows":[["1","2"]]}`) {
		t.Errorf("table JSON moved: %s", c.JSON)
	}
	if !strings.Contains(
		string(c.JSON),
		`{"t":"columns","cols":[[{"t":"p","text":"l"}],[{"t":"stat","value":"1","label":"x"}]]}`,
	) {
		t.Errorf("columns JSON: %s", c.JSON)
	}
	var again Doc
	if err := json.Unmarshal(c.JSON, &again); err != nil {
		t.Fatalf("re-parse canonical JSON: %v", err)
	}
	if again.Sections[0].Blocks[1].Columns[0][0].Text != "l" {
		t.Errorf("canonical JSON lost the columns: %s", c.JSON)
	}
	if _, err := Compile([]byte(`{"sections":[{"h":"S","blocks":[{"t":"table","cols":[["x"]]}]}]}`), "T"); err == nil {
		t.Error("table with nested cols accepted")
	}
	if _, err := Compile([]byte(`{"sections":[{"h":"S","blocks":[{"t":"columns","cols":["x","y"]}]}]}`), "T"); err == nil {
		t.Error("columns with string cols accepted")
	}
}

func TestRenderDocDeckChrome(t *testing.T) {
	plain, err := RenderDoc(
		Doc{Meta: "m", Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}},
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "deck-chrome") || strings.Contains(plain, "brief-presenter") {
		t.Errorf("chrome emitted for a Doc without chrome fields: %s", plain)
	}
	if !strings.HasPrefix(plain, "<h1 ") {
		t.Errorf("fragment without chrome must start with the title: %q", plain[:40])
	}

	out, err := RenderDoc(Doc{
		Meta:         "2026-10-01 · review",
		Presenter:    "Alex, platform",
		Footer:       "Incident review",
		Logo:         "https://example.com/mark.png",
		LogoPosition: "top-right",
		Progress:     "bar",
		Sections:     []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
	}, "T")
	if err != nil {
		t.Fatal(err)
	}
	want := `<script type="application/json" class="deck-chrome">` +
		`{"logo":"https://example.com/mark.png","logo_position":"top-right","progress":"bar","presenter":"Alex, platform","footer":"Incident review"}` +
		`</script>` + "\n<h1 "
	if !strings.HasPrefix(out, want) {
		t.Errorf("chrome island:\n got %q\nwant prefix %q", out, want)
	}
	if !strings.Contains(
		out,
		"<div class=\"brief-meta\">2026-10-01 · review</div>\n<div class=\"brief-presenter\">Alex, platform</div>",
	) {
		t.Errorf("presenter line not under the meta line: %s", out)
	}
	if strings.Count(out, "data-fixation") != 3 {
		t.Errorf("presenter line must stay out of the fixation walk: %s", out)
	}

	// One field alone is enough for the island, and only that key is written.
	out, err = RenderDoc(
		Doc{
			Logo:     "none",
			Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
		},
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(
		out,
		`<script type="application/json" class="deck-chrome">{"logo":"none"}</script>`,
	) {
		t.Errorf("logo none island: %s", out)
	}
}

// A chrome string must never break out of its script island.
func TestRenderDocDeckChromeScriptSafe(t *testing.T) {
	out, err := RenderDoc(Doc{
		Footer:   "</script><script>alert(1)</script>",
		Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
	}, "T")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "<script") != 1 || strings.Contains(out, "<script>alert") {
		t.Errorf("footer broke out of the island: %s", out)
	}
}

func TestRenderDocDeckChromeValidation(t *testing.T) {
	cases := []struct {
		name string
		doc  Doc
		want string
	}{
		{"position", Doc{LogoPosition: "middle"}, `logo_position "middle"`},
		{"progress", Doc{Progress: "ring"}, `progress "ring"`},
		{"logo scheme", Doc{Logo: "javascript:alert(1)"}, `logo "javascript:alert(1)"`},
		{"logo relative", Doc{Logo: "/assets/x.png"}, `logo "/assets/x.png"`},
		{"logo data", Doc{Logo: "data:image/png;base64,AAAA"}, "logo "},
		{"logo no host", Doc{Logo: "https:///x.png"}, "logo "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.doc.Sections = []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}
			_, err := RenderDoc(c.doc, "T")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
	for _, ok := range []Doc{
		{Logo: "none"},
		{Logo: "http://example.com/a.png"},
		{Logo: "HTTPS://example.com/a.png"},
		{LogoPosition: "bottom-left"},
		{LogoPosition: "top-left"},
		{Progress: "dots"},
		{Progress: "none"},
	} {
		ok.Sections = []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}
		if _, err := RenderDoc(ok, "T"); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
}

// The chrome fields ride through Compile's re-marshal, so a stored deck
// keeps them, and a Doc without them marshals as before.
func TestCompileKeepsDeckChrome(t *testing.T) {
	c, err := Compile(
		[]byte(`{"logo":"none","progress":"bar","presenter":"A","footer":"F","sections":[]}`),
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.JSON); got != `{"sections":[],"logo":"none","progress":"bar","presenter":"A","footer":"F"}` {
		t.Errorf("canonical JSON = %s", got)
	}
	if c.Doc.Logo != "none" || c.Doc.Progress != "bar" {
		t.Errorf("doc = %+v", c.Doc)
	}
	if _, err := Compile([]byte(`{"progress":"ring","sections":[]}`), "T"); err == nil {
		t.Error("Compile accepted an unknown progress value")
	}
}

// A stat's value is shown verbatim: the symbols a figure carries are the
// point. The label and the sub line are prose and normalise like any other.
func TestRenderDocStatValueVerbatim(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "stat", Value: "4×", Label: "faster & cheaper", Subtitle: "A → B"},
		Block{T: "stat", Value: "≈ 40%", Label: "of requests"},
	)
	for _, want := range []string{
		`<wk-stat-value>4×</wk-stat-value>`, `<wk-stat-value>≈ 40%</wk-stat-value>`,
		`faster and cheaper`, `<wk-stat-sub data-fixation>A to B</wk-stat-sub>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	for _, bad := range []string{"4times", "approximately 40%"} {
		if strings.Contains(out, bad) {
			t.Errorf("stat value was normalised to %q: %s", bad, out)
		}
	}
}
