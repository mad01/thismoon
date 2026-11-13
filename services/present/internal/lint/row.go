package lint

import (
	"maps"
	"math"
	"slices"
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

// A group as viz.js draws it (DIAGRAM_GROUP_PAD, DIAGRAM_GROUP_LABEL), and
// the room ELK leaves where an arrow meets a group's border, in px at
// scale 1.
const (
	groupPadPx      = 16 // a group's padding left, right, and below what it holds
	groupTopPx      = 36 // a group's padding above what it holds: its label's strip
	groupLabelPadPx = 12 // a group's label from either side of the group
	groupPortPx     = 7  // past the layer gap, where an arrow meets a group's border
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

// stretch is a diagram's longest run along its flow in one form: how long
// it draws in px, the boxes on it (node indexes) first to last, and how
// many groups it passes through.
type stretch struct {
	px     int
	nodes  []int
	groups int
}

// flowStretch estimates how long a diagram draws along its flow in form d:
// its width laid out left to right, its height top to bottom (tb). ELK
// lays each group out on its own and places it whole, as one box, among
// the boxes and groups beside it. So the stretch is the longest run at the
// top level, where a group on the run counts as long as its own longest
// run plus its padding, and in a row at least as wide as its label. A run
// counts each box or group, a layer gap between two, a labelled arrow's
// label in a layer of its own with a gap either side, and the room where
// an arrow meets a group's border. Two units with several arrows between
// them take the widest label. ELK breaks a cycle by turning arrows around,
// and orders tells which ways it may go, so a level's run is the shortest
// of its longest runs over those ways. On a chain with no group it is what
// ELK lays out. It is a floor: a wider box beside the run in the same
// layer, and an arrow that ELK spreads over more layers, draw longer.
func flowStretch(b *render.Block, d density, tb bool) stretch {
	s := newFlow(b, d, tb).run("")
	s.px += 2 * graphPadPx
	return s
}

// flow is a diagram in one form, sorted into levels: the top level ("")
// and each group, each holding the boxes and groups directly in it, its
// units, and the arrows between two of them.
type flow struct {
	b      *render.Block
	d      density
	tb     bool
	node   map[string]int                 // a box's index, by id
	parent map[string]string              // the group a box or group sits in, "" at the top
	groups map[string]render.DiagramGroup // a group, by id
	units  map[string][]string            // a level's units in reading order, by the level's id
	arrows map[string][]arrow             // a level's arrows, in the order of the edges
	runs   map[string]stretch             // a level's run, once worked out
}

// arrow is the edges from one unit of a level to another: how many, and
// the widest label's room along the flow (0 when none has a label).
type arrow struct {
	from, to string
	count    int
	room     int
}

// newFlow sorts b into levels. A box or group whose group is not one of
// b's sits at the top, and so does a group nested in a loop.
func newFlow(b *render.Block, d density, tb bool) *flow {
	f := &flow{
		b: b, d: d, tb: tb,
		node:   map[string]int{},
		parent: map[string]string{},
		groups: map[string]render.DiagramGroup{},
		units:  map[string][]string{},
		arrows: map[string][]arrow{},
		runs:   map[string]stretch{},
	}
	for _, g := range b.Groups {
		f.groups[g.ID] = g
	}
	for k, n := range b.Nodes {
		f.node[n.ID] = k
		f.parent[n.ID] = f.known(n.Group)
	}
	for _, g := range b.Groups {
		f.parent[g.ID] = f.known(g.Group)
	}
	for _, g := range b.Groups {
		if f.loops(g.ID) {
			f.parent[g.ID] = ""
		}
	}
	seen := map[string]bool{}
	for _, n := range b.Nodes {
		for _, id := range f.chain(n.ID) {
			if !seen[id] {
				seen[id] = true
				f.units[f.parent[id]] = append(f.units[f.parent[id]], id)
			}
		}
	}
	for _, e := range b.Edges {
		f.addArrow(e)
	}
	return f
}

// known is id when it names one of b's groups, else "".
func (f *flow) known(id string) string {
	if _, ok := f.groups[id]; ok {
		return id
	}
	return ""
}

// loops reports whether walking up from group id comes back to it.
func (f *flow) loops(id string) bool {
	at := f.parent[id]
	for range len(f.groups) {
		if at == "" {
			return false
		}
		if at == id {
			return true
		}
		at = f.parent[at]
	}
	return at != ""
}

// chain is id and the groups around it, innermost first, without the top.
func (f *flow) chain(id string) []string {
	out := []string{id}
	for at := f.parent[id]; at != "" && len(out) <= len(f.groups); at = f.parent[at] {
		out = append(out, at)
	}
	return out
}

// addArrow files edge e at the innermost level holding both its ends, as
// an arrow between the two units of that level its ends sit in. An edge
// whose ends are not two different boxes is left out.
func (f *flow) addArrow(e render.DiagramEdge) {
	_, okFrom := f.node[e.From]
	_, okTo := f.node[e.To]
	if !okFrom || !okTo || e.From == e.To {
		return
	}
	from, to := f.chain(e.From), f.chain(e.To)
	for len(from) > 1 && len(to) > 1 && from[len(from)-1] == to[len(to)-1] {
		from, to = from[:len(from)-1], to[:len(to)-1]
	}
	a := arrow{from: from[len(from)-1], to: to[len(to)-1], count: 1}
	if e.Label != "" {
		a.room = f.labelRoom(e.Label)
	}
	level := f.parent[a.from]
	for k, old := range f.arrows[level] {
		if old.from == a.from && old.to == a.to {
			f.arrows[level][k].count++
			f.arrows[level][k].room = max(old.room, a.room)
			return
		}
	}
	f.arrows[level] = append(f.arrows[level], a)
}

// labelRoom is how long an edge label's halo draws along the flow.
func (f *flow) labelRoom(label string) int {
	if f.tb {
		return edgeLabelHeight(label, f.d)
	}
	return edgeLabelWidth(label, f.d)
}

// unit is how long a box or group draws along the flow, as a run of its
// own: a box alone, or a group's run inside its padding.
func (f *flow) unit(id string) stretch {
	if k, ok := f.node[id]; ok {
		if f.tb {
			return stretch{px: boxHeight(f.b.Nodes[k], f.d), nodes: []int{k}}
		}
		return stretch{px: boxWidth(f.b.Nodes[k], f.d), nodes: []int{k}}
	}
	s := f.run(id)
	s.groups++
	if f.tb {
		s.px += groupTopPx + groupPadPx
		return s
	}
	label := int(
		math.Ceil(float64(utf8.RuneCountInString(f.groups[id].Label))*textCharPx),
	) + 2*groupLabelPadPx
	s.px = max(s.px+2*groupPadPx, label)
	return s
}

// run is a level's run: the shortest, over the orders ELK may put its
// units in, of the longest run along its arrows once each points the way
// the order does. A level with too many orders to try counts as its
// longest unit alone.
func (f *flow) run(level string) stretch {
	if s, ok := f.runs[level]; ok {
		return s
	}
	own := map[string]stretch{}
	var out stretch
	for _, u := range f.units[level] {
		own[u] = f.unit(u)
		if own[u].px > out.px {
			out = own[u]
		}
	}
	for k, order := range f.orders(level) {
		if s := f.longest(level, order, own); k == 0 || s.px < out.px {
			out = s
		}
	}
	f.runs[level] = out
	return out
}

// longest is the longest run through a level's units with every arrow
// pointing from the unit earlier in order to the later, given each unit's
// own run. An arrow turned around keeps its label out of the run, since
// ELK may put that label in a layer beside either end. Of equal runs the
// one starting earlier in reading order wins.
func (f *flow) longest(level string, order []string, own map[string]stretch) stretch {
	place := map[string]int{}
	for k, u := range order {
		place[u] = k
	}
	next, room := map[string][]string{}, map[[2]string]int{}
	for _, a := range f.arrows[level] {
		key, label := [2]string{a.from, a.to}, a.room
		if place[a.from] > place[a.to] {
			key, label = [2]string{a.to, a.from}, 0
		}
		if r, ok := room[key]; ok {
			room[key] = max(r, label)
			continue
		}
		room[key] = label
		next[key[0]] = append(next[key[0]], key[1])
	}
	best := map[string]stretch{}
	var walk func(u string) stretch
	walk = func(u string) stretch {
		if s, ok := best[u]; ok {
			return s
		}
		s := own[u]
		for _, v := range next[u] {
			rest := walk(v)
			if px := own[u].px + f.gap(u, v, room[[2]string{u, v}]) + rest.px; px > s.px {
				s = stretch{
					px:     px,
					nodes:  append(slices.Clone(own[u].nodes), rest.nodes...),
					groups: own[u].groups + rest.groups,
				}
			}
		}
		best[u] = s
		return s
	}
	var out stretch
	for _, u := range f.units[level] {
		if s := walk(u); s.px > out.px {
			out = s
		}
	}
	return out
}

// gap is the room between two units an arrow joins: a layer gap, the
// widest label's layer (room) and the gap after it, and the room at each
// end that is a group.
func (f *flow) gap(from, to string, room int) int {
	g := layerGapPx
	if room > 0 {
		g += room + layerGapPx
	}
	for _, id := range []string{from, to} {
		if _, box := f.node[id]; !box {
			g += groupPortPx
		}
	}
	return g
}

// maxOrders caps how many orders of one level orders works through.
const maxOrders = 1024

// orders are the orders ELK's greedy cycle breaking may put a level's
// units in. While a unit no arrow leaves is left, it goes to the right
// end, else one no arrow enters to the left end; when neither is left,
// the unit with the most arrows out over in goes to the left, and ELK
// picks any one of a tie, so each pick is an order of its own. It is nil
// past maxOrders.
func (f *flow) orders(level string) [][]string {
	var out [][]string
	var peel func(left, right []string, gone map[string]bool)
	peel = func(left, right []string, gone map[string]bool) {
		for len(out) <= maxOrders {
			in, outs := f.degrees(level, gone)
			end, ok := f.pick(level, gone, func(u string) bool { return outs[u] == 0 })
			if ok {
				gone[end] = true
				right = append(right, end)
				continue
			}
			start, ok := f.pick(level, gone, func(u string) bool { return in[u] == 0 })
			if ok {
				gone[start] = true
				left = append(left, start)
				continue
			}
			tied := f.mostOut(level, gone, in, outs)
			if len(tied) == 0 {
				slices.Reverse(right)
				out = append(out, append(left, right...))
				return
			}
			for _, u := range tied {
				g := maps.Clone(gone)
				g[u] = true
				peel(append(slices.Clone(left), u), slices.Clone(right), g)
			}
			return
		}
	}
	peel(nil, nil, map[string]bool{})
	if len(out) > maxOrders {
		return nil
	}
	return out
}

// degrees counts the arrows entering and leaving each unit of a level,
// leaving out those to and from the units gone.
func (f *flow) degrees(level string, gone map[string]bool) (in, out map[string]int) {
	in, out = map[string]int{}, map[string]int{}
	for _, a := range f.arrows[level] {
		if !gone[a.from] && !gone[a.to] {
			out[a.from] += a.count
			in[a.to] += a.count
		}
	}
	return in, out
}

// pick is the first unit of a level, in reading order, not gone and
// passing ok.
func (f *flow) pick(level string, gone map[string]bool, ok func(string) bool) (string, bool) {
	for _, u := range f.units[level] {
		if !gone[u] && ok(u) {
			return u, true
		}
	}
	return "", false
}

// mostOut is the units of a level not gone with the most arrows out over
// in, in reading order.
func (f *flow) mostOut(level string, gone map[string]bool, in, out map[string]int) []string {
	var tied []string
	most := 0
	for _, u := range f.units[level] {
		if gone[u] {
			continue
		}
		switch d := out[u] - in[u]; {
		case len(tied) == 0 || d > most:
			tied, most = []string{u}, d
		case d == most:
			tied = append(tied, u)
		}
	}
	return tied
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
// while they fit, the closing marks that end a word hanging past the
// line's end as viz.js's wrapText lets them, and a word wider than a line
// fills whole lines and leaves its last piece to start the next. It
// returns the widest line and the line count, both floors: the browser may
// break a long word sooner, after a separator in the second half of the
// line, so its first piece holds at least half a line and it may take more
// lines.
func wrapped(s string, maxPx, charPx float64) (widest float64, lines int) {
	perLine := max(1, int(maxPx/charPx))
	line := 0
	for _, word := range strings.Fields(s) {
		n := utf8.RuneCountInString(word)
		fit := n - hangingMarks(word)
		if line > 0 && float64(line+1+fit)*charPx <= maxPx {
			line += 1 + n
			continue
		}
		if line > 0 {
			widest = max(widest, float64(line)*charPx)
			lines++
		}
		line = n
		if float64(fit)*charPx > maxPx {
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

// closingMarks are the marks viz.js lets hang past a line's end
// (DIAGRAM_CLOSING), and maxHanging how many of them may (DIAGRAM_HANG).
const (
	closingMarks = ",.;:)]"
	maxHanging   = 2
)

// hangingMarks is how many closing marks end word and may hang, never all
// of it.
func hangingMarks(word string) int {
	runes := []rune(word)
	n := 0
	for n < maxHanging && n < len(runes)-1 && strings.ContainsRune(closingMarks, runes[len(runes)-1-n]) {
		n++
	}
	return n
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
