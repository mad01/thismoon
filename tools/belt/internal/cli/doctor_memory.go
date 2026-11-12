package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// memoryProbe measures the agent-memory indexes the session-start hint
// injects. Injected so tests never read the real stores under $HOME.
type memoryProbe func(cfg config.Config) []hint.MemoryUsage

// liveMemoryProbe measures the stores at their real paths, reading the
// Claude settings files cfg allows.
func liveMemoryProbe(cfg config.Config) []hint.MemoryUsage {
	return hint.NewAgentMemory(cfg).Usage()
}

// printAgentMemory renders each store's size against its share of the
// budget. Stores near the budget together get a WARNING line and leave the
// exit status alone, since every fact still reaches the session. A store
// over its share also lands in the warnings section, named: its facts are
// being dropped. A store Claude Code loads itself gets one line saying so.
// With the hint disabled nothing is injected, so no size can warn and no
// store is left out.
func printAgentMemory(w io.Writer, cfg config.Config, probe memoryProbe) []string {
	fmt.Fprintf(
		w,
		"\nagent memory, injected at session start (budget: %d bytes for the whole block):\n",
		hint.MemoryBudgetBytes,
	)
	enabled := cfg.HintEnabled(hint.NewAgentMemory(cfg).ID())
	usage := probe(cfg)
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
	if enabled {
		starts := config.LoadClaudeHooks("SessionStart", cfg.ClaudeSettingsFiles...)
		for _, u := range usage {
			if u.AutoMemorySettings != "" {
				fmt.Fprintf(w, "  %s\n", autoMemoryNote(u, starts))
			}
		}
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

// autoMemoryNote is the one line for a store Claude Code loads itself: what
// a Claude Code session started with the flag skips, and why. Doctor has no
// agent of its own, so the line speaks of such a session. When a
// SessionStart entry in the Claude settings runs belt hint session-start
// without the flag, the line adds that the store still loads twice there.
func autoMemoryNote(u hint.MemoryUsage, starts []config.ClaudeHook) string {
	note := fmt.Sprintf(
		"%s: a Claude Code session started with --agent %s skips it, "+
			"because autoMemoryDirectory in %s points at %s and Claude Code loads that MEMORY.md itself",
		u.Label, hint.AgentClaudeCode, u.AutoMemorySettings, filepath.Dir(u.Path),
	)
	for _, h := range starts {
		if runs, agent := beltSessionStart(h.Command); runs && agent != hint.AgentClaudeCode {
			return note + fmt.Sprintf(
				"; the SessionStart entry in %s runs belt hint session-start without --agent %s, "+
					"so sessions here still load it twice",
				h.File, hint.AgentClaudeCode,
			)
		}
	}
	return note
}

// beltSessionStart reports whether a hook command runs `belt hint
// session-start`, matched like runsBeltHintRead, and the --agent value it
// passes, lowercased as the hint command reads it.
func beltSessionStart(command string) (runs bool, agent string) {
	words := strings.Fields(command)
	for i := 0; i+2 < len(words); i++ {
		if filepath.Base(words[i]) != "belt" || words[i+1] != "hint" ||
			words[i+2] != hint.EventSessionStart {
			continue
		}
		rest := words[i+3:]
		for j, word := range rest {
			if v, ok := strings.CutPrefix(word, "--agent="); ok {
				agent = v
			} else if word == "--agent" && j+1 < len(rest) {
				agent = rest[j+1]
			}
		}
		return true, strings.ToLower(agent)
	}
	return false, ""
}
