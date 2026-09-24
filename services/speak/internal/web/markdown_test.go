package web

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// sequentialKeys names parts p1, p2, ... in the order they are planned.
func sequentialKeys() func(string) string {
	n := 0
	return func(string) string {
		n++
		return fmt.Sprintf("p%d", n)
	}
}

func plan(t *testing.T, src string) Plan {
	t.Helper()
	got, err := PlanSections([]byte(src), sequentialKeys())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// partTexts flattens a plan's part texts in reading order.
func partTexts(p Plan) []string {
	var out []string
	for _, parts := range p.Sections {
		for _, part := range parts {
			out = append(out, part.Text)
		}
	}
	return out
}

func TestPlanSectionsSplitsAtH2(t *testing.T) {
	got := plan(t, "intro text\n\n## First\n\nbody one\n\n## Second\n\nbody two\n")
	for i := 1; i <= 3; i++ {
		want := fmt.Sprintf(`<section class="doc-section" data-section="%d" data-ra-parts="p%d">`,
			i, i)
		if !strings.Contains(got.HTML, want) {
			t.Errorf("missing %s (preamble + two h2 sections)\n%s", want, got.HTML)
		}
	}
	if len(got.Sections) != 3 {
		t.Errorf("planned %d sections, want 3", len(got.Sections))
	}
	if !strings.Contains(got.HTML, `<h2 data-ra-chunk="p2">First</h2>`) ||
		!strings.Contains(got.HTML, "body two") {
		t.Errorf("rendered content missing tagged headings/bodies:\n%s", got.HTML)
	}
}

func TestPlanSectionsKeepsH3WithinSection(t *testing.T) {
	got := plan(t, "## Top\n\n### Sub\n\ntext\n")
	if n := strings.Count(got.HTML, `class="doc-section"`); n != 1 {
		t.Errorf("section count = %d, want 1 (h3 must not split)\n%s", n, got.HTML)
	}
	if texts := partTexts(got); len(texts) != 1 || texts[0] != "Top. Sub. text." {
		t.Errorf("parts = %q, want one part reading every block", texts)
	}
}

func TestPlanSectionsWholeDocWithoutHeadings(t *testing.T) {
	got := plan(t, "just a paragraph\n")
	if n := strings.Count(got.HTML, `class="doc-section"`); n != 1 {
		t.Errorf("section count = %d, want 1\n%s", n, got.HTML)
	}
	if !strings.Contains(got.HTML, `<p data-ra-chunk="p1">just a paragraph</p>`) {
		t.Errorf("paragraph not tagged:\n%s", got.HTML)
	}
}

// TestPlanSectionsSkipsWhatIsNotRead pins what stays silent: code blocks,
// tables, raw HTML and images are rendered but carry no chunk and add no
// text, while inline code is read.
func TestPlanSectionsSkipsWhatIsNotRead(t *testing.T) {
	src := "## T\n\nRun `make test` now.\n\n```go\nfunc main() {}\n```\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n<div>raw</div>\n\n![a chart](chart.png)\n\n---\n"
	got := plan(t, src)
	for _, want := range []string{"<table>", "<pre><code", `<img src="chart.png"`} {
		if !strings.Contains(got.HTML, want) {
			t.Errorf("%s not rendered:\n%s", want, got.HTML)
		}
	}
	if n := strings.Count(got.HTML, "data-ra-chunk"); n != 2 {
		t.Errorf("%d tagged elements, want 2 (the heading and the paragraph)\n%s", n, got.HTML)
	}
	if texts := partTexts(got); len(texts) != 1 || texts[0] != "T. Run make test now." {
		t.Errorf("parts = %q, want only the heading and the paragraph with its inline code",
			texts)
	}
}

// TestPlanSectionsNestedLists pins that a list item reads its own text and
// a nested list's items are blocks of their own, tagged on their own li.
func TestPlanSectionsNestedLists(t *testing.T) {
	got := plan(t, "- outer item\n  - inner item\n- second item\n")
	liTags := regexp.MustCompile(`<li data-ra-chunk="[^"]+">`).FindAllString(got.HTML, -1)
	if len(liTags) != 3 {
		t.Errorf("%d tagged li, want 3\n%s", len(liTags), got.HTML)
	}
	if texts := partTexts(got); len(texts) != 1 ||
		texts[0] != "outer item. inner item. second item." {
		t.Errorf("parts = %q, want each item read once, in order", texts)
	}
}

func TestPlanSectionsLooseListAndQuote(t *testing.T) {
	got := plan(t, "- first para\n\n  second para\n\n> quoted words\n")
	if !regexp.MustCompile(`<li data-ra-chunk="p1">\s*<p>first para</p>`).
		MatchString(got.HTML) {
		t.Errorf("loose list item not tagged on the li:\n%s", got.HTML)
	}
	if !strings.Contains(got.HTML, `<p data-ra-chunk="p1">quoted words</p>`) {
		t.Errorf("paragraph inside the blockquote not tagged:\n%s", got.HTML)
	}
}

// TestPlanSectionsLongParagraphSpansParts pins the ramp: a first part of
// at most 250 characters, then up to 600, and a paragraph longer than a
// part splits at sentences and carries every key that reads it.
func TestPlanSectionsLongParagraphSpansParts(t *testing.T) {
	sentence := "This sentence is here to make the paragraph long enough to split. "
	got := plan(t, "## Long\n\n"+strings.Repeat(sentence, 20)+"\n")
	parts := got.Sections[0]
	if len(parts) < 3 {
		t.Fatalf("planned %d parts, want the paragraph spread over several", len(parts))
	}
	for i, p := range parts {
		limit := 600
		if i == 0 {
			limit = 250
		}
		if len(p.Text) > limit {
			t.Errorf("part %d is %d chars, over its bound %d", i+1, len(p.Text), limit)
		}
	}
	tag := regexp.MustCompile(`<p data-ra-chunk="([^"]+)">`).FindStringSubmatch(got.HTML)
	if tag == nil {
		t.Fatalf("long paragraph not tagged:\n%s", got.HTML)
	}
	// p1 holds the heading and the paragraph's first sentences.
	if keys := strings.Fields(tag[1]); len(keys) != len(parts) || keys[0] != "p1" {
		t.Errorf("paragraph keys = %v, want all %d parts in order", keys, len(parts))
	}
}

// TestPlanSectionsListsPartsInReadingOrder pins data-ra-parts: the order the
// section is read in, which element order does not give when a list item's
// paragraphs surround a nested list, and with a repeated part kept.
func TestPlanSectionsListsPartsInReadingOrder(t *testing.T) {
	// 300-450 characters: over the first part's 250, two together over 600,
	// so each block is a part of its own.
	long := func(word string) string { return strings.Repeat(word+" ", 60) }
	got := plan(t, "- "+long("first")+"\n\n  - "+long("nested")+"\n\n  "+long("last")+"\n")
	if !strings.Contains(got.HTML, `<section class="doc-section" data-section="1" `+
		`data-ra-parts="p1 p2 p3">`) {
		t.Errorf("section does not list its parts in reading order:\n%s", got.HTML)
	}
	outer := regexp.MustCompile(`<li data-ra-chunk="([^"]+)">\s*<p>first`).
		FindStringSubmatch(got.HTML)
	nested := regexp.MustCompile(`<li data-ra-chunk="([^"]+)">nested`).
		FindStringSubmatch(got.HTML)
	if outer == nil || outer[1] != "p1 p3" || nested == nil || nested[1] != "p2" {
		t.Errorf("li tags = %v / %v, want the outer li on p1 p3 and the nested one on p2\n%s",
			outer, nested, got.HTML)
	}

	byText := func(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:8] }
	twice, err := PlanSections([]byte(long("again")+"\n\n"+long("again")+"\n"), byText)
	if err != nil {
		t.Fatal(err)
	}
	key := twice.Sections[0][0].Key
	if !strings.Contains(twice.HTML, `data-ra-parts="`+key+" "+key+`"`) {
		t.Errorf("a repeated part is not listed twice:\n%s", twice.HTML)
	}
}

// TestPlanBlocksMatchesPlanSections pins that the two forms of POST /read
// agree: a block posted as text gets the keys the same block rendered from
// markdown does, part for part and block for block, with the same
// normalization (inline code is read as written, quotes are dropped).
func TestPlanBlocksMatchesPlanSections(t *testing.T) {
	long := strings.Repeat("This sentence is here to make the paragraph long enough to split. ", 20)
	blocks := []string{"Title", "Run `make test` and say \"done\".", "one", "two", long}
	markdown := "## Title\n\nRun `make test` and say \"done\".\n\n- one\n- two\n\n" + long + "\n"
	byText := func(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:8] }

	fromMarkdown, err := PlanSections([]byte(markdown), byText)
	if err != nil {
		t.Fatal(err)
	}
	fromBlocks := planBlocks(blocks, byText)
	if got, want := keysOf(fromBlocks.parts), keysOf(fromMarkdown.Sections[0]); !slices.Equal(
		got,
		want,
	) {
		t.Errorf("parts from blocks = %v, from markdown = %v", got, want)
	}
	if len(fromBlocks.parts) < 3 {
		t.Errorf("planned %d parts, want the long block spread over several", len(fromBlocks.parts))
	}
	tags := regexp.MustCompile(`data-ra-chunk="([^"]+)"`).
		FindAllStringSubmatch(fromMarkdown.HTML, -1)
	if len(tags) != len(blocks) {
		t.Fatalf("%d tagged elements, want one per block (%d)\n%s", len(tags), len(blocks),
			fromMarkdown.HTML)
	}
	for j, tag := range tags {
		if got, want := fromBlocks.blockKeys[j], strings.Fields(tag[1]); !slices.Equal(got, want) {
			t.Errorf("block %d keys from blocks = %v, from markdown = %v", j, got, want)
		}
	}
}

// TestPlanBlocksEmptyBlocksKeepTheirPlace pins the alignment the JSON form
// relies on: a block with nothing speakable still has an entry, an empty
// one, and a section with no blocks plans no parts.
func TestPlanBlocksEmptyBlocksKeepTheirPlace(t *testing.T) {
	got := planBlocks([]string{`""`, "Spoken.", "   "}, sequentialKeys())
	if len(got.parts) != 1 || got.parts[0].Text != "Spoken." {
		t.Errorf("parts = %+v, want the one spoken block", got.parts)
	}
	if len(got.blockKeys) != 3 || got.blockKeys[0] == nil || len(got.blockKeys[0]) != 0 ||
		!slices.Equal(got.blockKeys[1], []string{"p1"}) || len(got.blockKeys[2]) != 0 {
		t.Errorf("block keys = %v, want [] p1 [] with the empty ones non-nil", got.blockKeys)
	}
	if none := planBlocks(nil, sequentialKeys()); len(none.parts) != 0 || len(none.blockKeys) != 0 {
		t.Errorf("no blocks planned %+v, want nothing", none)
	}
}

func TestPlanSectionsOmitsPartsWhenNothingIsRead(t *testing.T) {
	got := plan(t, "```\ncode only\n```\n")
	if strings.Contains(got.HTML, "data-ra-parts") || len(got.Sections[0]) != 0 {
		t.Errorf("a section with nothing to read lists parts:\n%s", got.HTML)
	}
}
