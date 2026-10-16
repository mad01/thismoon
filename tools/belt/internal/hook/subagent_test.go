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
