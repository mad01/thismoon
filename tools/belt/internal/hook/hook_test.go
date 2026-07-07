package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The bash event with a work-profile push to main must emit a deny decision.
// Config.Load() reads real machine config, so hook-level tests only cover the
// payload plumbing paths that do not depend on it: malformed payloads and
// field mapping.

func TestRunMalformedPayload(t *testing.T) {
	var out bytes.Buffer
	Run("bash", strings.NewReader("not json"), &out)
	if out.Len() != 0 {
		t.Errorf("malformed payload must not produce a decision, got %q", out.String())
	}
}

func TestToInputBash(t *testing.T) {
	in := toInput("bash", payload{
		ToolInput: map[string]any{"command": "git push"},
		Cwd:       "/x",
	})
	if in.Command != "git push" || in.Cwd != "/x" || in.Event != "bash" {
		t.Errorf("unexpected input: %+v", in)
	}
}

func TestToInputWrite(t *testing.T) {
	in := toInput("write", payload{
		ToolInput: map[string]any{"file_path": "/repo/a.md", "content": "hello"},
	})
	if in.FilePath != "/repo/a.md" || in.Content != "hello" {
		t.Errorf("unexpected input: %+v", in)
	}
}

func TestToInputEditUsesNewString(t *testing.T) {
	in := toInput("write", payload{
		ToolInput: map[string]any{"file_path": "/repo/a.go", "new_string": "changed"},
	})
	if in.Content != "changed" {
		t.Errorf("Edit new_string not mapped: %+v", in)
	}
}

func TestDecisionShape(t *testing.T) {
	d := decision{HookSpecificOutput: hookOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "deny",
		PermissionDecisionReason: "belt[x]: nope",
	}}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hookSpecificOutput", "hookEventName", "permissionDecision", "permissionDecisionReason"} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("decision JSON missing %q: %s", key, raw)
		}
	}
}
