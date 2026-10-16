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
		ClaudeVersion: func(context.Context) ([3]int, string, error) {
			return [3]int{2, 1, 200}, "2.1.200 (Claude Code)", nil
		},
	}
	failed := Failed(Common(context.Background(), env))
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
