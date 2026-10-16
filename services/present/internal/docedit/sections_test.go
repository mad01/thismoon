package docedit_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

const pageTitle = "Weekly"

// page builds a stored page whose brief and deck are Docs with one section
// per heading, rendered the way the store would hold them.
func page(t *testing.T, headings ...string) store.Snapshot {
	t.Helper()
	doc := docOf(headings...)
	brief, err := render.CompileDoc(doc, pageTitle)
	if err != nil {
		t.Fatal(err)
	}
	deck, err := render.CompileDeckDoc(doc, pageTitle)
	if err != nil {
		t.Fatal(err)
	}
	return store.Snapshot{
		Page: store.Page{
			ID: "abcdef0123", Title: pageTitle, Version: 3,
			Content: brief.HTML, Deck: deck.HTML, HasBrief: true, HasDeck: true, HasDoc: true,
		},
		Sources: store.Sources{Doc: brief.JSON, Deck: deck.JSON},
	}
}

func docOf(headings ...string) render.Doc {
	var d render.Doc
	for _, h := range headings {
		d.Sections = append(d.Sections, section(h))
	}
	return d
}

func section(h string) render.Section {
	return render.Section{Heading: h, Blocks: []render.Block{{T: "p", Text: "about " + h}}}
}

func sectionJSON(t *testing.T, s render.Section) []byte {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// run parses req and applies it to cur, the way a store edit would.
func run(
	t *testing.T,
	cur store.Snapshot,
	req docedit.Request,
) (store.Snapshot, docedit.Result, error) {
	t.Helper()
	p, err := docedit.Parse(req)
	if err != nil {
		return store.Snapshot{}, docedit.Result{}, err
	}
	var res docedit.Result
	next, err := p.Edit(&res)(cur)
	return next, res, err
}

// headingsOf decodes a stored Doc source and lists its section headings.
func headingsOf(t *testing.T, src []byte) []string {
	t.Helper()
	var d render.Doc
	if err := json.Unmarshal(src, &d); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range d.Sections {
		out = append(out, s.Heading)
	}
	return out
}

func TestOpsLandWhereTheAnchorSays(t *testing.T) {
	cases := []struct {
		name      string
		op        string
		anchor    string
		heading   string
		want      []string
		wantIndex int
		wantAt    string
	}{
		{
			"replace",
			"replace",
			"beta",
			"Beta two",
			[]string{"Alpha", "Beta two", "Gamma"},
			1,
			"beta-two",
		},
		{
			"insert before",
			"insert_before",
			"beta",
			"New",
			[]string{"Alpha", "New", "Beta", "Gamma"},
			1,
			"new",
		},
		{
			"insert after",
			"insert_after",
			"beta",
			"New",
			[]string{"Alpha", "Beta", "New", "Gamma"},
			2,
			"new",
		},
		{"remove", "remove", "beta", "", []string{"Alpha", "Gamma"}, 1, "beta"},
		{"append", "append", "", "New", []string{"Alpha", "Beta", "Gamma", "New"}, 3, "new"},
		{"index form", "replace", "index:0", "Zero", []string{"Zero", "Beta", "Gamma"}, 0, "zero"},
		{"heading form", "remove", "Gamma", "", []string{"Alpha", "Beta"}, 2, "gamma"},
		{
			"fragment form",
			"insert_after",
			"#gamma",
			"New",
			[]string{"Alpha", "Beta", "Gamma", "New"},
			3,
			"new",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := docedit.Request{Op: tc.op, Anchor: tc.anchor}
			if tc.heading != "" {
				req.Section = sectionJSON(t, section(tc.heading))
			}
			next, res, err := run(t, page(t, "Alpha", "Beta", "Gamma"), req)
			if err != nil {
				t.Fatalf("patch: %v", err)
			}
			if got := headingsOf(t, next.Sources.Doc); !slices.Equal(got, tc.want) {
				t.Errorf("headings = %v, want %v", got, tc.want)
			}
			if res.Index != tc.wantIndex || res.Anchor != tc.wantAt {
				t.Errorf("result = %+v, want index %d anchor %q", res, tc.wantIndex, tc.wantAt)
			}
			wantContent, err := render.Compile(next.Sources.Doc, pageTitle)
			if err != nil || next.Page.Content != wantContent.HTML {
				t.Errorf("content was not rendered from the patched Doc (err %v)", err)
			}
		})
	}
}

func TestDeckPatchLeavesTheBriefAlone(t *testing.T) {
	cur := page(t, "Alpha", "Beta")
	next, _, err := run(t, cur, docedit.Request{
		Op: "append", Rendition: "deck", Section: sectionJSON(t, section("Slide")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := headingsOf(t, next.Sources.Deck); !slices.Equal(
		got,
		[]string{"Alpha", "Beta", "Slide"},
	) {
		t.Errorf("deck headings = %v", got)
	}
	if !strings.Contains(next.Page.Deck, `id="slide"`) {
		t.Error("the deck rendition was not re-rendered")
	}
	if string(next.Sources.Doc) != string(cur.Sources.Doc) ||
		next.Page.Content != cur.Page.Content {
		t.Error("a deck patch changed the brief")
	}
}

func TestBaseVersionGuardsThePatch(t *testing.T) {
	cur := page(t, "Alpha")
	req := docedit.Request{Op: "append", BaseVersion: 2, Section: sectionJSON(t, section("New"))}
	_, _, err := run(t, cur, req)
	var conflict *docedit.VersionConflictError
	if !errors.As(err, &conflict) || conflict.Current != 3 || conflict.Base != 2 {
		t.Fatalf("stale base: err = %v, want a version conflict at 3", err)
	}
	req.BaseVersion = 3
	if _, _, err := run(t, cur, req); err != nil {
		t.Fatalf("current base: %v", err)
	}
}

func TestSharedAnchorsAreRefusedWithTheirIndices(t *testing.T) {
	cur := page(t, "Alpha", "Dup", "Dup")
	_, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: "dup"})
	var conflict *docedit.AnchorConflictError
	if !errors.As(err, &conflict) || conflict.Creating ||
		!slices.Equal(conflict.Indices, []int{1, 2}) {
		t.Fatalf("ambiguous anchor: err = %v, want indices [1 2]", err)
	}
	next, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: "index:2"})
	if err != nil {
		t.Fatalf("index form: %v", err)
	}
	if got := headingsOf(t, next.Sources.Doc); !slices.Equal(got, []string{"Alpha", "Dup"}) {
		t.Errorf("headings after removing index:2 = %v", got)
	}
}

func TestAWriteMayNotCreateADuplicateAnchor(t *testing.T) {
	cur := page(t, "Alpha", "Beta", "Alpha and Beta")
	cases := []struct {
		name, op, anchor, heading string
		refused                   bool
	}{
		{"append a taken heading", "append", "", "Alpha", true},
		{"insert a taken heading", "insert_after", "beta", "ALPHA!", true},
		{"replace onto another section's anchor", "replace", "beta", "Alpha", true},
		{"replace keeping its own anchor", "replace", "alpha", "Alpha", false},
		{"heading without an anchor", "append", "", "日本語", false},
		{"heading equal once normalized", "append", "", "Alpha & Beta", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, cur, docedit.Request{
				Op: tc.op, Anchor: tc.anchor, Section: sectionJSON(t, section(tc.heading)),
			})
			var conflict *docedit.AnchorConflictError
			if got := errors.As(err, &conflict) && conflict.Creating; got != tc.refused {
				t.Fatalf("err = %v, refused %v, want refused %v", err, got, tc.refused)
			}
			if !tc.refused && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMissingSectionsAndDocs(t *testing.T) {
	cur := page(t, "Alpha", "Beta")
	for _, anchor := range []string{"zeta", "index:9", "index:-1", "index:x"} {
		if _, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: anchor}); !errors.Is(
			err, docedit.ErrAnchorNotFound,
		) {
			t.Errorf("anchor %q: err = %v, want ErrAnchorNotFound", anchor, err)
		}
	}
	if _, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: "日本語"}); !errors.Is(
		err, docedit.ErrInvalid,
	) {
		t.Errorf("anchor without letters: err = %v, want ErrInvalid", err)
	}
	legacy := cur
	legacy.Sources.Doc = nil
	if _, _, err := run(t, legacy, docedit.Request{Op: "remove", Anchor: "alpha"}); !errors.Is(
		err, docedit.ErrNoDoc,
	) {
		t.Errorf("legacy HTML brief: err = %v, want ErrNoDoc", err)
	}
	noDeck := cur
	noDeck.Sources.Deck = nil
	if _, _, err := run(t, noDeck, docedit.Request{Op: "remove", Anchor: "alpha", Rendition: "deck"}); !errors.Is(
		err,
		docedit.ErrNoDoc,
	) {
		t.Errorf("page without a deck: err = %v, want ErrNoDoc", err)
	}
}

func TestTheRendererJudgesTheWholeDoc(t *testing.T) {
	cur := page(t, "Alpha")
	bad := section("Toned")
	bad.Tone = "chartreuse"
	if _, _, err := run(t, cur, docedit.Request{Op: "append", Section: sectionJSON(t, bad)}); !errors.Is(
		err,
		docedit.ErrInvalid,
	) {
		t.Errorf("unknown tone: err = %v, want ErrInvalid", err)
	}
	graph := render.Section{Heading: "Map", Blocks: []render.Block{{T: "graph"}}}
	withGraph, _, err := run(t, cur, docedit.Request{Op: "append", Section: sectionJSON(t, graph)})
	if err != nil {
		t.Fatalf("first graph block: %v", err)
	}
	graph.Heading = "Map two"
	if _, _, err := run(t, withGraph, docedit.Request{Op: "append", Section: sectionJSON(t, graph)}); !errors.Is(
		err,
		docedit.ErrInvalid,
	) {
		t.Errorf("second graph block: err = %v, want ErrInvalid (one graph per page)", err)
	}
}

func TestMarkdownBecomesTheSection(t *testing.T) {
	md := "## Results\n\nShipped **it**.\n\n### Detail\n\n- one\n- two\n"
	next, res, err := run(t, page(t, "Alpha"), docedit.Request{
		Op: "append", BodyFormat: "markdown", Section: []byte(md),
	})
	if err != nil {
		t.Fatal(err)
	}
	var d render.Doc
	if err := json.Unmarshal(next.Sources.Doc, &d); err != nil {
		t.Fatal(err)
	}
	got := d.Sections[1]
	if got.Heading != "Results" || res.Anchor != "results" || len(got.Blocks) != 3 ||
		got.Blocks[0].Text != "Shipped **it**." || got.Blocks[1].T != "h3" || got.Blocks[2].T != "list" {
		t.Fatalf("markdown section = %+v, result %+v", got, res)
	}
}

func TestParseRefusesMalformedRequests(t *testing.T) {
	body := []byte(`{"h":"New","blocks":[]}`)
	cases := map[string]docedit.Request{
		"unknown op":              {Op: "move", Anchor: "a", Section: body},
		"unknown rendition":       {Op: "append", Rendition: "slides", Section: body},
		"append with an anchor":   {Op: "append", Anchor: "a", Section: body},
		"replace without anchor":  {Op: "replace", Section: body},
		"remove with a section":   {Op: "remove", Anchor: "a", Section: body},
		"replace without section": {Op: "replace", Anchor: "a"},
		"unknown body format":     {Op: "append", BodyFormat: "html", Section: body},
		"negative base version":   {Op: "append", BaseVersion: -1, Section: body},
		"malformed section json":  {Op: "append", Section: []byte(`{"h":`)},
		"markdown without heading": {
			Op: "append", BodyFormat: "markdown", Section: []byte("just text"),
		},
		"markdown with two sections": {
			Op: "append", BodyFormat: "markdown", Section: []byte("## One\n\na\n\n## Two\n\nb"),
		},
	}
	for name, req := range cases {
		if _, err := docedit.Parse(req); !errors.Is(err, docedit.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// The renderer stores "A & B" as "A and B", so the anchor a caller gives is
// normalized the same way before it is looked up.
func TestAnchorsFollowNameNormalization(t *testing.T) {
	next, res, err := run(t, page(t, "Alpha"), docedit.Request{
		Op: "append", Section: sectionJSON(t, section("Q & A")),
	})
	if err != nil || res.Anchor != "q-and-a" {
		t.Fatalf("append: result %+v, err %v; want anchor q-and-a", res, err)
	}
	if _, _, err := run(t, next, docedit.Request{Op: "remove", Anchor: "Q & A"}); err != nil {
		t.Fatalf("remove by the heading as typed: %v", err)
	}
}

// TestRepeatedOperatorsAddressByTheRenderedID pins a heading whose name
// normalization needs more than one pass: the section answers to the id
// the page renders and to the heading as typed, and the anchor a patch
// reports is that rendered id.
func TestRepeatedOperatorsAddressByTheRenderedID(t *testing.T) {
	cur := page(t, "Alpha", "A - - B")
	for _, anchor := range []string{"a-minus-minus-b", "#a-minus-minus-b", "A - - B"} {
		if _, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: anchor}); err != nil {
			t.Errorf("remove by %q: %v", anchor, err)
		}
	}
	_, res, err := run(t, cur, docedit.Request{
		Op: "append", Section: sectionJSON(t, section("x & & y")),
	})
	if err != nil || res.Anchor != "x-and-and-y" {
		t.Fatalf("append: result %+v, err %v; want anchor x-and-and-y", res, err)
	}
}

// TestHeadingsWithoutASCIIAreIndexOnly pins the fallback for a heading with
// no ASCII letter or digit: its anchor is empty, a patch naming it is
// refused with a pointer to the index form, and the index form works.
func TestHeadingsWithoutASCIIAreIndexOnly(t *testing.T) {
	cur := page(t, "设计 概览", "Beta")
	if _, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: "设计 概览"}); !errors.Is(
		err, docedit.ErrInvalid,
	) || !strings.Contains(err.Error(), docedit.IndexPrefix) {
		t.Errorf("remove by the heading: err = %v, want ErrInvalid naming the index form", err)
	}
	next, _, err := run(t, cur, docedit.Request{Op: "remove", Anchor: docedit.IndexPrefix + "0"})
	if err != nil {
		t.Fatalf("remove by index: %v", err)
	}
	if got := headingsOf(t, next.Sources.Doc); !slices.Equal(got, []string{"Beta"}) {
		t.Errorf("headings after removing index:0 = %v", got)
	}
}
