// Package limits is the one table of every number present holds a page to:
// the warnings the lint raises and the refusals the renderer makes. Each
// entry carries its number, its level, where it applies, how strong its
// source is, and the rule and its reason as one line each. The lint, the
// generated limits reference in the skill, and the numbers in the MCP
// schema text all read from here. The renderer keeps its refusal caps as
// constants, and a render test holds each to its entry here, so a number
// changes in the table and the constant together. The package imports
// nothing from present.
package limits

import (
	"fmt"
	"slices"
)

// ID names one limit. A warning's ID is the rule id its findings carry.
type ID string

// Level says what happens when a page passes a limit.
type Level string

const (
	// Warn is a lint warning: the page is stored and the result says why
	// it could read better.
	Warn Level = "warn"
	// Refuse is a renderer refusal: the page is not stored.
	Refuse Level = "refuse"
)

// Source is how strong the evidence behind a number is.
type Source string

const (
	// Study is a measured result.
	Study Source = "study"
	// Guideline is a published style guide.
	Guideline Source = "guideline"
	// House is present's own number.
	House Source = "house"
	// Renderer is a fact of how present draws a page.
	Renderer Source = "renderer"
)

// The warnings, in the order the lint reports them.
const (
	SummaryWords        ID = "summary.words"
	HeadingLabel        ID = "heading.label"
	BriefFigureGap      ID = "brief.figure_gap"
	TableRows           ID = "table.rows"
	TableCell           ID = "table.cell"
	DiagramBoxes        ID = "diagram.boxes"
	DiagramChain        ID = "diagram.chain"
	DiagramSlideBoxes   ID = "diagram.slide_boxes"
	DiagramSlideColumns ID = "diagram.slide_columns"
	SlideWords          ID = "slide.words"
	DeckTextOnly        ID = "deck.text_only"
	InlineStrayMark     ID = "inline.stray_mark"
	PlaceholderLeft     ID = "placeholder.left"
)

// The refusals the renderer makes with a number.
const (
	RibbonSeries     ID = "ribbon.series"
	DiagramNodes     ID = "diagram.nodes"
	DiagramGroups    ID = "diagram.groups"
	DiagramDepth     ID = "diagram.depth"
	DiagramLabel     ID = "diagram.label"
	DiagramText      ID = "diagram.text"
	DiagramEdgeLabel ID = "diagram.edge_label"
	TerminalRowsMin  ID = "terminal.rows_min"
	TerminalRowsMax  ID = "terminal.rows_max"
	TerminalSession  ID = "terminal.session"
	TerminalPrefill  ID = "terminal.prefill"
)

// Limit is one entry of the table.
type Limit struct {
	ID ID `json:"id"`
	// Number is the limit itself, counted in Unit; 0 when the rule has no
	// number.
	Number int    `json:"number,omitempty"`
	Unit   string `json:"unit,omitempty"`
	Level  Level  `json:"level"`
	// Scope is where the rule applies: the brief, the deck, or both, and
	// which part of them.
	Scope  string `json:"scope"`
	Source Source `json:"source"`
	// Basis names the evidence behind a study or guideline source.
	Basis  string `json:"basis,omitempty"`
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}

// table is every limit: the warnings first, in the order the lint reports
// them (summary and headings, then sections, tables, figures, slides, and
// the marks left in the text), then the refusals.
var table = []Limit{
	{
		ID: SummaryWords, Number: 50, Unit: "words", Level: Warn,
		Scope:  "the summary of a brief or a deck",
		Source: House,
		Rule:   "The summary holds at most 50 words.",
		Reason: "The summary sits above the first section; a longer one pushes the first heading and figure off the first screen.",
	},
	{
		ID: HeadingLabel, Number: 2, Unit: "words", Level: Warn,
		Scope:  "section headings in a brief, slide headings in a deck",
		Source: Guideline, Basis: "assertion-evidence studies, one research group",
		Rule:   "A heading is the claim its section proves, a sentence with a verb, not a label of 2 words or fewer or a word on the label list.",
		Reason: "A reader who skims only the headings should leave with the argument; a label names the topic and nothing else.",
	},
	{
		ID: BriefFigureGap, Number: 4, Unit: "sections", Level: Warn,
		Scope:  "the sections of a brief",
		Source: House,
		Rule:   "At most 4 sections in a row go without a figure (a chart, diagram, graph, image, or stat).",
		Reason: "A long run of text-only sections reads as a wall; a figure gives the reader who skims a place to land.",
	},
	{
		ID: TableRows, Number: 8, Unit: "rows", Level: Warn,
		Scope:  "every table, brief and deck",
		Source: House,
		Rule:   "A table holds at most 8 body rows.",
		Reason: "Past that a reader scans instead of comparing; keep the rows the point needs and move the full list into details.",
	},
	{
		ID: TableCell, Number: 160, Unit: "characters", Level: Warn,
		Scope:  "every table cell, brief and deck",
		Source: Guideline, Basis: "Google developer style: more than two sentences in a cell belong elsewhere",
		Rule:   "A table cell holds at most 160 characters.",
		Reason: "A cell is read across a row; a paragraph in it breaks the row, so the prose belongs beside the table.",
	},
	{
		ID: DiagramBoxes, Number: 9, Unit: "boxes", Level: Warn,
		Scope:  "a diagram in a brief",
		Source: House,
		Rule:   "A diagram in a brief holds at most 9 boxes.",
		Reason: "A bigger diagram draws taller than a screen at a size the reader can read; split it or show one part.",
	},
	{
		ID: DiagramChain, Number: 5, Unit: "boxes", Level: Warn,
		Scope:  "a diagram, brief and deck",
		Source: Renderer,
		Rule:   "The longest path through a diagram passes at most 5 boxes.",
		Reason: "Each box on the path is one layer of the layout, and every layer adds height in any direction.",
	},
	{
		ID: DiagramSlideBoxes, Number: 6, Unit: "boxes", Level: Warn,
		Scope:  "a diagram on a slide",
		Source: House,
		Rule:   "A diagram on a slide holds at most 6 boxes.",
		Reason: "A slide cannot scroll, so a fuller diagram draws its text under the size a room can read.",
	},
	{
		ID: DiagramSlideColumns, Level: Warn,
		Scope:  "a diagram on a slide",
		Source: Renderer,
		Rule:   "A diagram on a slide sits alone, not inside columns.",
		Reason: "In part of the slide's width the diagram's text draws under the size a room can read.",
	},
	{
		ID: SlideWords, Number: 50, Unit: "words", Level: Warn,
		Scope:  "a slide, outside its heading",
		Source: House,
		Rule:   "A slide holds at most 50 words outside its heading; of a stepped chart or diagram only the longest caption counts.",
		Reason: "A room reads a slide at a glance; the detail belongs in the brief or the speaker notes.",
	},
	{
		ID: DeckTextOnly, Number: 50, Unit: "percent", Level: Warn,
		Scope:  "the default-layout slides of a deck",
		Source: Study, Basis: "one study, 110 students",
		Rule:   "At most half (50 percent) of the default-layout slides hold only prose or a list.",
		Reason: "A claim shown beside a picture is remembered better than the same claim in text alone.",
	},
	{
		ID: InlineStrayMark, Level: Warn,
		Scope:  "every text field, brief and deck",
		Source: Renderer,
		Rule:   "No backtick or pair of stars sits in a field drawn as plain text, and a field that takes inline markdown holds an even number of backticks.",
		Reason: "The marks print as they are, so the reader sees raw markdown instead of code or bold.",
	},
	{
		ID: PlaceholderLeft, Level: Warn,
		Scope:  "every text field, brief and deck",
		Source: House,
		Rule:   "No text holds a skeleton placeholder between << and >>.",
		Reason: "A placeholder left in prints the note to the author on the page.",
	},

	{
		ID: RibbonSeries, Number: 4, Unit: "series", Level: Refuse,
		Scope:  "a ribbon chart",
		Source: Renderer,
		Rule:   "A ribbon chart takes at most 4 series.",
		Reason: "The palette has four series colours; a fifth repeats the first, and ranks that reorder every column make two bands of one colour unreadable.",
	},
	{
		ID: DiagramNodes, Number: 24, Unit: "boxes", Level: Refuse,
		Scope:  "a diagram",
		Source: Renderer,
		Rule:   "A diagram holds at most 24 boxes.",
		Reason: "Past it the picture is a hairball, and the layout takes long enough to notice.",
	},
	{
		ID: DiagramGroups, Number: 12, Unit: "groups", Level: Refuse,
		Scope:  "a diagram",
		Source: Renderer,
		Rule:   "A diagram holds at most 12 groups.",
		Reason: "Past it the boundaries crowd out the boxes they hold.",
	},
	{
		ID: DiagramDepth, Number: 2, Unit: "levels", Level: Refuse,
		Scope:  "the groups of a diagram",
		Source: Renderer,
		Rule:   "Groups nest at most 2 levels deep: a group inside a group inside the diagram.",
		Reason: "Each level adds a boundary and its label around the boxes.",
	},
	{
		ID: DiagramLabel, Number: 32, Unit: "characters", Level: Refuse,
		Scope:  "a diagram box or group label",
		Source: Renderer,
		Rule:   "A box or group label is at most 32 characters.",
		Reason: "A box label draws on one line in a box at most 240 px wide, and a group label sets the group's least width.",
	},
	{
		ID: DiagramText, Number: 60, Unit: "characters", Level: Refuse,
		Scope:  "a diagram box's text",
		Source: Renderer,
		Rule:   "A box's text is at most 60 characters.",
		Reason: "It wraps under the label, and the cap keeps a box to a few lines.",
	},
	{
		ID: DiagramEdgeLabel, Number: 24, Unit: "characters", Level: Refuse,
		Scope:  "a diagram edge label",
		Source: Renderer,
		Rule:   "An edge label is at most 24 characters.",
		Reason: "The widest label sets the gap between two layers.",
	},
	{
		ID: TerminalRowsMin, Number: 8, Unit: "rows", Level: Refuse,
		Scope:  "a terminal block that sets rows",
		Source: Renderer,
		Rule:   "A terminal block's rows is at least 8.",
		Reason: "With fewer a full-screen program has no room to draw.",
	},
	{
		ID: TerminalRowsMax, Number: 60, Unit: "rows", Level: Refuse,
		Scope:  "a terminal block that sets rows",
		Source: Renderer,
		Rule:   "A terminal block's rows is at most 60.",
		Reason: "With more the terminal outgrows a slide.",
	},
	{
		ID: TerminalSession, Number: 32, Unit: "characters", Level: Refuse,
		Scope:  "a terminal block's session name",
		Source: Renderer,
		Rule:   "A session name is 1 to 32 of a-z, 0-9, and -.",
		Reason: "It is a path segment of the attach route and the key serve holds the session under.",
	},
	{
		ID: TerminalPrefill, Number: 512, Unit: "characters", Level: Refuse,
		Scope:  "a terminal block's prefill",
		Source: Renderer,
		Rule:   "A prefill is one line of at most 512 characters.",
		Reason: "It is typed into the session as keystrokes, so a newline would run it.",
	},
}

// All returns every limit, the warnings first in the order the lint
// reports them, then the refusals.
func All() []Limit { return slices.Clone(table) }

// Warnings returns the warn entries in the order the lint reports them.
func Warnings() []Limit {
	var out []Limit
	for _, l := range table {
		if l.Level == Warn {
			out = append(out, l)
		}
	}
	return out
}

// Get returns the limit named id.
func Get(id ID) (Limit, bool) {
	k := slices.IndexFunc(table, func(l Limit) bool { return l.ID == id })
	if k < 0 {
		return Limit{}, false
	}
	return table[k], true
}

// Number returns the number of the limit named id. Every caller names one
// of the IDs declared above, so an unknown id is a broken invariant of this
// package, not an input error, and it panics; any test that runs the
// caller catches it.
func Number(id ID) int {
	l, ok := Get(id)
	if !ok {
		panic(fmt.Sprintf("limits: unknown id %q", id))
	}
	return l.Number
}
