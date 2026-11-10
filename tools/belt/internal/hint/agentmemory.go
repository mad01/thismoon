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
	facts []string
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
// share of the budget. belt doctor reports it; Check renders from it.
func (h *AgentMemory) Usage() []MemoryUsage {
	usage := []MemoryUsage{
		readStore(h.path(), "shared agent memory"),
		readStore(h.workPath(), "work agent memory"),
	}
	room := MemoryBudgetBytes - len(Render([]Advice{{Hint: h.ID()}}))
	personal, work := memoryShares(usage[0], usage[1], room)
	usage[0].fit(personal)
	usage[1].fit(work)
	return usage
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

func (h *AgentMemory) Check(_ Input) *Advice {
	var parts []string
	for _, u := range h.Usage() {
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
