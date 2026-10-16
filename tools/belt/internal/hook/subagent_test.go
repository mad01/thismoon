package hook

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestRunHintSubagentStartBriefsTheWorker pins the subagent-start wiring end
// to end: a custom hint registered on the event runs on `belt hint
// subagent-start`, receives the subagent's id and type on stdin, and its
// advice comes back in the SubagentStart envelope, the name Claude Code
// needs to put it in the subagent's context. HOME and the events URL point
// away from the machine so nothing else speaks and nothing is archived.
func TestRunHintSubagentStartBriefsTheWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	dir := t.TempDir()
	captured := filepath.Join(dir, "payload.json")
	script := filepath.Join(dir, "brief.sh")
	body := "#!/bin/sh\ncat > " + captured + "\necho 'work only inside your worktree'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	load := func() (config.Config, error) {
		return config.Config{CustomHints: map[string]config.CustomHint{
			"worker-brief": {
				Event:     config.HintEventSubagentStart,
				Command:   []string{script},
				TimeoutMS: 10_000, // a fresh script's first launch can be slow under load
			},
		}}, nil
	}
	p, err := json.Marshal(map[string]string{
		"session_id": "s1", "cwd": dir, "agent_id": "agent-abc123", "agent_type": "Explore",
		"hook_event_name": "SubagentStart",
	})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	RunHint("subagent-start", load, bytes.NewReader(p), &out)

	var a adviceDecision
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatalf("decode advice from %q: %v", out.String(), err)
	}
	if a.HookSpecificOutput.HookEventName != "SubagentStart" {
		t.Errorf("hookEventName = %q, want SubagentStart", a.HookSpecificOutput.HookEventName)
	}
	if got, want := a.HookSpecificOutput.AdditionalContext,
		"belt[worker-brief]: work only inside your worktree"; got != want {
		t.Errorf("additionalContext = %q, want %q", got, want)
	}
	var stdin map[string]string
	raw, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &stdin); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"event": "subagent-start", "cwd": dir, "session_id": "s1", "transcript_path": "",
		"agent_id": "agent-abc123", "agent_type": "Explore",
	}
	if !maps.Equal(stdin, want) {
		t.Errorf("external stdin = %v, want %v", stdin, want)
	}
}

// TestRunHintSubagentStartGatesByAgentType pins agent_types end to end, from
// a config file through `belt hint subagent-start`: a spawn of an unlisted
// type prints nothing and never execs the command (the script leaves a
// marker when it runs), and a listed type gets the brief as before.
func TestRunHintSubagentStartGatesByAgentType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	tests := []struct {
		agentType string
		wantBrief bool
	}{
		{"Explore", false},
		{"factory-worker-high", true},
	}
	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			script := filepath.Join(dir, "brief.sh")
			body := "#!/bin/sh\ntouch " + marker + "\necho 'work only inside your worktree'\n"
			if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			cfgPath := filepath.Join(dir, "config.yaml")
			cfgYAML := "custom_hints:\n  worker-brief:\n    event: subagent-start\n" +
				"    command: [" + script + "]\n    timeout_ms: 10000\n" +
				"    agent_types: [factory-worker-low, factory-worker-medium, factory-worker-high]\n"
			if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o644); err != nil {
				t.Fatal(err)
			}
			load := func() (config.Config, error) {
				return config.LoadFrom(config.Paths{BeltYAML: cfgPath})
			}
			p, err := json.Marshal(map[string]string{
				"session_id": "s1", "cwd": dir, "agent_id": "agent-abc123",
				"agent_type": tt.agentType, "hook_event_name": "SubagentStart",
			})
			if err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			RunHint("subagent-start", load, bytes.NewReader(p), &out)

			_, statErr := os.Stat(marker)
			if ran := statErr == nil; ran != tt.wantBrief {
				t.Errorf("external ran = %v, want %v", ran, tt.wantBrief)
			}
			if !tt.wantBrief {
				if out.Len() != 0 {
					t.Errorf("stdout = %q, want nothing for an unlisted agent type", out.String())
				}
				return
			}
			var a adviceDecision
			if err := json.Unmarshal(out.Bytes(), &a); err != nil {
				t.Fatalf("decode advice from %q: %v", out.String(), err)
			}
			if got, want := a.HookSpecificOutput.AdditionalContext,
				"belt[worker-brief]: work only inside your worktree"; got != want {
				t.Errorf("additionalContext = %q, want %q", got, want)
			}
		})
	}
}

func TestHintAdviceFor(t *testing.T) {
	tests := []struct {
		event string
		want  hintAdvice
	}{
		{"session-start", hintAdvice{hookEvent: "SessionStart"}},
		{"subagent-start", hintAdvice{hookEvent: "SubagentStart"}},
		{"prompt", hintAdvice{hookEvent: "UserPromptSubmit", plain: true}},
		{"search", hintAdvice{hookEvent: "PostToolUse"}},
		{"bash", hintAdvice{hookEvent: "PostToolUse"}},
		{"external-text", hintAdvice{hookEvent: "PostToolUse"}},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			if got := hintAdviceFor(tt.event); got != tt.want {
				t.Errorf("hintAdviceFor(%q) = %+v, want %+v", tt.event, got, tt.want)
			}
		})
	}
}
