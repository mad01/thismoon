package render

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// An inline link in a prose field may point at http, https, mailto, a
// relative path, or a fragment; any other scheme renders as the literal
// text, so no Doc source, MCP or import, can put a javascript: href on the
// page through inline markup. (A `t: html` block is raw passthrough by
// design and outside this check.)
func TestInlineMdLinkSchemeAllowlist(t *testing.T) {
	for _, href := range []string{
		"https://ex.com/a", "http://ex.com", "mailto:a@b.c", "/p/abc", "../rel", "#frag", "?q=1",
	} {
		got := string(inlineMd("[x](" + href + ")"))
		if !strings.Contains(got, `<a href="`+html.EscapeString(href)+`">x</a>`) {
			t.Errorf("href %q not rendered as a link: %s", href, got)
		}
	}
	for _, href := range []string{
		"javascript:alert(1)", "JavaScript:alert%281%29", "java\tscript:alert(1)",
		" javascript:alert(1)", "data:text/html,hi", "vbscript:msgbox", "file:///etc/passwd",
	} {
		got := string(inlineMd("see [x](" + href + ") now"))
		if strings.Contains(got, "<a ") {
			t.Errorf("href %q rendered as a link: %s", href, got)
		}
		if !strings.Contains(got, html.EscapeString("[x]("+href+")")) {
			t.Errorf("href %q not kept as literal text: %s", href, got)
		}
	}
}

// reOpenTag captures every opening tag and its attribute list in rendered
// inline HTML.
var reOpenTag = regexp.MustCompile(`<([a-z-]+)([^>]*)>`)

// checkInlineAttrs fails when any tag in got carries an attribute the
// renderer does not write: an anchor may have href, a badge may have
// variant, nothing else may have any. It is how a test tells a spliced-in
// attribute (onmouseover=...) from a legitimate one.
func checkInlineAttrs(t *testing.T, in, got string) {
	t.Helper()
	allowed := map[string]string{"a": "href", "wk-badge": "variant"}
	for _, m := range reOpenTag.FindAllStringSubmatch(got, -1) {
		tag, attrs := m[1], m[2]
		want := allowed[tag]
		ok := attrs == "" ||
			(want != "" && regexp.MustCompile(`^ `+want+`="[^"]*"$`).MatchString(attrs))
		if !ok {
			t.Errorf(
				"input %q: tag <%s> carries unexpected attributes %q in: %s",
				in,
				tag,
				attrs,
				got,
			)
		}
	}
}

// inlineMd stands in for each generated piece with a NUL-delimited token and
// swaps it back at its first match, so a NUL in the input could forge a
// token and splice one generated piece into another's attribute (a second
// link's href inside the first link's href, say, where the browser then
// reads onmouseover as a live attribute). NUL is replaced with U+FFFD before
// anything else happens, so no forged token can match.
func TestInlineMdNULCannotForgePlaceholders(t *testing.T) {
	forged := []string{
		"[a](https://x/\x00PH1\x00) [t](https://ok/onmouseover=document.title=`pwned`//)",
		"[a](https://x/\x00PH0\x00) [t](https://ok/onmouseover=alert(1)//)",
		"`\x00PH1\x00` [t](https://ok/onmouseover=alert(1)//)",
		"@chip(a:\x00PH1\x00) [t](https://ok/onmouseover=alert(1)//)",
		"**\x00PH1\x00** `<img src=x onerror=alert(1)>`",
		"\x00PH0\x00 [t](https://ok/x) \x00PH1\x00",
		"[\x00PH1\x00](https://x/) [t](javascript:alert(1))",
	}
	for _, in := range forged {
		got := string(inlineMd(in))
		if strings.Contains(got, "\x00") {
			t.Errorf("input %q: NUL survived into the output: %q", in, got)
		}
		if strings.Contains(got, "<img") {
			t.Errorf("input %q: code span content escaped as markup: %s", in, got)
		}
		checkInlineAttrs(t, in, got)
	}
	// The replacement character stands where the NUL was, so the text is not
	// silently shortened.
	if got := string(inlineMd("a\x00b")); got != "a\uFFFDb" {
		t.Errorf("NUL in plain text = %q, want a\uFFFDb", got)
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
	if !strings.Contains(out, `<h1 class="brief-title" data-fixation>Title</h1>`) {
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
		{"section-heading", `<wk-section-heading data-fixation>`},
		{"paragraph", `<p data-fixation>para</p>`},
		{"h3", `<wk-section-subheading data-fixation>`},
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
			{Heading: "Alpha", Blocks: []Block{{T: "p", Text: "x"}}},
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
		`<a href="#alpha">Alpha</a>`,
		`</wk-toc>`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("toc missing %q in: %s", want, out)
		}
	}
	// The heading is the bare heading text, like the TOC entry.
	if !strings.Contains(
		out,
		`<wk-section-heading data-fixation>Alpha</wk-section-heading>`,
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

// Name normalization rewrites operators in prose (" & " to " and ") but must
// leave a code block's text alone.
func TestRenderDocCodeBlockSkipsNormalize(t *testing.T) {
	doc := Doc{
		Sections: []Section{{
			Heading: "S",
			Blocks: []Block{
				{T: "p", Text: "a & b = c"},
				{T: "code", Lang: "sh", Text: "a & b = c"},
			},
		}},
	}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if !strings.Contains(out, "<p data-fixation>a and b equals c</p>") {
		t.Errorf("prose not normalized in: %s", out)
	}
	if !strings.Contains(out, `<code class="language-sh">a &amp; b = c</code>`) {
		t.Errorf("code block was normalized in: %s", out)
	}
}

// Compile parses, renders, and returns the canonical JSON the store keeps.
func TestCompileReturnsHTMLAndCanonicalJSON(t *testing.T) {
	c, err := Compile(
		[]byte(`{"sections":[{"h":"S","blocks":[{"t":"p","text":"hi"}],"extra":1}]}`),
		"T",
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !strings.Contains(c.HTML, "<p data-fixation>hi</p>") {
		t.Errorf("html = %s", c.HTML)
	}
	if strings.Contains(string(c.JSON), "extra") {
		t.Errorf("canonical json kept an unknown field: %s", c.JSON)
	}
	if len(c.Doc.Sections) != 1 || c.Doc.Sections[0].Heading != "S" {
		t.Errorf("doc = %+v", c.Doc)
	}
	if _, err := Compile([]byte(`{not json`), "T"); err == nil {
		t.Error("Compile accepted malformed JSON")
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

// A panel accent is a palette role name, validated against webkit's
// vocabulary so a page cannot store a var() no family resolves; terracotta
// stays accepted as the alias of primary older pages carry and renders as
// the role, so the stored HTML never names the alias.
func TestRenderDocPanelAccent(t *testing.T) {
	render := func(accent string) (string, error) {
		return RenderDoc(Doc{Sections: []Section{{
			Heading: "S",
			Blocks:  []Block{{T: "panel", Title: "P", Accent: accent}},
		}}}, "T")
	}
	for accent, role := range map[string]string{
		"primary": "primary", "blue": "blue", "series-3": "series-3",
		"tone-amber-bg": "tone-amber-bg", "terracotta": "primary",
	} {
		out, err := render(accent)
		if err != nil {
			t.Errorf("accent %q: %v", accent, err)
			continue
		}
		if want := `border-left: 3px solid var(--` + role + `)`; !strings.Contains(out, want) {
			t.Errorf("accent %q: output lacks %q", accent, want)
		}
		if strings.Contains(out, "var(--terracotta)") {
			t.Errorf("accent %q: output names the alias instead of its role", accent)
		}
	}
	if out, err := render(""); err != nil || strings.Contains(out, "border-left") {
		t.Errorf("no accent: err=%v, border rendered=%v", err, strings.Contains(out, "border-left"))
	}
	_, err := render("wg600")
	if err == nil || !strings.Contains(err.Error(), `unknown accent "wg600"`) {
		t.Errorf("unknown accent: err = %v, want unknown accent", err)
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

// TestRenderDocFixationCoverage pins which rendered elements carry
// data-fixation, the attribute the webkit header's fixation walk targets:
// every element that holds prose the reader scans, and none of the labels
// and chips that are not prose.
func TestRenderDocFixationCoverage(t *testing.T) {
	doc := Doc{
		Summary: "the summary",
		Meta:    "2026-09-30 · meta",
		Chips:   []Chip{{Text: "chip", Style: "a"}},
		Sections: []Section{
			{Heading: "First", Blocks: []Block{
				{T: "p", Text: "para"},
				{T: "h3", Text: "sub"},
				{T: "callout", Text: "note"},
				{T: "table", Cols: []string{"Col"}, Rows: [][]string{{"cell"}}},
				{T: "kv", KV: []KVPair{{K: "key", V: "value"}}},
				{T: "list", Items: []string{"item"}},
				{T: "panel", Title: "Panel", Subtitle: "sub"},
			}},
			{Heading: "Second", Blocks: []Block{{T: "p", Text: "more"}}},
		},
	}
	out, err := RenderDoc(doc, "The Title")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}

	fixated := []struct{ name, want string }{
		{"title", `<h1 class="brief-title" data-fixation>The Title</h1>`},
		{"summary", `<div class="brief-summary" data-fixation>`},
		{"toc list", `<ul data-fixation>`},
		{"section heading", `<wk-section-heading data-fixation>`},
		{"subheading", `<wk-section-subheading data-fixation>sub</wk-section-subheading>`},
		{"paragraph", `<p data-fixation>para</p>`},
		{"callout", `<wk-callout data-fixation>note</wk-callout>`},
		{"table body", `<tbody data-fixation>`},
	}
	for _, c := range fixated {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: missing %q", c.name, c.want)
		}
	}

	// The shell's fixation-targets attribute adds kv values, list items,
	// panel titles and subtitles by element; the renderer leaves those bare.
	// Table headers, the meta line, and chips are labels, not prose, and
	// stay out of the walk.
	bare := []struct{ name, want string }{
		{"table header", `<th>Col</th>`},
		{"meta", `<div class="brief-meta">`},
		{"chip", `<wk-badge variant="a">chip</wk-badge>`},
		{"kv value", `<wk-kv-value>value</wk-kv-value>`},
		{"list item", `<li>item</li>`},
	}
	for _, c := range bare {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: missing %q", c.name, c.want)
		}
	}
	if n := strings.Count(out, "data-fixation"); n != 10 {
		t.Errorf(
			"data-fixation count = %d, want 10 (title, summary, toc, 2 headings, subheading, 2 paragraphs, callout, table body)",
			n,
		)
	}
}

// TestInlineMdNestedMarkup covers inline markup inside a bold span, which
// used to leak the renderer's placeholder tokens (" PH0 ") into the page.
func TestInlineMdNestedMarkup(t *testing.T) {
	cases := []struct{ in, want string }{
		{"**run `make build` first**", "<strong>run <code>make build</code> first</strong>"},
		{
			"**`resource.k8s.io/v1` is served**",
			"<strong><code>resource.k8s.io/v1</code> is served</strong>",
		},
		{
			"**see [the PR](https://example.com/pr/1)**",
			`<strong>see <a href="https://example.com/pr/1">the PR</a></strong>`,
		},
		{
			"**@chip(b:done)** and `x`",
			`<strong><wk-badge variant="b">done</wk-badge></strong> and <code>x</code>`,
		},
		{
			"[@chip(a:x)](https://example.com)",
			`<a href="https://example.com"><wk-badge variant="a">x</wk-badge></a>`,
		},
		{
			"plain **bold** and `code` apart",
			"plain <strong>bold</strong> and <code>code</code> apart",
		},
	}
	for _, c := range cases {
		if got := string(inlineMd(c.in)); got != c.want {
			t.Errorf("inlineMd(%q)\n got %s\nwant %s", c.in, got, c.want)
		}
		if got := string(inlineMd(c.in)); strings.Contains(got, "PH") ||
			strings.Contains(got, "\x00") {
			t.Errorf("inlineMd(%q) leaks a placeholder: %s", c.in, got)
		}
	}
}

// TestRenderDocChartSteps renders a stepped chart: the block carries the
// step count, the captions follow the canvas as a fixated numbered list
// with inline markdown and name normalization applied, and the series
// steps ride in the spec. A chart without steps emits neither.
func TestRenderDocChartSteps(t *testing.T) {
	stepped := Block{
		T: "chart", Kind: "stacked-bar", Title: "Errors",
		Steps:  []Step{{Caption: "The **JIRA** queue fills."}, {Caption: "Retries pile on."}},
		Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "x", Y: 1}}}, {Name: "b", Step: 2, Points: []ChartPoint{{X: "x", Y: 2}}}},
	}
	plain := Block{T: "chart", Kind: "bar", Series: []ChartSeries{{Points: []ChartPoint{{X: "x", Y: 1}}}}}
	out, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: []Block{stepped, plain}}}}, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	for _, want := range []string{
		`<div class="present-chart" data-chart-title="Errors" data-steps="2">`,
		"</script>\n  <ol class=\"present-steps\">\n    <li data-fixation>The <strong>jira</strong> queue fills.</li>\n    <li data-fixation>Retries pile on.</li>\n  </ol>\n</div>",
		`"name":"b","points":[{"x":"x","y":2}],"step":2}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if n := strings.Count(out, "data-steps="); n != 1 {
		t.Errorf("data-steps appears %d times, want 1 (the plain chart carries none)", n)
	}
	if n := strings.Count(out, "present-steps"); n != 1 {
		t.Errorf("present-steps appears %d times, want 1", n)
	}
}

// TestRenderDocRibbon renders a ribbon chart: the block carries the
// is-ribbon class from the template (so the frameless style applies before
// the renderer runs), data-frame only when the chart keeps its card, and
// the order rides in the spec. The periods are every distinct x in order
// of first appearance, so three captions for three periods pass.
func TestRenderDocRibbon(t *testing.T) {
	series := []ChartSeries{
		{Name: "search", Points: []ChartPoint{{X: "Q1", Y: 40}, {X: "Q2", Y: 35}}},
		{Name: "social", Points: []ChartPoint{{X: "Q1", Y: 20}, {X: "Q2", Y: 45}, {X: "Q3", Y: 50}}},
	}
	steps := []Step{{Caption: "Search leads."}, {Caption: "Social passes it."}, {Caption: "Search drops out."}}
	frameless := Block{T: "chart", Kind: "ribbon", Title: "Traffic by channel", Series: series, Steps: steps}
	yes := true
	framed := Block{T: "chart", Kind: "ribbon", Order: "given", Frame: &yes, Series: series}
	out, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: []Block{frameless, framed}}}}, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	for _, want := range []string{
		`<div class="present-chart is-ribbon" data-chart-title="Traffic by channel" data-steps="3">`,
		`<div class="present-chart is-ribbon" data-frame="true">`,
		`"kind":"ribbon","series":[{"name":"search","points":[{"x":"Q1","y":40},{"x":"Q2","y":35}]},{"name":"social",`,
		`"order":"given"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if n := strings.Count(out, `"order":`); n != 1 {
		t.Errorf("order appears %d times, want 1 (the default rank is not written)", n)
	}
	if n := strings.Count(out, "data-frame"); n != 1 {
		t.Errorf("data-frame appears %d times, want 1 (a chart carries none unless frame is set)", n)
	}
}

// TestRibbonPeriods pins the period order both sides share: the vector in
// testdata/ribbon-order.json is read by this test and by the viz.js tests,
// and both must list the same periods (first appearance across the
// series, a numeric x read as its decimal string).
func TestRibbonPeriods(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ribbon-order.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vec struct {
		Series  []ChartSeries `json:"series"`
		Periods []string      `json:"periods"`
	}
	if err := json.Unmarshal(raw, &vec); err != nil {
		t.Fatal(err)
	}
	got, err := ribbonPeriods(vec.Series)
	if err != nil {
		t.Fatalf("ribbonPeriods: %v", err)
	}
	if !slices.Equal(got, vec.Periods) {
		t.Errorf("ribbonPeriods = %v, want %v", got, vec.Periods)
	}
}

// TestCompileDeckRibbonAccepts compiles a ribbon written the way the
// schema allows: a numeric x, a zero value, an explicit rank order, and
// four captions for the four periods the numbers make.
func TestCompileDeckRibbonAccepts(t *testing.T) {
	raw := []byte(`{"sections":[{"h":"S","blocks":[{"t":"chart","kind":"ribbon","order":"rank",
		"steps":[{"caption":"a"},{"caption":"b"},{"caption":"c"},{"caption":"d"}],
		"series":[{"name":"a","points":[{"x":2023,"y":1},{"x":2024,"y":0}]},
		{"name":"b","points":[{"x":2023,"y":2},{"x":2024,"y":3},{"x":2025,"y":1}]},
		{"name":"c","points":[{"x":"2025","y":4},{"x":"2026","y":5}]}]}]}]}`)
	c, err := CompileDeck(raw, "T")
	if err != nil {
		t.Fatalf("CompileDeck: %v", err)
	}
	for _, want := range []string{`data-steps="4"`, `"order":"rank"`, `{"x":"2023","y":1}`} {
		if !strings.Contains(c.HTML, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

// TestRenderDocDiagram renders a diagram block: the block carries the step
// count, the island holds the direction, groups, nodes, edges, and the
// steps with their focus ids, the caption and the step captions follow it
// as fixated prose with inline markdown, and a diagram without caption or
// steps emits neither.
func TestRenderDocDiagram(t *testing.T) {
	stepped := Block{
		T: "diagram", Direction: "TB", Caption: "The **checkout** path",
		Groups: []DiagramGroup{{ID: "prod", Label: "prod cluster", Tone: "blue"}},
		Nodes: []DiagramNode{
			{ID: "web", Label: "Web app", Text: "Next.js", Kind: "person"},
			{ID: "api", Label: "Checkout API", Group: "prod", Step: 2},
		},
		Edges: []DiagramEdge{{From: "web", To: "api", Label: "POST /checkout", Flow: true, Weight: 120, Step: 2}},
		Steps: []Step{{Caption: "The browser posts the cart."}, {Caption: "The API prices it.", Focus: []string{"api"}}},
	}
	plain := Block{T: "diagram", Nodes: []DiagramNode{{ID: "a", Label: "A"}}}
	out, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: []Block{stepped, plain}}}}, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	for _, want := range []string{
		`<div class="present-diagram" data-steps="2">`,
		`<div class="present-diagram-canvas"></div>`,
		`<script type="application/json" class="diagram-spec">{"direction":"TB","groups":[{"id":"prod","label":"prod cluster","tone":"blue"}],"nodes":[{"id":"web","label":"Web app","text":"Next.js","kind":"person"},{"id":"api","label":"Checkout API","group":"prod","step":2}],"edges":[{"from":"web","to":"api","label":"POST /checkout","flow":true,"weight":120,"step":2}],"steps":[{"caption":"The browser posts the cart."},{"caption":"The API prices it.","focus":["api"]}]}</script>`,
		`<p class="present-diagram-caption" data-fixation>The <strong>checkout</strong> path</p>`,
		"<ol class=\"present-steps\">\n    <li data-fixation>The browser posts the cart.</li>\n    <li data-fixation>The API prices it.</li>\n  </ol>",
		`<div class="present-diagram">` + "\n" + `  <div class="present-diagram-canvas"></div>` + "\n" + `  <script type="application/json" class="diagram-spec">{"nodes":[{"id":"a","label":"A"}]}</script>` + "\n" + `</div>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if n := strings.Count(out, "present-diagram-caption"); n != 1 {
		t.Errorf("caption appears %d times, want 1", n)
	}
}

// TestRenderDocDiagramScriptSafe keeps a node label from breaking out of
// the diagram's script island, the way the chart island is kept safe.
func TestRenderDocDiagramScriptSafe(t *testing.T) {
	doc := Doc{Sections: []Section{{Heading: "X", Blocks: []Block{{
		T: "diagram", Nodes: []DiagramNode{{ID: "a", Label: "</script><script>alert(1)</script>"}},
	}}}}}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if strings.Contains(out, "<script>alert(1)") {
		t.Errorf("unescaped script breakout in: %s", out)
	}
	if strings.Count(out, "<script") != 1 {
		t.Errorf("want exactly one <script tag, got %d in: %s", strings.Count(out, "<script"), out)
	}
}
