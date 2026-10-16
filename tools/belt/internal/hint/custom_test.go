package hint

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// writeHintScript drops an executable /bin/sh script into dir. Scripts are
// generated per test rather than committed as testdata so the exec bit never
// depends on checkout behavior.
func writeHintScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// generousBudget keeps every test that is not about the timeout off the
// timeout path: a freshly written script's first launch can take seconds
// under a full-repo `go test ./...`, and the production 400ms budget would
// turn that into a spurious silence.
const generousBudget = 10_000

// newCustomHint builds the hint under test with a warning recorder, a
// repo resolver that never resolves (so exclude_repos cannot match unless a
// test says so), and the generous budget.
func newCustomHint(entry config.CustomHint, warned *[]string) *Custom {
	if entry.Event == "" {
		entry.Event = EventSessionStart
	}
	if entry.TimeoutMS == 0 {
		entry.TimeoutMS = generousBudget
	}
	cfg := config.Config{CustomHints: map[string]config.CustomHint{"test-hint": entry}}
	c := NewCustom("test-hint", cfg)
	c.resolveRepo = func(string) string { return "" }
	c.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return c
}

func TestCustomHintOutcomes(t *testing.T) {
	dir := t.TempDir()
	speaks := writeHintScript(t, dir, "speaks.sh",
		"printf 'open actions:\\n  - review the release PR  \\n\\n'")
	fails := writeHintScript(t, dir, "fails.sh", "echo partial\nexit 1")
	silent := writeHintScript(t, dir, "silent.sh", "exit 0")
	whitespace := writeHintScript(t, dir, "whitespace.sh", "printf '  \\n\\n'")
	indented := writeHintScript(t, dir, "indented.sh", "printf '  - first\\n  - second\\n\\n'")

	tests := []struct {
		name     string
		command  []string
		wantText string // "" means silence
		wantWarn string // substring of the warn message; "" means no warn
	}{
		{
			"exit 0 with output advises, trailing whitespace trimmed, lines kept",
			[]string{speaks},
			"open actions:\n  - review the release PR",
			"",
		},
		{
			"leading indentation on line one is kept",
			[]string{indented},
			"  - first\n  - second",
			"",
		},
		{"non-zero exit is silent with a warn", []string{fails}, "", "failed"},
		{"empty stdout is silent with a warn", []string{silent}, "", "empty stdout"},
		{"whitespace-only stdout counts as empty", []string{whitespace}, "", "empty stdout"},
		{
			"missing binary is silent with a warn",
			[]string{filepath.Join(dir, "nope")},
			"",
			"failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			h := newCustomHint(config.CustomHint{Command: tt.command}, &warned)

			got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})

			if tt.wantText == "" && got != nil {
				t.Errorf("Check = %+v, want silence", got)
			}
			if tt.wantText != "" && (got == nil || got.Text != tt.wantText) {
				t.Errorf("Check = %+v, want text %q", got, tt.wantText)
			}
			if got != nil && got.Hint != "test-hint" {
				t.Errorf("advice attributed to %q, want test-hint", got.Hint)
			}
			if tt.wantWarn == "" && len(warned) > 0 {
				t.Errorf("unexpected warnings %v", warned)
			}
			if tt.wantWarn != "" &&
				(len(warned) != 1 || !strings.Contains(warned[0], tt.wantWarn)) {
				t.Errorf("warnings = %v, want one containing %q", warned, tt.wantWarn)
			}
		})
	}
}

// TestCustomHintTimeout pins the budget: a tiny timeout_ms against an
// external that sleeps far past it must end in silence plus a warn, and
// promptly, even though the script's sleep child still holds stdout after
// the script itself is killed.
func TestCustomHintTimeout(t *testing.T) {
	slow := writeHintScript(t, t.TempDir(), "slow.sh", "sleep 5\necho late")
	var warned []string
	h := newCustomHint(config.CustomHint{Command: []string{slow}, TimeoutMS: 50}, &warned)

	if got := h.Check(Input{Event: EventSessionStart, Cwd: "/", SessionID: "s"}); got != nil {
		t.Errorf("timeout should be silent, got %+v", got)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "timed out after 50ms") {
		t.Errorf("timeout warning missing: %v", warned)
	}
}

// TestCustomHintKeepsOutputWhenAChildLingers pins the ErrWaitDelay path: an
// external that prints, exits 0, and leaves a background child holding
// stdout is a successful run whose output is the advice, not a failure.
func TestCustomHintKeepsOutputWhenAChildLingers(t *testing.T) {
	dir := t.TempDir()
	lingers := writeHintScript(t, dir, "lingers.sh", "echo open actions: none\nsleep 2 &\n")
	var warned []string
	h := newCustomHint(config.CustomHint{Command: []string{lingers}}, &warned)

	got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})

	if got == nil || got.Text != "open actions: none" {
		t.Errorf("Check = %+v, want the output printed before the child lingered", got)
	}
	if len(warned) > 0 {
		t.Errorf("a lingering child is not a failure, got warnings %v", warned)
	}
}

// TestCustomHintWarnDoesNotHoldTheSession pins the warn path's bound: an
// events endpoint that accepts but never answers must not stretch the hint
// past its budget plus the grace periods, since hints run serially on the
// session-start hook. The script is launched once untimed first, because a
// fresh script's first launch can be slow under load and this test is
// about the warn, not the exec.
func TestCustomHintWarnDoesNotHoldTheSession(t *testing.T) {
	dir := t.TempDir()
	fails := writeHintScript(t, dir, "fails.sh", "exit 1")
	_ = exec.Command(fails).Run()
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	cfg := config.Config{CustomHints: map[string]config.CustomHint{"test-hint": {
		Event:     EventSessionStart,
		Command:   []string{fails},
		TimeoutMS: 400,
	}}}
	h := NewCustom("test-hint", cfg)
	h.resolveRepo = func(string) string { return "" }
	h.emit = func(string, string, string, string, map[string]string) {
		close(started)
		<-release // the endpoint that never answers
	}

	start := time.Now()
	got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})
	elapsed := time.Since(start)

	if got != nil {
		t.Errorf("exit 1 should be silent, got %+v", got)
	}
	select {
	case <-started:
	default:
		t.Fatal("the warn was never emitted")
	}
	if limit := 400*time.Millisecond + 200*time.Millisecond; elapsed > limit {
		t.Errorf("Check took %s with a hung events endpoint, want under %s", elapsed, limit)
	}
}

// TestCustomHintTimeoutKillsGrandchildren pins the process-group kill: a
// grandchild the script started (the curl inside a journal script, say)
// must be gone after the timeout, not left to outlive the hook.
func TestCustomHintTimeoutKillsGrandchildren(t *testing.T) {
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "grandchild.pid")
	hangs := writeHintScript(t, dir, "hangs.sh", "sleep 30 &\necho $! > "+pidfile+"\nwait")
	var warned []string
	h := newCustomHint(config.CustomHint{Command: []string{hangs}, TimeoutMS: 1000}, &warned)

	if got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"}); got != nil {
		t.Errorf("timeout should be silent, got %+v", got)
	}

	raw, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatalf("grandchild pid never recorded; the script did not reach line two"+
			" inside the budget: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("pidfile %q: %v", raw, err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return // gone, as it should be
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("grandchild %d is still alive after the timeout", pid)
}

// TestCustomHintPayload pins the exact key set an external reads. The
// main-session payload must stay the original four keys, so an external
// written against that shape sees nothing new; the agent keys appear only
// inside a subagent.
func TestCustomHintPayload(t *testing.T) {
	session := Input{
		Event:          EventSessionStart,
		Cwd:            "/work",
		SessionID:      "s1",
		TranscriptPath: "/t.jsonl",
	}
	subagent := session
	subagent.AgentID = "agent-abc123"
	subagent.AgentType = "Explore"
	mainKeys := map[string]string{
		"event":           "session-start",
		"cwd":             "/work",
		"session_id":      "s1",
		"transcript_path": "/t.jsonl",
	}
	subagentKeys := maps.Clone(mainKeys)
	subagentKeys["agent_id"] = "agent-abc123"
	subagentKeys["agent_type"] = "Explore"

	tests := []struct {
		name string
		in   Input
		want map[string]string
	}{
		{"main session keeps the original keys", session, mainKeys},
		{"subagent adds its identity", subagent, subagentKeys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			captured := filepath.Join(dir, "payload.json")
			capture := writeHintScript(t, dir, "capture.sh", "cat > "+captured+"\necho ok")
			var warned []string
			h := newCustomHint(config.CustomHint{Command: []string{capture}}, &warned)

			h.Check(tt.in)

			raw, err := os.ReadFile(captured)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("payload = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCustomHintAgentTypesGate pins the subagent-start gate. A spawn whose
// type is outside agent_types must not exec the external (the script leaves
// a marker when it runs) and must not warn, since a skipped spawn is the
// configured outcome. A listed type runs the external as before, including
// the empty-stdout warn for a hint that runs and says nothing.
func TestCustomHintAgentTypesGate(t *testing.T) {
	workers := []string{"factory-worker-low", "factory-worker-medium", "factory-worker-high"}
	tests := []struct {
		name       string
		agentTypes []string
		agentType  string
		silent     bool   // the script prints nothing
		wantRan    bool   // the external was exec'd
		wantText   string // "" means silence
		wantWarn   string // substring of the one warn; "" means no warn
	}{
		{
			name:      "unset list runs for any subagent",
			agentType: "Explore",
			wantRan:   true, wantText: "brief",
		},
		{
			name:       "listed type runs",
			agentTypes: workers, agentType: "factory-worker-medium",
			wantRan: true, wantText: "brief",
		},
		{
			name:       "unlisted type skips with no exec and no warn",
			agentTypes: workers, agentType: "Explore",
		},
		{
			name:       "a prefix of a listed type does not match",
			agentTypes: workers, agentType: "factory-worker",
		},
		{
			name:       "a listed type in another case does not match",
			agentTypes: workers, agentType: "Factory-Worker-Low",
		},
		{
			name:       "listed type with empty stdout still warns",
			agentTypes: workers, agentType: "factory-worker-low", silent: true,
			wantRan: true, wantWarn: "empty stdout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			body := "touch " + marker + "\necho brief"
			if tt.silent {
				body = "touch " + marker
			}
			script := writeHintScript(t, dir, "brief.sh", body)
			var warned []string
			h := newCustomHint(config.CustomHint{
				Event:      EventSubagentStart,
				Command:    []string{script},
				AgentTypes: tt.agentTypes,
			}, &warned)

			got := h.Check(Input{
				Event: EventSubagentStart, Cwd: dir, SessionID: "s",
				AgentID: "agent-abc123", AgentType: tt.agentType,
			})

			_, err := os.Stat(marker)
			if ran := err == nil; ran != tt.wantRan {
				t.Errorf("external ran = %v, want %v", ran, tt.wantRan)
			}
			if tt.wantText == "" && got != nil {
				t.Errorf("Check = %+v, want silence", got)
			}
			if tt.wantText != "" && (got == nil || got.Text != tt.wantText) {
				t.Errorf("Check = %+v, want text %q", got, tt.wantText)
			}
			if tt.wantWarn == "" && len(warned) > 0 {
				t.Errorf("unexpected warnings %v", warned)
			}
			if tt.wantWarn != "" &&
				(len(warned) != 1 || !strings.Contains(warned[0], tt.wantWarn)) {
				t.Errorf("warnings = %v, want one containing %q", warned, tt.wantWarn)
			}
		})
	}
}

// TestCustomHintExcludeRepos pins both places the opt-out list may sit and
// that an excluded repo costs no process launch: the script leaves a marker
// when it runs, and the marker must be absent.
func TestCustomHintExcludeRepos(t *testing.T) {
	tests := []struct {
		name      string
		entry     []string // custom_hints.<name>.exclude_repos
		toggle    []string // hints.<name>.exclude_repos
		wantSpoke bool
	}{
		{"no lists", nil, nil, true},
		{"entry list excludes", []string{"github.com/mad01/thismoon"}, nil, false},
		{"toggle list excludes", nil, []string{"github.com/mad01/*"}, false},
		{
			"lists naming other repos",
			[]string{"github.com/you/x"},
			[]string{"github.com/you/y"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			script := writeHintScript(t, dir, "speaks.sh", "touch "+marker+"\necho hi")
			cfg := config.Config{
				CustomHints: map[string]config.CustomHint{"test-hint": {
					Event:        EventSessionStart,
					Command:      []string{script},
					TimeoutMS:    generousBudget,
					ExcludeRepos: tt.entry,
				}},
				Hints: map[string]config.Toggle{"test-hint": {ExcludeRepos: tt.toggle}},
			}
			h := NewCustom("test-hint", cfg)
			h.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
			h.emit = func(string, string, string, string, map[string]string) {}

			got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})

			if (got != nil) != tt.wantSpoke {
				t.Errorf("Check = %+v, want spoke %v", got, tt.wantSpoke)
			}
			_, err := os.Stat(marker)
			if ran := err == nil; ran != tt.wantSpoke {
				t.Errorf("external ran = %v, want %v", ran, tt.wantSpoke)
			}
		})
	}
}

func TestCustomHintsRegisterInAll(t *testing.T) {
	off := false
	cfg := config.Config{CustomHints: map[string]config.CustomHint{
		"zeta-journal":  {Event: EventSessionStart, Command: []string{"true"}},
		"alpha-journal": {Event: EventSessionStart, Command: []string{"true"}, Enabled: &off},
	}}
	var ids []string
	for _, h := range All(cfg) {
		ids = append(ids, h.ID())
	}
	// Custom hints come after the built-ins, alphabetical by name.
	n := len(ids)
	if n < 2 || ids[n-2] != "alpha-journal" || ids[n-1] != "zeta-journal" {
		t.Errorf("custom hints not appended sorted: %v", ids)
	}
	if cfg.HintEnabled("alpha-journal") {
		t.Error("disabled custom hint reported enabled")
	}
	if !cfg.HintEnabled("zeta-journal") {
		t.Error("default custom hint reported disabled")
	}
	var active []string
	for _, h := range ForEvent(EventSessionStart, cfg) {
		active = append(active, h.ID())
	}
	if slices.Contains(active, "alpha-journal") {
		t.Error("disabled custom hint still runs")
	}
	if !slices.Contains(active, "zeta-journal") {
		t.Error("enabled custom hint does not run on session-start")
	}
}

// TestHintFieldsListsEveryBuiltinHint pins config.HintFields to the hints
// All registers: the table is what config validation reads to refuse a
// custom hint that shadows a built-in name, so a hint added here without a
// row there would be shadowable.
func TestHintFieldsListsEveryBuiltinHint(t *testing.T) {
	var ids []string
	for _, h := range All(config.Config{}) {
		ids = append(ids, h.ID())
	}
	slices.Sort(ids)
	ids = slices.Compact(ids) // a hint registered on two events appears twice
	if want := slices.Sorted(maps.Keys(config.HintFields)); !slices.Equal(ids, want) {
		t.Errorf("hint.All ids = %v, config.HintFields keys = %v", ids, want)
	}
}
