package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// The config load is injected, so hook-level tests cover the payload
// plumbing (malformed payloads, field mapping) and the two config outcomes
// the entrypoint owns: defaults run the guards, an unreadable file denies.

// defaults is the load a machine with no config file gets.
func defaults() (config.Config, error) { return config.Config{}, nil }

// failing is the load a machine with a broken config file gets.
func failing() (config.Config, error) {
	return config.Config{}, errors.New(
		"config: parse /tmp/config.yaml: yaml: line 2: did not find expected key",
	)
}

func TestRunMalformedPayload(t *testing.T) {
	var out bytes.Buffer
	Run("bash", defaults, strings.NewReader("not json"), &out)
	if out.Len() != 0 {
		t.Errorf("malformed payload must not produce a decision, got %q", out.String())
	}
}

// TestRunDeniesOnUnreadableConfig pins the fail-closed entrypoint: belt
// cannot tell "no rules" from "the rules did not load", so it blocks and says
// which file to fix rather than letting the call through unguarded.
func TestRunDeniesOnUnreadableConfig(t *testing.T) {
	var out bytes.Buffer
	Run(
		"bash",
		failing,
		strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`),
		&out,
	)

	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("decode decision from %q: %v", out.String(), err)
	}
	if d.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want deny", d.HookSpecificOutput.PermissionDecision)
	}
	reason := d.HookSpecificOutput.PermissionDecisionReason
	for _, want := range []string{"belt[config]:", "/tmp/config.yaml", "belt doctor"} {
		if !strings.Contains(reason, want) {
			t.Errorf("deny reason %q does not mention %q", reason, want)
		}
	}
}

// TestRunHintAdvisesOnUnreadableConfig: a hint has no denial path, so the
// same failure has to arrive as advice on the events guards never see.
func TestRunHintAdvisesOnUnreadableConfig(t *testing.T) {
	var out bytes.Buffer
	RunHint("search", failing, strings.NewReader(`{"tool_name":"mcp__csl__csl_search"}`), &out)

	var a adviceDecision
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatalf("decode advice from %q: %v", out.String(), err)
	}
	if !strings.Contains(a.HookSpecificOutput.AdditionalContext, "belt[config]:") {
		t.Errorf("advice = %q, want the config failure", a.HookSpecificOutput.AdditionalContext)
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

func TestToInputExternalTextCarriesTheWholeCall(t *testing.T) {
	in := toInput("external-text", payload{
		ToolName:  "mcp__gh_com__create_pull_request",
		ToolInput: map[string]any{"owner": "o", "repo": "r", "body": "text"},
		Cwd:       "/x",
	})
	if in.ToolName != "mcp__gh_com__create_pull_request" || in.Cwd != "/x" {
		t.Errorf("unexpected input: %+v", in)
	}
	if body, _ := in.ToolInput["body"].(string); body != "text" {
		t.Errorf("tool input not mapped: %+v", in.ToolInput)
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

func TestToHintInputCarriesSessionFields(t *testing.T) {
	in := toHintInput("prompt", payload{
		Cwd:            "/x",
		SessionID:      "s1",
		TranscriptPath: "/t.jsonl",
	})
	if in.SessionID != "s1" || in.TranscriptPath != "/t.jsonl" || in.Cwd != "/x" {
		t.Errorf("unexpected hint input: %+v", in)
	}
}

// The prompt event must emit plain text: UserPromptSubmit adds stdout to
// context and is not in the hookSpecificOutput.additionalContext family.
func TestRunHintPromptEmitsPlainText(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	transcript := filepath.Join(t.TempDir(), "t.jsonl")
	line := `{"message":{"content":[{"type":"tool_use","name":"mcp__csl__csl_search"}]}}` + "\n"
	if err := os.WriteFile(transcript, []byte(strings.Repeat(line, 40)), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := json.Marshal(map[string]string{
		"session_id":      "prompt-test",
		"transcript_path": transcript,
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	RunHint("prompt", defaults, bytes.NewReader(p), &out)
	got := out.String()
	if !strings.HasPrefix(got, "belt[kof-deposit]:") {
		t.Fatalf("prompt advice = %q, want plain text starting with belt[kof-deposit]:", got)
	}
	if strings.Contains(got, "hookSpecificOutput") {
		t.Errorf("prompt advice %q must not be wrapped in the JSON envelope", got)
	}
}
