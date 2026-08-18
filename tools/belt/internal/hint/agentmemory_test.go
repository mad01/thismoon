package hint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func memoryHint(t *testing.T, content string) *AgentMemory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewAgentMemory(testConfig())
	h.path = func() string { return path }
	return h
}

func TestAgentMemoryInjectsFactLines(t *testing.T) {
	h := memoryHint(t, `# Shared Agent Memory Index

Read this index at session start.

- [a.md](a.md) — never send messages directly, draft only
- [b.md](b.md) — verify before claiming done
`)
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want the memory index injected")
	}
	if !strings.Contains(got.Text, "draft only") || !strings.Contains(got.Text, "verify before claiming done") {
		t.Errorf("advice %q is missing fact lines", got.Text)
	}
	if strings.Contains(got.Text, "Read this index at session start") {
		t.Errorf("advice %q carries the header prose this hint replaces", got.Text)
	}
}

func TestAgentMemoryRepeatsEverySession(t *testing.T) {
	h := memoryHint(t, "- [a.md](a.md) — a fact\n")
	for i := range 2 {
		if got := h.Check(Input{Event: EventSessionStart, SessionID: "same"}); got == nil {
			t.Fatalf("Check #%d = nil; the index must inject every session, no dedupe", i+1)
		}
	}
}

func TestAgentMemorySilentWithoutIndex(t *testing.T) {
	h := NewAgentMemory(testConfig())
	h.path = func() string { return filepath.Join(t.TempDir(), "MEMORY.md") }
	if got := h.Check(Input{Event: EventSessionStart}); got != nil {
		t.Errorf("Check without an index file = %+v, want nil", got)
	}
}

func TestAgentMemorySilentOnFactlessIndex(t *testing.T) {
	h := memoryHint(t, "# header only\n\nprose without bullets\n")
	if got := h.Check(Input{Event: EventSessionStart}); got != nil {
		t.Errorf("Check on a factless index = %+v, want nil", got)
	}
}

func TestAgentMemoryFlagsOvergrownIndex(t *testing.T) {
	var b strings.Builder
	for i := range maxMemoryFacts + 5 {
		b.WriteString("- fact number ")
		b.WriteString(strings.Repeat("x", i%3+1))
		b.WriteString("\n")
	}
	h := memoryHint(t, b.String())
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want truncated advice")
	}
	if !strings.Contains(got.Text, "outgrown its budget") {
		t.Errorf("advice %q does not flag the overgrown index", got.Text)
	}
	if n := strings.Count(got.Text, "- fact number"); n != maxMemoryFacts {
		t.Errorf("advice carries %d facts, want the cap of %d", n, maxMemoryFacts)
	}
}
