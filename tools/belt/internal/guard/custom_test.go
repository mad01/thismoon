package guard

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// writeGuardScript drops an executable /bin/sh script into dir. Scripts are
// generated per test rather than committed as testdata so the exec bit never
// depends on checkout behavior.
func writeGuardScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	return writeScript(t, dir, name, "#!/bin/sh\n"+body)
}

// newCustomGuard builds the guard with a warning recorder, keeping the
// production exec budget.
//
// Only the timeout test may shorten that budget. This helper used to set it
// to 500ms for every case, which made each test race /bin/sh startup: under
// a full-repo `go test ./...` the exec could outlast the budget, and the
// affected case took the timeout-allow path and reported a missing denial.
// A test that is not about the timeout must not be able to hit it.
func newCustomGuard(cfg config.CustomGuard, warned *[]string) *Custom {
	c := NewCustom("test-guard", cfg)
	c.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return c
}

func TestCustomGuardExitCodes(t *testing.T) {
	dir := t.TempDir()
	allow := writeGuardScript(t, dir, "allow.sh", "exit 0")
	deny := writeGuardScript(t, dir, "deny.sh", "echo branch name is wrong\nexit 1")
	denySilent := writeGuardScript(t, dir, "deny-silent.sh", "exit 1")
	errorOut := writeGuardScript(t, dir, "error.sh", "echo broken >&2\nexit 2")

	tests := []struct {
		name       string
		command    []string
		mode       string
		wantDeny   bool
		wantWarn   bool
		wantReason string
	}{
		{"exit 0 allows", []string{allow}, "", false, false, ""},
		{
			"exit 1 denies with stdout reason",
			[]string{deny},
			"",
			true,
			false,
			"branch name is wrong",
		},
		{"exit 1 soft mode warns and allows", []string{deny}, "soft", false, true, ""},
		{
			"exit 1 without output gets a fallback reason",
			[]string{denySilent},
			"",
			true,
			false,
			"no reason on stdout",
		},
		{"exit 2 allows with warn", []string{errorOut}, "", false, true, ""},
		{
			"missing binary allows with warn",
			[]string{filepath.Join(dir, "nope")},
			"",
			false,
			true,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newCustomGuard(
				config.CustomGuard{Event: EventBash, Command: tt.command, Mode: tt.mode},
				&warned,
			)
			d := g.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("denial = %v, wantDeny %v", d, tt.wantDeny)
			}
			if (len(warned) > 0) != tt.wantWarn {
				t.Errorf("warnings = %v, wantWarn %v", warned, tt.wantWarn)
			}
			if tt.wantReason != "" && (d == nil || !strings.Contains(d.Reason, tt.wantReason)) {
				t.Errorf("reason = %v, want contains %q", d, tt.wantReason)
			}
		})
	}
}

// TestCustomGuardTimeout pins the fail-open timeout path. It is the one
// place that shortens the budget, and it pairs a tiny one with an external
// that sleeps far past it, so the verdict cannot depend on machine speed in
// either direction: the script always outlasts the budget, and a denial
// (exit 1) would mean the timeout did not fire.
func TestCustomGuardTimeout(t *testing.T) {
	slow := writeGuardScript(t, t.TempDir(), "slow.sh", "sleep 5\nexit 1")
	var warned []string
	g := newCustomGuard(config.CustomGuard{Event: EventBash, Command: []string{slow}}, &warned)
	g.timeout = 50 * time.Millisecond
	if d := g.Check(Input{Event: EventBash, Command: "git commit", Cwd: "/tmp"}); d != nil {
		t.Errorf("timeout should fail open, got %v", d)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "timed out") {
		t.Errorf("timeout warning missing: %v", warned)
	}
}

func TestCustomGuardMatchGate(t *testing.T) {
	// The script always denies; the match gate must keep it from running at
	// all for non-matching input.
	deny := writeGuardScript(t, t.TempDir(), "deny.sh", "echo no\nexit 1")
	var warned []string
	g := newCustomGuard(
		config.CustomGuard{Event: EventBash, Command: []string{deny}, Match: "git commit"},
		&warned,
	)
	if d := g.Check(Input{Event: EventBash, Command: "ls -la", Cwd: "/tmp"}); d != nil {
		t.Errorf("non-matching command reached the external: %v", d)
	}
	if d := g.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/tmp"}); d == nil {
		t.Error("matching command did not reach the external")
	}
}

// TestCustomGuardPayload pins the exact key set an external reads per
// event. The identity keys are additive: a call with no session (a `belt
// check` dry-run) keeps the original shape, a main-session call adds only
// session_id, and a subagent call adds its agent id and type as well.
func TestCustomGuardPayload(t *testing.T) {
	bash := Input{Event: EventBash, Command: "git commit -m x", Cwd: "/work"}
	write := Input{Event: EventWrite, FilePath: "/repo/file.md", Content: "text", Cwd: "/work"}
	inSession := func(in Input) Input {
		in.SessionID = "s1"
		return in
	}
	inSubagent := func(in Input) Input {
		in.SessionID, in.AgentID, in.AgentType = "s1", "agent-abc123", "Explore"
		return in
	}
	bashKeys := map[string]string{"command": "git commit -m x", "cwd": "/work"}
	writeKeys := map[string]string{"file_path": "/repo/file.md", "content": "text", "cwd": "/work"}
	sessionKeys := map[string]string{"session_id": "s1"}
	subagentKeys := map[string]string{
		"session_id": "s1", "agent_id": "agent-abc123", "agent_type": "Explore",
	}

	tests := []struct {
		name string
		in   Input
		want map[string]string
	}{
		{"bash without a session", bash, bashKeys},
		{"bash in the main session", inSession(bash), withKeys(bashKeys, sessionKeys)},
		{"bash in a subagent", inSubagent(bash), withKeys(bashKeys, subagentKeys)},
		{"write without a session", write, writeKeys},
		{"write in a subagent", inSubagent(write), withKeys(writeKeys, subagentKeys)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			captured := filepath.Join(dir, "payload.json")
			capture := writeGuardScript(t, dir, "capture.sh", "cat > "+captured+"\nexit 0")
			var warned []string
			g := newCustomGuard(
				config.CustomGuard{Event: tt.in.Event, Command: []string{capture}},
				&warned,
			)

			g.Check(tt.in)

			var got map[string]string
			mustReadJSON(t, captured, &got)
			if !maps.Equal(got, tt.want) {
				t.Errorf("payload = %v, want %v", got, tt.want)
			}
		})
	}
}

// withKeys returns a copy of base with extra merged in.
func withKeys(base, extra map[string]string) map[string]string {
	out := maps.Clone(base)
	maps.Copy(out, extra)
	return out
}

func mustReadJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

func TestCustomGuardsRegisterInAll(t *testing.T) {
	off := false
	cfg := config.Config{CustomGuards: map[string]config.CustomGuard{
		"zeta-check":  {Event: EventBash, Command: []string{"true"}},
		"alpha-check": {Event: EventBash, Command: []string{"true"}, Enabled: &off},
	}}
	var ids []string
	for _, g := range All(cfg) {
		ids = append(ids, g.ID())
	}
	// Custom guards come after the built-ins, alphabetical by name.
	n := len(ids)
	if n < 2 || ids[n-2] != "alpha-check" || ids[n-1] != "zeta-check" {
		t.Errorf("custom guards not appended sorted: %v", ids)
	}
	if cfg.GuardEnabled("alpha-check") {
		t.Error("disabled custom guard reported enabled")
	}
	if !cfg.GuardEnabled("zeta-check") {
		t.Error("default custom guard reported disabled")
	}
	for _, g := range ForEvent(EventBash, cfg) {
		if g.ID() == "alpha-check" {
			t.Error("disabled custom guard still runs")
		}
	}
}
