package lint

import (
	"encoding/json"
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

// TestRowWidth holds the estimate to what the vendored ELK lays out for
// the same rows in the compact form with Fira Code's widths: five short
// boxes, the same with every arrow labelled, and a person with wrapped
// text.
func TestRowWidth(t *testing.T) {
	row := func(labels ...string) *render.Block {
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
	path := []int{0, 1, 2, 3, 4}
	if got := rowWidth(row(), path); got != 767 {
		t.Errorf("unlabelled row: %d px, ELK lays it out at 767", got)
	}
	labelled := row(
		"raw json lines",
		"typed record rows",
		"tagged record rows",
		"nightly sql query",
	)
	if got := rowWidth(labelled, path); got != 1198 {
		t.Errorf("labelled row: %d px, ELK lays it out at 1198", got)
	}
	person := &render.Block{T: "diagram", Nodes: []render.DiagramNode{
		{ID: "u", Label: "Reader", Kind: "person"},
		{ID: "w", Label: "belt watch", Text: "asyncRewake hook; moves its cursor, then wakes"},
	}, Edges: []render.DiagramEdge{{From: "u", To: "w", Label: "ledger entry"}}}
	// 120 + 14, then 23 characters on the widest wrapped line: 159 + 28 = 187,
	// and the one-line label's 83 + 8 between two gaps.
	if got, want := rowWidth(person, []int{0, 1}), 24+134+187+32+91+32; got != want {
		t.Errorf("person and wrapped text: %d px, want %d", got, want)
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
