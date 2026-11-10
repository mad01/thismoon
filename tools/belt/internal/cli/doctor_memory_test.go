package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

func usageProbe(u ...hint.MemoryUsage) memoryProbe {
	return func() []hint.MemoryUsage { return u }
}

func TestDoctorReportsAgentMemoryBudget(t *testing.T) {
	budget := hint.MemoryBudgetBytes
	off := false
	disabled := config.Config{Hints: map[string]config.Toggle{"agent-memory": {Enabled: &off}}}
	fits := hint.MemoryUsage{
		Label: "shared agent memory", Path: "/m/MEMORY.md", Present: true,
		Facts: 35, Kept: 35, Bytes: budget / 2, Share: budget - 20,
	}
	near := fits
	near.Bytes = budget * 9 / 10
	over := fits
	over.Facts, over.Kept, over.Bytes = 70, 60, budget+500
	absent := hint.MemoryUsage{Label: "work agent memory", Path: "/w/MEMORY.md"}

	tests := []struct {
		name     string
		cfg      config.Config
		usage    hint.MemoryUsage
		wantLine string
		wantWarn bool
	}{
		{"fits", config.Config{}, fits, "35 facts", false},
		{"near the budget", config.Config{}, near, "WARNING: near the budget", false},
		{"over the budget", config.Config{}, over, "OVER ITS SHARE (10 facts dropped)", true},
		{"over with the hint disabled", disabled, over, "70 facts", false},
		{"absent store", config.Config{}, absent, "(absent)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			warns := printAgentMemory(&b, tt.cfg, usageProbe(tt.usage))
			if !strings.Contains(b.String(), tt.wantLine) {
				t.Errorf("section %q is missing %q", b.String(), tt.wantLine)
			}
			if got := len(warns) > 0; got != tt.wantWarn {
				t.Errorf("warnings = %q, want any: %v", warns, tt.wantWarn)
			}
			if tt.wantWarn && !strings.Contains(warns[0], "only 60 of its 70 facts") {
				t.Errorf("warning %q does not say how many facts reach a session", warns[0])
			}
		})
	}
}

// TestDoctorWarnsWhenStoresNearTheBudgetTogether: the stores share one
// budget, so two stores that each look small can still crowd it.
func TestDoctorWarnsWhenStoresNearTheBudgetTogether(t *testing.T) {
	half := hint.MemoryUsage{
		Label: "shared agent memory", Path: "/m/MEMORY.md", Present: true,
		Facts: 20, Kept: 20, Bytes: hint.MemoryBudgetBytes * 45 / 100,
	}
	work := half
	work.Label, work.Path = "work agent memory", "/w/MEMORY.md"
	var b strings.Builder
	if warns := printAgentMemory(&b, config.Config{}, usageProbe(half, work)); len(warns) != 0 {
		t.Errorf("warnings = %q, want none while every fact fits", warns)
	}
	if !strings.Contains(b.String(), "WARNING: near the budget") {
		t.Errorf("section %q does not warn about two stores at 90%% together", b.String())
	}
}

// TestDoctorExitsOnAnOverBudgetMemoryIndex: an index past its budget drops
// facts, so it joins the warnings section that makes doctor exit 1.
func TestDoctorExitsOnAnOverBudgetMemoryIndex(t *testing.T) {
	p := armedConfig(t, t.TempDir())
	probes := stubProbes(nil)
	probes.memory = usageProbe(hint.MemoryUsage{
		Label: "shared agent memory", Path: "/m/MEMORY.md", Present: true,
		Facts: 2, Kept: 1, Bytes: hint.MemoryBudgetBytes + 1,
	})
	var b strings.Builder
	warns := runDoctor(&b, p, probes)
	if len(warns) != 1 || !strings.HasPrefix(warns[0], "agent-memory:") {
		t.Errorf("warnings = %q, want the one agent-memory budget warning", warns)
	}
}

// TestDoctorNamesTheStoreOverItsShare: with both stores present, the
// warning names the store whose facts are dropped, not the other one.
func TestDoctorNamesTheStoreOverItsShare(t *testing.T) {
	personal := hint.MemoryUsage{
		Label: "shared agent memory", Path: "/m/MEMORY.md", Present: true,
		Facts: 60, Kept: 40, Bytes: 9000, Share: 6730,
	}
	work := hint.MemoryUsage{
		Label: "work agent memory", Path: "/w/MEMORY.md", Present: true,
		Facts: 8, Kept: 8, Bytes: 1500, Share: 2250,
	}
	var b strings.Builder
	warns := printAgentMemory(&b, config.Config{}, usageProbe(personal, work))
	if len(warns) != 1 || !strings.Contains(warns[0], "shared agent memory index (/m/MEMORY.md)") ||
		!strings.Contains(warns[0], "6730-byte share") {
		t.Errorf("warnings = %q, want one naming the shared store and its share", warns)
	}
	if !strings.Contains(b.String(), "8 facts, 1500 bytes of a 2250-byte share") {
		t.Errorf("section %q does not show the work store against its share", b.String())
	}
}
