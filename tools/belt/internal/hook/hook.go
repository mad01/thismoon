// Package hook adapts Claude Code hook payloads to belt's two halves:
// PreToolUse payloads become guard inputs and guard denials become deny
// decisions, and PostToolUse payloads become hint inputs and hint advice
// becomes additionalContext. Both travel as JSON on stdout with exit 0 — a
// hook must not break tool calls on its own bugs.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// payload is the subset of the hook payload belt reads. ToolName, ToolInput,
// and ToolResponse are only populated on PreToolUse/PostToolUse; the session
// fields ride every hook event.
type payload struct {
	ToolName       string          `json:"tool_name"`
	ToolInput      map[string]any  `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	Cwd            string          `json:"cwd"`
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
}

type decision struct {
	HookSpecificOutput hookOutput `json:"hookSpecificOutput"`
}

type hookOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type adviceDecision struct {
	HookSpecificOutput adviceOutput `json:"hookSpecificOutput"`
}

type adviceOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// Load resolves belt's config for one hook invocation. It is injected so
// the caller owns where the config comes from (--config, BELT_CONFIG, the
// default path) and the hook owns what to do when it cannot be read.
type Load func() (config.Config, error)

// ConfigDenyID labels the denial a broken config produces, in the same
// belt[<id>] form the guards use.
const ConfigDenyID = "config"

// Run reads a payload from r, runs the guards for event, and writes a deny
// decision to w when one fires. It never returns an error for malformed
// payloads — a broken hook must not break tool calls.
//
// A config belt cannot read is the one exception: it denies. Every guard's
// rules live in that file, so continuing would run write-internal-names with
// no names and the commit guards with no rules while reporting nothing at
// all. The deny is loud on purpose — a wedged session with a clear reason
// beats a session that quietly stopped being guarded.
func Run(event string, load Load, r io.Reader, w io.Writer) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return
	}
	cfg, err := load()
	if err != nil {
		d := configDenial(err)
		notify.EmitEventSync("belt", "error", "blocked every tool call (unreadable config)", d.Reason,
			map[string]string{"guard": d.Guard, "event": event})
		writeDeny(w, d)
		return
	}
	in := toInput(event, p)
	d := guard.Run(in, cfg)
	if d == nil {
		return
	}
	notify.EmitEventSync("belt", "warn", fmt.Sprintf("blocked %s (%s)", p.ToolName, d.Guard), d.Reason,
		map[string]string{"guard": d.Guard, "event": event})
	writeDeny(w, d)
}

// writeDeny emits the deny decision. It travels in the JSON with exit 0: a
// non-zero exit is a hook error, which Claude Code reports and then lets the
// tool call through.
func writeDeny(w io.Writer, d *guard.Denial) {
	_ = json.NewEncoder(w).Encode(decision{
		HookSpecificOutput: hookOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: d.Reason,
		},
	})
}

// configDenial explains a config belt could not load, naming the file (the
// wrapped error carries the path) and the two ways out.
func configDenial(err error) *guard.Denial {
	return guard.Reasonf(ConfigDenyID,
		"belt cannot read its config, so every guarded tool call is blocked until it is fixed: %v. "+
			"Run `belt doctor` to see what belt resolved, then fix the file (or move it aside to fall back to the defaults).",
		err)
}

// RunHint reads a hook payload from r, runs the hints for event, and writes
// any advice as additionalContext. Silence is the common case: when no hint
// fires, nothing is written at all, so the model sees no extra context. Most
// hint events ride PostToolUse; session-start rides SessionStart, and the
// emitted hookEventName must match the hook that invoked belt or Claude Code
// drops the output.
func RunHint(event string, load Load, r io.Reader, w io.Writer) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return
	}
	cfg, err := load()
	if err != nil {
		// A hint has no denial path, so a broken config travels as advice:
		// the guards are already blocking every tool call, and this says why
		// on the events (a search or a prompt) they never see.
		writeAdvice(w, event, configDenial(err).Reason)
		return
	}
	advice := hint.Run(toHintInput(event, p), cfg)
	text := hint.Render(advice)
	if text == "" {
		return
	}
	for _, a := range advice {
		notify.EmitEventSync("belt", "info", fmt.Sprintf("hinted %s (%s)", p.ToolName, a.Hint), a.Text,
			map[string]string{"hint": a.Hint, "event": event})
	}
	writeAdvice(w, event, text)
}

// writeAdvice emits advice in the form the invoking hook event accepts.
// UserPromptSubmit adds plain stdout to context on exit 0; it is not in the
// hookSpecificOutput.additionalContext event family, so the JSON envelope
// would be dropped (or injected verbatim) there.
func writeAdvice(w io.Writer, event, text string) {
	if event == hint.EventPrompt {
		fmt.Fprintln(w, text)
		return
	}
	_ = json.NewEncoder(w).Encode(adviceDecision{
		HookSpecificOutput: adviceOutput{
			HookEventName:     hintHookEventName(event),
			AdditionalContext: text,
		},
	})
}

// hintHookEventName maps a belt hint event to the Claude Code hook event it
// is registered under.
func hintHookEventName(event string) string {
	if event == hint.EventSessionStart {
		return "SessionStart"
	}
	return "PostToolUse"
}

// toHintInput maps a PostToolUse payload onto the shared hint input. A search
// carries its repo and hit paths in the response; a bash command carries only
// what it ran; an external-text call is identified by the tool that made it.
func toHintInput(event string, p payload) hint.Input {
	in := hint.Input{Event: event, Cwd: p.Cwd, SessionID: p.SessionID, TranscriptPath: p.TranscriptPath}
	switch event {
	case hint.EventBash:
		if s, ok := p.ToolInput["command"].(string); ok {
			in.Command = s
		}
	case hint.EventExternalText:
		in.ToolName = p.ToolName
		in.ToolInput = p.ToolInput
	case hint.EventSearch:
		in.Paths = hint.PathsFromResponse(p.ToolResponse)
		in.Repo = hint.RepoFromResponse(p.ToolResponse)
		if in.Repo == "" {
			// Fall back to the request's repo filter, which is a regex or
			// substring rather than a resolved name — usable when it is
			// already a plain org/repo.
			if s, ok := p.ToolInput["repo"].(string); ok && !strings.ContainsAny(s, `.*+?[]()|\$^`) {
				in.Repo = s
			}
		}
	}
	return in
}

// toInput maps tool-specific payload fields onto the shared guard input.
// Write carries `content`; Edit carries `new_string` — the new text is what
// can leak, so that is what gets scanned. An external-text call carries its
// whole input: the guard decides which fields are text.
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
	case guard.EventExternalText:
		in.ToolName = p.ToolName
		in.ToolInput = p.ToolInput
	}
	return in
}
