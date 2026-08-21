package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// maxMemoryFacts bounds the injected index. The store is meant to stay a
// short list of durable facts; past this the problem is store hygiene, and
// injecting a wall of text would just teach the model to skip the block
// (docs/adr/0008).
const maxMemoryFacts = 30

// AgentMemory injects the shared agent memory index when a session starts.
// The instruction files say "read ~/.config/agent-memory/MEMORY.md at session
// start", and instruction-only prose does not trigger reliably — the same
// failure mode the kof consult had before the session-start hint. Unlike the
// kof hints this one has no per-session dedupe: the index is the thing to see
// every session, not advice to show once.
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

func (h *AgentMemory) Check(_ Input) *Advice {
	sections := []string{
		renderStore(h.path(), "shared agent memory"),
		renderStore(h.workPath(), "work agent memory"),
	}
	var parts []string
	for _, s := range sections {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: strings.Join(parts, "\n")}
}

// renderStore reads one store's index and renders its section; a missing file
// or factless index renders nothing (that store doesn't exist here).
func renderStore(path, label string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "" // no such store on this machine: silence, not an error
	}
	facts := factLines(string(raw))
	if len(facts) == 0 {
		return ""
	}
	return renderMemory(facts, filepath.Dir(path), label)
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

func renderMemory(facts []string, dir, label string) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"%s (%s) — durable cross-agent facts; open a linked file only when the one-liner isn't enough.\n",
		label,
		dir,
	)
	over := len(facts) > maxMemoryFacts
	if over {
		facts = facts[:maxMemoryFacts]
	}
	for _, f := range facts {
		b.WriteString("  ")
		b.WriteString(f)
		b.WriteString("\n")
	}
	if over {
		fmt.Fprintf(
			&b,
			"  (index truncated at %d facts — it has outgrown its budget; prune or consolidate MEMORY.md)\n",
			maxMemoryFacts,
		)
	}
	return strings.TrimRight(b.String(), "\n")
}
