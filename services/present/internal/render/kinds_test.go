package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChartKindsRender accepts every kind in ChartKinds and the empty kind,
// which the client draws as bar.
func TestChartKindsRender(t *testing.T) {
	for _, kind := range append(ChartKinds(), "") {
		b := Block{T: "chart", Kind: kind, Series: []ChartSeries{{Points: []ChartPoint{{X: "a", Y: 1}}}}}
		if kind == "sankey" {
			b = Block{T: "chart", Kind: kind, Flows: []ChartFlow{{From: "a", To: "b", Value: 1}}}
		}
		doc := Doc{Sections: []Section{{Heading: "S", Blocks: []Block{b}}}}
		if _, err := RenderDoc(doc, "T"); err != nil {
			t.Errorf("kind %q: %v", kind, err)
		}
	}
}

// TestChartKindsReachTheSkill pins the skill's kind lists to ChartKinds: the
// block table and the chart fields table each write the kinds as one
// backticked, comma-separated list in this order, so a kind added or removed
// in Go has to reach the skill too.
func TestChartKindsReachTheSkill(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "skills", "present", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "`" + strings.Join(ChartKinds(), "`, `") + "`"
	if n := strings.Count(string(raw), want); n < 2 {
		t.Errorf("SKILL.md lists the chart kinds as %s %d time(s), want at least 2", want, n)
	}
}
