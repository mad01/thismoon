package mcpserver

import (
	"github.com/mad01/thismoon/services/present/internal/lint"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// warningsShown is how many findings a tool result lists. The rest are
// counted, so a page with many faults costs the caller a short list and a
// number rather than a wall of text.
const warningsShown = 8

// pageWarnings shapes a page's findings for a create or update result: the
// first warningsShown, and how many more there are.
func pageWarnings(findings []lint.Finding) (shown []lint.Finding, more int) {
	if len(findings) <= warningsShown {
		return findings, 0
	}
	return findings[:warningsShown], len(findings) - warningsShown
}

// sectionWarnings shapes a page's findings for a section patch result: the
// findings about the patched section first, at most warningsShown with the
// rest in more, and a count of the page's other findings in elsewhere. A
// removed section has no findings of its own, so everything is elsewhere.
func sectionWarnings(
	findings []lint.Finding,
	r lint.Rendition,
	index int,
	removed bool,
) (shown []lint.Finding, more, elsewhere int) {
	var mine []lint.Finding
	for _, f := range findings {
		if !removed && f.Covers(r, index) {
			mine = append(mine, f)
		} else {
			elsewhere++
		}
	}
	shown, more = pageWarnings(mine)
	return shown, more, elsewhere
}

// capture runs fn and keeps the snapshot it returned, so a tool can lint
// the page as it was written. A store may run an edit more than once; the
// last successful run is the one it writes.
func capture(fn store.EditFunc, into *store.Snapshot) store.EditFunc {
	return func(cur store.Snapshot) (store.Snapshot, error) {
		next, err := fn(cur)
		if err == nil {
			*into = next
		}
		return next, err
	}
}

// writtenFindings lints the page an edit wrote, read from the Doc sources it
// stored. The lint never fails a write that succeeded: a page whose sources
// do not parse gets no findings, and present_source and present rerender
// are where a broken source is reported.
func writtenFindings(title string, written store.Snapshot) []lint.Finding {
	page, err := lint.FromSources(title, written.Sources.Doc, written.Sources.Deck)
	if err != nil {
		return nil
	}
	return lint.Check(page)
}
