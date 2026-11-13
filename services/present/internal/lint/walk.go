package lint

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// role says what a text field is to the rules that count words.
type role int

const (
	roleTitle   role = iota // the page title
	roleChrome              // a Doc's summary, meta line, chips, presenter, footer
	roleHeading             // a section's heading
	roleNotes               // a section's speaker notes, never on the slide
	roleProse               // text a reader reads in a block
	roleFigure              // a label drawn inside a chart or diagram, an image's alt text
	roleCaption             // one step caption of a stepped chart or diagram
)

// text is one text field of a page, with the pointer the field lives at.
type text struct {
	where   string // path into the tool's arguments
	locate  string // how a reader finds it: `section "X", list item 3`
	inline  bool   // the renderer reads inline markdown in it
	role    role
	rend    Rendition // empty for the title
	section int       // -1 outside the sections
	block   string    // where of the block the field belongs to; empty outside blocks
	s       *string
}

// docOwner opens the name of a field outside the sections: "the summary"
// in a brief, "the deck's summary" in a deck.
func docOwner(r Rendition) string {
	if r == Deck {
		return "the deck's "
	}
	return "the "
}

// section is where a section sits and how a message names it.
type section struct {
	rend  Rendition
	index int
	where string
	place string
}

// sectionOf names section i of a Doc. A brief names it by heading; a deck
// by slide number and heading, counting the title slide as slide 1, the
// way the deck view's #<n> and present_deck goto count. A section without
// a heading is named by the index a section patch takes.
func sectionOf(r Rendition, i int, heading string) section {
	s := section{rend: r, index: i, where: fmt.Sprintf("%s.sections[%d]", r, i)}
	h := strings.TrimSpace(heading)
	switch {
	case r == Deck && h != "":
		s.place = fmt.Sprintf("slide %d %q", i+2, h)
	case r == Deck:
		s.place = fmt.Sprintf("slide %d", i+2)
	case h != "":
		s.place = fmt.Sprintf("section %q", h)
	default:
		s.place = fmt.Sprintf("section index:%d", i)
	}
	return s
}

// texts calls fn on every text field the renderer draws or reads aloud, in
// page order: the title, then the brief, then the deck. Code, html, and
// terminal fields are verbatim and not visited, nor are fields that name a
// vocabulary entry or an id (a kind, a tone, a node id).
func (p *Page) texts(fn func(text)) {
	fn(text{
		where: "title", locate: "the title", role: roleTitle, section: -1, s: &p.Title,
	})
	for _, rd := range p.docs() {
		docTexts(rd.r, rd.d, fn)
	}
}

// docTexts visits one Doc's text fields. The deck chrome is read by the
// deck view only, so a brief's presenter and footer are never drawn and not
// visited.
func docTexts(r Rendition, d *render.Doc, fn func(text)) {
	owner := docOwner(r)
	top := func(key, what string, inline bool, s *string) {
		fn(text{
			where: string(r) + "." + key, locate: owner + what, inline: inline,
			role: roleChrome, rend: r, section: -1, s: s,
		})
	}
	top("summary", "summary", true, &d.Summary)
	top("meta", "meta line", false, &d.Meta)
	for i := range d.Chips {
		top(fmt.Sprintf("chips[%d].text", i), fmt.Sprintf("chip %d", i+1), false, &d.Chips[i].Text)
	}
	if r == Deck {
		top("presenter", "presenter", false, &d.Presenter)
		top("footer", "footer", false, &d.Footer)
	}
	for i := range d.Sections {
		s := &d.Sections[i]
		sec := sectionOf(r, i, s.Heading)
		emit := func(where, what string, inline bool, ro role, block string, str *string) {
			fn(text{
				where: where, locate: sec.place + ", " + what, inline: inline,
				role: ro, rend: r, section: i, block: block, s: str,
			})
		}
		emit(sec.where+".h", "heading", false, roleHeading, "", &s.Heading)
		emit(sec.where+".notes", "speaker notes", true, roleNotes, "", &s.Notes)
		for j := range s.Blocks {
			blockTexts(emit, fmt.Sprintf("%s.blocks[%d]", sec.where, j), &s.Blocks[j])
		}
	}
}

// emitFunc reports one text field of a block: its path, its name in a
// message, whether it takes inline markdown, its role, the block's path,
// and where the field lives.
type emitFunc func(where, what string, inline bool, ro role, block string, s *string)

// blockTexts visits the text fields block b draws, following a container
// into the blocks it holds. Which fields take inline markdown follows the
// renderer's block templates; TestInlineFieldsMatchTheRenderer holds the
// two together.
func blockTexts(emit emitFunc, path string, b *render.Block) {
	f := func(key, what string, inline bool, ro role, s *string) {
		emit(path+"."+key, what, inline, ro, path, s)
	}
	switch b.T {
	case "p":
		f("text", "paragraph", true, roleProse, &b.Text)
	case "h3":
		f("text", "subheading", false, roleProse, &b.Text)
	case "callout":
		f("text", "callout", true, roleProse, &b.Text)
	case "quote":
		f("text", "quote", true, roleProse, &b.Text)
		f("cite", "quote's cite", true, roleProse, &b.Cite)
	case "table":
		tableTexts(f, b)
	case "kv":
		for k := range b.KV {
			f(
				fmt.Sprintf("kv[%d].k", k),
				fmt.Sprintf("label of pair %d", k+1),
				true,
				roleProse,
				&b.KV[k].K,
			)
			f(
				fmt.Sprintf("kv[%d].v", k),
				fmt.Sprintf("value of pair %d", k+1),
				true,
				roleProse,
				&b.KV[k].V,
			)
		}
	case "list":
		for k := range b.Items {
			f(
				fmt.Sprintf("items[%d]", k),
				fmt.Sprintf("list item %d", k+1),
				true,
				roleProse,
				&b.Items[k],
			)
		}
	case "panel":
		f("title", "panel title", false, roleProse, &b.Title)
		f("sub", "panel text", true, roleProse, &b.Subtitle)
	case "progress":
		f("label", "progress label", false, roleProse, &b.Label)
		f("note", "progress note", false, roleProse, &b.Note)
	case "stat":
		f("value", "stat value", false, roleProse, &b.Value)
		f("label", "stat label", true, roleProse, &b.Label)
		f("sub", "stat line", true, roleProse, &b.Subtitle)
	case "image":
		f("alt", "image alt text", false, roleFigure, &b.Alt)
		f("caption", "image caption", true, roleProse, &b.Caption)
	case "chart":
		chartTexts(f, b)
	case "diagram":
		diagramTexts(f, b)
	case "details":
		f("summary", "details summary", true, roleProse, &b.Summary)
		for k := range b.Blocks {
			blockTexts(emit, fmt.Sprintf("%s.blocks[%d]", path, k), &b.Blocks[k])
		}
	case "columns":
		for c := range b.Columns {
			for k := range b.Columns[c] {
				blockTexts(emit, fmt.Sprintf("%s.cols[%d][%d]", path, c, k), &b.Columns[c][k])
			}
		}
	}
}

type fieldFunc func(key, what string, inline bool, ro role, s *string)

func tableTexts(f fieldFunc, b *render.Block) {
	for c := range b.Cols {
		f(
			fmt.Sprintf("cols[%d]", c),
			fmt.Sprintf("table header %d", c+1),
			false,
			roleProse,
			&b.Cols[c],
		)
	}
	for r := range b.Rows {
		for c := range b.Rows[r] {
			f(
				fmt.Sprintf("rows[%d][%d]", r, c),
				fmt.Sprintf(
					"table cell (row %d, column %d)",
					r+1,
					c+1,
				),
				true,
				roleProse,
				&b.Rows[r][c],
			)
		}
	}
}

func chartTexts(f fieldFunc, b *render.Block) {
	f("title", "chart title", false, roleProse, &b.Title)
	f("unit", "chart unit", false, roleFigure, &b.Unit)
	f("xunit", "chart x unit", false, roleFigure, &b.XUnit)
	for s := range b.Series {
		f(fmt.Sprintf("series[%d].name", s), fmt.Sprintf("series %d name", s+1), false, roleFigure,
			&b.Series[s].Name)
		for k := range b.Series[s].Points {
			f(
				fmt.Sprintf("series[%d].points[%d].x", s, k),
				fmt.Sprintf("series %d label %d", s+1, k+1),
				false,
				roleFigure,
				&b.Series[s].Points[k].X,
			)
		}
	}
	for k := range b.Flows {
		f(fmt.Sprintf("flows[%d].from", k), fmt.Sprintf("flow %d source", k+1), false, roleFigure,
			&b.Flows[k].From)
		f(fmt.Sprintf("flows[%d].to", k), fmt.Sprintf("flow %d target", k+1), false, roleFigure,
			&b.Flows[k].To)
	}
	stepTexts(f, b)
}

func diagramTexts(f fieldFunc, b *render.Block) {
	f("caption", "diagram caption", true, roleProse, &b.Caption)
	for k := range b.Nodes {
		n := &b.Nodes[k]
		f(
			fmt.Sprintf("nodes[%d].label", k),
			fmt.Sprintf("box %q label", n.ID),
			false,
			roleFigure,
			&n.Label,
		)
		f(
			fmt.Sprintf("nodes[%d].text", k),
			fmt.Sprintf("box %q text", n.ID),
			false,
			roleFigure,
			&n.Text,
		)
	}
	for k := range b.Groups {
		g := &b.Groups[k]
		f(
			fmt.Sprintf("groups[%d].label", k),
			fmt.Sprintf("group %q label", g.ID),
			false,
			roleFigure,
			&g.Label,
		)
	}
	for k := range b.Edges {
		e := &b.Edges[k]
		f(
			fmt.Sprintf("edges[%d].label", k),
			fmt.Sprintf("edge %s to %s label", e.From, e.To),
			false,
			roleFigure,
			&e.Label,
		)
	}
	for k := range b.Key {
		item := &b.Key[k]
		f(
			fmt.Sprintf("key[%d].label", k),
			fmt.Sprintf("key item %d (%s) label", k+1, item.Tone),
			false,
			roleFigure,
			&item.Label,
		)
	}
	stepTexts(f, b)
}

func stepTexts(f fieldFunc, b *render.Block) {
	for k := range b.Steps {
		f(
			fmt.Sprintf("steps[%d].caption", k),
			fmt.Sprintf("step %d caption", k+1),
			true,
			roleCaption,
			&b.Steps[k].Caption,
		)
	}
}

// block is one block of a page with where it sits.
type block struct {
	b         *render.Block
	where     string
	sec       section
	inColumns bool
	inDetails bool
}

// blocks calls fn on every block of the page in page order, the brief
// first, a container before the blocks it holds.
func (p *Page) blocks(fn func(block)) {
	for _, rd := range p.docs() {
		for i := range rd.d.Sections {
			s := &rd.d.Sections[i]
			sec := sectionOf(rd.r, i, s.Heading)
			for j := range s.Blocks {
				walkBlock(fn, block{
					b: &s.Blocks[j], where: fmt.Sprintf("%s.blocks[%d]", sec.where, j), sec: sec,
				})
			}
		}
	}
}

func walkBlock(fn func(block), at block) {
	fn(at)
	b := at.b
	for c := range b.Columns {
		for k := range b.Columns[c] {
			inner := at
			inner.b, inner.inColumns = &b.Columns[c][k], true
			inner.where = fmt.Sprintf("%s.cols[%d][%d]", at.where, c, k)
			walkBlock(fn, inner)
		}
	}
	for k := range b.Blocks {
		inner := at
		inner.b, inner.inDetails = &b.Blocks[k], true
		inner.where = fmt.Sprintf("%s.blocks[%d]", at.where, k)
		walkBlock(fn, inner)
	}
}
