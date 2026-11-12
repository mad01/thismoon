package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// mkAllowRepo lays out a repo at top the way git init leaves it, so the
// git-c-read allow accepts it as a work tree. No git runs.
func mkAllowRepo(t *testing.T, top string) {
	t.Helper()
	files := map[string]string{
		"HEAD":               "ref: refs/heads/main\n",
		"config":             "[core]\n\tbare = false\n",
		"objects/info/.keep": "",
		"refs/heads/.keep":   "",
	}
	for rel, content := range files {
		path := filepath.Join(top, ".git", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// allowOn turns git-c-read on with ~/code/src as its root.
var allowOn = map[string]config.Allow{
	guard.GitCReadID: {Enabled: true, Roots: []string{"~/code/src"}},
}

// bashPayload is a Bash PreToolUse payload for one command, with extra
// tool_input keys merged in.
func bashPayload(t *testing.T, tool, command, cwd string, extra map[string]any) []byte {
	t.Helper()
	input := map[string]any{"command": command}
	for k, v := range extra {
		input[k] = v
	}
	raw, err := json.Marshal(map[string]any{
		"tool_name":  tool,
		"tool_input": input,
		"cwd":        cwd,
		"session_id": "s1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// runGuards runs the guard entrypoint on raw with cfg and returns what it
// wrote and what it archived.
func runGuards(raw []byte, cfg config.Config) (string, archived) {
	var out bytes.Buffer
	var a archived
	load := func() (config.Config, error) { return cfg, nil }
	runner{load: load, emit: a.emit}.guards("bash", bytes.NewReader(raw), &out)
	return out.String(), a
}

// TestRunAnswersAllowOnlyWhenNoGuardDenies runs the guard entrypoint on a
// read-only git -C call inside the git-c-read root. With the allow on and
// every guard passing cleanly, belt answers allow. A guard that denies the
// same call wins, and with the allow off (the default) belt writes
// nothing. HOME is a temp dir, so the repo, the root and the session cwd
// are all hermetic.
func TestRunAnswersAllowOnlyWhenNoGuardDenies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("GIT_DIR", "")
	mkAllowRepo(t, filepath.Join(home, "code", "src", "x"))
	guardRunning := func(script, mode string) map[string]config.CustomGuard {
		return map[string]config.CustomGuard{
			"external": {
				Event:   config.EventBash,
				Command: []string{"/bin/sh", "-c", script},
				Mode:    mode,
			},
		}
	}
	tests := []struct {
		name     string
		cfg      config.Config
		extra    map[string]any
		tool     string
		decision string // "" means belt wrote nothing
		prefix   string
	}{
		{"allow on", config.Config{Allows: allowOn}, nil, "Bash", "allow", "belt[git-c-read]: "},
		{
			"a custom guard that passes leaves the allow on",
			config.Config{Allows: allowOn, CustomGuards: guardRunning("exit 0", "")},
			nil, "Bash", "allow", "belt[git-c-read]: ",
		},
		{
			"a deny wins over the allow",
			config.Config{Allows: allowOn, CustomGuards: guardRunning("echo no; exit 1", "")},
			nil, "Bash", "deny", "belt[external]: ",
		},
		// Review finding 3: a guard that lets the call through with a
		// warning is not a pass the allow may act on.
		{
			"a soft-mode guard that would deny",
			config.Config{Allows: allowOn, CustomGuards: guardRunning("echo no; exit 1", "soft")},
			nil, "Bash", "", "",
		},
		{
			"a custom guard that exits 2 fails open",
			config.Config{Allows: allowOn, CustomGuards: guardRunning("exit 2", "")},
			nil, "Bash", "", "",
		},
		{
			"a custom guard that cannot start fails open",
			config.Config{Allows: allowOn, CustomGuards: map[string]config.CustomGuard{
				"external": {Event: config.EventBash, Command: []string{"/nonexistent/belt-guard"}},
			}},
			nil, "Bash", "", "",
		},
		// Review finding 4: the payload around the command counts too.
		{
			"sandbox switched off",
			config.Config{Allows: allowOn},
			map[string]any{"dangerouslyDisableSandbox": true},
			"Bash", "", "",
		},
		{
			"another tool with a command field",
			config.Config{Allows: allowOn},
			nil,
			"Monitor",
			"",
			"",
		},
		{"allow off by default", config.Config{}, nil, "Bash", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := bashPayload(t, tt.tool, "git -C ~/code/src/x status", home, tt.extra)
			out, a := runGuards(raw, tt.cfg)
			assertDecision(t, out, tt.decision, tt.prefix)
			if tt.decision == "allow" && len(a.events) != 0 {
				t.Errorf("an approval archived %+v, want no event", a.events)
			}
		})
	}
}

// TestRunNoAllowAfterACustomGuardTimesOut: a custom guard that times out
// lets the call through with a warning, so the allow must stay silent
// (review finding 3). It waits out the real five-second guard timeout, so
// it runs in parallel and builds its paths without HOME.
func TestRunNoAllowAfterACustomGuardTimesOut(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "x")
	mkAllowRepo(t, repo)
	cfg := config.Config{
		Allows: map[string]config.Allow{guard.GitCReadID: {Enabled: true, Roots: []string{root}}},
		CustomGuards: map[string]config.CustomGuard{
			"slow": {Event: config.EventBash, Command: []string{"/bin/sleep", "30"}},
		},
	}
	out, _ := runGuards(bashPayload(t, "Bash", "git -C "+repo+" status", repo, nil), cfg)
	assertDecision(t, out, "", "")
}

// assertDecision checks what the guard entrypoint wrote: nothing when
// want is "", else one decision of that kind with the reason prefix.
func assertDecision(t *testing.T, out, want, prefix string) {
	t.Helper()
	if want == "" {
		if out != "" {
			t.Errorf("wrote %q, want nothing", out)
		}
		return
	}
	var d decision
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("decode decision from %q: %v", out, err)
	}
	got := d.HookSpecificOutput
	if got.HookEventName != "PreToolUse" || got.PermissionDecision != want ||
		!strings.HasPrefix(got.PermissionDecisionReason, prefix) {
		t.Errorf("decision = %+v, want %s with reason prefix %q", got, want, prefix)
	}
}
