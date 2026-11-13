package lint

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// The compact form of a diagram as viz.js lays it out (DIAGRAM_BOX,
// DIAGRAM_DENSITY.compact, DIAGRAM_HALO, DIAGRAM_SPACING, and ELK's own
// padding), in px at scale 1. rowWidth reads them to say how wide a path
// draws in a row before the browser draws it.
const (
	nameCharPx   = 7.8 // a box name's character: Fira Code, the default face, at 600 13px
	textCharPx   = 6.9 // a text line's or an edge label's character, at 11.5px
	boxMinPx     = 120 // the narrowest box
	compactBoxPx = 190 // the widest box in the compact form
	boxPadPx     = 14  // a box's padding either side of its text
	personRoomPx = 14  // the strip a person's figure adds to its box
	layerGapPx   = 32  // between two layers, and either side of a label's own layer
	labelSplitPx = 84  // an edge label wider than this takes two lines
	haloPadPx    = 8   // an edge label's halo across its widest line
	graphPadPx   = 12  // ELK's padding around the whole graph, either side
)

// rowWidth estimates how wide the boxes on path (node indexes, in order)
// draw laid out in a row in the compact form, in px: each box as wide as
// its widest wrapped line plus padding, a layer gap between two boxes,
// and a labelled edge's label in a layer of its own with a gap either
// side. Text is measured as the default face draws it, so another face
// the reader picks draws somewhat wider or narrower. It is a floor: a
// group's padding, a wider box beside the path in the same layer, and a
// return edge's label widen the layout past it.
func rowWidth(b *render.Block, path []int) int {
	w := 2 * graphPadPx
	for k, v := range path {
		w += boxWidth(b.Nodes[v])
		if k == 0 {
			continue
		}
		w += layerGapPx
		if label := edgeLabel(b, b.Nodes[path[k-1]].ID, b.Nodes[v].ID); label != "" {
			w += edgeLabelWidth(label) + layerGapPx
		}
	}
	return w
}

// boxWidth is a node's box in the compact form: its name and text wrapped
// inside the widest box, the widest line plus padding, kept between the
// narrowest and widest box, and a person's figure beside it.
func boxWidth(n render.DiagramNode) int {
	inner := float64(compactBoxPx - 2*boxPadPx)
	widest := max(widestLine(n.Label, inner, nameCharPx), widestLine(n.Text, inner, textCharPx))
	w := min(compactBoxPx, max(boxMinPx, int(math.Ceil(widest))+2*boxPadPx))
	if n.Kind == "person" {
		w += personRoomPx
	}
	return w
}

// widestLine is the widest line of s wrapped at maxPx, at charPx a
// character: words fill a line while they fit, and a word wider than a
// line takes the line whole.
func widestLine(s string, maxPx, charPx float64) float64 {
	widest, line := 0.0, 0
	for _, word := range strings.Fields(s) {
		n := utf8.RuneCountInString(word)
		if line > 0 && float64(line+1+n)*charPx <= maxPx {
			line += 1 + n
			continue
		}
		widest = max(widest, float64(line)*charPx)
		line = n
	}
	return min(maxPx, max(widest, float64(line)*charPx))
}

// edgeLabelWidth is an edge label's halo in the compact form: one line,
// or two split at the space that leaves the shorter longest line once the
// label is wider than labelSplitPx.
func edgeLabelWidth(label string) int {
	widest := float64(utf8.RuneCountInString(label)) * textCharPx
	if widest > labelSplitPx {
		words := strings.Fields(label)
		for i := 1; i < len(words); i++ {
			a := utf8.RuneCountInString(strings.Join(words[:i], " "))
			b := utf8.RuneCountInString(strings.Join(words[i:], " "))
			widest = min(widest, float64(max(a, b))*textCharPx)
		}
	}
	return int(math.Ceil(widest)) + haloPadPx
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
