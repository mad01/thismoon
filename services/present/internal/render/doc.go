// Package render compiles a page's structured JSON source into the HTML
// fragment stored on disk: RenderDoc turns a Doc into a briefing-block fragment,
// RenderGraph turns a GraphInput into the Cytoscape init script, and the upgrade
// path deterministically rewrites legacy raw-HTML pages. The server serves the
// rendered fragment; the browser assembles the full page client-side.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strconv"
	"strings"
)

// Doc is the structured input format for page content. The model passes this
// as JSON; the MCP layer calls RenderDoc to produce the HTML body fragment
// stored in content.html.
type Doc struct {
	Summary  string    `json:"summary,omitempty"`
	Meta     string    `json:"meta,omitempty"`
	Chips    []Chip    `json:"chips,omitempty"`
	Sections []Section `json:"sections"`
}

// Chip is an inline tag rendered in a chip-row.
type Chip struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"` // stat, a, b, c, outline; empty = plain
}

// Section is a content section with a heading and blocks.
type Section struct {
	Heading string  `json:"h"`
	ID      string  `json:"id,omitempty"`
	Blocks  []Block `json:"blocks"`
}

// Block is a discriminated union on T.
type Block struct {
	T string `json:"t"` // p, h3, callout, table, kv, list, panel, progress, graph, chart, code, html

	// t=p, t=callout, t=h3, t=code, t=html
	Text string `json:"text,omitempty"`

	// t=code
	Lang string `json:"lang,omitempty"` // language badge + Prism grammar hint; empty = "text"

	// t=callout
	Severity string `json:"sev,omitempty"` // info, warn

	// t=table
	Cols []string   `json:"cols,omitempty"`
	Rows [][]string `json:"rows,omitempty"`

	// t=kv
	KV []KVPair `json:"kv,omitempty"`

	// t=list
	Items   []string `json:"items,omitempty"`
	Ordered bool     `json:"ordered,omitempty"`

	// t=panel
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"sub,omitempty"`
	Accent   string `json:"accent,omitempty"` // CSS variable name, e.g. terracotta

	// t=progress
	Percent float64 `json:"pct,omitempty"`
	Label   string  `json:"label,omitempty"`

	// t=chart
	Kind   string        `json:"kind,omitempty"`   // bar, line, area, sparkline, stacked-bar, horizontal-bar, doughnut, scatter, sankey
	Unit   string        `json:"unit,omitempty"`   // value-axis unit label, e.g. ms
	XUnit  string        `json:"xunit,omitempty"`  // x-axis unit label (scatter only)
	Series []ChartSeries `json:"series,omitempty"` // every kind except sankey
	Flows  []ChartFlow   `json:"flows,omitempty"`  // sankey only
}

// KVPair is a label-value pair within a kv block.
type KVPair struct {
	K string `json:"k"`
	V string `json:"v"`
}

// ChartSeries is one data series in a chart block.
type ChartSeries struct {
	Name   string       `json:"name,omitempty"`
	Color  string       `json:"color,omitempty"` // blue, green, terracotta, purple; empty = auto by index
	Points []ChartPoint `json:"points"`
}

// ChartPoint is one x/y datum. X is a category label (or the numeric x for a
// scatter chart, kept as its decimal string); Y the value.
type ChartPoint struct {
	X string  `json:"x,omitempty"`
	Y float64 `json:"y"`
}

// UnmarshalJSON accepts x as either a JSON string or a JSON number, so a
// scatter point can be written {"x": 12.5, "y": 3} without quoting. Numbers are
// stored as their shortest decimal form; the client parses them back.
func (p *ChartPoint) UnmarshalJSON(data []byte) error {
	var raw struct {
		X json.RawMessage `json:"x"`
		Y float64         `json:"y"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Y = raw.Y
	p.X = ""
	if len(raw.X) == 0 || string(raw.X) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw.X, &s); err == nil {
		p.X = s
		return nil
	}
	var n float64
	if err := json.Unmarshal(raw.X, &n); err != nil {
		return fmt.Errorf("chart point x: want string or number, got %s", raw.X)
	}
	p.X = strconv.FormatFloat(n, 'f', -1, 64)
	return nil
}

// ChartFlow is one link in a sankey chart: Value units flow from From to To.
type ChartFlow struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Value float64 `json:"value"`
}

// textNorms normalizes text for fixation reading and TTS pronunciation.
var textNorms = []struct {
	re   *regexp.Regexp
	repl string
}{
	// All-caps names pronounced as words.
	{regexp.MustCompile(`\bJIRA\b`), "jira"},

	// Unicode symbols to spoken words.
	{regexp.MustCompile(`→`), "to"},
	{regexp.MustCompile(`←`), "from"},
	{regexp.MustCompile(`↔`), "between"},
	{regexp.MustCompile(`×`), "times"},
	{regexp.MustCompile(`÷`), "divided by"},
	{regexp.MustCompile(`≈`), "approximately"},
	{regexp.MustCompile(`≠`), "not equal to"},
	{regexp.MustCompile(`≤`), "at most"},
	{regexp.MustCompile(`≥`), "at least"},
	{regexp.MustCompile(`[✓✔]`), "yes"},
	{regexp.MustCompile(`[✗✘]`), "no"},

	// Standalone ASCII operators (space-bounded to avoid mangling code/hyphens).
	{regexp.MustCompile(`(^| )& `), "${1}and "},
	{regexp.MustCompile(`(^| )\+ `), "${1}plus "},
	{regexp.MustCompile(`(^| )- `), "${1}minus "},
	{regexp.MustCompile(`(^| )= `), "${1}equals "},
}

func normalizeNames(s string) string {
	for _, n := range textNorms {
		s = n.re.ReplaceAllString(s, n.repl)
	}
	return s
}

// Inline markdown patterns — applied in order. Chip must go first because
// its parens would confuse the link regex.
var (
	reChip = regexp.MustCompile(`@chip\((\w+):([^)]+)\)`)
	reLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reCode = regexp.MustCompile("`([^`]+)`")
)

// inlineMd converts inline markdown to HTML. Input is plain text (will be
// HTML-escaped), output is trusted HTML.
func inlineMd(s string) template.HTML {
	// Extract chips and links before escaping so their delimiters survive.
	// We use placeholder tokens, escape the rest, then restore.
	type placeholder struct {
		token string
		html  string
	}
	var phs []placeholder
	idx := 0

	replace := func(re *regexp.Regexp, fn func([]string) string) {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			parts := re.FindStringSubmatch(m)
			tok := fmt.Sprintf("\x00PH%d\x00", idx)
			idx++
			phs = append(phs, placeholder{tok, fn(parts)})
			return tok
		})
	}

	// Chips: @chip(style:text)
	replace(reChip, func(p []string) string {
		if p[1] != "" {
			return fmt.Sprintf(
				`<wk-badge variant="%s">%s</wk-badge>`,
				html.EscapeString(p[1]),
				html.EscapeString(p[2]),
			)
		}
		return fmt.Sprintf(`<wk-badge>%s</wk-badge>`, html.EscapeString(p[2]))
	})

	// Links: [text](url). A url outside the allowlist stays literal text.
	replace(reLink, func(p []string) string {
		if !LinkHrefAllowed(p[2]) {
			return html.EscapeString(p[0])
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(p[2]), html.EscapeString(p[1]))
	})

	// Code: `text`
	replace(reCode, func(p []string) string {
		return fmt.Sprintf(`<code>%s</code>`, html.EscapeString(p[1]))
	})

	// Bold: **text**
	replace(reBold, func(p []string) string {
		return fmt.Sprintf(`<strong>%s</strong>`, html.EscapeString(p[1]))
	})

	// Escape the remaining text.
	s = html.EscapeString(s)

	// Restore placeholders (they contain pre-escaped HTML).
	for _, ph := range phs {
		s = strings.Replace(s, html.EscapeString(ph.token), ph.html, 1)
	}

	// Italic: *text* — applied after escaping since it doesn't need special chars.
	// Use a simple regex on the escaped output.
	reItalicPost := regexp.MustCompile(`\*([^*]+?)\*`)
	s = reItalicPost.ReplaceAllString(s, `<em>$1</em>`)

	return template.HTML(s)
}

// LinkHrefAllowed reports whether an inline link may point at href: http,
// https, and mailto URLs, plus relative paths and fragments (no scheme at
// all). Anything else, javascript: and data: above all, renders as literal
// text instead of a link, whichever way the Doc was authored. Whitespace and
// control characters are ignored when reading the scheme, because browsers
// ignore them too and `java\tscript:` would otherwise slip through.
func LinkHrefAllowed(href string) bool {
	h := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, href)
	scheme, _, ok := strings.Cut(h, ":")
	if !ok || strings.ContainsAny(scheme, "/?#") {
		return true
	}
	switch strings.ToLower(scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// langClass sanitizes a code block's language into a Prism class suffix:
// lowercase [a-z0-9-] only, "text" when empty or fully stripped. webkit's
// enhancer derives the badge from this and skips highlighting for grammars
// it doesn't bundle.
func langClass(lang string) string {
	s := strings.ToLower(lang)
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return -1
		}
	}, s)
	if s == "" {
		return "text"
	}
	return s
}

// sectionID derives an anchor ID from a heading.
func sectionID(heading string) string {
	s := strings.ToLower(heading)
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, s)
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

var (
	docTemplate    *template.Template
	blockTemplates *template.Template
)

func init() {
	funcs := template.FuncMap{
		"inlineMd":  inlineMd,
		"sectionID": sectionID,
		"langClass": langClass,
		"rawHTML":   func(s string) template.HTML { return template.HTML(s) },
		"chartSpec": chartSpec,
		"renderBlock": func(b Block) template.HTML {
			return "" // placeholder, replaced below
		},
	}

	blockTemplates = template.Must(
		template.New("blocks").Funcs(funcs).Parse(blockTemplatesSrc),
	)

	funcs["renderBlock"] = func(b Block) template.HTML {
		var buf bytes.Buffer
		if err := blockTemplates.ExecuteTemplate(&buf, "block-"+b.T, b); err != nil {
			return template.HTML(
				fmt.Sprintf(`<!-- render error: %s -->`, html.EscapeString(err.Error())),
			)
		}
		return template.HTML(buf.String())
	}

	docTemplate = template.Must(
		template.New("doc").Funcs(funcs).Parse(docTemplateSrc),
	)
}

const docTemplateSrc = `<h1 class="brief-title">{{.Title}}</h1>
{{- with .Meta}}
<div class="brief-meta">{{.}}</div>
{{- end}}
{{- with .Summary}}
<div class="brief-summary" data-fixation>{{inlineMd .}}</div>
{{- end}}
{{- with .Chips}}
<div class="chip-row">
{{- range .}}
  <wk-badge{{with .Style}} variant="{{.}}"{{end}}>{{.Text}}</wk-badge>
{{- end}}
</div>
{{- end}}
{{- if gt (len .Sections) 1}}
<wk-toc>
  <wk-toc-title>Sections</wk-toc-title>
  <ul>
{{- range .Sections}}
    <li><a href="#{{sectionID .Heading}}">{{with .ID}}<wk-section-id>{{.}}</wk-section-id>{{end}}{{.Heading}}</a></li>
{{- end}}
  </ul>
</wk-toc>
{{- end}}
{{- range .Sections}}
<wk-section id="{{sectionID .Heading}}">
  <wk-section-heading>{{with .ID}}<wk-section-id>{{.}}</wk-section-id>{{end}}{{.Heading}}</wk-section-heading>
{{- range .Blocks}}
  {{renderBlock .}}
{{- end}}
</wk-section>
{{- end}}`

const blockTemplatesSrc = `{{define "block-p"}}<p data-fixation>{{inlineMd .Text}}</p>{{end}}

{{define "block-h3"}}<wk-section-subheading>{{.Text}}</wk-section-subheading>{{end}}

{{define "block-callout"}}<wk-callout{{with .Severity}} variant="{{.}}"{{end}} data-fixation>{{inlineMd .Text}}</wk-callout>{{end}}

{{define "block-table"}}<wk-table><table>
  <thead><tr>{{range .Cols}}<th>{{.}}</th>{{end}}</tr></thead>
  <tbody>
{{- range .Rows}}
    <tr>{{range .}}<td>{{inlineMd .}}</td>{{end}}</tr>
{{- end}}
  </tbody>
</table></wk-table>{{end}}

{{define "block-kv"}}<wk-kv>{{range .KV}}<wk-kv-row><wk-kv-label>{{.K}}</wk-kv-label><wk-kv-value>{{inlineMd .V}}</wk-kv-value></wk-kv-row>
{{end}}</wk-kv>{{end}}

{{define "block-list"}}{{if .Ordered}}<ol class="brief-list">
{{- range .Items}}
  <li>{{inlineMd .}}</li>
{{- end}}
</ol>{{else}}<ul class="brief-list">
{{- range .Items}}
  <li>{{inlineMd .}}</li>
{{- end}}
</ul>{{end}}{{end}}

{{define "block-panel"}}<wk-panel{{with .Accent}} style="border-left: 3px solid var(--{{.}})"{{end}}>
  <wk-panel-title>{{.Title}}</wk-panel-title>
{{- with .Subtitle}}
  <wk-panel-subtitle>{{inlineMd .}}</wk-panel-subtitle>
{{- end}}
</wk-panel>{{end}}

{{define "block-progress"}}<wk-progress>
  <wk-progress-bar><wk-progress-fill style="width:{{.Percent}}%"></wk-progress-fill></wk-progress-bar>
{{- with .Label}}
  <wk-progress-label>{{.}}</wk-progress-label>
{{- end}}
</wk-progress>{{end}}

{{define "block-graph"}}<wk-panel>
  <div id="cy-graph" class="cy-container"></div>
</wk-panel>{{end}}

{{define "block-chart"}}<div class="present-chart"{{with .Title}} data-chart-title="{{.}}"{{end}}>
  <div class="present-chart-canvas"><canvas></canvas></div>
  <script type="application/json" class="chart-spec">{{chartSpec .}}</script>
</div>{{end}}

{{define "block-code"}}<pre class="wk-code-block"><code class="language-{{langClass .Lang}}">{{.Text}}</code></pre>{{end}}

{{define "block-html"}}{{rawHTML .Text}}{{end}}`

// normalize applies name normalization to every text field in the Doc so
// names like JIRA render as words, not spelled-out acronyms. Code blocks are
// left alone: their text is verbatim, and turning `a & b` into `a and b`
// would change the program shown.
func (d *Doc) normalize() {
	d.Summary = normalizeNames(d.Summary)
	d.Meta = normalizeNames(d.Meta)
	for i := range d.Chips {
		d.Chips[i].Text = normalizeNames(d.Chips[i].Text)
	}
	for i := range d.Sections {
		d.Sections[i].Heading = normalizeNames(d.Sections[i].Heading)
		for j := range d.Sections[i].Blocks {
			b := &d.Sections[i].Blocks[j]
			if b.T == "code" {
				continue
			}
			b.Text = normalizeNames(b.Text)
			b.Title = normalizeNames(b.Title)
			b.Subtitle = normalizeNames(b.Subtitle)
			b.Label = normalizeNames(b.Label)
			for k := range b.Items {
				b.Items[k] = normalizeNames(b.Items[k])
			}
			for k := range b.KV {
				b.KV[k].K = normalizeNames(b.KV[k].K)
				b.KV[k].V = normalizeNames(b.KV[k].V)
			}
			for k := range b.Cols {
				b.Cols[k] = normalizeNames(b.Cols[k])
			}
			for k := range b.Rows {
				for l := range b.Rows[k] {
					b.Rows[k][l] = normalizeNames(b.Rows[k][l])
				}
			}
		}
	}
}

// chartSpec marshals a chart block to the JSON spec the client-side chart
// bootstrap reads to build the Chart.js instance. It is embedded inside a <script type="application/json"> island,
// which html/template treats as a script context — so we return template.JS to
// emit the JSON verbatim instead of letting the escaper re-encode it as a JS
// string literal. encoding/json already escapes <, >, & to \u-escapes, so an
// injected </script> in the data stays inert and cannot break out of the tag.
func chartSpec(b Block) template.JS {
	spec := struct {
		Kind   string        `json:"kind"`
		Title  string        `json:"title,omitempty"`
		Unit   string        `json:"unit,omitempty"`
		XUnit  string        `json:"xunit,omitempty"`
		Series []ChartSeries `json:"series"`
		Flows  []ChartFlow   `json:"flows,omitempty"`
	}{Kind: b.Kind, Title: b.Title, Unit: b.Unit, XUnit: b.XUnit, Series: b.Series, Flows: b.Flows}
	out, err := json.Marshal(spec)
	if err != nil {
		return template.JS(`{"kind":"","series":[]}`)
	}
	return template.JS(out)
}

// RenderDoc converts a Doc to an HTML body fragment.
func RenderDoc(d Doc, title string) (string, error) {
	d.normalize()
	title = normalizeNames(title)

	type docWithTitle struct {
		Doc
		Title string
	}
	var buf bytes.Buffer
	if err := docTemplate.Execute(&buf, docWithTitle{Doc: d, Title: title}); err != nil {
		return "", fmt.Errorf("render doc: %w", err)
	}
	return buf.String(), nil
}
