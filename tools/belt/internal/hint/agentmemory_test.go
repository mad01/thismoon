package hint

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
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

// usageOf is h.Usage(cwd), failing the test on its error.
func usageOf(t *testing.T, h *AgentMemory, cwd string) []MemoryUsage {
	t.Helper()
	usage, err := h.Usage(cwd)
	if err != nil {
		t.Fatalf("Usage(%q): %v", cwd, err)
	}
	return usage
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
	u := usageOf(t, h, "")[0]
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
			usage := usageOf(t, h, "")
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
	usage := usageOf(t, h, "")
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
			usage := usageOf(t, h, "")
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
	if usage := usageOf(t, h, ""); usage[1].Over() || usage[1].Kept != 10 {
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

// memoryLayout is a temporary HOME with both stores at their real paths,
// each index holding one fact that names its store, and a session
// directory outside any git repository. The work store is a symlink into
// ~/sync, the way a store linked into a synced folder looks.
type memoryLayout struct {
	home, cwd string
	sources   config.ClaudeSources
}

func autoMemoryHome(t *testing.T, personalIndex, workIndex string) memoryLayout {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.EnvDisableAutoMemory, "")
	personal := filepath.Join(home, ".config", "agent-memory")
	work := filepath.Join(home, "sync", "agent-memory-work")
	for dir, index := range map[string]string{personal: personalIndex, work: workIndex} {
		mkdir(t, dir)
		writeIn(t, filepath.Join(dir, "MEMORY.md"), index)
	}
	if err := os.Symlink(work, filepath.Join(home, ".config", "agent-memory-work")); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(home, "proj")
	mkdir(t, filepath.Join(cwd, ".claude"))
	mkdir(t, filepath.Join(home, ".claude"))
	p, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	return memoryLayout{home: home, cwd: cwd, sources: config.ClaudeSources{User: p.ClaudeUser}}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeIn writes content to path; "" leaves the file missing.
func writeIn(t *testing.T, path, content string) {
	t.Helper()
	if content == "" {
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hint builds the hint over the layout's settings sources.
func (l memoryLayout) hint() *AgentMemory {
	return NewAgentMemory(config.Config{ClaudeSources: &l.sources})
}

// autoMemoryCase is one session: the settings files by where they sit, the
// session's CLAUDE_CODE_DISABLE_AUTO_MEMORY, its --agent value and
// directory, and what the session gets.
type autoMemoryCase struct {
	name string
	// user is ~/.claude/settings.json and homeLocal is
	// ~/.claude/settings.local.json; project and local sit in the session
	// directory's .claude. "" leaves a file missing.
	user, homeLocal, project, local string
	env                             string
	agent                           string // "" means AgentClaudeCode
	noFlag                          bool   // the session starts without --agent
	inHome                          bool   // the session starts in the home directory
	keepPersonal, keepWork          bool
	marked                          string // the store Usage marks: "personal", "work" or ""
	off                             bool   // the marked store carries an off reason
	cannotTell                      bool   // Usage returns an error: a source does not read
}

func (tt autoMemoryCase) run(t *testing.T) {
	l := autoMemoryHome(t, "- [p.md](p.md) — a personal fact\n", "- [w.md](w.md) — a work fact\n")
	writeIn(t, l.sources.User, tt.user)
	writeIn(t, filepath.Join(l.home, ".claude", "settings.local.json"), tt.homeLocal)
	writeIn(t, filepath.Join(l.cwd, ".claude", "settings.json"), tt.project)
	writeIn(t, filepath.Join(l.cwd, ".claude", "settings.local.json"), tt.local)
	t.Setenv(config.EnvDisableAutoMemory, tt.env)
	cwd, agent := l.cwd, cmp.Or(tt.agent, AgentClaudeCode)
	if tt.noFlag {
		agent = ""
	}
	if tt.inHome {
		cwd = l.home
	}
	h := l.hint()
	checkKept(t, h.Check(Input{Event: EventSessionStart, Agent: agent, Cwd: cwd}),
		tt.keepPersonal, tt.keepWork)
	// Usage is doctor's view: every fact counted, the store marked.
	usage, err := h.Usage(cwd)
	if (err != nil) != tt.cannotTell {
		t.Errorf("Usage error = %v, want one: %v", err, tt.cannotTell)
	}
	for i, store := range []string{"personal", "work"} {
		u := usage[i]
		marked := u.AutoMemory.Directory != ""
		if marked != (store == tt.marked) || u.Facts != 1 {
			t.Errorf("%s usage = %+v, want marked %v with its fact counted",
				store, u, store == tt.marked)
		}
		if marked && (u.AutoMemory.Off != "") != tt.off {
			t.Errorf("%s off reason = %q, want one: %v", store, u.AutoMemory.Off, tt.off)
		}
	}
}

func checkKept(t *testing.T, got *Advice, personal, work bool) {
	t.Helper()
	text := ""
	if got != nil {
		text = got.Text
	}
	if has := strings.Contains(text, "a personal fact"); has != personal {
		t.Errorf("personal store injected = %v, want %v: %q", has, personal, text)
	}
	if has := strings.Contains(text, "a work fact"); has != work {
		t.Errorf("work store injected = %v, want %v: %q", has, work, text)
	}
}

const toWork = `{"autoMemoryDirectory": "~/sync/agent-memory-work/"}`

// TestAgentMemoryLeavesOutTheStoreClaudeCodeLoads: with --agent
// claude-code, a store whose directory autoMemoryDirectory names is left
// out, after ~ expansion and symlink resolution on both sides. Anything
// belt cannot read with certainty keeps the store, and so does a session
// without the flag.
func TestAgentMemoryLeavesOutTheStoreClaudeCodeLoads(t *testing.T) {
	for _, tt := range []autoMemoryCase{
		{name: "no setting", user: `{"model": "opus"}`, keepPersonal: true, keepWork: true},
		{name: "no settings files", keepPersonal: true, keepWork: true},
		{name: "work store through its symlink", user: toWork, keepPersonal: true, marked: "work"},
		{
			name: "work store by its link path", user: `{"autoMemoryDirectory": "~/.config/agent-memory-work"}`,
			keepPersonal: true, marked: "work",
		},
		{
			name: "personal store", user: `{"autoMemoryDirectory": "~/.config/agent-memory"}`,
			keepWork: true, marked: "personal",
		},
		{name: "elsewhere", user: `{"autoMemoryDirectory": "~/sync"}`, keepPersonal: true, keepWork: true},
		{
			name: "broken settings", user: `{"autoMemoryDirectory": `,
			keepPersonal: true, keepWork: true, cannotTell: true,
		},
		{
			name: "broken project local", user: toWork, local: `{not json`,
			keepPersonal: true, keepWork: true, cannotTell: true,
		},
		{
			name: "unresolvable path", user: `{"autoMemoryDirectory": "~/no/such/dir"}`,
			keepPersonal: true, keepWork: true,
		},
		{
			name: "relative path", user: `{"autoMemoryDirectory": "sync/agent-memory-work"}`,
			keepPersonal: true, keepWork: true,
		},
		{
			name: "matching setting without the flag", user: toWork, noFlag: true,
			keepPersonal: true, keepWork: true, marked: "work",
		},
		{
			name: "matching setting for another agent", user: toWork, agent: "codex",
			keepPersonal: true, keepWork: true, marked: "work",
		},
	} {
		t.Run(tt.name, tt.run)
	}
}

// TestAgentMemoryReadsTheSourcesClaudeCodeReads: autoMemoryDirectory comes
// from the user file, the session directory's shared and local project
// files, with Claude Code's precedence. ~/.claude/settings.local.json is a
// source only in a session started in the home directory.
func TestAgentMemoryReadsTheSourcesClaudeCodeReads(t *testing.T) {
	elsewhere := `{"autoMemoryDirectory": "~/sync"}`
	for _, tt := range []autoMemoryCase{
		{name: "home local alone keeps the store", homeLocal: toWork, keepPersonal: true, keepWork: true},
		{
			name: "home local beside a user file", user: elsewhere, homeLocal: toWork,
			keepPersonal: true, keepWork: true,
		},
		{
			name: "home local in a home session", homeLocal: toWork, inHome: true,
			keepPersonal: true, marked: "work",
		},
		{name: "project names the store", project: toWork, keepPersonal: true, marked: "work"},
		{name: "project over user", user: elsewhere, project: toWork, keepPersonal: true, marked: "work"},
		{name: "local over project", project: toWork, local: elsewhere, keepPersonal: true, keepWork: true},
		{name: "local names the store", user: elsewhere, local: toWork, keepPersonal: true, marked: "work"},
	} {
		t.Run(tt.name, tt.run)
	}
}

// TestAgentMemoryKeepsTheStoreWhenAutoMemoryIsOff: every switch that stops
// Claude Code loading the directory puts the store back in the session, and
// Usage still marks it with the reason.
func TestAgentMemoryKeepsTheStoreWhenAutoMemoryIsOff(t *testing.T) {
	block := `{"permissions": {"blockReadsOutsideWorkingDirectories": true}}`
	for _, tt := range []autoMemoryCase{
		{name: "autoMemoryEnabled false", user: `{"autoMemoryDirectory": "~/sync/agent-memory-work", ` +
			`"autoMemoryEnabled": false}`, keepPersonal: true, keepWork: true, marked: "work", off: true},
		{
			name: "disabled for the project", user: toWork, project: `{"autoMemoryEnabled": false}`,
			keepPersonal: true, keepWork: true, marked: "work", off: true,
		},
		{
			name: "environment variable", user: toWork, env: "1",
			keepPersonal: true, keepWork: true, marked: "work", off: true,
		},
		{
			name: "environment forces it on", user: toWork, project: `{"autoMemoryEnabled": false}`, env: "0",
			keepPersonal: true, marked: "work",
		},
		{
			name: "read block with a project directory", user: block, project: toWork,
			keepPersonal: true, keepWork: true, marked: "work", off: true,
		},
		{
			name: "read block with a user directory", user: toWork, project: block,
			keepPersonal: true, marked: "work",
		},
	} {
		t.Run(tt.name, tt.run)
	}
}

// gitIn runs git in dir with a fixed identity, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	args = append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestAgentMemoryReadsTheLocalFileAtTheMainCheckout: inside a git
// repository Claude Code keeps the project local file at the repository
// root, and for a linked worktree at the main checkout's root, so a session
// in a subdirectory or a worktree reads it there.
func TestAgentMemoryReadsTheLocalFileAtTheMainCheckout(t *testing.T) {
	l := autoMemoryHome(t, "- [p.md](p.md) — a personal fact\n", "- [w.md](w.md) — a work fact\n")
	repo := filepath.Join(l.home, "repo")
	mkdir(t, filepath.Join(repo, "sub"))
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "wt", filepath.Join(l.home, "wt"))
	mkdir(t, filepath.Join(repo, ".claude"))
	writeIn(t, filepath.Join(repo, ".claude", "settings.local.json"), toWork)

	for _, cwd := range []string{repo, filepath.Join(repo, "sub"), filepath.Join(l.home, "wt")} {
		got := l.hint().Check(Input{Event: EventSessionStart, Agent: AgentClaudeCode, Cwd: cwd})
		checkKept(t, got, true, false)
	}
}

// fakeGit puts a git that runs script alone on PATH, or no git at all when
// script is "".
func fakeGit(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if script != "" {
		err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+script+"\n"), 0o755)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

// TestAgentMemoryKeepsTheStoreWhenGitFails: only git's "not a git
// repository" status, 128, places the session outside a repository. The
// real git gives it, since the session directory is in none. Any other
// failure could hide the main checkout's local file, so every store stays
// in and Usage returns the error. A git killed by a signal stands in for
// the timeout kill, which ends git the same way.
func TestAgentMemoryKeepsTheStoreWhenGitFails(t *testing.T) {
	for _, tt := range []struct {
		name       string
		realGit    bool   // run the git on PATH; else fakeGit(script)
		script     string // "" with realGit false: no git on PATH
		cannotTell bool
	}{
		{name: "not a git repository", realGit: true},
		{name: "other error", script: "exit 1", cannotTell: true},
		{name: "killed like a timeout", script: "kill -KILL $$", cannotTell: true},
		{name: "git missing", cannotTell: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := autoMemoryHome(
				t,
				"- [p.md](p.md) — a personal fact\n",
				"- [w.md](w.md) — a work fact\n",
			)
			writeIn(t, l.sources.User, toWork)
			if !tt.realGit {
				fakeGit(t, tt.script)
			}
			h := l.hint()
			got := h.Check(Input{Event: EventSessionStart, Agent: AgentClaudeCode, Cwd: l.cwd})
			checkKept(t, got, true, tt.cannotTell)
			usage, err := h.Usage(l.cwd)
			if (err != nil) != tt.cannotTell {
				t.Errorf("Usage error = %v, want one: %v", err, tt.cannotTell)
			}
			if marked := usage[1].AutoMemory.Directory != ""; marked == tt.cannotTell {
				t.Errorf("work store marked = %v, want %v", marked, !tt.cannotTell)
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
	l := autoMemoryHome(t, factIndex(facts, size), workIndex)
	writeIn(t, l.sources.User, `{"autoMemoryDirectory": "~/.config/agent-memory-work"}`)
	h := l.hint()

	both := h.Check(Input{Event: EventSessionStart, Cwd: l.cwd})
	if n := strings.Count(both.Text, "- fact "); n >= facts {
		t.Fatalf("with both stores the personal index kept %d of %d facts; "+
			"the fixture must overflow its share to prove anything", n, facts)
	}

	alone := h.Check(Input{Event: EventSessionStart, Agent: AgentClaudeCode, Cwd: l.cwd})
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
