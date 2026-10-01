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
		`html[data-transition="slide"].deck-next::view-transition-new(deck-slide)`,
		"prefers-reduced-motion", ".cy-container.ready",
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
