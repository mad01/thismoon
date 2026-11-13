package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

func usageProbe(u ...hint.MemoryUsage) memoryProbe {
	return func(config.Config) []hint.MemoryUsage { return u }
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

// TestDoctorNamesAStoreClaudeCodeLoadsItself: a store autoMemoryDirectory
// points at gets one line saying a Claude Code session with the flag skips
// it, and why. When the SessionStart entry runs belt hint session-start
// without the flag, the line says the store still loads twice.
func TestDoctorNamesAStoreClaudeCodeLoadsItself(t *testing.T) {
	work := hint.MemoryUsage{
		Label: "work agent memory", Path: "/w/MEMORY.md", Present: true,
		Facts: 8, Kept: 8, Bytes: 1500, Share: 2250,
		AutoMemorySettings: "/h/.claude/settings.json",
	}
	off := false
	tests := []struct {
		name      string
		command   string // the SessionStart hook command; "" writes no entry
		disabled  bool
		wantLine  bool
		wantTwice bool
	}{
		{"no SessionStart entry", "", false, true, false},
		{
			"entry with the flag",
			"~/code/bin/belt hint session-start --agent claude-code",
			false,
			true,
			false,
		},
		{
			"entry with the flag as one word",
			"belt hint session-start --agent=claude-code",
			false,
			true,
			false,
		},
		{
			"entry with the flag before the event",
			"~/code/bin/belt hint --agent claude-code session-start",
			false,
			true,
			false,
		},
		{
			"entry with a global flag before hint",
			"belt --config /x/config.yaml hint session-start --agent claude-code",
			false,
			true,
			false,
		},
		{"entry without the flag", "~/code/bin/belt hint session-start", false, true, true},
		{
			"full path without the flag",
			"/Users/me/code/bin/belt --config /x/config.yaml hint session-start",
			false,
			true,
			true,
		},
		{
			"entry with a quoted flag value",
			`belt hint session-start --agent=\"claude-code\"`,
			false,
			true,
			false,
		},
		{"entry for another agent", "belt hint session-start --agent codex", false, true, true},
		{
			"session-start after a read hint",
			"belt hint read && belt hint session-start",
			false,
			true,
			true,
		},
		{"another belt command only", "belt watch", false, true, false},
		{"hint disabled", "belt hint session-start", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := `{}`
			if tt.command != "" {
				settings = `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "` +
					tt.command + `"}]}]}}`
			}
			cfg := config.Config{
				ClaudeSettingsFiles: []string{writeFile(t, t.TempDir(), "settings.json", settings)},
			}
			if tt.disabled {
				cfg.Hints = map[string]config.Toggle{"agent-memory": {Enabled: &off}}
			}
			var b strings.Builder
			if warns := printAgentMemory(&b, cfg, usageProbe(work)); len(warns) != 0 {
				t.Errorf("warnings = %q, want none: a skipped store is no fault", warns)
			}
			line := "work agent memory: a Claude Code session started with --agent claude-code skips it, " +
				"because autoMemoryDirectory in /h/.claude/settings.json points at /w"
			if got := strings.Contains(b.String(), line); got != tt.wantLine {
				t.Errorf("section %q has the skip line: %v, want %v", b.String(), got, tt.wantLine)
			}
			twice := "runs belt hint session-start without --agent claude-code, so sessions here still load it twice"
			if got := strings.Contains(b.String(), twice); got != tt.wantTwice {
				t.Errorf(
					"section %q says it loads twice: %v, want %v",
					b.String(),
					got,
					tt.wantTwice,
				)
			}
		})
	}
}
