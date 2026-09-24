package mdimport

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

func convert(t *testing.T, name, src string) Page {
	t.Helper()
	p, err := Convert(name, []byte(src))
	if err != nil {
		t.Fatalf("Convert: %v\n--- source ---\n%s", err, src)
	}
	return p
}

// firstText is the text of the first block of the first section, which is
// where a one-paragraph document lands.
func firstText(t *testing.T, p Page) string {
	t.Helper()
	if len(p.Doc.Sections) == 0 || len(p.Doc.Sections[0].Blocks) == 0 {
		t.Fatalf("no blocks in %+v", p.Doc)
	}
	return p.Doc.Sections[0].Blocks[0].Text
}

func TestConvertOutline(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		src      string
		title    string
		summary  string
		sections []render.Section
	}{
		{
			name: "h1 titles, h2 sections, lead paragraph is the summary",
			file: "notes.md",
			src: "# Weekly\n\nA short brief.\n\n## Done\n\nShipped it.\n\n## Next\n\n" +
				"### Soon\n\nMore.\n",
			title:   "Weekly",
			summary: "A short brief.",
			sections: []render.Section{
				{Heading: "Done", Blocks: []render.Block{{T: "p", Text: "Shipped it."}}},
				{Heading: "Next", Blocks: []render.Block{
					{T: "h3", Text: "Soon"}, {T: "p", Text: "More."},
				}},
			},
		},
		{
			name:  "no h1: the file name titles the page",
			file:  "dir/release-notes.markdown",
			src:   "## Changes\n\nOne.\n",
			title: "release-notes",
			sections: []render.Section{
				{Heading: "Changes", Blocks: []render.Block{{T: "p", Text: "One."}}},
			},
		},
		{
			name:  "no usable name: Untitled",
			file:  ".md",
			src:   "Only text.\n",
			title: "Untitled",
			sections: []render.Section{
				{Heading: "Untitled", Blocks: []render.Block{{T: "p", Text: "Only text."}}},
			},
		},
		{
			name:  "no h2: one section named after the page, lone paragraph stays the body",
			file:  "x.md",
			src:   "# Solo\n\nJust this.\n",
			title: "Solo",
			sections: []render.Section{
				{Heading: "Solo", Blocks: []render.Block{{T: "p", Text: "Just this."}}},
			},
		},
		{
			name:    "no h2 with more than a paragraph: summary plus the rest",
			file:    "x.md",
			src:     "# Solo\n\nLead.\n\n- a\n- b\n",
			title:   "Solo",
			summary: "Lead.",
			sections: []render.Section{
				{Heading: "Solo", Blocks: []render.Block{
					{T: "list", Items: []string{"a", "b"}},
				}},
			},
		},
		{
			name:    "content before the first h2 becomes an Introduction section",
			file:    "x.md",
			src:     "# T\n\nLead.\n\n#### Deep\n\nMore lead.\n\n## Body\n\nText.\n",
			title:   "T",
			summary: "Lead.",
			sections: []render.Section{
				{Heading: "Introduction", Blocks: []render.Block{
					{T: "h3", Text: "Deep"}, {T: "p", Text: "More lead."},
				}},
				{Heading: "Body", Blocks: []render.Block{{T: "p", Text: "Text."}}},
			},
		},
		{
			name:  "a second h1 is a section; a leading list is not a summary",
			file:  "x.md",
			src:   "# T\n\n- item\n\n# Also\n\nText.\n",
			title: "T",
			sections: []render.Section{
				{Heading: "Introduction", Blocks: []render.Block{
					{T: "list", Items: []string{"item"}},
				}},
				{Heading: "Also", Blocks: []render.Block{{T: "p", Text: "Text."}}},
			},
		},
		{
			name:  "heading text is plain: markup and links stripped",
			file:  "x.md",
			src:   "# **Bold** `code` [link](http://x)\n\n## *Sec* ~~gone~~\n\n### `h3` _it_\n\nP.\n",
			title: "Bold code link",
			sections: []render.Section{
				{Heading: "Sec gone", Blocks: []render.Block{
					{T: "h3", Text: "h3 it"}, {T: "p", Text: "P."},
				}},
			},
		},
		{
			name:  "an empty section keeps its heading",
			file:  "x.md",
			src:   "## Todo\n\n## Done\n\nAll.\n",
			title: "x",
			sections: []render.Section{
				{Heading: "Todo", Blocks: []render.Block{}},
				{Heading: "Done", Blocks: []render.Block{{T: "p", Text: "All."}}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := convert(t, c.file, c.src)
			if p.Title != c.title {
				t.Errorf("title = %q, want %q", p.Title, c.title)
			}
			if p.Doc.Summary != c.summary {
				t.Errorf("summary = %q, want %q", p.Doc.Summary, c.summary)
			}
			if !reflect.DeepEqual(p.Doc.Sections, c.sections) {
				t.Errorf("sections =\n%+v\nwant\n%+v", p.Doc.Sections, c.sections)
			}
		})
	}
}

func TestConvertInline(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"asterisk italic", "a *b* c", "a *b* c"},
		{"underscore italic", "a _b_ c", "a *b* c"},
		{"asterisk bold", "a **b** c", "a **b** c"},
		{"underscore bold", "a __b__ c", "a **b** c"},
		{"code span", "run `go test` now", "run `go test` now"},
		{"bold around code loses the bold", "**use `x`**", "use `x`"},
		{"bold around a link loses the bold", "**[a](http://a)**", "[a](http://a)"},
		{"italic around code keeps the italic", "*use `x`*", "*use `x`*"},
		{"nested emphasis keeps only the outer mark", "***a***", "*a*"},
		{"link", "see [docs](https://ex.com/p) here", "see [docs](https://ex.com/p) here"},
		{"link label is plain", "[**b** `c`](http://x)", "[b c](http://x)"},
		{"link with a paren in the url", "[w](https://x/a_(b))", "[w](https://x/a_(b%29)"},
		{"javascript link is words", "[x](javascript:alert(1))", "x"},
		{"data link is words", "[x](data:text/html,hi)", "x"},
		{"vbscript link is words", "[x](VBScript:msgbox)", "x"},
		{
			"link goldmark refuses cannot become a present link",
			"[x](java\tscript:alert(1))", "[x] (java\tscript:alert(1))",
		},
		{"literal link syntax inside code is broken up", "the `[a](b)` form", "the `[a] (b)` form"},
		{"literal chip syntax inside code is broken up", "`@chip(a:b)`", "`@chip (a:b)`"},
		{
			"autolink",
			"go to https://ex.com/x now",
			"go to [https://ex.com/x](https://ex.com/x) now",
		},
		{"angle autolink", "<https://ex.com>", "[https://ex.com](https://ex.com)"},
		{"email autolink", "<me@ex.com>", "[me@ex.com](mailto:me@ex.com)"},
		{"strikethrough is words", "a ~~b~~ c", "a b c"},
		{"image is its alt text", "see ![the alt](x.png) here", "see the alt here"},
		{"image without alt vanishes", "see ![](x.png) here", "see  here"},
		{"soft break is a space", "one\ntwo", "one two"},
		{"hard break is a space", "one  \ntwo", "one two"},
		{"backslash escapes resolve", `a \*b\* c`, "a *b* c"},
		{"entities resolve", "a &amp; b &lt; c", "a & b < c"},
		{"inline html is dropped", "a <b>bold</b> c", "a bold c"},
		{"code span holding a backtick is words", "a `` x`y `` c", "a x`y c"},
		{"footnote reference is dropped", "a[^1] b\n\n[^1]: note\n", "a b"},
		{
			"entity in a destination resolves",
			"[a](http://x/?q=1&amp;b=2)",
			"[a](http://x/?q=1&b=2)",
		},
		{"escapes in a destination resolve", `[a](http://x/\(1\))`, "[a](http://x/(1%29)"},
		{
			"chip syntax in a label is broken up",
			"[@chip(a:b)](http://x)",
			"[@chip (a:b)](http://x)",
		},
		{"file scheme is words", "[x](file:///etc/passwd)", "x"},
		{"relative link survives", "[x](../p/1)", "[x](../p/1)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := convert(t, "x.md", c.src+"\n")
			if got := firstText(t, p); got != c.want {
				t.Errorf("text = %q, want %q", got, c.want)
			}
		})
	}
}

func TestConvertBlocks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []render.Block
	}{
		{
			name: "bullet list",
			src:  "- a\n- *b*\n",
			want: []render.Block{{T: "list", Items: []string{"a", "*b*"}}},
		},
		{
			name: "ordered list",
			src:  "1. a\n2. b\n",
			want: []render.Block{{T: "list", Ordered: true, Items: []string{"a", "b"}}},
		},
		{
			name: "nested list flattens after its parent item",
			src:  "- a\n  - a1\n  - a2\n- b\n",
			want: []render.Block{{T: "list", Items: []string{"a", "a1", "a2", "b"}}},
		},
		{
			name: "multi-paragraph item joins with a space",
			src:  "- a\n\n  more a\n\n- b\n",
			want: []render.Block{{T: "list", Items: []string{"a more a", "b"}}},
		},
		{
			name: "task list",
			src:  "- [x] done\n- [ ] open\n",
			want: []render.Block{{T: "list", Items: []string{"[x] done", "[ ] open"}}},
		},
		{
			name: "code block inside an item splits the list",
			src:  "1. a\n\n   ```sh\n   run\n   ```\n\n2. b\n",
			want: []render.Block{
				{T: "list", Ordered: true, Items: []string{"a"}},
				{T: "code", Lang: "sh", Text: "run"},
				{T: "list", Ordered: true, Items: []string{"b"}},
			},
		},
		{
			name: "html block or rule inside an item does not split the list",
			src:  "1. a\n   <div>x</div>\n2. b\n\n   ---\n3. c\n",
			want: []render.Block{{T: "list", Ordered: true, Items: []string{"a", "b", "c"}}},
		},
		{
			name: "code inside a blockquote becomes a code span in the callout",
			src:  "> Run:\n>\n> ```sh\n> make all\n> ```\n",
			want: []render.Block{{T: "callout", Severity: "info", Text: "Run: `make all`"}},
		},
		{
			name: "fenced code keeps its language and text verbatim",
			src:  "```go\nfunc main() {\n\tfmt.Println(\"*hi*\")\n}\n```\n",
			want: []render.Block{
				{T: "code", Lang: "go", Text: "func main() {\n\tfmt.Println(\"*hi*\")\n}"},
			},
		},
		{
			name: "mermaid stays a code block with its language",
			src:  "```mermaid\ngraph TD; A-->B\n```\n",
			want: []render.Block{{T: "code", Lang: "mermaid", Text: "graph TD; A-->B"}},
		},
		{
			name: "indented code has no language",
			src:  "    x = 1\n    y = 2\n",
			want: []render.Block{{T: "code", Text: "x = 1\ny = 2"}},
		},
		{
			name: "table: header plain, cells inline",
			src:  "| **Name** | Value |\n|---|---|\n| `a` | *one* |\n| b | [l](http://x) |\n",
			want: []render.Block{{
				T:    "table",
				Cols: []string{"Name", "Value"},
				Rows: [][]string{{"`a`", "*one*"}, {"b", "[l](http://x)"}},
			}},
		},
		{
			name: "blockquote is an info callout",
			src:  "> Mind the *gap*.\n> Second line.\n",
			want: []render.Block{
				{T: "callout", Severity: "info", Text: "Mind the *gap*. Second line."},
			},
		},
		{
			name: "warning alert",
			src:  "> [!WARNING]\n> Hot.\n",
			want: []render.Block{{T: "callout", Severity: "warn", Text: "Hot."}},
		},
		{
			name: "caution alert",
			src:  "> [!CAUTION]\n> Sharp.\n",
			want: []render.Block{{T: "callout", Severity: "warn", Text: "Sharp."}},
		},
		{
			name: "note, tip and important alerts are info without the marker",
			src:  "> [!NOTE]\n> N.\n\n> [!TIP]\n> T.\n\n> [!IMPORTANT]\n> I.\n",
			want: []render.Block{
				{T: "callout", Severity: "info", Text: "N."},
				{T: "callout", Severity: "info", Text: "T."},
				{T: "callout", Severity: "info", Text: "I."},
			},
		},
		{
			name: "blockquote with a list and code flattens into one callout",
			src:  "> Steps:\n>\n> - one\n> - two\n>\n> ```\n> run\n> ```\n",
			want: []render.Block{{T: "callout", Severity: "info", Text: "Steps: one two `run`"}},
		},
		{
			name: "raw html, thematic breaks and footnotes are dropped",
			src:  "<div>raw</div>\n\nkept\n\n---\n\n[^1]: note\n",
			want: []render.Block{{T: "p", Text: "kept"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := convert(t, "x.md", "## S\n\n"+c.src)
			got := p.Doc.Sections[0].Blocks
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("blocks =\n%+v\nwant\n%+v", got, c.want)
			}
		})
	}
}

func TestConvertEmptyIsAnError(t *testing.T) {
	for _, src := range []string{"", "   \n", "# Only a title\n", "---\n", "<div>x</div>\n", "## Empty\n"} {
		_, err := Convert("x.md", []byte(src))
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("Convert(%q) err = %v, want ErrEmpty", src, err)
		}
	}
}

// A link goldmark did not parse must not become one in present, however its
// pieces reach the text: split across nodes by an escape or an entity, or
// glued together by a node that emits nothing. Every prose field is covered,
// since the renderer applies inline markup to all of them.
func TestConvertNeverAssemblesAScriptLink(t *testing.T) {
	const payload = "(javascript:alert%281%29)"
	cases := map[string]string{
		"comment between":         "[x]<!-- -->" + payload,
		"empty inline html":       "[x]<b></b>" + payload,
		"image without alt":       "[x]![](y)" + payload,
		"footnote reference":      "[x][^1]" + payload + "\n\n[^1]: note\n",
		"escaped paren":           `[x]\` + payload,
		"entity parens":           "[x]&#40;javascript:alert%281%29&#41;",
		"in a summary":            "# T\n\n[x]<!-- -->" + payload + "\n\n## S\n\nbody\n",
		"in a table cell":         "## S\n\n| h |\n|---|\n| [x]<!-- -->" + payload + " |\n",
		"in a list item":          "## S\n\n- [x]<!-- -->" + payload + "\n",
		"in a callout":            "## S\n\n> [x]<!-- -->" + payload + "\n",
		"in a callout heading":    "## S\n\n> # [x]<!-- -->" + payload + "\n",
		"in a callout code block": "## S\n\n> ```\n> [x](javascript:alert%281%29)\n> ```\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			p := convert(t, "x.md", src)
			c, err := render.CompileDoc(p.Doc, p.Title)
			if err != nil {
				t.Fatalf("CompileDoc: %v", err)
			}
			if strings.Contains(string(c.JSON), "](") {
				t.Errorf("stored doc keeps link syntax: %s", c.JSON)
			}
			if strings.Contains(c.HTML, "<a ") {
				t.Errorf("rendered a link: %s", c.HTML)
			}
			if !strings.Contains(c.HTML, "javascript:alert") {
				t.Errorf("payload text lost instead of neutralized: %s", c.HTML)
			}
		})
	}
}

// reAnchor captures each rendered anchor's attribute list.
var reAnchor = regexp.MustCompile(`<a([^>]*)>`)

// The renderer marks the pieces it generates with NUL-delimited tokens and
// swaps each back at its first match. goldmark passes a raw NUL through in
// a link destination, so markdown could forge a token and splice a second
// link into the first one's href, where the browser reads an onmouseover=
// as a live attribute. Convert replaces NUL with U+FFFD before parsing, and
// the renderer does the same on its own input, so neither the stored Doc
// nor the page can carry one.
func TestConvertNULCannotForgeRendererTokens(t *testing.T) {
	cases := map[string]string{
		"link destination":  "## S\n\n[a](https://x/\x00PH1\x00) [t](https://ok/onmouseover=document.title=`pwned`//)\n",
		"code span":         "## S\n\n`\x00PH1\x00` [t](https://ok/onmouseover=alert(1)//)\n",
		"plain text":        "## S\n\n\x00PH0\x00 [t](https://ok/x)\n",
		"heading and title": "# \x00PH0\x00\n\n## \x00PH1\x00\n\n[t](https://ok/x)\n",
		"code block":        "## S\n\n```\n\x00PH0\x00\n```\n\n[t](https://ok/x)\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			p := convert(t, "x.md", src)
			c, err := render.CompileDoc(p.Doc, p.Title)
			if err != nil {
				t.Fatalf("CompileDoc: %v", err)
			}
			if strings.Contains(string(c.JSON), "\\u0000") || strings.Contains(p.Title, "\x00") {
				t.Errorf("stored doc carries a NUL: %s", c.JSON)
			}
			if strings.Contains(c.HTML, "\x00") {
				t.Errorf("rendered page carries a NUL: %s", c.HTML)
			}
			for _, m := range reAnchor.FindAllStringSubmatch(c.HTML, -1) {
				if !regexp.MustCompile(`^ href="[^"]*"$`).MatchString(m[1]) {
					t.Errorf("anchor with attributes other than href: %s", m[0])
				}
			}
			if !strings.Contains(c.HTML, "�") {
				t.Errorf("NUL was dropped rather than replaced: %s", c.HTML)
			}
		})
	}
}

// A code block survives the renderer's name normalization, which rewrites
// " & " and " = " in prose.
func TestConvertCodeSurvivesNormalize(t *testing.T) {
	p := convert(t, "x.md", "## S\n\nprose a & b\n\n```\na & b = c\n```\n")
	c, err := render.CompileDoc(p.Doc, p.Title)
	if err != nil {
		t.Fatalf("CompileDoc: %v", err)
	}
	if !strings.Contains(c.HTML, "a &amp; b = c") {
		t.Errorf("code block was normalized: %s", c.HTML)
	}
	if !strings.Contains(c.HTML, "prose a and b") {
		t.Errorf("prose was not normalized: %s", c.HTML)
	}
}

// The Doc round-trips through the renderer: inline syntax the converter
// emits is what present's renderer expects.
func TestConvertRendersPresentMarkup(t *testing.T) {
	p := convert(
		t,
		"x.md",
		"# T\n\nLead **bold**.\n\n## S\n\nUse `x` and *y*, see [d](https://d).\n",
	)
	c, err := render.CompileDoc(p.Doc, p.Title)
	if err != nil {
		t.Fatalf("CompileDoc: %v", err)
	}
	for _, want := range []string{
		`<div class="brief-summary" data-fixation>Lead <strong>bold</strong>.</div>`,
		"<code>x</code>", "<em>y</em>", `<a href="https://d">d</a>`,
		`<wk-section id="s">`,
	} {
		if !strings.Contains(c.HTML, want) {
			t.Errorf("missing %q in: %s", want, c.HTML)
		}
	}
}
