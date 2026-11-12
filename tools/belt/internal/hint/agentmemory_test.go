package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func memoryHint(t *testing.T, content string) *AgentMemory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewAgentMemory(testConfig())
	h.path = func() string { return path }
	// Keep the work store out of the real $HOME: absent unless a test sets it.
	h.workPath = func() string { return filepath.Join(t.TempDir(), "MEMORY.md") }
	return h
}

func withWorkMemory(t *testing.T, h *AgentMemory, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h.workPath = func() string { return path }
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
	if !strings.Contains(got.Text, "draft only") ||
		!strings.Contains(got.Text, "verify before claiming done") {
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
	h.workPath = func() string { return filepath.Join(t.TempDir(), "MEMORY.md") }
	if got := h.Check(Input{Event: EventSessionStart}); got != nil {
		t.Errorf("Check without an index file = %+v, want nil", got)
	}
}

func TestAgentMemoryInjectsWorkStoreWhenPresent(t *testing.T) {
	h := memoryHint(t, "- [p.md](p.md) — a personal fact\n")
	withWorkMemory(t, h, "- [w.md](w.md) — an internal-only fact\n")
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want both stores injected")
	}
	if !strings.Contains(got.Text, "a personal fact") ||
		!strings.Contains(got.Text, "an internal-only fact") {
		t.Errorf("advice %q is missing a store's facts", got.Text)
	}
	if !strings.Contains(got.Text, "shared agent memory") ||
		!strings.Contains(got.Text, "work agent memory") {
		t.Errorf("advice %q is missing a store label", got.Text)
	}
}

func TestAgentMemoryWorkStoreAloneStillInjects(t *testing.T) {
	h := NewAgentMemory(testConfig())
	h.path = func() string { return filepath.Join(t.TempDir(), "MEMORY.md") }
	withWorkMemory(t, h, "- [w.md](w.md) — an internal-only fact\n")
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want the work store injected without the personal one")
	}
	if strings.Contains(got.Text, "shared agent memory") {
		t.Errorf("advice %q renders an absent personal store", got.Text)
	}
}

func TestAgentMemorySilentOnFactlessIndex(t *testing.T) {
	h := memoryHint(t, "# header only\n\nprose without bullets\n")
	if got := h.Check(Input{Event: EventSessionStart}); got != nil {
		t.Errorf("Check on a factless index = %+v, want nil", got)
	}
}

// factIndex builds an index of n fact lines, each rendering to exactly
// size bytes in the injected block (indent and newline included).
func factIndex(n, size int) string {
	var b strings.Builder
	for i := range n {
		line := fmt.Sprintf("- fact %03d ", i)
		b.WriteString(line)
		b.WriteString(strings.Repeat("x", size-len(line)-3))
		b.WriteString("\n")
	}
	return b.String()
}

// TestAgentMemoryInjectsAThirtyFiveFactIndexWhole pins the case the old
// 30-fact cap broke: a 35-line index of about 7 KB drops nothing.
func TestAgentMemoryInjectsAThirtyFiveFactIndexWhole(t *testing.T) {
	h := memoryHint(t, factIndex(35, 200))
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want the whole index injected")
	}
	if n := strings.Count(got.Text, "- fact "); n != 35 {
		t.Errorf("advice carries %d facts, want all 35", n)
	}
	if strings.Contains(got.Text, "outgrown its budget") {
		t.Errorf("advice %q flags an index that fits", got.Text)
	}
}

// renderedBlock is the block a session reads: the advice as the hook
// renders it, belt[agent-memory] prefix included.
func renderedBlock(a *Advice) string {
	if a == nil {
		return ""
	}
	return Render([]Advice{*a})
}

// headerSize is a store's header line as the block renders it.
func headerSize(label, path string) int {
	return len(memoryHeader(MemoryUsage{Label: label, Path: path}))
}

// blockRoom is what the stores may fill once the prefix is paid.
var blockRoom = MemoryBudgetBytes - len(Render([]Advice{{Hint: "agent-memory"}}))

func TestAgentMemoryFlagsOvergrownIndex(t *testing.T) {
	const size = 400
	h := memoryHint(t, factIndex(MemoryBudgetBytes/size+5, size))
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want truncated advice")
	}
	u := h.Usage()[0]
	if !u.Over() || !strings.Contains(got.Text, "the shared agent memory index is over its") {
		t.Errorf("advice %q does not name the overgrown store", got.Text)
	}
	if n := strings.Count(got.Text, "- fact "); n != u.Kept {
		t.Errorf("advice carries %d facts, want the %d that fit", n, u.Kept)
	}
	if !strings.Contains(got.Text, fmt.Sprintf("- fact %03d ", u.Kept-1)) {
		t.Errorf("advice drops facts from the top; want the first %d kept in index order", u.Kept)
	}
	block := renderedBlock(got)
	if len(block) > MemoryBudgetBytes {
		t.Errorf("block is %d bytes, past the %d-byte budget", len(block), MemoryBudgetBytes)
	}
	if len(block)+size <= MemoryBudgetBytes {
		t.Errorf("block is %d bytes; one more %d-byte fact would still fit", len(block), size)
	}
}

func TestAgentMemoryUsage(t *testing.T) {
	const size = 100
	// One index path for every case, so the header costs the same in each.
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	hdr := headerSize("shared agent memory", path)
	fit := (blockRoom - hdr) / size
	// near is the fewest facts that reach memoryNearPercent of the budget.
	near := (MemoryBudgetBytes*memoryNearPercent/100 - hdr + size - 1) / size
	tests := []struct {
		name             string
		facts            int
		wantNear, wantOv bool
	}{
		{"well under", 10, false, false},
		{"just short of near", near - 1, false, false},
		{"near the budget", near, true, false},
		{"exactly full", fit, true, false},
		{"over the budget", fit + 3, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(factIndex(tt.facts, size)), 0o644); err != nil {
				t.Fatal(err)
			}
			h := memoryHint(t, "")
			h.path = func() string { return path }
			usage := h.Usage()
			u := usage[0]
			if !u.Present || u.Facts != tt.facts || u.Share != blockRoom {
				t.Errorf("usage = %+v, want present with %d facts and the whole room", u, tt.facts)
			}
			if u.Bytes != hdr+tt.facts*size {
				t.Errorf("Bytes = %d, want %d", u.Bytes, hdr+tt.facts*size)
			}
			if MemoryNear(usage) != tt.wantNear || u.Over() != tt.wantOv {
				t.Errorf("near, over = %v, %v, want %v, %v",
					MemoryNear(usage), u.Over(), tt.wantNear, tt.wantOv)
			}
		})
	}
}

func TestAgentMemoryUsageReportsAnAbsentStore(t *testing.T) {
	h := memoryHint(t, "- [a.md](a.md) — a fact\n")
	usage := h.Usage()
	if work := usage[1]; work.Present || work.Facts != 0 || work.Over() || MemoryNear(usage) {
		t.Errorf("absent work store usage = %+v, want not present and nothing to warn about", work)
	}
}

// TestAgentMemoryReservesAShareForTheWorkStore: the personal store renders
// first, so the work store holds workMemoryShare when both have facts. A
// store that needs less hands the rest to the other one.
func TestAgentMemoryReservesAShareForTheWorkStore(t *testing.T) {
	const size = 100
	tests := []struct {
		name           string
		personal, work int
		wantShareWork  func(personal, work MemoryUsage) int
	}{
		{"both fit", 30, 20, func(p, _ MemoryUsage) int { return blockRoom - p.Bytes }},
		{
			"small work index hands back its share",
			120,
			3,
			func(_, w MemoryUsage) int { return w.Bytes },
		},
		{"both large", 120, 60, func(_, _ MemoryUsage) int { return workMemoryShare }},
		{
			"small personal index hands back its share",
			5,
			120,
			func(p, _ MemoryUsage) int { return blockRoom - p.Bytes },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := memoryHint(t, factIndex(tt.personal, size))
			withWorkMemory(t, h, factIndex(tt.work, size))
			usage := h.Usage()
			p, w := usage[0], usage[1]
			if want := tt.wantShareWork(p, w); w.Share != want || p.Share != blockRoom-want {
				t.Errorf("shares = %d personal, %d work, want %d, %d",
					p.Share, w.Share, blockRoom-want, want)
			}
			for _, u := range usage {
				used := len(memoryHeader(u)) + u.Kept*size
				if u.Over() {
					used += len(memoryNote(u, u.Kept))
				}
				if used > u.Share {
					t.Errorf("%s uses %d bytes of a %d-byte share", u.Label, used, u.Share)
				}
			}
			if block := renderedBlock(h.Check(Input{Event: EventSessionStart})); len(
				block,
			) > MemoryBudgetBytes {
				t.Errorf(
					"block is %d bytes, past the %d-byte budget",
					len(block),
					MemoryBudgetBytes,
				)
			}
		})
	}
}

// TestAgentMemoryNoteNamesTheStoreOverItsShare: a large personal index is
// cut to its share and named; the work index inside its share is not.
func TestAgentMemoryNoteNamesTheStoreOverItsShare(t *testing.T) {
	h := memoryHint(t, factIndex(120, 100))
	withWorkMemory(t, h, factIndex(10, 100))
	got := h.Check(Input{Event: EventSessionStart})
	if got == nil {
		t.Fatal("Check = nil, want both stores injected")
	}
	if !strings.Contains(got.Text, "the shared agent memory index is over its") {
		t.Errorf("advice %q does not name the personal store as over its share", got.Text)
	}
	if strings.Contains(got.Text, "the work agent memory index is over") {
		t.Errorf("advice %q blames the work store, which fits its share", got.Text)
	}
	if usage := h.Usage(); usage[1].Over() || usage[1].Kept != 10 {
		t.Errorf("work usage = %+v, want all 10 facts kept", usage[1])
	}
}

// TestAgentMemoryWorstCaseFitsTheHookCap renders the largest blocks the
// hint can produce, both stores over their shares behind long paths, across
// fact sizes, and checks each stays within the budget, which sits under
// Claude Code's 10,000-character cap on one hook output string.
func TestAgentMemoryWorstCaseFitsTheHookCap(t *testing.T) {
	deep := filepath.Join(t.TempDir(), strings.Repeat("a-long-directory-name/", 8))
	if err := os.MkdirAll(filepath.Join(deep, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(deep, "w"), 0o755); err != nil {
		t.Fatal(err)
	}
	longest := 0
	for size := 20; size <= 700; size += 7 {
		index := strings.Replace(factIndex(1000, size), "x", "—", size/4)
		personal := filepath.Join(deep, "p", "MEMORY.md")
		work := filepath.Join(deep, "w", "MEMORY.md")
		for path, content := range map[string]string{personal: index, work: index} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		h := NewAgentMemory(testConfig())
		h.path = func() string { return personal }
		h.workPath = func() string { return work }
		block := renderedBlock(h.Check(Input{Event: EventSessionStart}))
		if len(block) > MemoryBudgetBytes || utf8.RuneCountInString(block) > MemoryBudgetBytes {
			t.Fatalf("fact size %d: block is %d bytes, %d characters, past %d",
				size, len(block), utf8.RuneCountInString(block), MemoryBudgetBytes)
		}
		longest = max(longest, len(block))
	}
	t.Logf("longest worst-case block: %d bytes of %d", longest, MemoryBudgetBytes)
}

// autoMemoryHome lays out a temporary HOME with both stores at their real
// paths, each index holding one fact that names its store. The work store
// is a symlink into ~/sync, the way a store linked into a synced folder
// looks. It returns the home and the Claude settings files under it.
func autoMemoryHome(t *testing.T, personalIndex, workIndex string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	personal := filepath.Join(home, ".config", "agent-memory")
	work := filepath.Join(home, "sync", "agent-memory-work")
	for dir, index := range map[string]string{personal: personalIndex, work: workIndex} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(index), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(work, filepath.Join(home, ".config", "agent-memory-work")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	return home, p.ClaudeSettings
}

// writeSettings writes the Claude settings files, settings.json first and
// settings.local.json second; "" leaves a file missing.
func writeSettings(t *testing.T, files []string, contents ...string) {
	t.Helper()
	for i, content := range contents {
		if content == "" {
			continue
		}
		if err := os.WriteFile(files[i], []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAgentMemoryLeavesOutTheStoreClaudeCodeLoads: with --agent
// claude-code, a store whose directory autoMemoryDirectory names is left
// out, after ~ expansion and symlink resolution on both sides. Anything
// belt cannot read with certainty keeps the store, and so does a session
// without the flag.
func TestAgentMemoryLeavesOutTheStoreClaudeCodeLoads(t *testing.T) {
	toWork := `{"autoMemoryDirectory": "~/sync/agent-memory-work/"}`
	tests := []struct {
		name            string
		settings, local string
		agent           string
		wantPersonal    bool
		wantWork        bool
		wantMarked      string // the store Usage marks: "personal", "work" or ""
	}{
		{"no setting", `{"model": "opus"}`, "", AgentClaudeCode, true, true, ""},
		{"no settings files", "", "", AgentClaudeCode, true, true, ""},
		{"work store through its symlink", toWork, "", AgentClaudeCode, true, false, "work"},
		{
			"work store by its link path",
			`{"autoMemoryDirectory": "~/.config/agent-memory-work"}`, "",
			AgentClaudeCode, true, false, "work",
		},
		{
			"personal store",
			`{"autoMemoryDirectory": "~/.config/agent-memory"}`, "",
			AgentClaudeCode, false, true, "personal",
		},
		{"elsewhere", `{"autoMemoryDirectory": "~/sync"}`, "", AgentClaudeCode, true, true, ""},
		{
			"local wins",
			toWork,
			`{"autoMemoryDirectory": "~/sync"}`,
			AgentClaudeCode,
			true,
			true,
			"",
		},
		{"local names the store", `{}`, toWork, AgentClaudeCode, true, false, "work"},
		{"broken local", toWork, `{not json`, AgentClaudeCode, true, true, ""},
		{"broken settings", `{"autoMemoryDirectory": `, "", AgentClaudeCode, true, true, ""},
		{
			"unresolvable path",
			`{"autoMemoryDirectory": "~/no/such/dir"}`, "",
			AgentClaudeCode, true, true, "",
		},
		{
			"relative path",
			`{"autoMemoryDirectory": "sync/agent-memory-work"}`, "",
			AgentClaudeCode, true, true, "",
		},
		{"matching setting without the flag", toWork, "", "", true, true, "work"},
		{"matching setting for another agent", toWork, "", "codex", true, true, "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, files := autoMemoryHome(t,
				"- [p.md](p.md) — a personal fact\n", "- [w.md](w.md) — a work fact\n")
			writeSettings(t, files, tt.settings, tt.local)
			h := NewAgentMemory(config.Config{ClaudeSettingsFiles: files})

			got := h.Check(Input{Event: EventSessionStart, Agent: tt.agent})
			text := ""
			if got != nil {
				text = got.Text
			}
			if has := strings.Contains(text, "a personal fact"); has != tt.wantPersonal {
				t.Errorf("personal store injected = %v, want %v: %q", has, tt.wantPersonal, text)
			}
			if has := strings.Contains(text, "a work fact"); has != tt.wantWork {
				t.Errorf("work store injected = %v, want %v: %q", has, tt.wantWork, text)
			}

			// Usage is doctor's view: every fact counted, the store marked.
			usage := h.Usage()
			for i, store := range []string{"personal", "work"} {
				marked := usage[i].AutoMemorySettings != ""
				if marked != (store == tt.wantMarked) || usage[i].Facts != 1 {
					t.Errorf("%s usage = %+v, want marked %v with its fact counted",
						store, usage[i], store == tt.wantMarked)
				}
			}
		})
	}
}

// TestAgentMemoryLeftOutStoreHandsOverTheBudget: a store left out counts as
// absent, so the other store gets the whole budget instead of what the work
// share leaves it.
func TestAgentMemoryLeftOutStoreHandsOverTheBudget(t *testing.T) {
	const facts, size = 80, 100
	workIndex := strings.ReplaceAll(factIndex(30, size), "- fact ", "- work ")
	_, files := autoMemoryHome(t, factIndex(facts, size), workIndex)
	writeSettings(t, files, `{"autoMemoryDirectory": "~/.config/agent-memory-work"}`)
	h := NewAgentMemory(config.Config{ClaudeSettingsFiles: files})

	both := h.Check(Input{Event: EventSessionStart})
	if n := strings.Count(both.Text, "- fact "); n >= facts {
		t.Fatalf("with both stores the personal index kept %d of %d facts; "+
			"the fixture must overflow its share to prove anything", n, facts)
	}

	alone := h.Check(Input{Event: EventSessionStart, Agent: AgentClaudeCode})
	if n := strings.Count(alone.Text, "- fact "); n != facts {
		t.Errorf("with the work store left out the personal index kept %d of %d facts", n, facts)
	}
	if strings.Contains(alone.Text, "- work ") || strings.Contains(alone.Text, "is over its") {
		t.Errorf("advice %q renders the work store or a truncation note", alone.Text)
	}
	if block := renderedBlock(alone); len(block) > MemoryBudgetBytes {
		t.Errorf("block is %d bytes, past the %d-byte budget", len(block), MemoryBudgetBytes)
	}
}

// TestAgentMemorySkipsTheEventsService: the injection fires on every
// session, so archiving it would only crowd guard history out of the cap.
func TestAgentMemorySkipsTheEventsService(t *testing.T) {
	got := memoryHint(t, "- [a.md](a.md) — a fact\n").Check(Input{Event: EventSessionStart})
	if got == nil || !got.SkipEvent {
		t.Errorf("Check = %+v, want advice marked SkipEvent", got)
	}
}
