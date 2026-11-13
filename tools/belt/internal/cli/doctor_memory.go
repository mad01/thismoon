package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// memoryProbe measures the agent-memory indexes the session-start hint
// injects, with hint.AgentMemory.Usage's error. Injected so tests never read
// the real stores under $HOME.
type memoryProbe func(cfg config.Config) ([]hint.MemoryUsage, error)

// liveMemoryProbe measures the stores at their real paths, reading the
// Claude settings sources cfg allows as a session started in doctor's own
// working directory would.
func liveMemoryProbe(cfg config.Config) ([]hint.MemoryUsage, error) {
	cwd, _ := os.Getwd()
	return hint.NewAgentMemory(cfg).Usage(cwd)
}

// printAgentMemory renders each store's size against its share of the
// budget. Stores near the budget together get a WARNING line and leave the
// exit status alone, since every fact still reaches the session. A store
// over its share also lands in the warnings section, named: its facts are
// being dropped. A store autoMemoryDirectory names gets one line saying
// which source names it, and whether Claude Code loads it itself. When belt
// cannot tell, one line says why. With the hint disabled nothing is
// injected, so no size can warn and no store is left out.
func printAgentMemory(w io.Writer, cfg config.Config, probe memoryProbe) []string {
	fmt.Fprintf(
		w,
		"\nagent memory, injected at session start (budget: %d bytes for the whole block):\n",
		hint.MemoryBudgetBytes,
	)
	enabled := cfg.HintEnabled(hint.NewAgentMemory(cfg).ID())
	usage, autoErr := probe(cfg)
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
		printAutoMemory(w, cfg, usage, autoErr)
	}
	return warns
}

// printAutoMemory prints the line for each store autoMemoryDirectory names,
// or, when err says belt cannot tell, one line with the error: no store is
// marked then, and a Claude Code session started here keeps every store.
func printAutoMemory(w io.Writer, cfg config.Config, usage []hint.MemoryUsage, err error) {
	if err != nil {
		fmt.Fprintf(w, "  cannot tell which store Claude Code loads itself: %v; "+
			"a Claude Code session started here keeps every store\n", err)
		return
	}
	starts := config.LoadClaudeHooks("SessionStart", cfg.ClaudeSettingsFiles...)
	for _, u := range usage {
		if u.AutoMemory.Directory != "" {
			fmt.Fprintf(w, "  %s\n", autoMemoryNote(u, starts))
		}
	}
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

// autoMemoryNote is the one line for a store autoMemoryDirectory names:
// which source names it, and what a Claude Code session started with the
// flag does with the store. Doctor has no agent of its own, so the line
// speaks of such a session. While auto memory may be off the session keeps
// the store, and the line says why. Otherwise the session skips it, and when a
// SessionStart entry in the Claude settings runs belt hint session-start
// without the flag, the line adds that the store still loads twice there.
func autoMemoryNote(u hint.MemoryUsage, starts []config.ClaudeHook) string {
	named := fmt.Sprintf(
		"autoMemoryDirectory in %s points at %s",
		u.AutoMemory.Source,
		filepath.Dir(u.Path),
	)
	if !u.LoadedByClaudeCode() {
		return fmt.Sprintf(
			"%s: %s, but Claude Code may load none of it: %s; a Claude Code session keeps the store",
			u.Label,
			named,
			u.AutoMemory.Off,
		)
	}
	note := fmt.Sprintf(
		"%s: a Claude Code session started with --agent %s skips it, "+
			"because %s and Claude Code loads that MEMORY.md itself",
		u.Label, hint.AgentClaudeCode, named,
	)
	for _, h := range starts {
		agent, runs := beltHint(h.Command, hint.EventSessionStart)
		if runs && agent != hint.AgentClaudeCode {
			return note + fmt.Sprintf(
				"; the SessionStart entry in %s runs belt hint session-start without --agent %s, "+
					"so sessions here still load it twice",
				h.File, hint.AgentClaudeCode,
			)
		}
	}
	return note
}
