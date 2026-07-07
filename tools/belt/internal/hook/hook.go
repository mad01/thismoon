// Package hook adapts Claude Code PreToolUse payloads to guard inputs and
// guard denials back to hook decisions. The deny is carried by JSON on
// stdout with exit 0 — same contract as the other hooks in this repo.
package hook

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/notify"
)

// payload is the subset of the PreToolUse hook payload belt reads.
type payload struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Cwd       string         `json:"cwd"`
}

type decision struct {
	HookSpecificOutput hookOutput `json:"hookSpecificOutput"`
}

type hookOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Run reads a payload from r, runs the guards for event, and writes a deny
// decision to w when one fires. It never returns an error for malformed
// payloads — a broken hook must not break tool calls.
func Run(event string, r io.Reader, w io.Writer) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return
	}
	in := toInput(event, p)
	d := guard.Run(in, config.Load())
	if d == nil {
		return
	}
	notify.EmitEvent("belt", "warn", fmt.Sprintf("blocked %s (%s)", p.ToolName, d.Guard), d.Reason,
		map[string]string{"guard": d.Guard, "event": event})
	_ = json.NewEncoder(w).Encode(decision{
		HookSpecificOutput: hookOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: d.Reason,
		},
	})
}

// toInput maps tool-specific payload fields onto the shared guard input.
// Write carries `content`; Edit carries `new_string` — the new text is what
// can leak, so that is what gets scanned.
func toInput(event string, p payload) guard.Input {
	in := guard.Input{Event: event, Cwd: p.Cwd}
	str := func(key string) string {
		s, _ := p.ToolInput[key].(string)
		return s
	}
	switch event {
	case guard.EventBash:
		in.Command = str("command")
	case guard.EventWrite:
		in.FilePath = str("file_path")
		in.Content = str("content")
		if in.Content == "" {
			in.Content = str("new_string")
		}
	}
	return in
}
