package prereq

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryRefusesAnEmptyHome(t *testing.T) {
	failed := Failed(Factory(Env{Home: t.TempDir()}))
	if len(failed) != 6 {
		t.Fatalf("failed = %+v, want all six factory checks to fail", failed)
	}
	msg := Explain("factory", failed)
	for _, want := range []string{"factory arm cannot run", "factory-worker-medium", "MAD-398"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Explain() lacks %q:\n%s", want, msg)
		}
	}
}

func TestFactoryPassesWhenEverythingIsInstalled(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	writeFile(t, filepath.Join(claudeDir, "skills", "factory", "SKILL.md"), "x")
	writeFile(t, filepath.Join(claudeDir, "skills", "loom", "SKILL.md"), "x")
	for _, w := range WorkerDefinitions {
		writeFile(t, filepath.Join(claudeDir, "agents", w+".md"), "x")
	}
	writeFile(t, filepath.Join(claudeDir, "settings.json"), `{"hooks":{"SubagentStart":[`+
		`{"hooks":[{"type":"command","command":"python3 rules.py"}]},`+
		`{"hooks":[{"type":"command","command":"~/code/bin/belt hint subagent-start"}]}]}}`)
	if failed := Failed(Factory(Env{Home: home})); len(failed) != 0 {
		t.Errorf("failed = %+v, want none", failed)
	}
}

func TestSubagentHookMustRunBelt(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, p, `{"hooks":{"SubagentStart":[{"hooks":[{"command":"python3 rules.py"}]}],`+
		`"PreToolUse":[{"hooks":[{"command":"belt hook bash"}]}]}}`)
	if r := beltSubagentHook(p); r.OK {
		t.Errorf("beltSubagentHook() = %+v, want a failure: belt only runs on PreToolUse", r)
	}
}

func TestCommonChecksClaudeVersionAndTools(t *testing.T) {
	env := Env{
		LookPath: func(f string) (string, error) {
			if f == "humanizer" {
				return "", errors.New("not found")
			}
			return "/bin/" + f, nil
		},
		Claude: ClaudeProbe{Version: [3]int{2, 1, 200}, Raw: "2.1.200 (Claude Code)"},
	}
	failed := Failed(Common(env))
	if len(failed) != 2 || failed[0].Name != "claude version" || failed[1].Name != "humanizer" {
		t.Errorf("failed = %+v, want the old claude and the missing humanizer", failed)
	}
}

func TestOlderComparesFieldByField(t *testing.T) {
	tests := []struct {
		v    [3]int
		want bool
	}{
		{[3]int{2, 1, 259}, false},
		{[3]int{2, 1, 301}, false},
		{[3]int{2, 2, 0}, false},
		{[3]int{2, 1, 258}, true},
		{[3]int{1, 9, 999}, true},
	}
	for _, tt := range tests {
		if got := older(tt.v, MinClaude); got != tt.want {
			t.Errorf("older(%v) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

func TestSnapshotRecordsWhatIsInstalled(t *testing.T) {
	home := t.TempDir()
	// The factory skill is a symlink to its source dir, the way the recipe
	// installs it; the loom skill is not installed.
	src := filepath.Join(home, "src", "factory")
	writeFile(t, filepath.Join(src, "SKILL.md"), "factory v1\n")
	writeFile(t, filepath.Join(src, "worker-brief.md"), "brief v1\n")
	writeFile(t, filepath.Join(src, "evals", "brief", "results.json"), "{}\n")
	writeFile(t, filepath.Join(src, ".DS_Store"), "finder\n")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, ".claude", "skills", "factory")); err != nil {
		t.Fatal(err)
	}
	env := Env{Home: home, BeltVersion: func(context.Context) (string, error) {
		return "d430918\n", nil
	}}
	in := Snapshot(context.Background(), env)
	if len(in.Files) != 6 || in.BeltVersion != "d430918" {
		t.Fatalf("Snapshot() = %+v, want two skill files, the missing loom dir, "+
			"three worker definitions and the trimmed belt version", in)
	}
	for _, key := range []string{
		"~/.claude/skills/factory/SKILL.md", "~/.claude/skills/factory/worker-brief.md",
	} {
		if got := in.Files[key]; !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
			t.Errorf("%s = %q, want a sha256", key, got)
		}
	}
	for key, want := range map[string]string{
		"~/.claude/skills/loom/":                  "missing",
		"~/.claude/agents/factory-worker-high.md": "missing",
	} {
		if got := in.Files[key]; got != want {
			t.Errorf("%s = %q, want %s", key, got, want)
		}
	}
	env.BeltVersion = func(context.Context) (string, error) { return "", errors.New("not on PATH") }
	if in := Snapshot(context.Background(), env); in.BeltVersion != "unavailable: not on PATH" {
		t.Errorf("BeltVersion = %q, want the reason it is unavailable", in.BeltVersion)
	}
}
