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
		"'#cy-graph, .present-chart, wk-figure, .present-diagram'", "solo-diagram",
		"buildDiagram(b, { duration: transition === 'none' ? 0 : 400 })", "b._diagramFit()",
		"initPresentDiagrams(document, { builtOnly: true })", "PresentViz.diagramGraph(spec, measure)",
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
		".present-diagram-svg { display: block;", ".slide.solo-diagram .present-diagram-canvas",
		"html.presenting .slide.has-viz .present-diagram-canvas { max-height: 52vh; }",
		".present-diagram-svg .dedge-label rect { fill: var(--surface); }",
		"@media (prefers-reduced-motion: reduce) {\n  .present-diagram-svg .dgroup, .present-diagram-svg .dnode, .present-diagram-svg .dedge { transition: none; }",
	} {
		if !contains(shell, want) {
			t.Errorf("shell.html lacks %q", want)
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
