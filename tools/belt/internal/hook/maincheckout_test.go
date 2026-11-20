package hook

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// subagentCall is a PreToolUse payload a subagent's tool call carries:
// the parent's session id plus the subagent's own agent_id and agent_type.
func subagentCall(t *testing.T, tool string, input map[string]any, cwd string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"tool_name":  tool,
		"tool_input": input,
		"cwd":        cwd,
		"session_id": "s1",
		"agent_id":   "agent-abc123",
		"agent_type": "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestMainCheckoutGuardsASubagentsCall: PreToolUse runs for a subagent's
// tool calls with the same payload shape plus its agent fields, so the
// guard judges an Edit or a git commit a subagent makes in a main checkout
// exactly as the main session's. In deny mode the call is refused; in hint
// mode, the default, it goes through with the advice as PreToolUse
// additionalContext and no permission decision, archived with the
// subagent's identity.
func TestMainCheckoutGuardsASubagentsCall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "repo")
	mkAllowRepo(t, main)
	edit := subagentCall(t, "Edit", map[string]any{
		"file_path": filepath.Join(main, "README.md"), "old_string": "a", "new_string": "b",
	}, dir)
	commit := subagentCall(t, "Bash", map[string]any{
		"command": "git -C " + main + " commit -m x",
	}, dir)
	deny := func() (config.Config, error) {
		return config.Config{Guards: map[string]config.Toggle{
			guard.MainCheckoutID: {Mode: config.ModeDeny},
		}}, nil
	}

	for _, tt := range []struct {
		event   string
		payload []byte
	}{{guard.EventWrite, edit}, {guard.EventBash, commit}} {
		t.Run(tt.event+" deny", func(t *testing.T) {
			var out bytes.Buffer
			var a archived
			runner{load: deny, emit: a.emit}.guards(tt.event, bytes.NewReader(tt.payload), &out)
			assertDecision(
				t,
				strings.TrimSpace(out.String()),
				"deny",
				"belt[main-checkout]: refused: ",
			)
			wantSubagentTags(t, a)
		})
		t.Run(tt.event+" hint", func(t *testing.T) {
			var out bytes.Buffer
			var a archived
			runner{load: defaults, emit: a.emit}.guards(tt.event, bytes.NewReader(tt.payload), &out)
			if strings.Contains(out.String(), "permissionDecision") {
				t.Errorf("hint mode wrote a permission decision: %s", out.String())
			}
			var got adviceDecision
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode advice from %q: %v", out.String(), err)
			}
			if got.HookSpecificOutput.HookEventName != "PreToolUse" {
				t.Errorf(
					"hookEventName = %q, want PreToolUse",
					got.HookSpecificOutput.HookEventName,
				)
			}
			text := got.HookSpecificOutput.AdditionalContext
			if !strings.HasPrefix(text, "belt[main-checkout]: ") ||
				!strings.Contains(text, "git -C "+main+" worktree add ~/.worktrees/repo/<slug>") {
				t.Errorf(
					"additionalContext = %q, want the main-checkout advice with the worktree command",
					text,
				)
			}
			wantSubagentTags(t, a)
		})
	}
}

// wantSubagentTags checks one archived event names the guard and the
// subagent that made the call.
func wantSubagentTags(t *testing.T, a archived) {
	t.Helper()
	if len(a.events) != 1 {
		t.Fatalf("archived %d events, want 1", len(a.events))
	}
	tags := a.events[0].Tags
	if tags["guard"] != guard.MainCheckoutID || tags["agent_id"] != "agent-abc123" ||
		tags["agent_type"] != "builder" {
		t.Errorf("event tags = %v, want the guard and the subagent's identity", tags)
	}
}
