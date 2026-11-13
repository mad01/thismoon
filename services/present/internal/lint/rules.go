package lint

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/render"
)

// labelHeadings are the headings heading.label treats as labels whatever
// their length: the words that name a topic and claim nothing.
var labelHeadings = []string{
	"overview", "background", "summary", "introduction", "context", "details", "conclusion",
	"results", "findings", "analysis", "discussion", "problem", "solution", "approach",
	"architecture", "timeline", "status", "update", "notes", "other", "what changed",
	"how it works", "key points", "takeaways", "next steps",
}

// labelExemptFirstWords open a heading heading.label leaves alone: the
// closing sections every page type ends on.
var labelExemptFirstWords = []string{"next", "references"}

// labelExemptHeadings are the ADHD mode's three headings, which are labels
// on purpose.
var labelExemptHeadings = []string{"do now", "now", "later"}

// figureKinds are the blocks that count as a figure: a picture or a number
// the eye lands on.
var figureKinds = []string{"chart", "diagram", "graph", "image", "stat"}

// textOnlyKinds are the blocks a slide that holds only prose or a list is
// made of.
var textOnlyKinds = []string{"p", "h3", "callout", "list"}

var (
	// starPair is a pair of stars a markdown reader would turn into bold
	// or italics: **x** anywhere, or *x* with no letter or star outside
	// either star and no space inside them. A glob such as *.pem, *.key
	// does not match, since a space precedes its second star.
	starPair = regexp.MustCompile(
		`\*\*\S(?:.*?\S)?\*\*|(?:^|[^\pL\pN*])\*[^\s*](?:[^*]*?[^\s*])?\*(?:[^\pL\pN*]|$)`,
	)
	// placeholder is a skeleton's marker for text the author still owes.
	placeholder = regexp.MustCompile(`<<[^<>]+>>`)
)

// message builds a finding's text: where the reader finds it, the fault,
// the rule's reason from the limits table, and the fix.
func message(id limits.ID, locate, fault, fix string) string {
	l, _ := limits.Get(id)
	return fmt.Sprintf("%s: %s. %s %s", locate, fault, l.Reason, fix)
}

// atSection is a finding about one section.
func atSection(id limits.ID, sec section, where, msg string) Finding {
	return Finding{
		Rule: id, Where: where, Message: msg, Rendition: sec.rend, First: sec.index, Last: sec.index,
	}
}

// words counts the words of s: the runs between spaces that hold a letter
// or a digit, so an arrow or a dash between two words is not one.
func words(s string) int {
	n := 0
	for _, f := range strings.Fields(s) {
		if strings.IndexFunc(
			f,
			func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) },
		) >= 0 {
			n++
		}
	}
	return n
}

func checkSummaryWords(p Page, n int) []Finding {
	var out []Finding
	for _, rd := range p.docs() {
		if w := words(rd.d.Summary); w > n {
			out = append(out, Finding{
				Rule: limits.SummaryWords, Where: string(rd.r) + ".summary", Rendition: rd.r,
				First: -1, Last: -1,
				Message: message(limits.SummaryWords, docOwner(rd.r)+"summary",
					fmt.Sprintf("%d words", w),
					fmt.Sprintf("Aim for %d or fewer; the rest can open the first section.", n)),
			})
		}
	}
	return out
}

// headingKey is a heading as the label checks compare it: lower case,
// without the punctuation that may close it.
func headingKey(h string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(h)), ".:?! ")
}

// labelExempt reports whether heading.label leaves a heading alone: one
// that opens with an exempt word, equals the page title, is one of the
// ADHD mode's headings, or heads a section divider slide.
func labelExempt(key, title string, s render.Section, r Rendition) bool {
	first, _, _ := strings.Cut(key, " ")
	return slices.Contains(labelExemptFirstWords, strings.TrimRight(first, ",:;")) ||
		key == headingKey(title) ||
		slices.Contains(labelExemptHeadings, key) ||
		(r == Deck && s.Layout == "section")
}

func checkHeadingLabel(p Page, n int) []Finding {
	var out []Finding
	for _, rd := range p.docs() {
		for i, s := range rd.d.Sections {
			key := headingKey(s.Heading)
			if key == "" || labelExempt(key, p.Title, s, rd.r) {
				continue
			}
			var fault string
			switch w := words(s.Heading); {
			case slices.Contains(labelHeadings, key):
				fault = "the heading is a label from the label list"
			case w <= n:
				fault = fmt.Sprintf(
					"the heading is a label of %d %s",
					w,
					plural(w, "word", "words"),
				)
			default:
				continue
			}
			sec := sectionOf(rd.r, i, s.Heading)
			out = append(out, atSection(limits.HeadingLabel, sec, sec.where+".h",
				message(limits.HeadingLabel, sec.place, fault,
					"Write the claim the section proves as a sentence with a verb.")))
		}
	}
	return out
}

// holdsFigure reports whether blocks hold a figure at the top or in a
// column. A figure in details does not count: the disclosure starts closed.
func holdsFigure(blocks []render.Block) bool {
	for _, b := range blocks {
		if slices.Contains(figureKinds, b.T) {
			return true
		}
		for _, col := range b.Columns {
			if holdsFigure(col) {
				return true
			}
		}
	}
	return false
}

func checkFigureGap(p Page, n int) []Finding {
	var out []Finding
	sections := p.Brief.Sections
	start := 0
	flush := func(end int) {
		if run := end - start + 1; run > n {
			first := sectionOf(Content, start, sections[start].Heading)
			last := sectionOf(Content, end, sections[end].Heading)
			out = append(out, Finding{
				Rule: limits.BriefFigureGap, Rendition: Content, First: start, Last: end,
				Where: fmt.Sprintf("content.sections[%d..%d]", start, end),
				Message: message(
					limits.BriefFigureGap,
					fmt.Sprintf("%s to %s", first.place, last.place),
					fmt.Sprintf(
						"%d sections in a row hold no chart, diagram, graph, image, or stat",
						run,
					),
					"Give one of them a figure, or fold the run into fewer sections.",
				),
			})
		}
	}
	for i, s := range sections {
		if holdsFigure(s.Blocks) {
			flush(i - 1)
			start = i + 1
		}
	}
	flush(len(sections) - 1)
	return out
}

// checkParagraph warns on a p block in the brief over n words, wherever it
// sits. A callout, quote, list item, or table cell is not a paragraph, and
// slide text has slide.words.
func checkParagraph(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "p" || at.sec.rend != Content {
			return
		}
		w := words(at.b.Text)
		if w <= n {
			return
		}
		out = append(out, atSection(limits.BriefParagraph, at.sec, at.where,
			message(limits.BriefParagraph, at.sec.place+", a paragraph", fmt.Sprintf("%d words", w),
				fmt.Sprintf(
					"Aim for %d or fewer: split it, or make the longer thought a list, a table, or a figure.",
					n,
				))))
	})
	return out
}

func checkTableRows(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "table" || len(at.b.Rows) <= n {
			return
		}
		out = append(out, atSection(limits.TableRows, at.sec, at.where,
			message(
				limits.TableRows,
				at.sec.place+", a table",
				fmt.Sprintf(
					"%d body rows",
					len(at.b.Rows),
				),
				fmt.Sprintf("Aim for %d or fewer.", n),
			)))
	})
	return out
}

func checkTableCell(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "table" {
			return
		}
		over, longest, row, col := 0, 0, 0, 0
		for r, cells := range at.b.Rows {
			for c, cell := range cells {
				l := utf8.RuneCountInString(cell)
				if l <= n {
					continue
				}
				over++
				if l > longest {
					longest, row, col = l, r+1, c+1
				}
			}
		}
		if over == 0 {
			return
		}
		fault := fmt.Sprintf("%d %s past %d characters, the longest %d (row %d, column %d)",
			over, plural(over, "cell runs", "cells run"), n, longest, row, col)
		out = append(out, atSection(limits.TableCell, at.sec, at.where,
			message(limits.TableCell, at.sec.place+", a table", fault,
				"Cut each to a phrase and put the rest in a paragraph after the table.")))
	})
	return out
}

func checkDiagramBoxes(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || at.sec.rend != Content || len(at.b.Nodes) <= n {
			return
		}
		out = append(out, atSection(limits.DiagramBoxes, at.sec, at.where,
			message(limits.DiagramBoxes, at.sec.place+", a diagram",
				fmt.Sprintf("%d boxes", len(at.b.Nodes)), fmt.Sprintf("Aim for %d or fewer.", n))))
	})
	return out
}

func checkDiagramChain(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" {
			return
		}
		path := longestPath(at.b)
		if len(path) <= n {
			return
		}
		from, to := at.b.Nodes[path[0]].Label, at.b.Nodes[path[len(path)-1]].Label
		out = append(out, atSection(limits.DiagramChain, at.sec, at.where,
			message(
				limits.DiagramChain,
				at.sec.place+", a diagram",
				fmt.Sprintf("a path of %d boxes, %q to %q", len(path), from, to),
				fmt.Sprintf(
					"Aim for %d or fewer: fold steps together or split the flow over two diagrams.",
					n,
				),
			)))
	})
	return out
}

func checkDiagramRow(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || at.sec.rend != Content || at.b.Direction == "TB" {
			return
		}
		path := longestPath(at.b)
		w := rowWidth(at.b, path, compactForm)
		if len(path) < 2 || w <= n {
			return
		}
		from, to := at.b.Nodes[path[0]].Label, at.b.Nodes[path[len(path)-1]].Label
		out = append(out, atSection(limits.DiagramRow, at.sec, at.where,
			message(
				limits.DiagramRow,
				at.sec.place+", a left-to-right diagram",
				fmt.Sprintf(
					"its path of %d boxes, %q to %q, is about %d px wide in a row, past the brief's %d",
					len(path),
					from,
					to,
					w,
					n,
				),
				"To keep the row, cut the box text and edge labels on that path to a few words or fold "+
					"boxes together; else set direction to TB and write the boxes for a column.",
			)))
	})
	return out
}

// longestPath returns the node indices on the longest path along a
// diagram's edges, first node first. Cycles are broken the way a layered
// layout breaks them: an edge a depth-first search in node order finds
// pointing back into its own stack is dropped. Of equal paths the one
// starting at the earlier node wins.
func longestPath(b *render.Block) []int {
	n := len(b.Nodes)
	index := make(map[string]int, n)
	for k, node := range b.Nodes {
		index[node.ID] = k
	}
	out := make([][]int, n)
	for _, e := range b.Edges {
		from, okFrom := index[e.From]
		to, okTo := index[e.To]
		if okFrom && okTo && from != to {
			out[from] = append(out[from], to)
		}
	}
	dag := acyclic(out)
	length, next := make([]int, n), make([]int, n)
	var walk func(v int) int
	walk = func(v int) int {
		if length[v] > 0 {
			return length[v]
		}
		length[v], next[v] = 1, -1
		for _, w := range dag[v] {
			if l := 1 + walk(w); l > length[v] {
				length[v], next[v] = l, w
			}
		}
		return length[v]
	}
	start := -1
	for v := range n {
		if l := walk(v); start < 0 || l > length[start] {
			start = v
		}
	}
	var path []int
	for v := start; v >= 0; v = next[v] {
		path = append(path, v)
	}
	return path
}

// acyclic returns the edges of out without the back edges a depth-first
// search in node order finds.
func acyclic(out [][]int) [][]int {
	const (
		unseen = iota
		onStack
		finished
	)
	state := make([]int, len(out))
	dag := make([][]int, len(out))
	var visit func(v int)
	visit = func(v int) {
		state[v] = onStack
		for _, w := range out[v] {
			switch state[w] {
			case onStack:
				continue
			case unseen:
				visit(w)
			}
			dag[v] = append(dag[v], w)
		}
		state[v] = finished
	}
	for v := range out {
		if state[v] == unseen {
			visit(v)
		}
	}
	return dag
}

func checkSlideBoxes(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || at.sec.rend != Deck || len(at.b.Nodes) <= n {
			return
		}
		out = append(out, atSection(limits.DiagramSlideBoxes, at.sec, at.where,
			message(limits.DiagramSlideBoxes, at.sec.place+", a diagram",
				fmt.Sprintf("%d boxes", len(at.b.Nodes)),
				fmt.Sprintf("Aim for %d or fewer, and leave the full picture to the brief.", n))))
	})
	return out
}

// The room a slide gives a diagram, in the slide's unit (shell.html): the
// body is the 100u canvas less 7u of padding either side, and the
// diagram's height cap is 33u, 29u under steps or a caption. The
// diagram.slide_path number is the body's width in px at scale 1 when the
// diagram's text draws at the slide's smallest size (TestSlidePathNumber),
// so a column holds that number times its cap over the width.
const (
	slideWidthU   = 86
	slideCapU     = 33
	slideCapStepU = 29
)

// The room a split slide's right column gives a diagram (shell.html): the
// body's width less the 6u gap, split 2:3 between the text and the
// visual, and a height cap of 42u, 36u under steps or a caption.
const (
	splitGapU     = 6
	splitWidthU   = (slideWidthU - splitGapU) * 3 / 5
	splitCapU     = 42
	splitCapStepU = 36
)

// slideRoom is the width and the height cap, in the slide's unit, that a
// slide gives a diagram: the body, or on a split slide the right column.
func slideRoom(split, stepped bool) (widthU, capU int) {
	switch {
	case split && stepped:
		return splitWidthU, splitCapStepU
	case split:
		return splitWidthU, splitCapU
	case stepped:
		return slideWidthU, slideCapStepU
	}
	return slideWidthU, slideCapU
}

// checkSlidePath warns when no form a slide tries draws a diagram's
// longest path at the slide's smallest text: the row is wider than n px
// and the column taller than its share of n. Both are floors, so a path
// it passes can still draw small.
func checkSlidePath(p Page, n int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || at.sec.rend != Deck {
			return
		}
		split := p.Deck.Sections[at.sec.index].Layout == splitLayout
		widthU, capU := slideRoom(split, len(at.b.Steps) > 0 || at.b.Caption != "")
		wide := int(math.Round(float64(n*widthU) / slideWidthU))
		tall := int(math.Round(float64(n*capU) / slideWidthU))
		row, column := slidePathSize(at.b)
		if len(row.nodes) < 2 || row.px <= wide || column.px <= tall {
			return
		}
		room := "the slide holds"
		if split {
			room = "a split slide's right column holds"
		}
		from, to := at.b.Nodes[row.nodes[0]].Label, at.b.Nodes[row.nodes[len(row.nodes)-1]].Label
		through := ""
		if row.groups > 0 {
			through = fmt.Sprintf(
				" through %d %s",
				row.groups,
				plural(row.groups, "group", "groups"),
			)
		}
		out = append(out, atSection(limits.DiagramSlidePath, at.sec, at.where,
			message(
				limits.DiagramSlidePath,
				at.sec.place+", a diagram",
				fmt.Sprintf(
					"its path of %d boxes, %q to %q%s, is about %d px wide in a row and %d px tall in a column, "+
						"past the %d by %d px %s at its smallest text",
					len(row.nodes),
					from,
					to,
					through,
					row.px,
					column.px,
					wide,
					tall,
					room,
				),
				"Drop or shorten the edge labels on that path, fold boxes together, or drop a group; "+
					"else show part of the flow on this slide and leave the whole to the brief.",
			)))
	})
	return out
}

// slidePathSize is the smallest a slide can draw a diagram's flow in: the
// narrowest row and the shortest column over the forms a slide tries.
func slidePathSize(b *render.Block) (row, column stretch) {
	for k, d := range []density{fullForm, compactForm, namesForm} {
		r, c := flowStretch(b, d, false), flowStretch(b, d, true)
		if k == 0 || r.px < row.px {
			row = r
		}
		if k == 0 || c.px < column.px {
			column = c
		}
	}
	return row, column
}

func checkSlideColumns(p Page, _ int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || at.sec.rend != Deck || !at.inColumns {
			return
		}
		out = append(out, atSection(
			limits.DiagramSlideColumns,
			at.sec,
			at.where,
			message(
				limits.DiagramSlideColumns,
				at.sec.place+", a diagram",
				"it sits inside columns",
				"Give it the slide to itself and move the text beside it to the speaker notes.",
			),
		))
	})
	return out
}

// checkKeyUnused warns once per diagram whose key names a tone none of its
// boxes or groups carries, listing those tones in key order.
func checkKeyUnused(p Page, _ int) []Finding {
	var out []Finding
	p.blocks(func(at block) {
		if at.b.T != "diagram" || len(at.b.Key) == 0 {
			return
		}
		used := map[string]bool{}
		for _, n := range at.b.Nodes {
			used[n.Tone] = true
		}
		for _, g := range at.b.Groups {
			used[g.Tone] = true
		}
		var unused []string
		for _, item := range at.b.Key {
			if !used[item.Tone] {
				unused = append(unused, item.Tone)
			}
		}
		if len(unused) == 0 {
			return
		}
		out = append(out, atSection(limits.DiagramKeyUnused, at.sec, at.where+".key",
			message(
				limits.DiagramKeyUnused,
				at.sec.place+", a diagram's key",
				fmt.Sprintf(
					"it names %s, which no box or group carries",
					strings.Join(unused, " and "),
				),
				"Tone the boxes the key means, or drop the items no box carries.",
			)))
	})
	return out
}

func checkSlideWords(p Page, n int) []Finding {
	total := make([]int, len(p.Deck.Sections))
	captions := map[string]int{} // a stepped block's longest caption
	owner := map[string]int{}    // the slide each stepped block sits on
	p.texts(func(t text) {
		if t.rend != Deck || t.section < 0 {
			return
		}
		switch t.role {
		case roleProse:
			total[t.section] += words(*t.s)
		case roleCaption:
			captions[t.block] = max(captions[t.block], words(*t.s))
			owner[t.block] = t.section
		}
	})
	for block, w := range captions {
		total[owner[block]] += w
	}
	var out []Finding
	for i, w := range total {
		if w <= n {
			continue
		}
		sec := sectionOf(Deck, i, p.Deck.Sections[i].Heading)
		out = append(out, atSection(limits.SlideWords, sec, sec.where,
			message(limits.SlideWords, sec.place, fmt.Sprintf("%d words outside the heading", w),
				fmt.Sprintf("Aim for %d or fewer.", n))))
	}
	return out
}

// checkSplitWords counts the words a split slide shows beside its visual:
// every text field outside the heading and outside the visual, whose title
// and caption sit in the right column.
func checkSplitWords(p Page, n int) []Finding {
	total := make([]int, len(p.Deck.Sections))
	visual := map[string]bool{} // the path of each split slide's visual
	for i, s := range p.Deck.Sections {
		for j, b := range s.Blocks {
			if s.Layout == splitLayout && render.IsSplitVisual(b) {
				visual[fmt.Sprintf("%s.blocks[%d]", sectionOf(Deck, i, s.Heading).where, j)] = true
			}
		}
	}
	p.texts(func(t text) {
		if t.rend != Deck || t.section < 0 || t.role != roleProse || visual[t.block] {
			return
		}
		total[t.section] += words(*t.s)
	})
	var out []Finding
	for i, s := range p.Deck.Sections {
		if s.Layout != splitLayout || total[i] <= n {
			continue
		}
		sec := sectionOf(Deck, i, s.Heading)
		out = append(out, atSection(
			limits.SplitWords,
			sec,
			sec.where,
			message(
				limits.SplitWords,
				sec.place,
				fmt.Sprintf("%d words beside the visual", total[i]),
				fmt.Sprintf("Aim for %d or fewer, and move the rest to the speaker notes.", n),
			),
		))
	}
	return out
}

// splitLayout is render's split layout: the claim and its text in a left
// column, one visual in a right one.
const splitLayout = "split"

// textOnly reports whether blocks hold only prose or a list, in columns
// too. A slide with no blocks holds no picture either.
func textOnly(blocks []render.Block) bool {
	for _, b := range blocks {
		switch {
		case b.T == "columns":
			for _, col := range b.Columns {
				if !textOnly(col) {
					return false
				}
			}
		case !slices.Contains(textOnlyKinds, b.T):
			return false
		}
	}
	return true
}

func checkTextOnly(p Page, n int) []Finding {
	slides, text := 0, 0
	for _, s := range p.Deck.Sections {
		if s.Layout != "" && s.Layout != "default" && s.Layout != splitLayout {
			continue
		}
		slides++
		if textOnly(s.Blocks) {
			text++
		}
	}
	if slides == 0 || text*100 <= n*slides {
		return nil
	}
	return []Finding{{
		Rule: limits.DeckTextOnly, Where: "deck", Rendition: Deck, First: -1, Last: -1,
		Message: message(
			limits.DeckTextOnly,
			"the deck",
			fmt.Sprintf(
				"%d of %d default-layout and split slides hold only prose or a list",
				text,
				slides,
			),
			"Give the strongest of them a chart, a diagram, or one big number.",
		),
	}}
}

func checkStrayMarks(p Page, _ int) []Finding {
	var out []Finding
	p.texts(func(t text) {
		var fault, fix string
		switch ticks := strings.Count(*t.s, "`"); {
		case t.inline && ticks%2 == 1:
			fault = fmt.Sprintf("%d %s, so one opens a code span nothing closes", ticks,
				plural(ticks, "backtick", "backticks"))
			fix = "Close the code span or drop the stray backtick."
		case !t.inline && ticks > 0:
			fault = "a backtick in text shown as written"
			fix = "Drop the marks: this field does not read markdown."
		case !t.inline && starPair.MatchString(*t.s):
			fault = "a pair of stars in text shown as written"
			fix = "Drop the marks: this field does not read markdown."
		default:
			return
		}
		out = append(out, textFinding(limits.InlineStrayMark, t, message(limits.InlineStrayMark,
			t.locate, fault, fix)))
	})
	return out
}

func checkPlaceholders(p Page, _ int) []Finding {
	var out []Finding
	p.texts(func(t text) {
		found := placeholder.FindAllString(*t.s, -1)
		if len(found) == 0 {
			return
		}
		fault := fmt.Sprintf("the placeholder %q is still there", found[0])
		if len(found) > 1 {
			fault += fmt.Sprintf(", with %d more", len(found)-1)
		}
		out = append(out, textFinding(limits.PlaceholderLeft, t, message(limits.PlaceholderLeft,
			t.locate, fault, "Write the text it asks for.")))
	})
	return out
}

// textFinding is a finding about one text field.
func textFinding(id limits.ID, t text, msg string) Finding {
	return Finding{
		Rule: id, Where: t.where, Message: msg, Rendition: t.rend, First: t.section, Last: t.section,
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
