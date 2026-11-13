package lint

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/render"
)

// fixture reads testdata/<kind>/<rule>.json, a present_create argument
// object.
func fixture(t *testing.T, kind string, rule limits.ID) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", kind, string(rule)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func lintArgs(t *testing.T, data []byte) []Finding {
	t.Helper()
	p, err := ParseArgs(data)
	if err != nil {
		t.Fatal(err)
	}
	return Check(p)
}

// TestBrokenFixturesTripTheirRule: each broken fixture raises its own rule
// and no other, exactly once, so the skill's fragment pair shows one
// warning beside its fix.
func TestBrokenFixturesTripTheirRule(t *testing.T) {
	for _, l := range limits.Warnings() {
		t.Run(string(l.ID), func(t *testing.T) {
			got := lintArgs(t, fixture(t, "broken", l.ID))
			if len(got) != 1 || got[0].Rule != l.ID {
				t.Fatalf("findings = %+v, want one %s", got, l.ID)
			}
		})
	}
}

// TestFixedFixturesAreClean: each fixed fixture raises nothing.
func TestFixedFixturesAreClean(t *testing.T) {
	for _, l := range limits.Warnings() {
		t.Run(string(l.ID), func(t *testing.T) {
			if got := lintArgs(t, fixture(t, "fixed", l.ID)); len(got) > 0 {
				t.Fatalf("findings = %+v, want none", got)
			}
		})
	}
}

// TestEveryWarningHasAFixture: every warn entry has both files, and every
// file names a warn entry.
func TestEveryWarningHasAFixture(t *testing.T) {
	var want []string
	for _, l := range limits.Warnings() {
		want = append(want, string(l.ID)+".json")
	}
	slices.Sort(want)
	for _, kind := range []string{"broken", "fixed"} {
		entries, err := os.ReadDir(filepath.Join("testdata", kind))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		if !slices.Equal(got, want) {
			t.Errorf("testdata/%s holds %v, want %v", kind, got, want)
		}
	}
}

// TestFixturesCompile: every fixture is a page present_create stores, so a
// fragment pair never teaches a Doc the renderer refuses.
func TestFixturesCompile(t *testing.T) {
	for _, l := range limits.Warnings() {
		for _, kind := range []string{"broken", "fixed"} {
			var raw struct {
				Title   string          `json:"title"`
				Content json.RawMessage `json:"content"`
				Deck    json.RawMessage `json:"deck"`
			}
			if err := json.Unmarshal(fixture(t, kind, l.ID), &raw); err != nil {
				t.Fatal(err)
			}
			if raw.Content != nil {
				if _, err := render.Compile(raw.Content, raw.Title); err != nil {
					t.Errorf("%s/%s content: %v", kind, l.ID, err)
				}
			}
			if raw.Deck != nil {
				if _, err := render.CompileDeck(raw.Deck, raw.Title); err != nil {
					t.Errorf("%s/%s deck: %v", kind, l.ID, err)
				}
			}
		}
	}
}

// TestEveryWarningHasACheck: the lint runs a check for each warn entry and
// for nothing else.
func TestEveryWarningHasACheck(t *testing.T) {
	warn := map[limits.ID]bool{}
	for _, l := range limits.Warnings() {
		warn[l.ID] = true
		if checks[l.ID] == nil {
			t.Errorf("%s: no check", l.ID)
		}
	}
	for id := range checks {
		if !warn[id] {
			t.Errorf("check %s names no warn entry", id)
		}
	}
}

// TestFindingOrder pins the order: by rule in the order of the limits
// table, then by position, the title before the brief and the brief before
// the deck.
func TestFindingOrder(t *testing.T) {
	long := strings.Repeat("word ", limits.Number(limits.SummaryWords)+5)
	p := Page{
		Title: "Title with `code`",
		Brief: render.Doc{Summary: long, Sections: []render.Section{
			{Heading: "Overview", Blocks: []render.Block{{T: "p", Text: "odd ` tick"}}},
			{
				Heading: "The cache cut p99 latency by 40%",
				Blocks:  []render.Block{{T: "stat", Value: "40%", Label: "less"}},
			},
		}},
		Deck: render.Doc{Summary: long, Sections: []render.Section{
			{Heading: "Results", Blocks: []render.Block{{T: "stat", Value: "**3**", Label: "x"}}},
		}},
	}
	type key struct {
		rule  limits.ID
		where string
	}
	var got []key
	for _, f := range Check(p) {
		got = append(got, key{f.Rule, f.Where})
	}
	want := []key{
		{limits.SummaryWords, "content.summary"},
		{limits.SummaryWords, "deck.summary"},
		{limits.HeadingLabel, "content.sections[0].h"},
		{limits.HeadingLabel, "deck.sections[0].h"},
		{limits.InlineStrayMark, "title"},
		{limits.InlineStrayMark, "content.sections[0].blocks[0].text"},
		{limits.InlineStrayMark, "deck.sections[0].blocks[0].value"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("order:\n got %v\nwant %v", got, want)
	}
}

// TestFindingCovers: a finding about a section covers it, a run covers each
// section in it, and a finding about a summary or a whole deck covers none.
func TestFindingCovers(t *testing.T) {
	section := Finding{Rendition: Content, First: 2, Last: 2}
	run := Finding{Rendition: Content, First: 1, Last: 4}
	page := Finding{Rendition: Deck, First: -1, Last: -1}
	cases := []struct {
		name  string
		f     Finding
		r     Rendition
		index int
		want  bool
	}{
		{"its section", section, Content, 2, true},
		{"another section", section, Content, 3, false},
		{"the other rendition", section, Deck, 2, false},
		{"inside a run", run, Content, 3, true},
		{"after a run", run, Content, 5, false},
		{"a whole deck", page, Deck, 0, false},
	}
	for _, tc := range cases {
		if got := tc.f.Covers(tc.r, tc.index); got != tc.want {
			t.Errorf("%s: Covers = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLongestPath(t *testing.T) {
	diagram := func(ids string, edges ...string) *render.Block {
		b := &render.Block{T: "diagram"}
		for _, id := range strings.Fields(ids) {
			b.Nodes = append(b.Nodes, render.DiagramNode{ID: id, Label: id})
		}
		for _, e := range edges {
			from, to, _ := strings.Cut(e, ">")
			b.Edges = append(b.Edges, render.DiagramEdge{From: from, To: to})
		}
		return b
	}
	cases := []struct {
		name string
		b    *render.Block
		want int
	}{
		{"no edges", diagram("a b c"), 1},
		{"a chain", diagram("a b c d", "a>b", "b>c", "c>d"), 4},
		{"the chain starts at the first node", diagram("a b c", "a>b", "b>c"), 3},
		{"a fan out", diagram("a b c d", "a>b", "a>c", "a>d"), 2},
		{"a cycle is broken", diagram("a b c", "a>b", "b>c", "c>a"), 3},
		{"two ways down, the longer wins", diagram("a b c d", "a>d", "a>b", "b>c", "c>d"), 4},
	}
	for _, tc := range cases {
		if got := len(longestPath(tc.b)); got != tc.want {
			t.Errorf("%s: path of %d boxes, want %d", tc.name, got, tc.want)
		}
	}
}

// chainOfFive is five short boxes in a row, the arrows labelled in order
// with labels.
func chainOfFive(labels ...string) *render.Block {
	b := &render.Block{T: "diagram", Nodes: []render.DiagramNode{
		{ID: "a", Label: "Ingest", Text: "reads the feed"},
		{ID: "b", Label: "Parse", Text: "splits records"},
		{ID: "c", Label: "Enrich", Text: "adds the owner"},
		{ID: "d", Label: "Store", Text: "writes rows", Kind: "store"},
		{ID: "e", Label: "Report", Text: "daily digest"},
	}}
	for k := range 4 {
		e := render.DiagramEdge{From: b.Nodes[k].ID, To: b.Nodes[k+1].ID}
		if k < len(labels) {
			e.Label = labels[k]
		}
		b.Edges = append(b.Edges, e)
	}
	return b
}

var chainLabels = []string{
	"raw json lines",
	"typed record rows",
	"tagged record rows",
	"nightly sql query",
}

// TestRowWidth holds the estimate to what the vendored ELK lays out for
// the same rows with Fira Code's widths: five short boxes, the same with
// every arrow labelled, in each form, and a person with wrapped text.
func TestRowWidth(t *testing.T) {
	path := []int{0, 1, 2, 3, 4}
	cases := []struct {
		name string
		b    *render.Block
		d    density
		want int
	}{
		{"unlabelled, compact", chainOfFive(), compactForm, 767},
		{"unlabelled, names", chainOfFive(), namesForm, 752},
		{"labelled, compact", chainOfFive(chainLabels...), compactForm, 1198},
		{"labelled, full", chainOfFive(chainLabels...), fullForm, 1385},
		{"labelled, names", chainOfFive(chainLabels...), namesForm, 1183},
	}
	for _, tc := range cases {
		if got := rowWidth(tc.b, path, tc.d); got != tc.want {
			t.Errorf("%s: %d px, ELK lays it out at %d", tc.name, got, tc.want)
		}
	}
	person := &render.Block{T: "diagram", Nodes: []render.DiagramNode{
		{ID: "u", Label: "Reader", Kind: "person"},
		{ID: "w", Label: "belt watch", Text: "asyncRewake hook; moves its cursor, then wakes"},
	}, Edges: []render.DiagramEdge{{From: "u", To: "w", Label: "ledger entry"}}}
	// 120 + 14, then 23 characters on the widest wrapped line: 159 + 28 = 187,
	// and the one-line label's 83 + 8 between two gaps.
	if got, want := rowWidth(person, []int{0, 1}, compactForm), 24+134+187+32+91+32; got != want {
		t.Errorf("person and wrapped text: %d px, want %d", got, want)
	}
}

// TestColumnHeight holds the column estimate to what the vendored ELK lays
// out top to bottom for the same chain: a store's lid, labels on one line
// in the full form and on two past 84 px in the others, and the box text
// left out of the names form.
func TestColumnHeight(t *testing.T) {
	cases := []struct {
		name string
		b    *render.Block
		d    density
		want int
	}{
		{"unlabelled, full", chainOfFive(), fullForm, 423},
		{"unlabelled, names", chainOfFive(), namesForm, 348},
		{"labelled, full", chainOfFive(chainLabels...), fullForm, 615},
		{"labelled, compact", chainOfFive(chainLabels...), compactForm, 671},
		{"labelled, names", chainOfFive(chainLabels...), namesForm, 596},
		{"short labels, names", chainOfFive("a", "bb", "ccc", "dddd"), namesForm, 540},
	}
	for _, tc := range cases {
		if got := flowStretch(tc.b, tc.d, true).px; got != tc.want {
			t.Errorf("%s: %d px, ELK lays it out at %d", tc.name, got, tc.want)
		}
	}
}

// grouped is chainOfFive with its boxes in the groups member names, by box
// id, and the groups given.
func grouped(member map[string]string, groups ...render.DiagramGroup) *render.Block {
	b := chainOfFive()
	for k := range b.Nodes {
		b.Nodes[k].Group = member[b.Nodes[k].ID]
	}
	b.Groups = groups
	return b
}

// letters is a diagram of boxes named by the ids in ids, with the edges
// given as "from>to", each labelled label.
func letters(ids, edges, label string) *render.Block {
	b := &render.Block{T: "diagram"}
	for _, id := range strings.Fields(ids) {
		b.Nodes = append(b.Nodes, render.DiagramNode{ID: id, Label: strings.ToUpper(id)})
	}
	for _, e := range strings.Fields(edges) {
		from, to, _ := strings.Cut(e, ">")
		b.Edges = append(b.Edges, render.DiagramEdge{From: from, To: to, Label: label})
	}
	return b
}

// diagramJSON reads a diagram block from its JSON.
func diagramJSON(t *testing.T, s string) *render.Block {
	t.Helper()
	var b render.Block
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		t.Fatal(err)
	}
	return &b
}

// platform is the example deck's context diagram: two people into a
// group, and a two-way pair between the group and a provider.
const platform = `{"t":"diagram","direction":"LR",
"groups":[{"id":"platform","label":"the platform"}],
"nodes":[
 {"id":"shopper","label":"Shopper","text":"a browser on a phone","kind":"person"},
 {"id":"support","label":"Support agent","text":"the back office app","kind":"person"},
 {"id":"gateway","label":"Gateway","text":"Envoy, TLS and routing","group":"platform"},
 {"id":"checkout","label":"Checkout","text":"orders, pricing, stock","group":"platform"},
 {"id":"psp","label":"Payment provider","text":"cards and wallets, hosted","kind":"external"},
 {"id":"mail","label":"Mail service","text":"receipts, hosted","kind":"external"}],
"edges":[
 {"from":"shopper","to":"gateway","label":"buys"},{"from":"support","to":"gateway","label":"refunds"},
 {"from":"gateway","to":"checkout","label":"routes"},{"from":"checkout","to":"psp","label":"charges"},
 {"from":"psp","to":"checkout","label":"webhooks"},{"from":"checkout","to":"mail","label":"sends receipts"}]}`

// cluster is the example deck's cluster diagram: two namespaces in a
// cluster, the second after the first because one box fans out to both.
const cluster = `{"t":"diagram","direction":"LR",
"groups":[{"id":"prod","label":"prod cluster"},
 {"id":"ns_checkout","label":"checkout namespace","group":"prod"},
 {"id":"ns_data","label":"data namespace","group":"prod"}],
"nodes":[
 {"id":"gw","label":"Gateway","text":"Envoy, terminates TLS","group":"prod"},
 {"id":"api","label":"Checkout API","text":"Go, validates the cart","group":"ns_checkout"},
 {"id":"pricing","label":"Pricing","text":"Go, quotes and discounts","group":"ns_checkout"},
 {"id":"stock","label":"Stock","text":"Go, reserves units","group":"ns_checkout"},
 {"id":"orders","label":"Orders DB","text":"Postgres, one row per order","kind":"store","group":"ns_data"},
 {"id":"events","label":"Order events","text":"Kafka, one topic","kind":"queue","group":"ns_data"}],
"edges":[
 {"from":"gw","to":"api","label":"route"},{"from":"api","to":"pricing","label":"quote"},
 {"from":"pricing","to":"api","label":"price"},{"from":"api","to":"stock","label":"reserve"},
 {"from":"api","to":"orders","label":"insert"},{"from":"api","to":"events","label":"order placed"}]}`

// TestFlowStretch holds the flow estimate to what the vendored ELK lays
// out with Fira Code's widths: exactly for groups on a chain and for
// cycles whose every way round draws the same, and at or under it where
// ELK's way round a cycle or a box beside the run draws longer.
func TestFlowStretch(t *testing.T) {
	g := func(id, label, in string) render.DiagramGroup {
		return render.DiagramGroup{ID: id, Label: label, Group: in}
	}
	middle := grouped(map[string]string{"b": "g", "c": "g"}, g("g", "x", ""))
	nested := grouped(
		map[string]string{"b": "out", "c": "in", "d": "out"},
		g("out", "x", ""),
		g("in", "y", "out"),
	)
	label := grouped(map[string]string{"c": "g"}, g("g", "a very long group label here ok", ""))
	cases := []struct {
		name      string
		b         *render.Block
		d         density
		tb        bool
		want, elk int
	}{
		// 767 and 423 with no group, plus the padding and 7 px where each arrow meets a border.
		{"a group in the middle, row", middle, compactForm, false, 813, 813},
		{"a group in the middle, column", middle, compactForm, true, 489, 489},
		{"a group in a group, row", nested, compactForm, false, 859, 859},
		{"a group in a group, column", nested, compactForm, true, 555, 555},
		{"a group's label wider than its box, row", label, compactForm, false, 894, 894},
		{"a group's label wider than its box, column", label, compactForm, true, 489, 489},
		{
			"a two-way pair between two boxes",
			letters("a u v b", "a>u u>v v>u v>b", "lbl"),
			namesForm,
			false,
			357,
			357,
		},
		{
			"a loop with a way in and out",
			letters("x a b c d y", "x>a a>b b>c c>d d>a d>y", ""),
			namesForm,
			false,
			600,
			600,
		},
		{
			"a loop closed by a return",
			letters("s p r w e", "s>r r>e p>e s>w e>w w>s", ""),
			namesForm,
			false,
			600,
			600,
		},
		{"a two-way pair beside a group", diagramJSON(t, platform), namesForm, false, 955, 955},
		{"a fan out across two groups, row", diagramJSON(t, cluster), namesForm, false, 1008, 1237},
		{"a fan out across two groups, column", diagramJSON(t, cluster), namesForm, true, 599, 719},
	}
	for _, tc := range cases {
		got := flowStretch(tc.b, tc.d, tc.tb).px
		if got != tc.want {
			t.Errorf("%s: %d px, want %d", tc.name, got, tc.want)
		}
		if got > tc.elk {
			t.Errorf("%s: %d px, past the %d px ELK lays it out at", tc.name, got, tc.elk)
		}
	}
}

// TestSlidePathCountsGroups: the cluster's longest single path fits a
// slide, but its groups put the data namespace after the checkout one, so
// no form draws it at the slide's smallest text.
func TestSlidePathCountsGroups(t *testing.T) {
	b := diagramJSON(t, cluster)
	p := Page{Deck: render.Doc{Sections: []render.Section{{
		Heading: "A request walks the cluster",
		Blocks:  []render.Block{*b},
	}}}}
	got := checkSlidePath(p, limits.Number(limits.DiagramSlidePath))
	if len(got) != 1 || !strings.Contains(got[0].Message, "through 3 groups") {
		t.Fatalf("findings = %+v, want one naming its 3 groups", got)
	}
}

// TestWrappedLongWord: a word wider than a line fills whole lines and
// leaves its last piece to the next, and counts at least half a line wide,
// since the browser may break it at a separator. Both stay at or under
// what the browser draws for "services/present/internal/server/viz.js is
// read" in a compact box: three lines, the widest 22 characters.
func TestWrappedLongWord(t *testing.T) {
	inner := compactForm.boxPx - 2*boxPadPx
	widest, lines := wrapped("services/present/internal/server/viz.js is read", inner, textCharPx)
	if lines != 3 {
		t.Errorf("lines = %d, want 3", lines)
	}
	if widest < inner/2 || widest > 22*textCharPx {
		t.Errorf("widest = %.1f px, want between %.1f and %.1f", widest, inner/2, 22*textCharPx)
	}
}

// TestWrappedHangsClosingMarks: a comma that ends a word hangs past the
// line, so "turn," stays on the first line the way viz.js wraps it, and
// the text takes two lines in a compact box, not three.
func TestWrappedHangsClosingMarks(t *testing.T) {
	inner := compactForm.boxPx - 2*boxPadPx
	if _, lines := wrapped("each edit with its turn, prompt and later commit", inner, textCharPx); lines != 2 {
		t.Errorf("lines = %d, want 2", lines)
	}
	cases := map[string]int{"turn,": 1, "end).": 2, "a,,,": 2, ",,": 1, "word": 0}
	for word, want := range cases {
		if got := hangingMarks(word); got != want {
			t.Errorf("hangingMarks(%q) = %d, want %d", word, got, want)
		}
	}
}

// TestSlidePathNumber holds diagram.slide_path's number to the slide it
// stands for: the body's 86u in px at scale 1 when box text, 11.5 px at
// scale 1, draws at the smallest slide text, 1.1u (shell.html's --floor
// and viz.js's DIAGRAM_SLIDE_FLOOR and DIAGRAM_TEXT_PX).
func TestSlidePathNumber(t *testing.T) {
	const floorU, textPx = 1.1, 11.5
	if got, want := limits.Number(limits.DiagramSlidePath), int(math.Round(slideWidthU*textPx/floorU)); got != want {
		t.Errorf("diagram.slide_path is %d px, the slide holds %d", got, want)
	}
}

// TestSplitRoom holds the split slide's right column to the slide it sits
// on: three fifths of the body's 86u once the 6u gap is out, 48u, and
// height caps under the 46.75u body with room left for a caption.
func TestSplitRoom(t *testing.T) {
	if splitWidthU != 48 || splitGapU+splitWidthU*5/3 != slideWidthU {
		t.Errorf(
			"split column %du with a %du gap, want 48u of the %du body",
			splitWidthU,
			splitGapU,
			slideWidthU,
		)
	}
	for _, c := range []struct {
		split, stepped bool
		w, h           int
	}{
		{false, false, slideWidthU, slideCapU},
		{false, true, slideWidthU, slideCapStepU},
		{true, false, splitWidthU, splitCapU},
		{true, true, splitWidthU, splitCapStepU},
	} {
		if w, h := slideRoom(c.split, c.stepped); w != c.w || h != c.h {
			t.Errorf(
				"slideRoom(split %v, stepped %v) = %du by %du, want %du by %du",
				c.split,
				c.stepped,
				w,
				h,
				c.w,
				c.h,
			)
		}
	}
}

// splitDeck is a deck of one split slide holding blocks.
func splitDeck(blocks ...render.Block) Page {
	return Page{Deck: render.Doc{Sections: []render.Section{
		{Heading: "The cache cut p99 latency by 40%", Layout: "split", Blocks: blocks},
	}}}
}

// TestSplitWordsLeaveTheVisualOut: the words beside a split slide's visual
// count, its title and caption do not, and a default slide never trips the
// rule.
func TestSplitWordsLeaveTheVisualOut(t *testing.T) {
	n := limits.Number(limits.SplitWords)
	long := strings.TrimSpace(strings.Repeat("word ", n+1))
	image := render.Block{T: "image", Src: "https://example.com/a.png", Alt: "a", Caption: long}
	if got := checkSplitWords(splitDeck(render.Block{T: "p", Text: "Short."}, image), n); len(
		got,
	) != 0 {
		t.Errorf("a long caption on the visual: %+v, want none", got)
	}
	got := checkSplitWords(splitDeck(render.Block{T: "p", Text: long}, image), n)
	if len(got) != 1 ||
		!strings.Contains(got[0].Message, fmt.Sprintf("%d words beside the visual", n+1)) {
		t.Errorf("a long paragraph: %+v, want one finding of %d words", got, n+1)
	}
	p := splitDeck(render.Block{T: "p", Text: long}, image)
	p.Deck.Sections[0].Layout = ""
	if got := checkSplitWords(p, n); len(got) != 0 {
		t.Errorf("a default slide: %+v, want none", got)
	}
}

// TestSplitSlidePathUsesTheColumn: a diagram path that fits a full slide's
// body can still be too long for a split slide's right column.
func TestSplitSlidePathUsesTheColumn(t *testing.T) {
	// Five stores named by one long word each: about 750 px in a row and
	// 460 px in a column, which a full slide's 899 px row holds and the
	// right column's 502 by 439 px does not.
	var nodes []render.DiagramNode
	var edges []render.DiagramEdge
	for k, id := range []string{"a", "b", "c", "d", "e"} {
		nodes = append(nodes, render.DiagramNode{
			ID: id, Kind: "store", Label: fmt.Sprintf("payments-reconciliation-step-%d", k),
		})
		if k > 0 {
			edges = append(edges, render.DiagramEdge{From: nodes[k-1].ID, To: id})
		}
	}
	diagram := render.Block{T: "diagram", Direction: "LR", Nodes: nodes, Edges: edges}
	n := limits.Number(limits.DiagramSlidePath)
	full := splitDeck(diagram)
	full.Deck.Sections[0].Layout = ""
	if got := checkSlidePath(full, n); len(got) != 0 {
		t.Fatalf("a full-width slide: %+v, want none", got)
	}
	got := checkSlidePath(splitDeck(diagram), n)
	if len(got) != 1 || !strings.Contains(got[0].Message, "a split slide's right column holds") {
		t.Errorf("a split slide: %+v, want one finding about the right column", got)
	}
}

// TestTextOnlyCountsSplitSlides: a split slide holds a picture, so it
// counts beside the default-layout slides and lowers the text-only share.
func TestTextOnlyCountsSplitSlides(t *testing.T) {
	text := render.Section{
		Heading: "Two changes close the gap",
		Blocks:  []render.Block{{T: "p", Text: "x"}},
	}
	split := render.Section{
		Heading: "The cache cut p99 by 40%",
		Layout:  "split",
		Blocks: []render.Block{
			{T: "image", Src: "https://example.com/a.png", Alt: "a"},
		},
	}
	n := limits.Number(limits.DeckTextOnly)
	if got := checkTextOnly(Page{Deck: render.Doc{Sections: []render.Section{text, text}}}, n); len(
		got,
	) != 1 {
		t.Fatalf("two text slides: %d findings, want 1", len(got))
	}
	p := Page{Deck: render.Doc{Sections: []render.Section{text, split, split}}}
	if got := checkTextOnly(p, n); len(got) != 0 {
		t.Errorf("one text slide of three with two split: %+v, want none", got)
	}
}

// TestHeadingLabelExemptions: the closing sections, the page title, the
// ADHD mode's headings, and a divider slide are labels on purpose.
func TestHeadingLabelExemptions(t *testing.T) {
	cases := []struct {
		heading string
		layout  string
		deck    bool
		want    bool
	}{
		{"Next steps", "", false, false},
		{"Next", "", false, false},
		{"References", "", false, false},
		{"Do now", "", false, false},
		{"Later", "", false, false},
		{"Release notes", "", false, false}, // the page title
		{"Release notes", "section", true, false},
		{"Part two", "section", true, false},
		{"Part two", "", true, true},
		{"Latency", "", false, true},
		{"How it works", "", false, true},
		{"Background:", "", false, true},
		{"Every request crosses the gateway twice", "", false, false},
	}
	for _, tc := range cases {
		doc := render.Doc{Sections: []render.Section{{Heading: tc.heading, Layout: tc.layout}}}
		p := Page{Title: "Release notes"}
		if tc.deck {
			p.Deck = doc
		} else {
			p.Brief = doc
		}
		got := len(checkHeadingLabel(p, limits.Number(limits.HeadingLabel))) > 0
		if got != tc.want {
			t.Errorf("%q (layout %q, deck %v): warned = %v, want %v",
				tc.heading, tc.layout, tc.deck, got, tc.want)
		}
	}
}

func TestStarPair(t *testing.T) {
	cases := map[string]bool{
		"**bold**":             true,
		"an *emphasis* here":   true,
		"(*aside*)":            true,
		"*.pem, *.key":         false,
		"5 * 3 * 2":            false,
		"a*b*c":                false,
		"one * star":           false,
		"a list: *, -, and +":  false,
		"**":                   false,
		"ends with a star *":   false,
		"*tight*, then a word": true,
	}
	for s, want := range cases {
		if got := starPair.MatchString(s); got != want {
			t.Errorf("%q: match = %v, want %v", s, got, want)
		}
	}
}

func TestWords(t *testing.T) {
	cases := map[string]int{
		"":                       0,
		"one":                    1,
		"two words":              2,
		"a → b - c":              3,
		"  spaced   out  ":       2,
		"**bold** and `code`":    3,
		"[the brief](https://x)": 2,
	}
	for s, want := range cases {
		if got := words(s); got != want {
			t.Errorf("words(%q) = %d, want %d", s, got, want)
		}
	}
}

// TestSlideWordsCountTheLongestCaption: of a stepped block only its
// longest caption counts, and a figure's labels and the notes never do.
func TestSlideWordsCountTheLongestCaption(t *testing.T) {
	long := strings.Repeat("w ", 30)
	p := Page{Deck: render.Doc{Sections: []render.Section{{
		Heading: "A stepped diagram walks the request",
		Notes:   strings.Repeat("note ", 80),
		Blocks: []render.Block{
			{T: "p", Text: strings.Repeat("p ", 15)},
			{
				T: "diagram",
				Nodes: []render.DiagramNode{
					{ID: "a", Label: strings.Repeat("label ", 5), Text: long},
				},
				Steps: []render.Step{{Caption: long}, {Caption: long + long}, {Caption: "short"}},
			},
		},
	}}}}
	got := checkSlideWords(p, 74)
	if len(got) != 1 || !strings.Contains(got[0].Message, "75 words") {
		t.Fatalf("findings = %+v, want one at 75 words (15 prose + 60 caption)", got)
	}
	if got := checkSlideWords(p, 75); len(got) != 0 {
		t.Fatalf("at the limit: findings = %+v, want none", got)
	}
}

// TestParagraphCoversBriefParagraphsOnly: a p block over the limit warns in
// the brief, in columns and details too; a callout, a list item, a quote,
// and a paragraph on a slide do not.
func TestParagraphCoversBriefParagraphsOnly(t *testing.T) {
	n := limits.Number(limits.BriefParagraph)
	long := strings.Repeat("**word** ", n+1)
	p := Page{
		Brief: render.Doc{Sections: []render.Section{{
			Heading: "Long prose in every block",
			Blocks: []render.Block{
				{T: "p", Text: strings.Repeat("word ", n)},
				{T: "p", Text: long},
				{T: "callout", Text: long},
				{T: "quote", Text: long},
				{T: "list", Items: []string{long}},
				{T: "columns", Columns: [][]render.Block{{{T: "p", Text: long}}, {}}},
				{T: "details", Summary: "More", Blocks: []render.Block{{T: "p", Text: long}}},
			},
		}}},
		Deck: render.Doc{Sections: []render.Section{{
			Heading: "Long prose on a slide",
			Blocks:  []render.Block{{T: "p", Text: long}},
		}}},
	}
	locate := fmt.Sprintf(`section "Long prose in every block", a paragraph: %d words`, n+1)
	var got []string
	for _, f := range checkParagraph(p, n) {
		got = append(got, f.Where)
		if !strings.Contains(f.Message, locate) {
			t.Errorf("message %q lacks the section or the count", f.Message)
		}
	}
	want := []string{
		"content.sections[0].blocks[1]",
		"content.sections[0].blocks[5].cols[0][0]",
		"content.sections[0].blocks[6].blocks[0]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings at %v, want %v", got, want)
	}
}

func TestSummarize(t *testing.T) {
	r := Summarize(nil)
	if r.Warnings == nil || len(r.Warnings) != 0 {
		t.Errorf("Warnings = %#v, want an empty list", r.Warnings)
	}
	if len(r.Counts) != len(limits.Warnings()) {
		t.Errorf("counts hold %d rules, want every warn rule", len(r.Counts))
	}
	r = Summarize([]Finding{{Rule: limits.TableRows}, {Rule: limits.TableRows}})
	if r.Counts[limits.TableRows] != 2 || r.Counts[limits.SummaryWords] != 0 {
		t.Errorf("counts = %v", r.Counts)
	}
}

func TestParseArgs(t *testing.T) {
	doc := `{"summary":"s","sections":[{"h":"A heading that claims","blocks":[]}]}`
	quoted, _ := json.Marshal(doc)
	cases := []struct {
		name     string
		args     string
		sections int
		wantErr  bool
	}{
		{"content as a string", `{"title":"T","content":` + string(quoted) + `}`, 1, false},
		{"content as an object", `{"title":"T","content":` + doc + `}`, 1, false},
		{"legacy HTML", `{"title":"T","content":"<p>hi</p>"}`, 0, false},
		{"no content", `{"title":"T"}`, 0, false},
		{"a number", `{"title":"T","content":4}`, 0, true},
		{"broken Doc", `{"title":"T","content":"{\"sections\":3}"}`, 0, true},
		{"not JSON", `title`, 0, true},
	}
	for _, tc := range cases {
		p, err := ParseArgs([]byte(tc.args))
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, want error %v", tc.name, err, tc.wantErr)
			continue
		}
		if err == nil && len(p.Brief.Sections) != tc.sections {
			t.Errorf("%s: %d sections, want %d", tc.name, len(p.Brief.Sections), tc.sections)
		}
	}
	p, err := ParseArgs([]byte(`{"title":"T","deck":` + doc + `}`))
	if err != nil || len(p.Deck.Sections) != 1 || p.Title != "T" {
		t.Errorf("deck: %+v, %v", p, err)
	}
}

func TestFromSources(t *testing.T) {
	p, err := FromSources("T", []byte(`{"sections":[{"h":"x","blocks":[]}]}`), nil)
	if err != nil || len(p.Brief.Sections) != 1 || len(p.Deck.Sections) != 0 {
		t.Errorf("FromSources = %+v, %v", p, err)
	}
	if _, err := FromSources("T", nil, []byte(`{"sections":`)); err == nil {
		t.Error("a broken deck source must be an error")
	}
}
