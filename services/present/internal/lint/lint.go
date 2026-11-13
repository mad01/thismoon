// Package lint checks a page's Docs against the warn entries of the limits
// table and says what would make the page read better. It is a pure
// function of the page: it stores nothing, reads no file, and never
// refuses anything. The renderer refuses what it cannot draw; the lint
// only warns. The MCP tools attach its findings to their results, and
// `present lint` prints them.
package lint

import (
	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/render"
)

// Rendition names which of a page's Docs a finding sits in. Its values are
// the MCP field names, so a finding's where reads as a path into the tool's
// arguments.
type Rendition string

const (
	// Content is the brief.
	Content Rendition = "content"
	// Deck is the slide deck.
	Deck Rendition = "deck"
)

// Page is what the lint reads: the title and both Docs. A rendition the
// page lacks, or one stored as legacy HTML, is the zero Doc and raises
// nothing.
type Page struct {
	Title string
	Brief render.Doc
	Deck  render.Doc
}

// Finding is one warning. Where is a path into the tool's arguments,
// zero-based like a section patch's index:N (content.sections[2].blocks[0]);
// Message names the place the way a reader finds it, then the fault, the
// reason, and the fix.
type Finding struct {
	Rule    limits.ID `json:"rule"`
	Where   string    `json:"where"`
	Message string    `json:"message"`

	// Rendition is where the finding sits; empty for the page title.
	Rendition Rendition `json:"-"`
	// First and Last are the zero-based sections the finding covers, both
	// -1 for a finding about the page, a summary, or a whole deck.
	First int `json:"-"`
	Last  int `json:"-"`
}

// Covers reports whether f is about section index of rendition r.
func (f Finding) Covers(r Rendition, index int) bool {
	return f.Rendition == r && f.First >= 0 && f.First <= index && index <= f.Last
}

// check finds one rule's faults in a page, in page order, given the rule's
// number from the limits table.
type check func(p Page, n int) []Finding

// checks holds a check for every warn entry of the limits table.
var checks = map[limits.ID]check{
	limits.SummaryWords:        checkSummaryWords,
	limits.HeadingLabel:        checkHeadingLabel,
	limits.BriefFigureGap:      checkFigureGap,
	limits.BriefParagraph:      checkParagraph,
	limits.TableRows:           checkTableRows,
	limits.TableCell:           checkTableCell,
	limits.DiagramBoxes:        checkDiagramBoxes,
	limits.DiagramChain:        checkDiagramChain,
	limits.DiagramRow:          checkDiagramRow,
	limits.DiagramSlideBoxes:   checkSlideBoxes,
	limits.DiagramSlidePath:    checkSlidePath,
	limits.DiagramSlideColumns: checkSlideColumns,
	limits.DiagramKeyUnused:    checkKeyUnused,
	limits.SlideWords:          checkSlideWords,
	limits.SplitWords:          checkSplitWords,
	limits.DeckTextOnly:        checkTextOnly,
	limits.InlineStrayMark:     checkStrayMarks,
	limits.PlaceholderLeft:     checkPlaceholders,
}

// Check returns every finding on p: by rule in the order of the limits
// table, then by position in the page (the title, the brief, the deck, and
// inside each from the top).
func Check(p Page) []Finding {
	var out []Finding
	for _, l := range limits.Warnings() {
		if c, ok := checks[l.ID]; ok {
			out = append(out, c(p, l.Number)...)
		}
	}
	return out
}

// Report is the lint's output for a script: every finding, and a count per
// rule with every warn rule present, so a row of counts has the same
// columns for every page.
type Report struct {
	Warnings []Finding         `json:"warnings"`
	Counts   map[limits.ID]int `json:"counts"`
}

// Summarize builds the report of findings.
func Summarize(findings []Finding) Report {
	r := Report{Warnings: findings, Counts: map[limits.ID]int{}}
	if r.Warnings == nil {
		r.Warnings = []Finding{}
	}
	for _, l := range limits.Warnings() {
		r.Counts[l.ID] = 0
	}
	for _, f := range findings {
		r.Counts[f.Rule]++
	}
	return r
}

// docs lists a page's Docs with their rendition, brief first.
func (p *Page) docs() []struct {
	r Rendition
	d *render.Doc
} {
	return []struct {
		r Rendition
		d *render.Doc
	}{{Content, &p.Brief}, {Deck, &p.Deck}}
}
