package guard

import (
	"encoding/json"
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

// newCustomGuard builds the guard with a short timeout and a warning
// recorder.
func newCustomGuard(cfg config.CustomGuard, warned *[]string) *Custom {
	c := NewCustom("test-guard", cfg)
	c.timeout = 500 * time.Millisecond
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
		{"exit 1 denies with stdout reason", []string{deny}, "", true, false, "branch name is wrong"},
		{"exit 1 soft mode warns and allows", []string{deny}, "soft", false, true, ""},
		{"exit 1 without output gets a fallback reason", []string{denySilent}, "", true, false, "no reason on stdout"},
		{"exit 2 allows with warn", []string{errorOut}, "", false, true, ""},
		{"missing binary allows with warn", []string{filepath.Join(dir, "nope")}, "", false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newCustomGuard(config.CustomGuard{Event: EventBash, Command: tt.command, Mode: tt.mode}, &warned)
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

func TestCustomGuardTimeout(t *testing.T) {
	slow := writeGuardScript(t, t.TempDir(), "slow.sh", "sleep 5\nexit 1")
	var warned []string
	g := newCustomGuard(config.CustomGuard{Event: EventBash, Command: []string{slow}}, &warned)
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
	g := newCustomGuard(config.CustomGuard{Event: EventBash, Command: []string{deny}, Match: "git commit"}, &warned)
	if d := g.Check(Input{Event: EventBash, Command: "ls -la", Cwd: "/tmp"}); d != nil {
		t.Errorf("non-matching command reached the external: %v", d)
	}
	if d := g.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/tmp"}); d == nil {
		t.Error("matching command did not reach the external")
	}
}

func TestCustomGuardPayload(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "payload.json")
	capture := writeGuardScript(t, dir, "capture.sh", "cat > "+captured+"\nexit 0")

	t.Run("bash payload", func(t *testing.T) {
		var warned []string
		g := newCustomGuard(config.CustomGuard{Event: EventBash, Command: []string{capture}}, &warned)
		g.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/work"})
		var got map[string]string
		mustReadJSON(t, captured, &got)
		if got["command"] != "git commit -m x" || got["cwd"] != "/work" {
			t.Errorf("bash payload = %v", got)
		}
	})

	t.Run("write payload", func(t *testing.T) {
		var warned []string
		g := newCustomGuard(config.CustomGuard{Event: EventWrite, Command: []string{capture}}, &warned)
		g.Check(Input{Event: EventWrite, FilePath: "/repo/file.md", Content: "text", Cwd: "/work"})
		var got map[string]string
		mustReadJSON(t, captured, &got)
		if got["file_path"] != "/repo/file.md" || got["content"] != "text" || got["cwd"] != "/work" {
			t.Errorf("write payload = %v", got)
		}
	})
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
