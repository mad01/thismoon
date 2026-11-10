package cli

import (
	"fmt"
	"io"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// memoryProbe measures the agent-memory indexes the session-start hint
// injects. Injected so tests never read the real stores under $HOME.
type memoryProbe func() []hint.MemoryUsage

// liveMemoryProbe measures the stores at their real paths.
func liveMemoryProbe() []hint.MemoryUsage {
	return hint.NewAgentMemory(config.Config{}).Usage()
}

// printAgentMemory renders each store's size against its share of the
// budget. Stores near the budget together get a WARNING line and leave the
// exit status alone, since every fact still reaches the session. A store
// over its share also lands in the warnings section, named: its facts are
// being dropped. With the hint disabled nothing is injected, so no size can
// warn.
func printAgentMemory(w io.Writer, cfg config.Config, probe memoryProbe) []string {
	fmt.Fprintf(
		w,
		"\nagent memory, injected at session start (budget: %d bytes for the whole block):\n",
		hint.MemoryBudgetBytes,
	)
	enabled := cfg.HintEnabled(hint.NewAgentMemory(cfg).ID())
	usage := probe()
	var warns []string
	total := 0
	for _, u := range usage {
		total += u.Bytes
		fmt.Fprintf(
			w,
			"  %-20s %s  %s%s\n",
			u.Label,
			u.Path,
			memorySizeNote(u),
			overNote(u, enabled),
		)
		if enabled && u.Over() {
			warns = append(warns, fmt.Sprintf(
				"agent-memory: the %s index (%s) is over its %d-byte share of the %d-byte budget, "+
					"so only %d of its %d facts reach a session; prune or consolidate that MEMORY.md",
				u.Label, u.Path, u.Share, hint.MemoryBudgetBytes, u.Kept, u.Facts))
		}
	}
	fmt.Fprintf(w, "  total %d of %d bytes (%d%%)\n", total, hint.MemoryBudgetBytes,
		total*100/hint.MemoryBudgetBytes)
	if enabled && hint.MemoryNear(usage) {
		fmt.Fprintln(
			w,
			"  WARNING: near the budget; prune or consolidate MEMORY.md before facts drop",
		)
	}
	return warns
}

func memorySizeNote(u hint.MemoryUsage) string {
	if !u.Present {
		return "(absent)"
	}
	return fmt.Sprintf("%d facts, %d bytes of a %d-byte share", u.Facts, u.Bytes, u.Share)
}

func overNote(u hint.MemoryUsage, enabled bool) string {
	if !enabled || !u.Over() {
		return ""
	}
	return fmt.Sprintf("  OVER ITS SHARE (%d facts dropped)", u.Facts-u.Kept)
}
