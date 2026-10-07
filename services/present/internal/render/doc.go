// Package render compiles a page's structured JSON source into the HTML
// fragment stored on disk: RenderDoc turns a Doc into a briefing-block fragment,
// RenderGraph turns a GraphInput into the Cytoscape init script, and the upgrade
// path deterministically rewrites legacy raw-HTML pages. The server serves the
// rendered fragment; the browser assembles the full page client-side.
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/webkit"
)

// Doc is the structured input format for page content. The model passes this
// as JSON; the MCP layer calls RenderDoc to produce the HTML body fragment
// stored in content.html.
type Doc struct {
	Summary  string    `json:"summary,omitempty"`
	Meta     string    `json:"meta,omitempty"`
	Chips    []Chip    `json:"chips,omitempty"`
	Sections []Section `json:"sections"`

	// Deck chrome: the view's furniture around the slides, read by the deck
	// view only; the brief ignores every one of them, and RenderDoc emits
	// nothing for them. All optional; a Doc that sets none renders as it did
	// before they existed. In the deck rendition (RenderDeck) they travel to
	// the browser as one JSON script island at the head of the fragment,
	// emitted only when at least one is set, and the presenter becomes a
	// byline under the meta line.
	Logo         string `json:"logo,omitempty"`          // "" = the embedded repo logo, "none" hides it, an http(s) URL replaces it
	LogoPosition string `json:"logo_position,omitempty"` // bottom-right (default), bottom-left, top-left, top-right
	Progress     string `json:"progress,omitempty"`      // dots (default), bar, none
	Presenter    string `json:"presenter,omitempty"`     // byline under the meta line on the title slide and in the footer
	Footer       string `json:"footer,omitempty"`        // footer text; "" = the deck title, "none" suppresses the line

	// Transition is how the deck view moves between slides: fade (the
	// default), slide (a 24 px nudge in the direction of travel), or none (a
	// cut). It rides in the chrome island only when set and not fade, so a
	// deck without it keeps its stored bytes.
	Transition string `json:"transition,omitempty"`
}

// Deck chrome vocabularies, validated at compile time like a panel accent.
var (
	logoPositions = []string{"bottom-right", "bottom-left", "top-left", "top-right"}
	progressKinds = []string{"dots", "bar", "none"}
	transitions   = []string{"fade", "slide", "none"}
	layouts       = []string{"default", "center", "statement", "section"}
	// chartKinds is the chart block's kind vocabulary. An empty kind draws
	// as bar, the client's default; any other name outside the list is
	// refused at compile time instead of drawing as a line chart.
	chartKinds = []string{
		"bar", "line", "area", "sparkline", "stacked-bar", "horizontal-bar", "doughnut", "scatter", "sankey",
	}
)

// ChartKinds returns the chart kinds a chart block may name, in the order
// the docs list them. The MCP schema text and the skill pin the same list.
func ChartKinds() []string { return slices.Clone(chartKinds) }

// chromeTransition is the transition value the island carries: empty for
// the default fade, which the deck view assumes when the island says
// nothing.
func (d Doc) chromeTransition() string {
	if d.Transition == "fade" {
		return ""
	}
	return d.Transition
}

// hasChrome reports whether any deck chrome field is set, which is when the
// renderer emits the chrome island.
func (d Doc) hasChrome() bool {
	return d.Logo != "" || d.LogoPosition != "" || d.Progress != "" || d.Presenter != "" ||
		d.Footer != "" || d.chromeTransition() != ""
}

// Chip is an inline tag rendered in a chip-row.
type Chip struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"` // stat, a, b, c, outline; empty = plain
}

// Section is a content section with a heading and blocks.
type Section struct {
	Heading string `json:"h"`
	// Deprecated: accepted and ignored since MAD-377, which removed the section
	// id badge. The field stays so stored doc.json and deck.json and the MCP
	// callers that still send it keep parsing.
	ID     string  `json:"id,omitempty"`
	Blocks []Block `json:"blocks"`

	// Slide fields, all optional. The deck view reads Layout, Notes, and
	// Reveal and the brief ignores them; Tone is honoured by both. Unset
	// fields emit nothing, so a section without them renders as before.
	Layout string `json:"layout,omitempty"` // default, center, statement, section
	Tone   string `json:"tone,omitempty"`   // a palette role (webkit.Roles): tints the slide, bands the brief's section
	Notes  string `json:"notes,omitempty"`  // speaker notes, inline markdown, shown in the deck's drawer
	Reveal bool   `json:"reveal,omitempty"` // list items and top-level blocks appear one per Next
}

// dataLayout is the data-layout attribute value: the layout when it is set
// and not the default.
func (s Section) dataLayout() string {
	if s.Layout == "default" {
		return ""
	}
	return s.Layout
}

// Block is a discriminated union on T.
type Block struct {
	T string `json:"t"` // p, h3, callout, table, kv, list, panel, progress, graph, chart, code, html, columns, stat, quote, details, image

	// t=p, t=callout, t=h3, t=code, t=html, t=quote
	Text string `json:"text,omitempty"`

	// t=code
	Lang string `json:"lang,omitempty"` // language badge + Prism grammar hint; empty = "text"

	// t=callout
	Severity string `json:"sev,omitempty"` // info, warn, ok, error

	// t=table. Cols shares its wire key with a columns block's Columns; the
	// custom (un)marshal below tells them apart by T.
	Cols []string   `json:"cols,omitempty"`
	Rows [][]string `json:"rows,omitempty"`

	// t=columns: two or three columns of blocks, equal widths, one column on
	// a narrow screen. Wire key cols. A column may hold any block but
	// columns and details; the page's one graph may sit in a column.
	Columns [][]Block `json:"-"`

	// t=stat: a large figure (Value, shown verbatim) over a Label, with an
	// optional Subtitle line.
	Value string `json:"value,omitempty"`

	// t=quote: Text is the quotation, Cite the attribution.
	Cite string `json:"cite,omitempty"`

	// t=details: a collapsible Summary line over Blocks, closed by default.
	// The body may hold any block but graph, columns, and details.
	Summary string  `json:"summary,omitempty"`
	Blocks  []Block `json:"blocks,omitempty"`

	// t=image: Src is an http(s) URL or the served path of an image in the
	// local store (the MCP tools turn a file on this machine into one before
	// the Doc is compiled); Alt is required, since read-aloud reads it in
	// the image's place; Caption is optional prose under the image.
	Src     string `json:"src,omitempty"`
	Alt     string `json:"alt,omitempty"`
	Caption string `json:"caption,omitempty"`

	// t=kv
	KV []KVPair `json:"kv,omitempty"`

	// t=list
	Items   []string `json:"items,omitempty"`
	Ordered bool     `json:"ordered,omitempty"`

	// t=panel
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"sub,omitempty"`
	Accent   string `json:"accent,omitempty"` // a palette role (webkit.Roles) or the alias terracotta

	// t=progress
	Percent float64 `json:"pct,omitempty"`
	Label   string  `json:"label,omitempty"`

	// t=chart
	Kind   string        `json:"kind,omitempty"`   // one of ChartKinds; empty draws as bar
	Unit   string        `json:"unit,omitempty"`   // value-axis unit label, e.g. ms
	XUnit  string        `json:"xunit,omitempty"`  // x-axis unit label (scatter only)
	Series []ChartSeries `json:"series,omitempty"` // every kind except sankey
	Flows  []ChartFlow   `json:"flows,omitempty"`  // sankey only

	// Steps makes a chart walk through its data on a deck slide, one step
	// per Next: each entry is the caption shown under the chart at that
	// step, and a series whose Step names one appears at that step. The brief
	// shows the finished chart with the captions as a numbered list.
	Steps []ChartStep `json:"steps,omitempty"`
}

// blockFields is Block without its methods, so the (un)marshalers below can
// hand the plain struct to encoding/json without recursing into themselves.
type blockFields Block

// UnmarshalJSON reads cols by block type: a table's cols are its header
// strings, a columns block's cols are its arrays of blocks. Every other
// field decodes as the struct tags say.
func (b *Block) UnmarshalJSON(data []byte) error {
	var raw struct {
		blockFields
		Cols json.RawMessage `json:"cols"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*b = Block(raw.blockFields)
	if len(raw.Cols) == 0 || string(raw.Cols) == "null" {
		return nil
	}
	if b.T == "columns" {
		if err := json.Unmarshal(raw.Cols, &b.Columns); err != nil {
			return fmt.Errorf("columns block: cols must be arrays of blocks: %w", err)
		}
		return nil
	}
	if err := json.Unmarshal(raw.Cols, &b.Cols); err != nil {
		return fmt.Errorf("%s block: cols must be strings: %w", b.T, err)
	}
	return nil
}

// MarshalJSON writes a columns block's Columns under cols; every other block
// marshals exactly as before, so the canonical JSON the store keeps for an
// existing page does not move.
func (b Block) MarshalJSON() ([]byte, error) {
	if b.T != "columns" {
		return json.Marshal(blockFields(b))
	}
	return json.Marshal(struct {
		blockFields
		Cols [][]Block `json:"cols,omitempty"`
	}{blockFields(b), b.Columns})
}

// KVPair is a label-value pair within a kv block.
type KVPair struct {
	K string `json:"k"`
	V string `json:"v"`
}

// ChartSeries is one data series in a chart block.
type ChartSeries struct {
	Name   string       `json:"name,omitempty"`
	Color  string       `json:"color,omitempty"` // a series role (series-1 to series-4) or a legacy name; empty = auto by index
	Points []ChartPoint `json:"points"`
	Step   int          `json:"step,omitempty"` // 1-based step the series first shows at; 0 = from the start
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

// ChartStep is one step of a stepped chart: the caption the deck shows
// under the chart at that step and the brief lists under it.
type ChartStep struct {
	Caption string `json:"caption"`
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
	// The placeholder tokens below are NUL-delimited and swapped back at
	// their first match, so a NUL in the input could forge a token and splice
	// one generated piece into another's attribute. CommonMark's rule for
	// NUL, replace it with U+FFFD, removes the only character a forgery
	// needs; after this line every NUL in s is one this function wrote.
	s = strings.ReplaceAll(s, "\x00", "�")

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

	// Links: [text](url). A url outside the allowlist stays literal text, and
	// so does one holding a placeholder token: the token would be swapped
	// back inside the href attribute, and generated markup belongs in text
	// content only.
	replace(reLink, func(p []string) string {
		if !LinkHrefAllowed(p[2]) || strings.Contains(p[2], "\x00") {
			return html.EscapeString(p[0])
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(p[2]), html.EscapeString(p[1]))
	})

	// Code: `text`
	replace(reCode, func(p []string) string {
		return fmt.Sprintf(`<code>%s</code>`, html.EscapeString(p[1]))
	})

	// Bold: **text**. The text may hold tokens of the pieces above (a code
	// span or a link inside the bold), which escaping leaves intact.
	replace(reBold, func(p []string) string {
		return fmt.Sprintf(`<strong>%s</strong>`, html.EscapeString(p[1]))
	})

	// Escape the remaining text.
	s = html.EscapeString(s)

	// Restore placeholders (they contain pre-escaped HTML). A restored piece
	// can carry a token of its own, code inside bold say, so keep going until
	// a pass swaps nothing. Every token occurs once, so this ends.
	for swapped := true; swapped; {
		swapped = false
		for _, ph := range phs {
			if !strings.Contains(s, ph.token) {
				continue
			}
			s = strings.Replace(s, ph.token, ph.html, 1)
			swapped = true
		}
	}

	// Italic: *text* — applied after escaping since it doesn't need special chars.
	// Use a simple regex on the escaped output.
	reItalicPost := regexp.MustCompile(`\*([^*]+?)\*`)
	s = reItalicPost.ReplaceAllString(s, `<em>$1</em>`)

	return template.HTML(s)
}

// LinkHrefAllowed reports whether an inline `[text](href)` link in a prose
// field may point at href: http, https, and mailto URLs, plus relative paths
// and fragments (no scheme at all). Anything else, javascript: and data:
// above all, renders as literal text instead of a link, whichever way the
// Doc was authored. It governs inline links only: a `t: html` block is raw
// passthrough by design and is not checked. Whitespace and control
// characters are ignored when reading the scheme, because browsers ignore
// them too and `java\tscript:` would otherwise slip through.
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

// maxBlockDepth is how deep blocks nest. A container (columns, details)
// holds plain blocks only, so a nested block sits at depth 1 and nothing
// sits below it.
const maxBlockDepth = 1

// isContainer reports whether b holds other blocks.
func isContainer(b Block) bool {
	return b.T == "columns" || b.T == "details"
}

func renderError(msg string) template.HTML {
	return template.HTML(fmt.Sprintf(`<!-- render error: %s -->`, html.EscapeString(msg)))
}

// renderBlockAt renders one block at a nesting depth. Past the depth limit,
// or a container below the top level, it emits an error comment instead of
// recursing: validateBlocks refuses such a Doc before it gets here, so this
// is the guard for a Block built in Go that skipped validation.
func renderBlockAt(b Block, depth int) template.HTML {
	if depth > maxBlockDepth || (depth > 0 && isContainer(b)) {
		return renderError(b.T + " block nested too deep")
	}
	var buf bytes.Buffer
	if err := blockTemplates.ExecuteTemplate(&buf, "block-"+b.T, b); err != nil {
		return renderError(err.Error())
	}
	return template.HTML(buf.String())
}

func init() {
	// renderBlock and renderNested read blockTemplates when a template runs,
	// never at parse time, so the block templates can call them to render
	// the blocks a container holds.
	funcs := template.FuncMap{
		"inlineMd":     inlineMd,
		"sectionID":    sectionID,
		"langClass":    langClass,
		"rawHTML":      func(s string) template.HTML { return template.HTML(s) },
		"chartSpec":    chartSpec,
		"renderBlock":  func(b Block) template.HTML { return renderBlockAt(b, 0) },
		"renderNested": func(b Block) template.HTML { return renderBlockAt(b, 1) },
		"dataLayout":   func(s Section) string { return s.dataLayout() },
		"accentRole":   accentRole,
	}

	blockTemplates = template.Must(
		template.New("blocks").Funcs(funcs).Parse(blockTemplatesSrc),
	)
	docTemplate = template.Must(
		template.New("doc").Funcs(funcs).Parse(docTemplateSrc),
	)
}

const docTemplateSrc = `{{with .Chrome}}<script type="application/json" class="deck-chrome">{{.}}</script>
{{end}}<h1 class="brief-title" data-fixation>{{.Title}}</h1>
{{- with .Meta}}
<div class="brief-meta">{{.}}</div>
{{- end}}
{{- if .Deck}}{{with .Presenter}}
<div class="brief-presenter">{{.}}</div>
{{- end}}{{end}}
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
  <ul data-fixation>
{{- range .Sections}}
    <li><a href="#{{sectionID .Heading}}">{{.Heading}}</a></li>
{{- end}}
  </ul>
</wk-toc>
{{- end}}
{{- range .Sections}}
<wk-section id="{{sectionID .Heading}}"{{with dataLayout .}} data-layout="{{.}}"{{end}}{{with .Tone}} data-tone="{{.}}" style="--slide-accent: var(--{{.}})"{{end}}{{if .Reveal}} data-reveal="true"{{end}}>
  <wk-section-heading data-fixation>{{.Heading}}</wk-section-heading>
{{- range .Blocks}}
  {{renderBlock .}}
{{- end}}
{{- with .Notes}}
  <template class="deck-notes">{{inlineMd .}}</template>
{{- end}}
</wk-section>
{{- end}}`

const blockTemplatesSrc = `{{define "block-p"}}<p data-fixation>{{inlineMd .Text}}</p>{{end}}

{{define "block-h3"}}<wk-section-subheading data-fixation>{{.Text}}</wk-section-subheading>{{end}}

{{define "block-callout"}}<wk-callout{{with .Severity}} variant="{{.}}"{{end}} data-fixation>{{inlineMd .Text}}</wk-callout>{{end}}

{{define "block-table"}}<wk-table><table>
  <thead><tr>{{range .Cols}}<th>{{.}}</th>{{end}}</tr></thead>
  <tbody data-fixation>
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

{{define "block-panel"}}<wk-panel{{with .Accent}} style="border-left: 3px solid var(--{{accentRole .}})"{{end}}>
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

{{define "block-chart"}}<div class="present-chart"{{with .Title}} data-chart-title="{{.}}"{{end}}{{with .Steps}} data-steps="{{len .}}"{{end}}>
  <div class="present-chart-canvas"><canvas></canvas></div>
  <script type="application/json" class="chart-spec">{{chartSpec .}}</script>
{{- with .Steps}}
  <ol class="present-steps">
{{- range .}}
    <li data-fixation>{{inlineMd .Caption}}</li>
{{- end}}
  </ol>
{{- end}}
</div>{{end}}

{{define "block-code"}}<pre class="wk-code-block"><code class="language-{{langClass .Lang}}">{{.Text}}</code></pre>{{end}}

{{define "block-html"}}{{rawHTML .Text}}{{end}}

{{define "block-columns"}}<wk-columns cols="{{len .Columns}}">
{{- range .Columns}}
  <wk-col>
{{- range .}}
    {{renderNested .}}
{{- end}}
  </wk-col>
{{- end}}
</wk-columns>{{end}}

{{define "block-stat"}}<wk-stat>
  <wk-stat-value>{{.Value}}</wk-stat-value>
  <wk-stat-label data-fixation>{{inlineMd .Label}}</wk-stat-label>
{{- with .Subtitle}}
  <wk-stat-sub data-fixation>{{inlineMd .}}</wk-stat-sub>
{{- end}}
</wk-stat>{{end}}

{{define "block-quote"}}<blockquote>
  <p data-fixation>{{inlineMd .Text}}</p>
{{- with .Cite}}
  <cite>{{inlineMd .}}</cite>
{{- end}}
</blockquote>{{end}}

{{define "block-details"}}<details>
  <summary data-fixation>{{inlineMd .Summary}}</summary>
{{- range .Blocks}}
  {{renderNested .}}
{{- end}}
</details>{{end}}

{{define "block-image"}}<wk-figure>
  <img src="{{.Src}}" alt="{{.Alt}}" loading="lazy">
{{- with .Caption}}
  <wk-figcaption data-fixation>{{inlineMd .}}</wk-figcaption>
{{- end}}
</wk-figure>{{end}}`

// normalize applies name normalization to every text field in the Doc so
// names like JIRA render as words, not spelled-out acronyms. Code blocks are
// left alone: their text is verbatim, and turning `a & b` into `a and b`
// would change the program shown.
func (d *Doc) normalize() {
	d.Summary = normalizeNames(d.Summary)
	d.Meta = normalizeNames(d.Meta)
	d.Presenter = normalizeNames(d.Presenter)
	d.Footer = normalizeNames(d.Footer)
	for i := range d.Chips {
		d.Chips[i].Text = normalizeNames(d.Chips[i].Text)
	}
	for i := range d.Sections {
		d.Sections[i].Heading = normalizeNames(d.Sections[i].Heading)
		d.Sections[i].Notes = normalizeNames(d.Sections[i].Notes)
		normalizeBlocks(d.Sections[i].Blocks, 0)
	}
}

// normalizeBlocks normalizes every text field of blocks in place, following
// a container into the blocks it holds down to maxBlockDepth.
func normalizeBlocks(blocks []Block, depth int) {
	_ = eachBlock(blocks, depth, func(b *Block) error {
		normalizeBlock(b)
		return nil
	})
}

// normalizeBlock normalizes one block's prose fields. A code block's text
// is verbatim, and a stat's Value is a figure shown verbatim: its symbols
// are the point ("4×", "≈ 40%"), so neither is normalized like prose.
func normalizeBlock(b *Block) {
	if b.T == "code" {
		return
	}
	b.Text = normalizeNames(b.Text)
	b.Title = normalizeNames(b.Title)
	b.Subtitle = normalizeNames(b.Subtitle)
	b.Label = normalizeNames(b.Label)
	b.Cite = normalizeNames(b.Cite)
	b.Summary = normalizeNames(b.Summary)
	b.Alt = normalizeNames(b.Alt)
	b.Caption = normalizeNames(b.Caption)
	for k := range b.Items {
		b.Items[k] = normalizeNames(b.Items[k])
	}
	for k := range b.Steps {
		b.Steps[k].Caption = normalizeNames(b.Steps[k].Caption)
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

// EachBlock calls fn on every block of the Doc in document order, the
// blocks a container holds included, and stops at the first error. The
// MCP tools use it to rewrite image sources before a Doc is compiled.
func (d *Doc) EachBlock(fn func(*Block) error) error {
	for i := range d.Sections {
		if err := eachBlock(d.Sections[i].Blocks, 0, fn); err != nil {
			return err
		}
	}
	return nil
}

// eachBlock visits blocks and what their containers hold, down to
// maxBlockDepth, the depth the renderer follows.
func eachBlock(blocks []Block, depth int, fn func(*Block) error) error {
	if depth > maxBlockDepth {
		return nil
	}
	for i := range blocks {
		b := &blocks[i]
		if err := fn(b); err != nil {
			return err
		}
		for k := range b.Columns {
			if err := eachBlock(b.Columns[k], depth+1, fn); err != nil {
				return err
			}
		}
		if err := eachBlock(b.Blocks, depth+1, fn); err != nil {
			return err
		}
	}
	return nil
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

// accentAliases are accent names older pages carry outside the palette
// roles, each mapped to the role it stands for. The alias is accepted on
// input and rendered as the role, so a page's HTML carries only names webkit
// declares for every family. Pages rendered before the mapping store
// var(--terracotta) in their HTML; shell.html declares that name as
// --primary so they keep resolving until a rerender rewrites them (MAD-371).
var accentAliases = map[string]string{"terracotta": "primary"}

// validAccent reports whether name may follow a panel's accent field: a
// palette role every family resolves, or a legacy alias.
func validAccent(name string) bool {
	_, alias := accentAliases[name]
	return alias || webkit.IsRole(name)
}

// accentRole returns the palette role a panel accent renders as: the role an
// alias stands for, or the name itself.
func accentRole(name string) string {
	if role, ok := accentAliases[name]; ok {
		return role
	}
	return name
}

// isGraph reports whether b places the page's graph.
func isGraph(b Block) bool { return b.T == "graph" }

// countGraphs counts the graph blocks in blocks and in the columns they
// hold. A details block never holds one, so it is not looked into.
func countGraphs(blocks []Block) int {
	n := 0
	for _, b := range blocks {
		if isGraph(b) {
			n++
		}
		for _, col := range b.Columns {
			n += countGraphs(col)
		}
	}
	return n
}

// validateBlocks returns an error for the first block the renderer refuses:
// a panel accent the palette does not define (the way validateTones does for
// graph nodes), a columns block with other than two or three columns, a
// details block without a summary or without blocks, a container below the
// top level, since a container holds plain blocks only, a graph inside a
// details block, where a closed disclosure would hide the page's one graph,
// and a chart kind outside chartKinds. A graph may sit in a column.
func validateBlocks(blocks []Block, depth int) error {
	for _, b := range blocks {
		if depth > 0 && isContainer(b) {
			return fmt.Errorf("%s block: not allowed inside a columns or details block", b.T)
		}
		switch b.T {
		case "panel":
			if b.Accent != "" && !validAccent(b.Accent) {
				return fmt.Errorf(
					"panel %q: unknown accent %q (want one of %s)",
					b.Title, b.Accent, strings.Join(webkit.Roles(), ", "),
				)
			}
		case "columns":
			if n := len(b.Columns); n < 2 || n > 3 {
				return fmt.Errorf("columns block: want 2 or 3 columns in cols, got %d", n)
			}
			for _, col := range b.Columns {
				if err := validateBlocks(col, depth+1); err != nil {
					return err
				}
			}
		case "details":
			if strings.TrimSpace(b.Summary) == "" {
				return errors.New("details block: summary is required")
			}
			if len(b.Blocks) == 0 {
				return fmt.Errorf("details %q: blocks is empty", b.Summary)
			}
			if slices.ContainsFunc(b.Blocks, isGraph) {
				return fmt.Errorf("graph block: not allowed inside a details block (%q)", b.Summary)
			}
			if slices.ContainsFunc(b.Blocks, hasSteps) {
				return fmt.Errorf("chart steps: not allowed inside a details block (%q)", b.Summary)
			}
			if err := validateBlocks(b.Blocks, depth+1); err != nil {
				return err
			}
		case "image":
			if err := validateImage(b); err != nil {
				return err
			}
		case "chart":
			if err := validateChart(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasSteps reports whether b is a chart that walks through steps.
func hasSteps(b Block) bool { return b.T == "chart" && len(b.Steps) > 0 }

// validateChart refuses a kind outside chartKinds, a step without a
// caption, a series step outside 1..len(steps) or on a chart without
// steps, steps on a sparkline (a 56 px strip with no room for captions),
// and a series step on a doughnut, which draws its first series only.
func validateChart(b Block) error {
	if b.Kind != "" && !slices.Contains(chartKinds, b.Kind) {
		return fmt.Errorf(
			"chart %q: unknown kind %q (want one of %s)",
			b.Title, b.Kind, strings.Join(chartKinds, ", "),
		)
	}
	if len(b.Steps) > 0 && b.Kind == "sparkline" {
		return fmt.Errorf("chart %q: a sparkline cannot carry steps", b.Title)
	}
	for k, st := range b.Steps {
		if strings.TrimSpace(st.Caption) == "" {
			return fmt.Errorf("chart %q: step %d has no caption", b.Title, k+1)
		}
	}
	for _, s := range b.Series {
		if s.Step == 0 {
			continue
		}
		if len(b.Steps) == 0 {
			return fmt.Errorf("chart %q: series %q names step %d but the chart has no steps", b.Title, s.Name, s.Step)
		}
		if s.Step < 1 || s.Step > len(b.Steps) {
			return fmt.Errorf("chart %q: series %q step %d: want 1 to %d", b.Title, s.Name, s.Step, len(b.Steps))
		}
		if b.Kind == "doughnut" {
			return fmt.Errorf("chart %q: a doughnut draws its first series only, so a series cannot carry a step", b.Title)
		}
	}
	return nil
}

// validateImage refuses an image without alt, which read-aloud reads in the
// image's place, and a src that is neither an http(s) URL nor the served
// path of a stored image.
func validateImage(b Block) error {
	if strings.TrimSpace(b.Alt) == "" {
		return fmt.Errorf(
			"image %q: alt is required (read-aloud reads it in the image's place)",
			b.Src,
		)
	}
	if !ImageSrcAllowed(b.Src) {
		return fmt.Errorf(
			"image %q: src must be an http or https URL or a stored image path %s<sha256 hex>.<png|jpg|gif|webp>",
			b.Src,
			images.Prefix,
		)
	}
	return nil
}

// ImageSrcAllowed reports whether an image block may point at src: an
// absolute http or https URL, or the served path of an image in the local
// store. A file path is neither; the MCP tools turn a file on this machine
// into a stored image before the Doc reaches the renderer.
func ImageSrcAllowed(src string) bool {
	_, stored := images.NameOf(src)
	return stored || IsHTTPURL(src)
}

// IsHTTPURL reports whether s is an absolute http or https URL, the only
// kind of URL a logo or an image may point at.
func IsHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validateChrome refuses a deck chrome field outside its vocabulary, like an
// unknown graph tone: an unknown logo_position or progress value, and a logo
// that is neither "none" nor an http(s) URL.
func validateChrome(d Doc) error {
	if d.LogoPosition != "" && !slices.Contains(logoPositions, d.LogoPosition) {
		return fmt.Errorf(
			"logo_position %q: want one of %s", d.LogoPosition, strings.Join(logoPositions, ", "),
		)
	}
	if d.Progress != "" && !slices.Contains(progressKinds, d.Progress) {
		return fmt.Errorf(
			"progress %q: want one of %s",
			d.Progress,
			strings.Join(progressKinds, ", "),
		)
	}
	if d.Logo != "" && d.Logo != "none" && !IsHTTPURL(d.Logo) {
		return fmt.Errorf(`logo %q: want "none" or an http(s) URL`, d.Logo)
	}
	if d.Transition != "" && !slices.Contains(transitions, d.Transition) {
		return fmt.Errorf(
			"transition %q: want one of %s", d.Transition, strings.Join(transitions, ", "),
		)
	}
	return nil
}

// validateSection refuses a layout outside its vocabulary and a tone that is
// not a palette role. A tone is a role name only: the panel accent's legacy
// alias is not one, because the slide reads the role as var(--<tone>) and
// an alias would pin the page to one family's choice.
func validateSection(s Section) error {
	if s.Layout != "" && !slices.Contains(layouts, s.Layout) {
		return fmt.Errorf(
			"section %q: unknown layout %q (want one of %s)",
			s.Heading, s.Layout, strings.Join(layouts, ", "),
		)
	}
	if s.Tone != "" && !webkit.IsRole(s.Tone) {
		return fmt.Errorf(
			"section %q: unknown tone %q (want one of %s)",
			s.Heading, s.Tone, strings.Join(webkit.Roles(), ", "),
		)
	}
	return nil
}

// validateDoc refuses a Doc the renderer cannot honour: a chrome field or a
// section field outside its vocabulary, a block validateBlocks refuses, and
// more than one graph block, since a page has one graph and the page view
// mounts it in the one container the graph block renders.
func validateDoc(d Doc) error {
	if err := validateChrome(d); err != nil {
		return err
	}
	graphs := 0
	for _, s := range d.Sections {
		if err := validateSection(s); err != nil {
			return err
		}
		if err := validateBlocks(s.Blocks, 0); err != nil {
			return err
		}
		graphs += countGraphs(s.Blocks)
	}
	if graphs > 1 {
		return fmt.Errorf("graph block: a page places its one graph once, found %d", graphs)
	}
	return nil
}

// chromeSpec marshals the deck chrome fields to the JSON island the deck
// view reads, under the same script-context rules as chartSpec. Empty when
// no field is set, so the template emits no island.
func chromeSpec(d Doc) template.JS {
	if !d.hasChrome() {
		return ""
	}
	spec := struct {
		Logo         string `json:"logo,omitempty"`
		LogoPosition string `json:"logo_position,omitempty"`
		Progress     string `json:"progress,omitempty"`
		Presenter    string `json:"presenter,omitempty"`
		Footer       string `json:"footer,omitempty"`
		Transition   string `json:"transition,omitempty"`
	}{d.Logo, d.LogoPosition, d.Progress, d.Presenter, d.Footer, d.chromeTransition()}
	out, err := json.Marshal(spec)
	if err != nil {
		return "{}"
	}
	return template.JS(out)
}

// RenderDoc converts a Doc to the brief's HTML body fragment. The deck
// chrome fields are validated but emit nothing: the brief ignores them.
func RenderDoc(d Doc, title string) (string, error) {
	return renderDoc(d, title, false)
}

// RenderDeck converts a Doc to the deck's HTML body fragment: the brief's
// markup plus the chrome island at its head and the presenter byline under
// the meta line, each only when the Doc sets the field.
func RenderDeck(d Doc, title string) (string, error) {
	return renderDoc(d, title, true)
}

func renderDoc(d Doc, title string, deck bool) (string, error) {
	if err := validateDoc(d); err != nil {
		return "", fmt.Errorf("render doc: %w", err)
	}
	d.normalize()
	title = normalizeNames(title)

	type docWithTitle struct {
		Doc
		Title  string
		Deck   bool
		Chrome template.JS
	}
	data := docWithTitle{Doc: d, Title: title, Deck: deck}
	if deck {
		data.Chrome = chromeSpec(d)
	}
	var buf bytes.Buffer
	if err := docTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render doc: %w", err)
	}
	return buf.String(), nil
}
