package render

import (
	"testing"

	"github.com/mad01/thismoon/services/present/internal/limits"
)

// TestCapsMatchLimits holds every cap the renderer refuses a Doc by to the
// number its limits table entry carries, so the lint, the skill reference
// and the MCP schema text name the number the renderer enforces.
func TestCapsMatchLimits(t *testing.T) {
	caps := []struct {
		id  limits.ID
		cap int
	}{
		{limits.RibbonSeries, ribbonMaxSeries},
		{limits.DiagramNodes, diagramMaxNodes},
		{limits.DiagramGroups, diagramMaxGroups},
		{limits.DiagramDepth, diagramMaxDepth},
		{limits.DiagramLabel, diagramMaxLabel},
		{limits.DiagramText, diagramMaxText},
		{limits.DiagramEdgeLabel, diagramMaxEdgeLabel},
		{limits.TerminalRowsMin, TerminalMinRows},
		{limits.TerminalRowsMax, TerminalMaxRows},
		{limits.TerminalSession, TerminalSessionMaxLen},
		{limits.TerminalPrefill, TerminalPrefillMaxLen},
	}
	for _, c := range caps {
		if got := limits.Number(c.id); got != c.cap {
			t.Errorf("%s: the limits table says %d, the renderer refuses past %d", c.id, got, c.cap)
		}
	}
}
