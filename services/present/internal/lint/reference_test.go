package lint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/limits"
)

// skillReferences is the skill's references directory, from this package.
const skillReferences = "../../../../skills/present/references"

// updateSkill is the switch that rewrites the generated skill files
// instead of comparing them: PRESENT_UPDATE_SKILL=1, which `make
// limits-doc` sets.
const updateSkill = "PRESENT_UPDATE_SKILL"

// pairFiles is the reference file each rule's fragment pair lives in.
// placeholder.left has none: a skeleton is the only place its marker comes
// from, and the skeletons say so themselves.
var pairFiles = map[string][]limits.ID{
	"page-types.md": {
		limits.SummaryWords, limits.HeadingLabel, limits.BriefFigureGap,
		limits.BriefParagraph, limits.TableRows, limits.TableCell,
	},
	"diagrams.md": {
		limits.DiagramBoxes, limits.DiagramChain, limits.DiagramRow, limits.DiagramKeyUnused,
	},
	"decks.md": {
		limits.SlideWords, limits.DeckTextOnly, limits.DiagramSlideBoxes,
		limits.DiagramSlidePath, limits.DiagramSlideColumns,
	},
	"blocks.md": {limits.InlineStrayMark},
}

// TestSkillFilesAreCurrent keeps the generated parts of the skill equal to
// their sources: references/limits.md to Reference(), and each fragment
// pair in the other reference files to its fixtures and the lint's own
// message for the broken one. PRESENT_UPDATE_SKILL=1 rewrites them. A
// reference file that does not exist yet is skipped, so the lint can land
// before the files that carry the pairs; one that exists must hold a pair
// block for each of its rules.
func TestSkillFilesAreCurrent(t *testing.T) {
	update := os.Getenv(updateSkill) == "1"
	path := filepath.Join(skillReferences, "limits.md")
	want := Reference()
	if update {
		if err := os.MkdirAll(skillReferences, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run make limits-doc in services/present", err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; run make limits-doc in services/present", path)
	}
	for name, rules := range pairFiles {
		checkPairs(t, filepath.Join(skillReferences, name), rules, update)
	}
}

// TestPairFilesNameEveryRule: every warning but placeholder.left has a
// pair, in one file.
func TestPairFilesNameEveryRule(t *testing.T) {
	seen := map[limits.ID]string{}
	for name, rules := range pairFiles {
		for _, id := range rules {
			if prev, ok := seen[id]; ok {
				t.Errorf("%s has a pair in %s and in %s", id, prev, name)
			}
			seen[id] = name
		}
	}
	for _, l := range limits.Warnings() {
		if _, ok := seen[l.ID]; !ok && l.ID != limits.PlaceholderLeft {
			t.Errorf("%s: no file holds its pair", l.ID)
		}
	}
}

func checkPairs(t *testing.T, path string, rules []limits.ID, update bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Logf("%s does not exist yet; its pairs are skipped", path)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, id := range rules {
		next, err := replacePair(doc, id, pairBlock(t, id))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if next != doc && !update {
			t.Errorf("%s: the %s pair is stale; run make limits-doc in services/present", path, id)
		}
		doc = next
	}
	if update && doc != string(data) {
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// replacePair swaps the body between <!-- pair:<id> --> and the next
// <!-- pair:end --> in doc for body.
func replacePair(doc string, id limits.ID, body string) (string, error) {
	open := fmt.Sprintf("<!-- pair:%s -->\n", id)
	const end = "<!-- pair:end -->"
	start := strings.Index(doc, open)
	if start < 0 {
		return "", fmt.Errorf("no %q marker for the %s pair", strings.TrimSpace(open), id)
	}
	from := start + len(open)
	stop := strings.Index(doc[from:], end)
	if stop < 0 {
		return "", fmt.Errorf("the %s pair has no %q", id, end)
	}
	return doc[:from] + body + doc[from+stop:], nil
}

// pairBlock renders a rule's pair: the broken fixture, the warning the
// lint gives for it, and the fixed fixture.
func pairBlock(t *testing.T, id limits.ID) string {
	t.Helper()
	broken, fixed := fixture(t, "broken", id), fixture(t, "fixed", id)
	findings := lintArgs(t, broken)
	if len(findings) != 1 {
		t.Fatalf("%s: broken fixture raises %d findings, want 1", id, len(findings))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This trips `%s`:\n\n```json\n%s```\n\n", id, broken)
	fmt.Fprintf(&b, "The lint says:\n\n> `%s` %s\n\n", findings[0].Where, findings[0].Message)
	fmt.Fprintf(&b, "Repaired:\n\n```json\n%s```\n", fixed)
	return b.String()
}

// TestPairBlocksRewriteInPlace runs the pair writer over a scratch file the
// way it will run over the skill's reference files once they exist: the
// body between the markers is replaced, the rest is kept, a second run
// changes nothing, and a missing marker is an error.
func TestPairBlocksRewriteInPlace(t *testing.T) {
	doc := "# Decks\n\nbefore\n<!-- pair:slide.words -->\nold body\n<!-- pair:end -->\nafter\n"
	body := pairBlock(t, limits.SlideWords)
	next, err := replacePair(doc, limits.SlideWords, body)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Decks\n\nbefore\n<!-- pair:slide.words -->\n" + body + "<!-- pair:end -->\nafter\n"
	if next != want {
		t.Fatalf("rewritten doc:\n%s\nwant:\n%s", next, want)
	}
	again, err := replacePair(next, limits.SlideWords, body)
	if err != nil || again != next {
		t.Errorf("a second run changed the file: %v", err)
	}
	if !strings.Contains(body, "deck.sections[0]") || !strings.Contains(body, "Repaired:") {
		t.Errorf("pair body lacks the warning or the fix:\n%s", body)
	}
	if _, err := replacePair("no markers", limits.SlideWords, body); err == nil {
		t.Error("a file without the marker must be an error")
	}
	if _, err := replacePair("<!-- pair:slide.words -->\nx", limits.SlideWords, body); err == nil {
		t.Error("a pair without its end marker must be an error")
	}
}
