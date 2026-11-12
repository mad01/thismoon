package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// MemoryBudgetBytes bounds the whole block this hint injects, as the session
// reads it: the belt[agent-memory] prefix, each store's header line, its fact
// lines, and any truncation note. A byte budget, not a fact count: a short
// fact and a long one cost the context differently. The ceiling it aims
// under is Claude Code's: it hands the model at most 10,000 characters of one
// hook output string and replaces anything longer with a 2 KB preview and a
// file path. A byte is never fewer characters than the cap counts, so the
// budget keeps the memory block alone under that cap. It does not bound the
// session-start output as a whole: the hook renders every session-start
// advice into one string (Render), so other hints that fire with this one
// (moondb-inflight, custom hints) share the cap, and when they are large the
// combined output can still pass it. Past the budget the problem is store
// hygiene (docs/adr/0008).
const MemoryBudgetBytes = 9000

// workMemoryShare is the part of MemoryBudgetBytes held for the work store
// when one has facts. The personal store renders first, and without a
// reserve a personal index near the budget would leave the work index a few
// hundred bytes. A quarter fits a work index of about a dozen facts. A store
// that needs less than its share hands the rest to the other one.
const workMemoryShare = MemoryBudgetBytes / 4

// memoryNearPercent is the share of the budget at which belt doctor starts
// warning, so a store gets pruned before its facts stop fitting.
const memoryNearPercent = 80

// AgentClaudeCode is the --agent value that names Claude Code, the one
// agent that loads a memory index on its own (autoMemoryDirectory). Codex
// and pi have no auto memory, so any other value, and none, keeps every
// store.
const AgentClaudeCode = "claude-code"

// AgentMemory injects the shared agent memory index when a session starts.
// The instruction files say "read ~/.config/agent-memory/MEMORY.md at session
// start", and instruction-only prose does not trigger reliably. This hint has
// no per-session dedupe: the index is the thing to see every session, not
// advice to show once. Its advice skips the events service: the injection
// fires on every session and says nothing about what the session did.
//
// Two stores exist: the personal one at ~/.config/agent-memory (cloned on
// every machine) and a work one at ~/.config/agent-memory-work, which only
// work-profile machines clone at all. Both are injected when present; absence
// of either is silence, so personal machines never see or need the work store.
//
// A session started with --agent claude-code leaves out a store whose
// directory Claude Code's autoMemoryDirectory setting names: Claude Code
// loads that MEMORY.md itself, so injecting it too would load it twice. The
// store left out takes no share of the budget. Any other agent, and a
// session started without the flag, keeps both stores.
type AgentMemory struct {
	cfg config.Config
	// path locates the personal index; overridable for tests.
	path func() string
	// workPath locates the work index; overridable for tests.
	workPath func() string
}

func NewAgentMemory(cfg config.Config) *AgentMemory {
	return &AgentMemory{cfg: cfg, path: memoryIndexPath, workPath: workMemoryIndexPath}
}

func (h *AgentMemory) ID() string    { return "agent-memory" }
func (h *AgentMemory) Event() string { return EventSessionStart }

func memoryIndexPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agent-memory", "MEMORY.md")
}

func workMemoryIndexPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agent-memory-work", "MEMORY.md")
}

// MemoryUsage is one store's index measured against its share of
// MemoryBudgetBytes.
type MemoryUsage struct {
	Label string
	Path  string
	// Present is false when the index file is missing or unreadable: that
	// store does not exist on this machine.
	Present bool
	Facts   int // fact lines in the index
	Kept    int // fact lines that fit the store's share, in index order
	// Bytes is what the store's block would cost with every fact in it:
	// header line and fact lines. Zero for a store with no facts.
	Bytes int
	// Share is the part of the budget this store may fill, after the
	// prefix and the other store's share.
	Share int
	// AutoMemorySettings names the Claude settings file whose
	// autoMemoryDirectory is this store's directory, so Claude Code loads
	// the index itself. Empty when no setting names it. A session started
	// with --agent claude-code leaves the store out.
	AutoMemorySettings string
	facts              []string
	// hidden is set when the share cannot hold even the header and the
	// truncation note: the store then renders nothing.
	hidden bool
}

// Over reports whether facts were dropped to fit the store's share.
func (u MemoryUsage) Over() bool { return u.Kept < u.Facts }

// MemoryNear reports whether every fact still fits but the stores together
// have used memoryNearPercent of the budget or more.
func MemoryNear(usage []MemoryUsage) bool {
	total := 0
	for _, u := range usage {
		if u.Over() {
			return false
		}
		total += u.Bytes
	}
	return total*100 >= MemoryBudgetBytes*memoryNearPercent
}

// Usage measures each store's index, personal first, and fits each into its
// share of the budget, as a session that keeps both stores sees it. It also
// marks a store Claude Code loads itself (AutoMemorySettings). belt doctor
// reports it.
func (h *AgentMemory) Usage() []MemoryUsage {
	usage := h.readStores()
	h.markAutoMemory(usage)
	h.fitStores(usage)
	return usage
}

func (h *AgentMemory) readStores() []MemoryUsage {
	return []MemoryUsage{
		readStore(h.path(), "shared agent memory"),
		readStore(h.workPath(), "work agent memory"),
	}
}

// fitStores splits the budget between the stores and fits each into its
// share.
func (h *AgentMemory) fitStores(usage []MemoryUsage) {
	room := MemoryBudgetBytes - len(Render([]Advice{{Hint: h.ID()}}))
	personal, work := memoryShares(usage[0], usage[1], room)
	usage[0].fit(personal)
	usage[1].fit(work)
}

// markAutoMemory sets AutoMemorySettings on each present store whose
// directory is the one Claude Code's autoMemoryDirectory names. Both sides
// are compared after ~ expansion and symlink resolution, since a store is
// often a link into a synced folder. A missing key, a broken settings file,
// or a path that is relative or does not resolve marks nothing. In doubt the
// store stays in: an index loaded twice costs less than one never loaded.
func (h *AgentMemory) markAutoMemory(usage []MemoryUsage) {
	setting, file, err := config.LoadClaudeAutoMemoryDirectory(h.cfg.ClaudeSettingsFiles...)
	if err != nil || setting == "" {
		return
	}
	dir, ok := resolvedDir(setting)
	if !ok {
		return
	}
	for i := range usage {
		if !usage[i].Present {
			continue
		}
		if store, ok := resolvedDir(filepath.Dir(usage[i].Path)); ok && store == dir {
			usage[i].AutoMemorySettings = file
		}
	}
}

// resolvedDir expands a leading ~ and resolves symlinks, so two spellings of
// one directory compare equal. A relative path, or one that does not
// resolve, is not ok.
func resolvedDir(path string) (string, bool) {
	expanded := config.ExpandHome(path)
	if !filepath.IsAbs(expanded) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(expanded)
	if err != nil {
		return "", false
	}
	return resolved, true
}

// leaveOut empties a store Claude Code loads itself. Like an absent store,
// it renders nothing and takes no share of the budget.
func (u *MemoryUsage) leaveOut() {
	u.facts, u.Facts, u.Kept, u.Bytes = nil, 0, 0, 0
}

// memoryShares splits room between the stores. A store with no facts takes
// nothing. With both, the work store holds workMemoryShare and the personal
// store the rest, and a store that needs less than that gives the remainder
// to the other.
func memoryShares(personal, work MemoryUsage, room int) (int, int) {
	switch {
	case work.Facts == 0:
		return room, 0
	case personal.Facts == 0:
		return 0, room
	}
	share := workMemoryShare
	switch {
	case personal.Bytes < room-share:
		share = room - personal.Bytes
	case work.Bytes < share:
		share = work.Bytes
	}
	return room - share, share
}

// Check renders both stores. Only a session started with --agent
// claude-code reads the Claude settings, and it leaves out a store Claude
// Code loads itself before the budget is split.
func (h *AgentMemory) Check(in Input) *Advice {
	usage := h.readStores()
	if in.Agent == AgentClaudeCode {
		h.markAutoMemory(usage)
		for i := range usage {
			if usage[i].AutoMemorySettings != "" {
				usage[i].leaveOut()
			}
		}
	}
	h.fitStores(usage)
	var parts []string
	for _, u := range usage {
		if block := renderMemory(u); block != "" {
			parts = append(parts, block)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: strings.Join(parts, "\n"), SkipEvent: true}
}

// readStore reads one store's index. A missing file is an absent store, not
// an error; a factless index is present with zero facts and renders nothing.
func readStore(path, label string) MemoryUsage {
	u := MemoryUsage{Label: label, Path: path}
	if path == "" {
		return u
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return u // no such store on this machine: silence, not an error
	}
	u.Present = true
	u.facts = factLines(string(raw))
	u.Facts = len(u.facts)
	u.Kept = u.Facts
	if u.Facts > 0 {
		u.Bytes = len(memoryHeader(u))
		for _, f := range u.facts {
			u.Bytes += len(memoryFact(f))
		}
	}
	return u
}

// fit keeps the leading facts that fit in share bytes. A store cut short
// also carries its truncation note, so the note's room is held back first,
// sized for the most digits it can print. A share too small for even the
// header and the note keeps nothing, and the store renders nothing.
func (u *MemoryUsage) fit(share int) {
	u.Share = share
	if u.Bytes <= share {
		return
	}
	room := share - len(memoryHeader(*u)) - len(memoryNote(*u, u.Facts))
	u.Kept, u.hidden = 0, room < 0
	for _, f := range u.facts {
		size := len(memoryFact(f))
		if size > room {
			break
		}
		room -= size
		u.Kept++
	}
}

// factLines keeps the index's fact bullets and drops the header prose — the
// prose instructs a reader to consult the index, which is the job this hint
// does.
func factLines(raw string) []string {
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// memoryHeader, memoryFact and memoryNote are the block's lines, each with
// its newline: the sizes fit counts are the lines renderMemory writes.
func memoryHeader(u MemoryUsage) string {
	return fmt.Sprintf(
		"%s (%s) — durable cross-agent facts; open a linked file only when the one-liner isn't enough.\n",
		u.Label,
		filepath.Dir(u.Path),
	)
}

func memoryFact(fact string) string { return "  " + fact + "\n" }

func memoryNote(u MemoryUsage, kept int) string {
	return fmt.Sprintf(
		"  (the %s index is over its %d-byte share of the %d-byte memory budget: "+
			"%d of %d facts fit; prune or consolidate %s)\n",
		u.Label,
		u.Share,
		MemoryBudgetBytes,
		kept,
		u.Facts,
		u.Path,
	)
}

// renderMemory renders one store's block, or nothing for a store with no
// facts or none that fit.
func renderMemory(u MemoryUsage) string {
	if u.Facts == 0 || u.hidden {
		return ""
	}
	var b strings.Builder
	b.WriteString(memoryHeader(u))
	for _, f := range u.facts[:u.Kept] {
		b.WriteString(memoryFact(f))
	}
	if u.Over() {
		b.WriteString(memoryNote(u, u.Kept))
	}
	return strings.TrimRight(b.String(), "\n")
}
