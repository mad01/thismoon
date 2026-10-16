package docedit_test

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

func pct(v float64) *float64 { return &v }

// report parses req and applies it to cur, the way a store edit would.
func report(
	t *testing.T, cur store.Snapshot, req docedit.ProgressRequest,
) (store.Snapshot, docedit.ProgressResult, error) {
	t.Helper()
	p, err := docedit.ParseProgress(req)
	if err != nil {
		return store.Snapshot{}, docedit.ProgressResult{}, err
	}
	var res docedit.ProgressResult
	next, err := p.Edit(&res)(cur)
	return next, res, err
}

// progressBlocks lists every progress block of a stored Doc with the
// heading of its section.
func progressBlocks(t *testing.T, src []byte) map[string][]render.Block {
	t.Helper()
	var d render.Doc
	if err := json.Unmarshal(src, &d); err != nil {
		t.Fatal(err)
	}
	out := map[string][]render.Block{}
	for _, s := range d.Sections {
		for _, b := range s.Blocks {
			if b.T == "progress" {
				out[s.Heading] = append(out[s.Heading], b)
			}
		}
	}
	return out
}

func TestProgressInsertsOnceThenUpdatesInPlace(t *testing.T) {
	cur := page(t, "Alpha")
	next, res, err := report(t, cur, docedit.ProgressRequest{Label: "Index", Percent: pct(25)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Inserted || res.Section != 1 || res.Anchor != "progress" {
		t.Fatalf("first report = %+v, want a new block in a new Progress section", res)
	}
	next, res, err = report(t, next, docedit.ProgressRequest{
		Label: "Index", Percent: pct(60), Note: "12 of 20",
	})
	if err != nil || res.Inserted || res.Section != 1 {
		t.Fatalf("second report = %+v, %v; want the same block updated", res, err)
	}
	if got := headingsOf(t, next.Sources.Doc); !slices.Equal(got, []string{"Alpha", "Progress"}) {
		t.Errorf("headings = %v, want one Progress section", got)
	}
	blocks := progressBlocks(t, next.Sources.Doc)["Progress"]
	if len(blocks) != 1 || blocks[0].Percent != 60 || blocks[0].Note != "12 of 20" {
		t.Fatalf("progress blocks = %+v", blocks)
	}
	if !strings.Contains(
		next.Page.Content,
		"<wk-progress-label>Index: 12 of 20</wk-progress-label>",
	) {
		t.Error("content was not re-rendered with the note")
	}
}

func TestProgressFindsItsLabelAnywhereOrWhereNamed(t *testing.T) {
	cur := page(t, "Alpha", "Build")
	next, res, err := report(t, cur, docedit.ProgressRequest{
		Label: "Compile", Percent: pct(10), Section: "build",
	})
	if err != nil || !res.Inserted || res.Section != 1 {
		t.Fatalf("report into a named section = %+v, %v", res, err)
	}
	next, res, err = report(t, next, docedit.ProgressRequest{Label: "Compile", Percent: pct(90)})
	if err != nil || res.Inserted || res.Section != 1 {
		t.Fatalf("report found anywhere = %+v, %v; want the Build block updated", res, err)
	}
	if got := progressBlocks(t, next.Sources.Doc); len(got) != 1 || got["Build"][0].Percent != 90 {
		t.Errorf("progress blocks = %+v", got)
	}
	if _, _, err := report(t, next, docedit.ProgressRequest{
		Label: "Compile", Percent: pct(1), Section: "zeta",
	}); !errors.Is(err, docedit.ErrAnchorNotFound) {
		t.Errorf("unknown section: err = %v, want ErrAnchorNotFound", err)
	}
}

func TestProgressRefusesALabelTwoBlocksCarry(t *testing.T) {
	cur := page(t, "Alpha", "Beta")
	var err error
	for _, section := range []string{"alpha", "beta"} {
		if cur, _, err = report(t, cur, docedit.ProgressRequest{
			Label: "Tests", Percent: pct(5), Section: section,
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err = report(t, cur, docedit.ProgressRequest{Label: "Tests", Percent: pct(50)})
	var conflict *docedit.LabelConflictError
	if !errors.As(err, &conflict) || !slices.Equal(conflict.Sections, []int{0, 1}) {
		t.Fatalf("shared label: err = %v, want sections [0 1]", err)
	}
	if _, res, err := report(t, cur, docedit.ProgressRequest{
		Label: "Tests", Percent: pct(50), Section: "index:1",
	}); err != nil || res.Inserted || res.Section != 1 {
		t.Fatalf("named section resolves it = %+v, %v", res, err)
	}
}

// A stored label went through name normalization ("A - B" became "A minus
// B"), and a report with the label as first typed still finds it.
func TestProgressMatchesANormalizedLabel(t *testing.T) {
	next, _, err := report(
		t,
		page(t, "Alpha"),
		docedit.ProgressRequest{Label: "Build - docs", Percent: pct(1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	next, res, err := report(
		t,
		next,
		docedit.ProgressRequest{Label: "Build - docs", Percent: pct(2)},
	)
	if err != nil || res.Inserted {
		t.Fatalf("second report = %+v, %v; want the stored block updated", res, err)
	}
	if blocks := progressBlocks(t, next.Sources.Doc)["Progress"]; len(blocks) != 1 {
		t.Errorf("progress blocks = %+v, want one", blocks)
	}
}

func TestProgressInTheDeck(t *testing.T) {
	cur := page(t, "Alpha")
	next, _, err := report(t, cur, docedit.ProgressRequest{
		Label: "Slides", Percent: pct(40), Rendition: "deck",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(progressBlocks(t, next.Sources.Deck)["Progress"]) != 1 ||
		string(next.Sources.Doc) != string(cur.Sources.Doc) {
		t.Error("a deck report must land in the deck and leave the brief alone")
	}
	cur.Sources.Doc = nil
	if _, _, err := report(t, cur, docedit.ProgressRequest{Label: "x", Percent: pct(1)}); !errors.Is(
		err,
		docedit.ErrNoDoc,
	) {
		t.Errorf("brief without a Doc: err = %v, want ErrNoDoc", err)
	}
}

func TestParseProgressRefusals(t *testing.T) {
	cases := map[string]docedit.ProgressRequest{
		"no label":          {Percent: pct(1)},
		"blank label":       {Label: "  ", Percent: pct(1)},
		"no pct":            {Label: "x"},
		"pct over 100":      {Label: "x", Percent: pct(101)},
		"negative pct":      {Label: "x", Percent: pct(-1)},
		"NaN pct":           {Label: "x", Percent: pct(math.NaN())},
		"unknown rendition": {Label: "x", Percent: pct(1), Rendition: "slides"},
	}
	for name, req := range cases {
		if _, err := docedit.ParseProgress(req); !errors.Is(err, docedit.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	for _, edge := range []float64{0, 100} {
		if _, err := docedit.ParseProgress(docedit.ProgressRequest{Label: "x", Percent: pct(edge)}); err != nil {
			t.Errorf("pct %v: %v", edge, err)
		}
	}
}
