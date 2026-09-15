package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInlineMdBold(t *testing.T) {
	got := string(inlineMd("hello **world** end"))
	if !strings.Contains(got, "<strong>world</strong>") {
		t.Errorf("bold not rendered: %s", got)
	}
	if strings.Contains(got, "**") {
		t.Errorf("raw ** still present: %s", got)
	}
}

func TestInlineMdCode(t *testing.T) {
	got := string(inlineMd("use `fmt.Println` here"))
	if !strings.Contains(got, "<code>fmt.Println</code>") {
		t.Errorf("code not rendered: %s", got)
	}
}

func TestInlineMdLink(t *testing.T) {
	got := string(inlineMd("see [Go docs](https://pkg.go.dev)"))
	if !strings.Contains(got, `<a href="https://pkg.go.dev">Go docs</a>`) {
		t.Errorf("link not rendered: %s", got)
	}
}

func TestInlineMdChip(t *testing.T) {
	got := string(inlineMd("status: @chip(b:done) ok"))
	if !strings.Contains(got, `<wk-badge variant="b">done</wk-badge>`) {
		t.Errorf("chip not rendered: %s", got)
	}
}

func TestInlineMdItalic(t *testing.T) {
	got := string(inlineMd("this is *important* text"))
	if !strings.Contains(got, "<em>important</em>") {
		t.Errorf("italic not rendered: %s", got)
	}
}

func TestInlineMdCombined(t *testing.T) {
	got := string(inlineMd("**Status:** `running` on [prod](https://example.com) @chip(a:live)"))
	checks := []string{
		"<strong>Status:</strong>",
		"<code>running</code>",
		`<a href="https://example.com">prod</a>`,
		`<wk-badge variant="a">live</wk-badge>`,
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in: %s", want, got)
		}
	}
}

func TestInlineMdHTMLEscaping(t *testing.T) {
	got := string(inlineMd("a <script>alert(1)</script> & b"))
	if strings.Contains(got, "<script>") {
		t.Errorf("script not escaped: %s", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("ampersand not escaped: %s", got)
	}
}

func TestInlineMdChipHTMLEscaping(t *testing.T) {
	got := string(inlineMd(`@chip(a:<script>)`))
	if strings.Contains(got, "<script>") {
		t.Errorf("chip text not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("chip should contain escaped text: %s", got)
	}
	if !strings.Contains(got, `<wk-badge variant="a">`) {
		t.Errorf("chip should render as wk-badge: %s", got)
	}
}

func TestNormalizeNames(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Acronym names.
		{"JIRA-1234 is the ticket", "jira-1234 is the ticket"},
		{"the JIRA board", "the jira board"},
		{"JIRAFFE stays", "JIRAFFE stays"},
		{"no change here", "no change here"},
		{"JIRA", "jira"},
		{"JIRA-99 and JIRA-100", "jira-99 and jira-100"},
		{"JIRA ticket", "jira ticket"},
		{"JIRA", "jira"},

		// Unicode symbols.
		{"A → B", "A to B"},
		{"A ← B", "A from B"},
		{"A ↔ B", "A between B"},
		{"3 × 4", "3 times 4"},
		{"10 ÷ 2", "10 divided by 2"},
		{"≈ 100", "approximately 100"},
		{"A ≠ B", "A not equal to B"},
		{"x ≤ 10", "x at most 10"},
		{"x ≥ 5", "x at least 5"},
		{"done ✓", "done yes"},
		{"done ✔", "done yes"},
		{"failed ✗", "failed no"},
		{"failed ✘", "failed no"},

		// Standalone ASCII operators.
		{"A & B", "A and B"},
		{"A + B", "A plus B"},
		{"A - B", "A minus B"},
		{"A = B", "A equals B"},
		{"& leading", "and leading"},
		{"+ leading", "plus leading"},

		// Must NOT mangle embedded operators.
		{"C++ language", "C++ language"},
		{"key=value", "key=value"},
		{"hyphen-word", "hyphen-word"},
		{"AT&T", "AT&T"},
	}
	for _, tt := range tests {
		got := normalizeNames(tt.in)
		if got != tt.want {
			t.Errorf("normalizeNames(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderDocNormalizesNames(t *testing.T) {
	doc := Doc{
		Summary: "JIRA-42 sprint summary",
		Chips:   []Chip{{Text: "JIRA"}},
		Sections: []Section{
			{
				Heading: "JIRA Sprint",
				Blocks: []Block{
					{T: "p", Text: "Ticket JIRA-123 is done"},
					{T: "kv", KV: []KVPair{{K: "Tracker", V: "JIRA"}}},
					{T: "table", Cols: []string{"TCK Ticket"}, Rows: [][]string{{"TCK-5"}}},
				},
			},
		},
	}
	out, err := RenderDoc(doc, "JIRA Report")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if strings.Contains(out, "JIRA") {
		t.Errorf("JIRA should be normalized to jira in output:\n%s", out)
	}
	if !strings.Contains(out, "jira Report") {
		t.Error("title not normalized")
	}
	if !strings.Contains(out, "jira-42") {
		t.Error("summary ticket not normalized")
	}
	if !strings.Contains(out, "jira-123") {
		t.Error("paragraph ticket not normalized")
	}
}

func TestSectionID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Overview", "overview"},
		{"1. First Section", "1-first-section"},
		{"Hello World!", "hello-world"},
		{"a--b", "a-b"},
	}
	for _, tt := range tests {
		got := sectionID(tt.in)
		if got != tt.want {
			t.Errorf("sectionID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderDocMinimal(t *testing.T) {
	doc := Doc{
		Sections: []Section{
			{Heading: "Test", Blocks: []Block{{T: "p", Text: "hello"}}},
		},
	}
	out, err := RenderDoc(doc, "Title")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if !strings.Contains(out, `<h1 class="brief-title">Title</h1>`) {
		t.Error("title not rendered")
	}
	if !strings.Contains(out, `data-fixation`) {
		t.Error("fixation attribute missing on paragraph")
	}
}

func TestRenderDocWithAllBlocks(t *testing.T) {
	doc := Doc{
		Summary: "sum",
		Meta:    "meta line",
		Chips:   []Chip{{Text: "tag", Style: "a"}},
		Sections: []Section{
			{
				Heading: "S1",
				Blocks: []Block{
					{T: "p", Text: "para"},
					{T: "h3", Text: "sub"},
					{T: "callout", Text: "note", Severity: "info"},
					{T: "table", Cols: []string{"A"}, Rows: [][]string{{"1"}}},
					{T: "kv", KV: []KVPair{{K: "k", V: "v"}}},
					{T: "list", Items: []string{"a", "b"}},
					{T: "panel", Title: "P", Subtitle: "s", Accent: "blue"},
					{T: "progress", Percent: 50, Label: "50%"},
					{T: "graph"},
					{T: "code", Text: "func main() {}", Lang: "go"},
					{T: "html", Text: "<custom/>"},
				},
			},
		},
	}
	out, err := RenderDoc(doc, "Full")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}

	checks := []struct {
		name string
		want string
	}{
		{"summary", `class="brief-summary"`},
		{"meta", `class="brief-meta"`},
		{"chip", `<wk-badge variant="a">tag</wk-badge>`},
		{"chip-row", `class="chip-row"`},
		{"section", `<wk-section`},
		{"section-heading", `<wk-section-heading>`},
		{"paragraph", `<p data-fixation>para</p>`},
		{"h3", `<wk-section-subheading>`},
		{"callout", `<wk-callout variant="info"`},
		{"table-wrap", `<wk-table><table>`},
		{"kv", `<wk-kv>`},
		{"kv-row", `<wk-kv-row>`},
		{"kv-label", `<wk-kv-label>`},
		{"kv-value", `<wk-kv-value>`},
		{"list", `class="brief-list"`},
		{"panel", `<wk-panel`},
		{"panel-title", `<wk-panel-title>`},
		{"panel-subtitle", `<wk-panel-subtitle>`},
		{"panel-accent", `var(--blue)`},
		{"progress", `<wk-progress>`},
		{"progress-bar", `<wk-progress-bar>`},
		{"progress-fill", `<wk-progress-fill style="width:50%"`},
		{"progress-label", `<wk-progress-label>`},
		{"graph", `id="cy-graph"`},
		{
			"code",
			`<pre class="wk-code-block"><code class="language-go">func main() {}</code></pre>`,
		},
		{"html-escape", `<custom/>`},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: missing %q in output", c.name, c.want)
		}
	}
}

func TestRenderDocTOC(t *testing.T) {
	doc := Doc{
		Sections: []Section{
			{Heading: "Alpha", ID: "A1", Blocks: []Block{{T: "p", Text: "x"}}},
			{Heading: "Beta", Blocks: []Block{{T: "p", Text: "y"}}},
		},
	}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	checks := []string{
		`<wk-toc>`,
		`<wk-toc-title>Sections</wk-toc-title>`,
		`<a href="#alpha">`,
		`<wk-section-id>A1</wk-section-id>`,
		`</wk-toc>`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("toc missing %q in: %s", want, out)
		}
	}
	// section-heading carries the section-id span too.
	if !strings.Contains(
		out,
		`<wk-section-heading><wk-section-id>A1</wk-section-id>Alpha</wk-section-heading>`,
	) {
		t.Errorf("section heading markup wrong: %s", out)
	}
}

func TestRenderDocCodeBlock(t *testing.T) {
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{
			"escapes html and skips inline markdown",
			Block{T: "code", Text: "if a < b && c > d { fmt.Println(`**raw**`) }", Lang: "go"},
			`<code class="language-go">if a &lt; b &amp;&amp; c &gt; d { fmt.Println(` + "`**raw**`" + `) }</code>`,
		},
		{
			"missing lang falls back to text",
			Block{T: "code", Text: "plain"},
			`<code class="language-text">plain</code>`,
		},
		{
			"lang is sanitized and lowercased",
			Block{T: "code", Text: "x", Lang: `Go"><script>`},
			`<code class="language-goscript">x</code>`,
		},
		{
			"multiline code keeps newlines",
			Block{T: "code", Text: "a\nb", Lang: "bash"},
			"<code class=\"language-bash\">a\nb</code>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := Doc{Sections: []Section{{Heading: "S", Blocks: []Block{c.block}}}}
			out, err := RenderDoc(doc, "T")
			if err != nil {
				t.Fatalf("RenderDoc: %v", err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("missing %q in: %s", c.want, out)
			}
		})
	}
}

func TestRenderDocChartBlock(t *testing.T) {
	doc := Doc{
		Sections: []Section{{
			Heading: "Latency",
			Blocks: []Block{{
				T:     "chart",
				Kind:  "bar",
				Title: "p95 latency",
				Unit:  "ms",
				Series: []ChartSeries{{
					Name:  "api",
					Color: "blue",
					Points: []ChartPoint{
						{X: "Mon", Y: 12},
						{X: "Tue", Y: 18.5},
					},
				}},
			}},
		}},
	}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	for _, want := range []string{
		`class="present-chart"`,
		`data-chart-title="p95 latency"`,
		`<canvas></canvas>`,
		`type="application/json" class="chart-spec"`,
		`"kind":"bar"`,
		`"unit":"ms"`,
		`"name":"api"`,
		`"x":"Tue","y":18.5`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

// A chart spec must never break out of the <script type="application/json">
// island: encoding/json escapes <, >, & so an injected </script> stays inert.
func TestRenderDocChartScriptSafe(t *testing.T) {
	doc := Doc{
		Sections: []Section{{
			Heading: "X",
			Blocks: []Block{{
				T:    "chart",
				Kind: "area",
				Series: []ChartSeries{{
					Name:   "</script><script>alert(1)</script>",
					Points: []ChartPoint{{X: "a", Y: 1}},
				}},
			}},
		}},
	}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if strings.Contains(out, "<script>alert(1)") {
		t.Errorf("unescaped script breakout in: %s", out)
	}
	// The literal angle brackets must be gone; the (harmless) payload text remains.
	if strings.Count(out, "<script") != 1 {
		t.Errorf("series name produced an extra <script tag (breakout) in: %s", out)
	}
	if !strings.Contains(out, "alert(1)") {
		t.Errorf("expected escaped payload to retain its text in: %s", out)
	}
}

func TestRenderDocUnknownBlockType(t *testing.T) {
	doc := Doc{
		Sections: []Section{
			{Heading: "S", Blocks: []Block{{T: "unknown"}}},
		},
	}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc should not error on unknown block: %v", err)
	}
	if !strings.Contains(out, "render error") {
		t.Error("unknown block type should produce a comment error")
	}
}

func TestChartPointNumericX(t *testing.T) {
	var pts []ChartPoint
	err := json.Unmarshal([]byte(`[{"x": 12.5, "y": 3}, {"x": "Mon", "y": 4}, {"y": 5}]`), &pts)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []ChartPoint{{X: "12.5", Y: 3}, {X: "Mon", Y: 4}, {X: "", Y: 5}}
	for i := range want {
		if pts[i] != want[i] {
			t.Errorf("point %d = %+v, want %+v", i, pts[i], want[i])
		}
	}
	if err := json.Unmarshal([]byte(`[{"x": true, "y": 1}]`), &pts); err == nil {
		t.Error("expected an error for a boolean x")
	}
}

func TestRenderDocSankeyBlock(t *testing.T) {
	doc := Doc{
		Sections: []Section{{
			Heading: "Traffic",
			Blocks: []Block{{
				T:     "chart",
				Kind:  "sankey",
				Title: "Requests",
				Flows: []ChartFlow{{From: "ingress", To: "api", Value: 1200}},
			}},
		}},
	}

	out, err := RenderDoc(doc, "t")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	for _, want := range []string{
		`"kind":"sankey"`,
		`"flows":[{"from":"ingress","to":"api","value":1200}]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output", want)
		}
	}
}
