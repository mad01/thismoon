package render

import (
	"strings"
	"testing"
)

// TestUpgradeLegacyHTML drives the deterministic legacy-class → wk-* tag rewrite.
// Each case mirrors one block type from the migration recorded in the
// internal/render/doc.go + render.go diff (the authoritative mapping).
func TestUpgradeLegacyHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string // substrings that MUST be present
		gone []string // substrings that MUST NOT be present
	}{
		{
			name: "section div becomes wk-section keeping id",
			in:   `<div class="section" id="overview"><p>x</p></div>`,
			want: []string{`<wk-section id="overview">`, `</wk-section>`, `<p>x</p>`},
			gone: []string{`class="section"`, `<div`},
		},
		{
			name: "section heading with section-id span",
			in:   `<h2 class="section-heading"><span class="section-id">A1</span>Heading</h2>`,
			want: []string{
				`<wk-section-heading>`,
				`<wk-section-id>A1</wk-section-id>`,
				`Heading`,
				`</wk-section-heading>`,
			},
			gone: []string{`class="section-heading"`, `class="section-id"`, `<h2`, `<span`},
		},
		{
			name: "section-text paragraph drops class keeps data-bionic",
			in:   `<p class="section-text" data-bionic>body</p>`,
			want: []string{`data-bionic`, `body`, `<p`},
			gone: []string{`class="section-text"`},
		},
		{
			name: "h3 subheading becomes wk-section-subheading",
			in:   `<h3 class="section-subheading">Sub</h3>`,
			want: []string{`<wk-section-subheading>Sub</wk-section-subheading>`},
			gone: []string{`class="section-subheading"`, `<h3`},
		},
		{
			name: "toc nav becomes wk-toc",
			in:   `<nav class="toc"><div class="toc-title">Sections</div><ul class="toc-list"><li><a href="#a">A</a></li></ul></nav>`,
			want: []string{
				`<wk-toc>`,
				`<wk-toc-title>Sections</wk-toc-title>`,
				`<ul>`,
				`<li><a href="#a">A</a></li>`,
				`</wk-toc>`,
			},
			gone: []string{`class="toc"`, `class="toc-title"`, `class="toc-list"`, `<nav`},
		},
		{
			name: "consecutive kv-rows wrapped in single wk-kv",
			in:   `<div class="kv-row"><span class="kv-label">L1</span><span class="kv-value">V1</span></div><div class="kv-row"><span class="kv-label">L2</span><span class="kv-value">V2</span></div>`,
			want: []string{
				`<wk-kv><wk-kv-row><wk-kv-label>L1</wk-kv-label><wk-kv-value>V1</wk-kv-value></wk-kv-row>`,
				`<wk-kv-row><wk-kv-label>L2</wk-kv-label><wk-kv-value>V2</wk-kv-value></wk-kv-row></wk-kv>`,
			},
			gone: []string{`class="kv-row"`, `class="kv-label"`, `class="kv-value"`},
		},
		{
			// Older pre-spec kv variant found in real legacy pages: the row wraps
			// an inner .kv div holding kv-key/kv-val spans. Collapse to the modern
			// wk-kv-row > wk-kv-label/wk-kv-value shape and drop the inner wrapper.
			name: "older kv variant with kv-key and kv-val",
			in:   `<div class="kv-row"><div class="kv"><span class="kv-key">Part 1</span><span class="kv-val">Collect</span></div></div>`,
			want: []string{
				`<wk-kv><wk-kv-row><wk-kv-label>Part 1</wk-kv-label><wk-kv-value>Collect</wk-kv-value></wk-kv-row></wk-kv>`,
			},
			gone: []string{`class="kv-row"`, `class="kv"`, `class="kv-key"`, `class="kv-val"`},
		},
		{
			name: "progress block",
			in:   `<div class="progress-inline"><div class="progress-bar"><div class="progress-fill" style="width:42%"></div></div><span class="progress-label">42%</span></div>`,
			want: []string{
				`<wk-progress>`,
				`<wk-progress-bar><wk-progress-fill style="width:42%"></wk-progress-fill></wk-progress-bar>`,
				`<wk-progress-label>42%</wk-progress-label>`,
				`</wk-progress>`,
			},
			gone: []string{
				`class="progress-inline"`,
				`class="progress-bar"`,
				`class="progress-fill"`,
				`class="progress-label"`,
			},
		},
		{
			name: "callout info default",
			in:   `<div class="callout callout-info" data-bionic>note</div>`,
			// The html serializer normalizes a bare boolean attribute to ="" —
			// semantically identical to the renderer's `data-bionic`.
			want: []string{`<wk-callout variant="info"`, `data-bionic`, `>note</wk-callout>`},
			gone: []string{`class="callout`},
		},
		{
			name: "callout warn",
			in:   `<div class="callout callout-warn">careful</div>`,
			want: []string{`<wk-callout variant="warn">careful</wk-callout>`},
			gone: []string{`class="callout`},
		},
		{
			name: "plain callout no variant",
			in:   `<div class="callout">plain</div>`,
			want: []string{`<wk-callout>plain</wk-callout>`},
			gone: []string{`class="callout"`, `variant=`},
		},
		{
			name: "chips become wk-badge with variant",
			in:   `<span class="chip chip-a">live</span><span class="chip chip-stat">42</span>`,
			want: []string{
				`<wk-badge variant="a">live</wk-badge>`,
				`<wk-badge variant="stat">42</wk-badge>`,
			},
			gone: []string{`class="chip`},
		},
		{
			name: "plain chip no variant",
			in:   `<span class="chip">plain</span>`,
			want: []string{`<wk-badge>plain</wk-badge>`},
			gone: []string{`class="chip"`, `variant=`},
		},
		{
			name: "data-table wrapped in wk-table",
			in:   `<table class="data-table"><thead><tr><th>A</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table>`,
			want: []string{`<wk-table><table>`, `<th>A</th>`, `<td>1</td>`, `</table></wk-table>`},
			gone: []string{`class="data-table"`},
		},
		{
			name: "panel block",
			in:   `<div class="panel"><div class="panel-title">T</div><div class="panel-subtitle">S</div></div>`,
			want: []string{
				`<wk-panel>`,
				`<wk-panel-title>T</wk-panel-title>`,
				`<wk-panel-subtitle>S</wk-panel-subtitle>`,
				`</wk-panel>`,
			},
			gone: []string{`class="panel"`, `class="panel-title"`, `class="panel-subtitle"`},
		},
		{
			name: "panel keeps accent style",
			in:   `<div class="panel" style="border-left: 3px solid var(--blue)"><div class="panel-title">T</div></div>`,
			want: []string{
				`<wk-panel style="border-left: 3px solid var(--blue)">`,
				`<wk-panel-title>T</wk-panel-title>`,
			},
			gone: []string{`class="panel"`},
		},
		{
			name: "references section keeps refs classes",
			in:   `<div class="section refs-section" id="references"><h2 class="section-heading">References</h2><ul class="refs-list"><li class="refs-item"><a href="https://x">X</a><span class="refs-url">x</span></li></ul></div>`,
			want: []string{
				`<wk-section class="refs-section" id="references">`,
				`<wk-section-heading>References</wk-section-heading>`,
				`class="refs-list"`,
				`class="refs-item"`,
				`class="refs-url"`,
			},
			gone: []string{`class="section refs-section"`, `class="section-heading"`, `<h2`},
		},
		{
			name: "idempotent on already-migrated wk markup",
			in:   `<wk-section id="x"><wk-section-heading>H</wk-section-heading><p data-bionic>t</p><wk-callout variant="info">c</wk-callout></wk-section>`,
			want: []string{
				`<wk-section id="x">`,
				`<wk-section-heading>H</wk-section-heading>`,
				`<wk-callout variant="info">c</wk-callout>`,
			},
			gone: []string{`<div`, `class="section"`},
		},
		{
			name: "leaves brief and chip-row containers alone",
			in:   `<h1 class="brief-title">T</h1><div class="brief-meta">m</div><div class="brief-summary">s</div><div class="chip-row"><span class="chip chip-a">x</span></div>`,
			want: []string{
				`<h1 class="brief-title">T</h1>`,
				`<div class="brief-meta">m</div>`,
				`<div class="brief-summary">s</div>`,
				`<div class="chip-row">`,
				`<wk-badge variant="a">x</wk-badge>`,
			},
			gone: []string{`class="chip chip-a"`},
		},
		{
			name: "leaves cytoscape container alone",
			in:   `<wk-panel><div id="cy-graph" class="cy-container"></div></wk-panel>`,
			want: []string{`<div id="cy-graph" class="cy-container">`},
			gone: nil,
		},
		{
			name: "content with no legacy classes passes through",
			in:   `<p>just text</p>`,
			want: []string{`<p>just text</p>`},
			gone: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpgradeLegacyHTML(tt.in)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("want substring %q\n got: %s", w, got)
				}
			}
			for _, g := range tt.gone {
				if strings.Contains(got, g) {
					t.Errorf("did not want substring %q\n got: %s", g, got)
				}
			}
		})
	}
}

// TestUpgradeLegacyHTMLIdempotent runs the transform twice and asserts the
// second pass is a no-op over the first.
func TestUpgradeLegacyHTMLIdempotent(t *testing.T) {
	in := `<div class="section" id="s"><h2 class="section-heading"><span class="section-id">A1</span>H</h2>` +
		`<p class="section-text" data-bionic>body</p>` +
		`<div class="kv-row"><span class="kv-label">L</span><span class="kv-value">V</span></div>` +
		`<div class="callout callout-warn">w</div>` +
		`<span class="chip chip-b">b</span>` +
		`<table class="data-table"><tbody><tr><td>1</td></tr></tbody></table></div>`
	once := UpgradeLegacyHTML(in)
	twice := UpgradeLegacyHTML(once)
	if once != twice {
		t.Errorf("not idempotent:\n once:  %s\n twice: %s", once, twice)
	}
}
