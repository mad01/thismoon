package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillDir is the present skill's directory, from this package.
var skillDir = filepath.Join("..", "..", "..", "..", "skills", "present")

// readSkillFile reads a file of the skill by its path inside the skill
// directory.
func readSkillFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(skillDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// backtickList writes a vocabulary the way the skill does: one backticked,
// comma-separated list in the renderer's order.
func backtickList(words []string) string {
	return "`" + strings.Join(words, "`, `") + "`"
}

// TestChartKindsRender accepts every kind in ChartKinds and the empty kind,
// which the client draws as bar.
func TestChartKindsRender(t *testing.T) {
	for _, kind := range append(ChartKinds(), "") {
		b := Block{T: "chart", Kind: kind, Series: []ChartSeries{{Points: []ChartPoint{{X: "a", Y: 1}}}}}
		if kind == "sankey" {
			b = Block{T: "chart", Kind: kind, Flows: []ChartFlow{{From: "a", To: "b", Value: 1}}}
		}
		if kind == "ribbon" {
			b.Series[0].Name = "a"
		}
		doc := Doc{Sections: []Section{{Heading: "S", Blocks: []Block{b}}}}
		if _, err := RenderDoc(doc, "T"); err != nil {
			t.Errorf("kind %q: %v", kind, err)
		}
	}
}

// TestChartKindsReachTheSkill pins the skill's kind lists to ChartKinds: the
// block table and the chart fields table in SKILL.md, and the opening of
// references/charts.md, each write the kinds as one backticked,
// comma-separated list in this order, so a kind added or removed in Go has
// to reach the skill too.
func TestChartKindsReachTheSkill(t *testing.T) {
	want := backtickList(ChartKinds())
	for name, least := range map[string]int{"SKILL.md": 2, "references/charts.md": 1} {
		if n := strings.Count(readSkillFile(t, name), want); n < least {
			t.Errorf("%s lists the chart kinds as %s %d time(s), want at least %d", name, want, n, least)
		}
	}
}

// TestDiagramVocabularyReachesTheSkill pins the skill's diagram kind and
// direction lists to DiagramKinds and DiagramDirections, written as one
// backticked, comma-separated list each in SKILL.md's block table and in
// the fields of references/diagrams.md.
func TestDiagramVocabularyReachesTheSkill(t *testing.T) {
	for _, name := range []string{"SKILL.md", "references/diagrams.md"} {
		raw := readSkillFile(t, name)
		for _, list := range [][]string{DiagramKinds(), DiagramDirections()} {
			if want := backtickList(list); !strings.Contains(raw, want) {
				t.Errorf("%s does not list %s", name, want)
			}
		}
	}
}

// TestDiagramKindsRender accepts every node kind and the empty kind.
func TestDiagramKindsRender(t *testing.T) {
	for _, kind := range append(DiagramKinds(), "") {
		doc := Doc{Sections: []Section{{Heading: "S", Blocks: []Block{{T: "diagram", Nodes: []DiagramNode{{ID: "a", Label: "A", Kind: kind}}}}}}}
		if _, err := RenderDoc(doc, "T"); err != nil {
			t.Errorf("kind %q: %v", kind, err)
		}
	}
}
