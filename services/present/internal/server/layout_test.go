package server

import "testing"

// Stage 2 of MAD-365 is wired across the renderer's attributes, the view code
// that reads them, and the shell's CSS. Pin the names together so one cannot
// drift from the others.
func TestDeckLayoutWiring(t *testing.T) {
	app := string(appJS)
	for _, want := range []string{
		"data-layout", "data-tone", "--slide-accent", "data-reveal", "solo-stat", "solo-quote",
		"deck-notes", "toggleNotes", "case 'n': case 'N'", "fragment", "data-step",
		"chrome.transition", "data-transition", "startViewTransition", "deck-vt", "deck-next", "deck-prev",
		"'layoutstop'", "classList.add('ready')", "buildChart(b, { duration: transition === 'none' ? 0 : 400 })", "builtOnly",
		"var pending = null;", "function at() { return pending ? pending.i : current; }", "pending.steps = n",
		"vt.ready.catch(function () {});",
		// In-block steps (MAD-385): the plan, the step attribute, the hook.
		"PresentViz.stepPlan(items)", "data-steps", "data-step-at", "block._presentStep = function (n)",
		"PresentViz.chartStepVisibility", "PresentViz.blockStepsAt", "PresentViz.seriesMax", "setDatasetVisibility", "legend.onClick = function () {}",
		// Diagram blocks (MAD-382): a visual, a solo slide, built on the
		// slide's first showing and refitted after, never laid out twice.
		"'#cy-graph, .present-chart, wk-figure, .present-diagram'",
		"buildDiagram(b, { duration: transition === 'none' ? 0 : 400 })", "b._diagramFit()",
		"initPresentDiagrams(document, { builtOnly: true })", "PresentViz.diagramElk(spec, measure, form)",
	} {
		if !contains(app, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	shell := string(shellHTML)
	for _, want := range []string{
		`.slide[data-layout="center"]`, `.slide[data-layout="statement"]`, `.slide[data-layout="section"]`,
		".slide.solo-stat", ".slide.solo-quote", ".slide[data-tone]", ".brief:not(.deck) wk-section[data-tone]",
		".deck .fragment { visibility: hidden; }", ".deck-notes-drawer", "view-transition-name: deck-slide",
		"::view-transition-old(deck-slide)", "html.deck-vt .slide.active, html[data-transition=\"none\"] .slide.active { animation: none; }",
		"div.deck-strip { view-transition-name: deck-strip; }", ".deck-notes-drawer { view-transition-name: deck-notes; }",
		"::view-transition-old(deck-logo), ::view-transition-new(deck-logo),",
		"::view-transition-old(deck-strip), ::view-transition-old(deck-logo), ::view-transition-old(deck-bar), ::view-transition-old(deck-notes) { display: none; }",
		`html[data-transition="slide"].deck-next::view-transition-new(deck-slide)`,
		"prefers-reduced-motion", ".cy-container.ready",
		".deck .present-steps li { grid-area: 1 / 1; visibility: hidden; }", ".deck .present-steps li.current { visibility: visible;",
		`html[data-transition="none"] .deck .present-steps li.current { animation: none; }`,
		".present-diagram-svg { --diagram-fade: 350ms; display: block;",
		".deck .slide .present-diagram:is([data-steps], :has(.present-diagram-caption)) .present-diagram-canvas { max-height: calc(29 * var(--u)); }",
		":root { --surface: var(--bg); }", ".present-chart[data-frame=\"true\"] {",
		".deck .slide .present-diagram-canvas { max-height: calc(33 * var(--u));",
		".present-diagram-svg .dedge-label rect { fill: var(--surface); }",
		"@media (prefers-reduced-motion: reduce) {\n  .present-diagram-svg .dgroup, .present-diagram-svg .dnode, .present-diagram-svg .dedge { transition: none; }",
		// Frameless slides (MAD-389): a stat sits plain, a section divider
		// and a toned statement carry a centred accent bar, no band.
		".slide wk-stat { border: 0; background: none;",
		`.deck .slide[data-layout="section"] .slide-body { text-align: center; }`,
		`.deck .slide[data-layout="section"] wk-section-heading::after, .deck .slide[data-layout="statement"][data-tone] wk-section-heading::after { content: '';`,
	} {
		if !contains(shell, want) {
			t.Errorf("shell.html lacks %q", want)
		}
	}
	// A slide paints no filled box of its own: no rule may give a toned
	// slide or a section body a background again.
	for _, box := range []string{".slide[data-tone] {", "html.presenting .slide.active[data-tone]", `.slide[data-layout="section"] .slide-body { --surface`} {
		if contains(shell, box) {
			t.Errorf("shell.html paints a box on a slide again: %q", box)
		}
	}
	// The slide tones mix roles; a palette value written out would pin a
	// family (the graph backdrop's rgba scrim predates this and is not one).
	for _, literal := range []string{"#C4704B", "#E8956A", "#1A1916", "#FAF9F7"} {
		if contains(shell, literal) {
			t.Errorf("shell.html carries a palette literal %q; read a role", literal)
		}
	}
}

// The deck canvas (MAD-559): every size on a slide is written in one unit
// the slide body declares from the slide, a size container, and the fit
// step scales it. Pin the unit, the floor, the centring that keeps an
// overflowing slide's top, the 16:9 stage, and the solo terminal's height
// in the unit; the fixed presenting zoom and the viewport heights for
// visuals stay gone. The fit step itself is PresentViz.fitSteps, which
// test/viz.test.mjs covers.
func TestDeckSlideUnit(t *testing.T) {
	shell := string(shellHTML)
	for _, want := range []string{
		".deck .slide.active { container-type: size;",
		"--u0: min(1cqw, 1.7778cqh);",
		"--u: calc(var(--u0) * var(--fit, 1));",
		"--floor: calc(1.1 * var(--u0));",
		"@property --u0 { syntax: '<length>'; inherits: true; initial-value: 0px; }",
		"@property --u { syntax: '<length>'; inherits: true; initial-value: 0px; }",
		":root { --bump: calc(1rem - 16px); }",
		"display: flex; flex-direction: column; justify-content: safe center;",
		".deck .slide.active.slide-overflow { overflow: auto; }",
		"width: min(100%, calc((100vh - 13.5rem) * 16 / 9)); aspect-ratio: 16 / 9;",
		"max(var(--floor), calc(1.5 * var(--u) + var(--bump)))",
		".deck .slide:not(.slide-overflow) wk-stat-value { white-space: nowrap; }",
		// The claim's rule spans the body at every fit; only its text is capped.
		`.deck .slide:not([data-layout="statement"], [data-layout="section"]) wk-section-heading { padding-right: max(0px, calc(100% - 86 * var(--u0))); }`,
		".slide.solo-terminal .present-terminal-canvas { --terminal-fill: 1; height: calc(33 * var(--u) - 2.125rem - 2px); }",
		// Split (MAD-581): 2:3 columns with a 6u gap, stacked on a narrow
		// window, and the diagram's height caps in the unit; the lint's
		// splitWidthU and splitCapU (internal/lint/rules.go) read the same
		// numbers.
		`.deck .slide[data-layout="split"] .split { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr); column-gap: calc(6 * var(--u)); align-items: center; }`,
		`.deck .slide[data-layout="split"] .present-diagram-canvas { max-height: calc(42 * var(--u)); }`,
		`.deck .slide[data-layout="split"] .present-diagram:is([data-steps], :has(.present-diagram-caption)) .present-diagram-canvas { max-height: calc(36 * var(--u)); }`,
		`html:not(.presenting) .deck .slide[data-layout="split"] .split { grid-template-columns: minmax(0, 1fr);`,
	} {
		if !contains(shell, want) {
			t.Errorf("shell.html lacks %q", want)
		}
	}
	for _, gone := range []string{
		"zoom:", "--deck-zoom", "44vh", "48vh", "52vh", "55vh", "56vh", "62vh", "64vh", "66vh", "max-width: 26em",
	} {
		if contains(shell, gone) {
			t.Errorf("shell.html sizes a slide by %q again; slide sizes are in the unit", gone)
		}
	}
}
