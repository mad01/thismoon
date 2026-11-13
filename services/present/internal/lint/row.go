package lint

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// A diagram as viz.js lays it out (DIAGRAM_BOX, DIAGRAM_DENSITY,
// DIAGRAM_HALO, DIAGRAM_SPACING, DIAGRAM_KIND_ROOM, and ELK's own padding),
// in px at scale 1. rowWidth and columnHeight read them to say how large a
// path draws before the browser draws it.
const (
	nameCharPx   = 7.8 // a box name's character: Fira Code, the default face, at 600 13px
	textCharPx   = 6.9 // a text line's or an edge label's character, at 11.5px
	boxMinPx     = 120 // the narrowest box
	boxPadPx     = 14  // a box's padding either side of its text
	boxPadYPx    = 10  // a box's padding above and below its text
	nameLinePx   = 18  // one line of a box's name
	textLinePx   = 15  // one line of a box's text
	personRoomPx = 14  // the strip a person's figure adds to its box
	storeRoomPx  = 6   // the lid a store's shape adds above its box
	layerGapPx   = 32  // between two layers, and either side of a label's own layer
	labelSplitPx = 84  // an edge label wider than this takes two lines where the form allows
	haloPadPx    = 8   // an edge label's halo across its widest line
	haloLinePx   = 14  // one line of an edge label's halo
	haloPadYPx   = 2   // an edge label's halo above and below its lines
	graphPadPx   = 12  // ELK's padding around the whole graph, either side
)

// density is how much a form of a diagram shows, as viz.js's
// DIAGRAM_DENSITY gives it: its widest box, how many lines an edge label
// may take, and whether the boxes keep their text.
type density struct {
	boxPx     float64
	edgeLines int
	text      bool
}

var (
	fullForm    = density{boxPx: 240, edgeLines: 1, text: true}
	compactForm = density{boxPx: 190, edgeLines: 2, text: true}
	// namesForm leaves the box text out; only a slide draws it.
	namesForm = density{boxPx: 240, edgeLines: 2, text: false}
)

// rowWidth estimates how wide the boxes on path (node indexes, in order)
// draw laid out in a row in form d, in px: each box as wide as its widest
// wrapped line plus padding, a layer gap between two boxes, and a
// labelled edge's label in a layer of its own with a gap either side.
// Text is measured as the default face draws it, so another face the
// reader picks draws somewhat wider or narrower. It is a floor: a group's
// padding, a wider box beside the path in the same layer, a branch that
// takes a layer of its own, and a return edge's label widen the layout
// past it.
func rowWidth(b *render.Block, path []int, d density) int {
	w := 2 * graphPadPx
	for k, v := range path {
		w += boxWidth(b.Nodes[v], d)
		if k == 0 {
			continue
		}
		w += layerGapPx
		if label := edgeLabel(b, b.Nodes[path[k-1]].ID, b.Nodes[v].ID); label != "" {
			w += edgeLabelWidth(label, d) + layerGapPx
		}
	}
	return w
}

// columnHeight is rowWidth top to bottom: how tall the boxes on path draw
// laid out in a column in form d, in px, each box one row per wrapped line
// and each labelled edge's label in a layer of its own. It is a floor the
// same way.
func columnHeight(b *render.Block, path []int, d density) int {
	h := 2 * graphPadPx
	for k, v := range path {
		h += boxHeight(b.Nodes[v], d)
		if k == 0 {
			continue
		}
		h += layerGapPx
		if label := edgeLabel(b, b.Nodes[path[k-1]].ID, b.Nodes[v].ID); label != "" {
			h += edgeLabelHeight(label, d) + layerGapPx
		}
	}
	return h
}

// boxWidth is a node's box in form d: its name, and its text where the
// form keeps it, wrapped inside the widest box, the widest line plus
// padding, kept between the narrowest and widest box, and a person's
// figure beside it.
func boxWidth(n render.DiagramNode, d density) int {
	inner := d.boxPx - 2*boxPadPx
	widest, _ := wrapped(n.Label, inner, nameCharPx)
	if d.text {
		text, _ := wrapped(n.Text, inner, textCharPx)
		widest = max(widest, text)
	}
	w := min(int(d.boxPx), max(boxMinPx, int(math.Ceil(widest))+2*boxPadPx))
	if n.Kind == "person" {
		w += personRoomPx
	}
	return w
}

// boxHeight is a node's box in form d: a row per line of its wrapped name
// (at least one) and of its text where the form keeps it, the padding
// above and below, and a store's lid.
func boxHeight(n render.DiagramNode, d density) int {
	inner := d.boxPx - 2*boxPadPx
	_, names := wrapped(n.Label, inner, nameCharPx)
	h := 2*boxPadYPx + max(1, names)*nameLinePx
	if d.text {
		_, lines := wrapped(n.Text, inner, textCharPx)
		h += lines * textLinePx
	}
	if n.Kind == "store" {
		h += storeRoomPx
	}
	return h
}

// wrapped is s wrapped at maxPx, at charPx a character: words fill a line
// while they fit, and a word wider than a line fills whole lines and
// leaves its last piece to start the next. It returns the widest line and
// the line count, both floors: the browser may break a long word sooner,
// after a separator in the second half of the line, so its first piece
// holds at least half a line and it may take more lines.
func wrapped(s string, maxPx, charPx float64) (widest float64, lines int) {
	perLine := max(1, int(maxPx/charPx))
	line := 0
	for _, word := range strings.Fields(s) {
		n := utf8.RuneCountInString(word)
		if line > 0 && float64(line+1+n)*charPx <= maxPx {
			line += 1 + n
			continue
		}
		if line > 0 {
			widest = max(widest, float64(line)*charPx)
			lines++
		}
		line = n
		if float64(n)*charPx > maxPx {
			whole := (n - 1) / perLine
			widest, lines, line = max(widest, maxPx/2), lines+whole, n-whole*perLine
		}
	}
	if line > 0 {
		widest = max(widest, float64(line)*charPx)
		lines++
	}
	return min(maxPx, widest), lines
}

// edgeLabelLines is an edge label's lines in form d: one, or two split at
// the space that leaves the shorter longest line once the label is wider
// than labelSplitPx and the form allows two. It returns the widest line
// in px and the line count.
func edgeLabelLines(label string, d density) (float64, int) {
	widest, lines := float64(utf8.RuneCountInString(label))*textCharPx, 1
	if d.edgeLines < 2 || widest <= labelSplitPx {
		return widest, lines
	}
	words := strings.Fields(label)
	for i := 1; i < len(words); i++ {
		a := utf8.RuneCountInString(strings.Join(words[:i], " "))
		b := utf8.RuneCountInString(strings.Join(words[i:], " "))
		if longest := float64(max(a, b)) * textCharPx; longest < widest {
			widest, lines = longest, 2
		}
	}
	return widest, lines
}

// edgeLabelWidth is an edge label's halo in form d across its widest line.
func edgeLabelWidth(label string, d density) int {
	widest, _ := edgeLabelLines(label, d)
	return int(math.Ceil(widest)) + haloPadPx
}

// edgeLabelHeight is an edge label's halo in form d from top to bottom.
func edgeLabelHeight(label string, d density) int {
	_, lines := edgeLabelLines(label, d)
	return lines*haloLinePx + haloPadYPx
}

// edgeLabel is the label of the edge from one node id to another, or "".
func edgeLabel(b *render.Block, from, to string) string {
	for _, e := range b.Edges {
		if e.From == from && e.To == to {
			return e.Label
		}
	}
	return ""
}
