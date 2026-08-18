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
type AgentMemory struct {
	cfg config.Config
	// path locates the index; overridable for tests.
	path func() string
}

func NewAgentMemory(cfg config.Config) *AgentMemory {
	return &AgentMemory{cfg: cfg, path: memoryIndexPath}
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

func (h *AgentMemory) Check(_ Input) *Advice {
	path := h.path()
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil // no shared memory on this machine: silence, not an error
	}
	facts := factLines(string(raw))
	if len(facts) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: renderMemory(facts, filepath.Dir(path))}
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

func renderMemory(facts []string, dir string) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"shared agent memory (%s) — durable cross-agent facts; open a linked file only when the one-liner isn't enough.\n",
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
