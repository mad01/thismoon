package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
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

// subagentPayload is a tool call made inside a subagent: the session fields
// plus the agent identity Claude Code adds there.
var subagentPayload = payload{
	ToolInput:      map[string]any{"command": "ls"},
	Cwd:            "/x",
	SessionID:      "s1",
	TranscriptPath: "/t.jsonl",
	AgentID:        "agent-abc123",
	AgentType:      "Explore",
}

// TestToHintInputCarriesSessionFields covers every hint event: the session
// and agent fields are not event-specific, so no event's switch case may
// drop them.
func TestToHintInputCarriesSessionFields(t *testing.T) {
	for _, event := range hint.Events() {
		t.Run(event, func(t *testing.T) {
			in := toHintInput(event, subagentPayload)
			if in.SessionID != "s1" || in.TranscriptPath != "/t.jsonl" || in.Cwd != "/x" ||
				in.AgentID != "agent-abc123" || in.AgentType != "Explore" {
				t.Errorf("unexpected hint input: %+v", in)
			}
		})
	}
}

func TestToInputCarriesSessionFields(t *testing.T) {
	for _, event := range guard.Events() {
		t.Run(event, func(t *testing.T) {
			in := toInput(event, subagentPayload)
			if in.SessionID != "s1" || in.AgentID != "agent-abc123" ||
				in.AgentType != "Explore" || in.Cwd != "/x" {
				t.Errorf("unexpected guard input: %+v", in)
			}
		})
	}
}

func TestWithSession(t *testing.T) {
	tests := []struct {
		name string
		p    payload
		want map[string]string
	}{
		{"no session adds nothing", payload{}, map[string]string{"guard": "g"}},
		{
			"main session adds only its id",
			payload{SessionID: "s1"},
			map[string]string{"guard": "g", "session_id": "s1"},
		},
		{
			"subagent adds its identity",
			subagentPayload,
			map[string]string{
				"guard": "g", "session_id": "s1", "agent_id": "agent-abc123", "agent_type": "Explore",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withSession(map[string]string{"guard": "g"}, tt.p)
			if !maps.Equal(got, tt.want) {
				t.Errorf("withSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRunDeniesDespiteMistypedOptionalFields pins that no field added for
// something other than a guard can disarm one. A guard drops a payload it
// cannot decode and allows the call, so a mistyped key must never fail the
// guard decode: the lifecycle fields live in recordPayload out of its
// reach, and the agent fields decode leniently. The push below is denied
// with no config, and has to stay denied whatever rides along with it.
func TestRunDeniesDespiteMistypedOptionalFields(t *testing.T) {
	tests := []struct {
		name  string
		extra string
	}{
		{
			"lifecycle fields",
			`"reason":{"nested":true},"trigger":7,"stop_hook_active":"yes","task_id":[1]`,
		},
		{"numeric agent_id", `"agent_id":7`},
		{"object agent_type", `"agent_type":{"name":"Explore"}`},
		{"both agent fields mistyped", `"agent_id":[1,2],"agent_type":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"tool_name":"Bash","tool_input":{"command":"git push origin main"},` +
				tt.extra + `}`
			var out bytes.Buffer
			Run("bash", defaults, strings.NewReader(raw), &out)

			var d decision
			if err := json.Unmarshal(out.Bytes(), &d); err != nil {
				t.Fatalf("decode decision from %q: %v", out.String(), err)
			}
			if d.HookSpecificOutput.PermissionDecision != "deny" {
				t.Errorf(
					"permissionDecision = %q, want deny",
					d.HookSpecificOutput.PermissionDecision,
				)
			}
		})
	}
}

// TestLenientStringKeepsOnlyStrings: a mistyped agent field is dropped to
// empty while a string survives, so a subagent's real identity still
// arrives.
func TestLenientStringKeepsOnlyStrings(t *testing.T) {
	var p payload
	raw := `{"session_id":"s1","agent_id":"agent-abc123","agent_type":7}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("decode error = %v, want none", err)
	}
	if p.AgentID != "agent-abc123" || p.AgentType != "" {
		t.Errorf(
			"agent_id, agent_type = %q, %q, want %q, \"\"",
			p.AgentID,
			p.AgentType,
			"agent-abc123",
		)
	}
}

// TestRecordSwallowsPanics pins the recover that makes the record path's
// exit 0 structural: a panic in the entry builder must neither escape
// (which would exit 2 and block the event) nor print a trace the model
// would read. Without the recover this test crashes the test binary.
func TestRecordSwallowsPanics(t *testing.T) {
	panicking := func(string, recordPayload, error) notify.Event { panic("builder bug") }
	got := captureStdio(t, func() {
		record(ledger.EventSubagentStop, strings.NewReader(`{"session_id":"s1"}`), panicking)
	})
	if got != "" {
		t.Errorf("record wrote %q after a panic, want nothing", got)
	}
}

func TestToLedgerInput(t *testing.T) {
	raw := `{
		"session_id": "s1",
		"transcript_path": "/t/session.jsonl",
		"cwd": "/x",
		"hook_event_name": "SubagentStop",
		"stop_hook_active": true,
		"agent_id": "def456",
		"agent_type": "Explore",
		"agent_transcript_path": "/t/agent-def456.jsonl",
		"last_assistant_message": "Analysis complete.",
		"task_id": "task-001",
		"task_subject": "Implement auth",
		"teammate_name": "implementer",
		"trigger": "auto",
		"reason": "clear"
	}`
	var p recordPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	want := ledger.Input{
		SessionID:            "s1",
		TranscriptPath:       "/t/session.jsonl",
		AgentID:              "def456",
		AgentType:            "Explore",
		AgentTranscriptPath:  "/t/agent-def456.jsonl",
		LastAssistantMessage: "Analysis complete.",
		StopHookActive:       true,
		TaskID:               "task-001",
		TaskSubject:          "Implement auth",
		TeammateName:         "implementer",
		Trigger:              "auto",
		Reason:               "clear",
	}
	if got := toLedgerInput(p); got != want {
		t.Errorf("toLedgerInput() = %+v, want %+v", got, want)
	}
}

// TestRunRecordWritesNothing pins the record path's output contract on
// every event: whatever the payload, nothing reaches stdout or stderr,
// because any output on SubagentStop keeps the subagent running. The
// process streams are captured, since RunRecord takes no writer to check.
func TestRunRecordWritesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	payloads := map[string]string{
		"full payload": `{"session_id":"s1","cwd":"` + cwd + `","agent_id":"a1",` +
			`"agent_type":"Explore","last_assistant_message":"done"}`,
		"empty stdin":       "",
		"malformed payload": "not json",
		"wrong field type":  `{"session_id":"s1","stop_hook_active":"yes"}`,
	}
	for _, event := range ledger.Events() {
		for name, raw := range payloads {
			t.Run(event+"/"+name, func(t *testing.T) {
				got := captureStdio(t, func() { RunRecord(event, strings.NewReader(raw)) })
				if got != "" {
					t.Errorf("RunRecord(%s) wrote %q, want nothing", event, got)
				}
			})
		}
	}
}

// captureStdio runs fn with the process stdout and stderr pointed at one
// pipe and returns everything written to either.
func captureStdio(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	read := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		read <- string(b)
	}()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return <-read
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

// TestRunHintSessionStartRunsCustomHints pins the wiring every harness
// shares: a custom hint registered on session-start rides `belt hint
// session-start` and lands in the SessionStart additionalContext envelope
// with the usual prefix. Any harness that wires that entry (Claude Code
// today, Codex once its hooks file carries it) gets the hint with no extra
// wiring. HOME and the events URL are pointed away from the machine so the
// built-in session-start hints stay silent and nothing is archived.
func TestRunHintSessionStartRunsCustomHints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	dir := t.TempDir()
	script := filepath.Join(dir, "journal.sh")
	body := "#!/bin/sh\necho 'yesterday: 2 open actions'\necho '  - review the release PR'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	load := func() (config.Config, error) {
		return config.Config{CustomHints: map[string]config.CustomHint{
			"daily-journal": {
				Event:     "session-start",
				Command:   []string{script},
				TimeoutMS: 10_000, // a fresh script's first launch can be slow under load
			},
		}}, nil
	}
	p, err := json.Marshal(map[string]string{"session_id": "s1", "cwd": dir})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	RunHint("session-start", load, bytes.NewReader(p), &out)

	var a adviceDecision
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatalf("decode advice from %q: %v", out.String(), err)
	}
	if a.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want SessionStart", a.HookSpecificOutput.HookEventName)
	}
	want := "belt[daily-journal]: yesterday: 2 open actions\n  - review the release PR"
	if got := a.HookSpecificOutput.AdditionalContext; !strings.Contains(got, want) {
		t.Errorf("additionalContext = %q, want it to contain %q", got, want)
	}
}
