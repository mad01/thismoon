// Package hook adapts Claude Code hook payloads to belt's three concepts.
// PreToolUse payloads become guard inputs and guard denials become deny
// decisions. PostToolUse, SessionStart and UserPromptSubmit payloads become
// hint inputs and hint advice becomes added context. Lifecycle payloads
// (subagents, tasks, compaction, session end, failed turns) become ledger
// entries and print nothing at all. Every entrypoint exits 0: a hook must
// not break tool calls, or steer a session, on its own bugs. A payload belt
// cannot read is denied on the guard path, like a config belt cannot read:
// no guard can check a call it cannot see (see decodePayload).
package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

// payload is the subset of the hook payload belt reads. ToolName, ToolInput,
// and ToolResponse are only populated on PreToolUse/PostToolUse; the session
// fields ride every hook event. The typed fields are the ones the guards
// read, and a value of another JSON type fails the decode, which denies the
// call (see decodePayload).
type payload struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    map[string]any  `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	Cwd          string          `json:"cwd"`
	// The session fields ride every event, and AgentID and AgentType every
	// event that fires inside a subagent (its tool calls included) and the
	// subagent lifecycle events; both are empty in the main session. They
	// decode leniently: no guard reads them, so a value of the wrong type
	// must not deny a call (see lenientString).
	SessionID      lenientString `json:"session_id"`
	TranscriptPath lenientString `json:"transcript_path"`
	AgentID        lenientString `json:"agent_id"`
	AgentType      lenientString `json:"agent_type"`
}

// lenientString is a payload field that keeps a JSON string and ignores any
// other JSON value, leaving the field empty instead of failing the whole
// payload decode. A guard entrypoint denies a payload it cannot decode, so
// every field added for something other than a guard decodes this way.
type lenientString string

// UnmarshalJSON keeps a string value and drops anything else. It never
// returns an error: the decoder has already checked the value is valid JSON.
func (s *lenientString) UnmarshalJSON(raw []byte) error {
	var v string
	if json.Unmarshal(raw, &v) == nil {
		*s = lenientString(v)
	}
	return nil
}

// recordPayload is what the ledger reads from a lifecycle event: the shared
// fields plus the ones only lifecycle events carry. They live here rather
// than in payload because a guard entrypoint denies a payload it cannot
// decode: a field the guards never read must not be able to fail their
// decode. Each is set only by the events that carry it: SubagentStop
// the first three, TaskCompleted the task fields, TaskCompleted and
// TeammateIdle the teammate name, PermissionDenied the tool use id and a
// reason (beside the shared tool_name), Notification the message, title
// and type, PreCompact and PostCompact the trigger, SessionEnd a reason,
// StopFailure the error type and details beside last_assistant_message.
type recordPayload struct {
	payload
	AgentTranscriptPath  string `json:"agent_transcript_path"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
	TaskID               string `json:"task_id"`
	TaskSubject          string `json:"task_subject"`
	TeammateName         string `json:"teammate_name"`
	ToolUseID            string `json:"tool_use_id"`
	Message              string `json:"message"`
	Title                string `json:"title"`
	NotificationType     string `json:"notification_type"`
	Trigger              string `json:"trigger"`
	Reason               string `json:"reason"`
	Error                string `json:"error"`
	ErrorDetails         string `json:"error_details"`
}

// Record budgets. SubagentStart holds the subagent's first turn until its
// hooks return, and SessionEnd gives all of its hooks 1.5 s in total, so
// each step of a record is bounded: placing the event in git (two git
// processes a stalled filesystem could hold) and the ledger POST, for which
// the one-second default emit budget is too generous. Together they stay
// inside SessionEnd's total.
const (
	locateBudget = time.Second
	recordBudget = 300 * time.Millisecond
)

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

// emitFunc archives one event to the events service.
type emitFunc func(source, level, title, message string, tags map[string]string)

// runner is the guard and hint entrypoints with their effects injected: the
// config load and the event emit, so a test can see what a run would
// archive (notify posts nothing under go test).
type runner struct {
	load Load
	emit emitFunc
}

// Run reads a payload from r, runs the guards for event, and writes a deny
// decision to w when one fires. It never returns an error: the deny is the
// only output, and it travels as data.
//
// Two failures deny before any guard runs, because a guard that cannot see
// its input or its rules would allow while reporting nothing at all. A
// payload belt cannot read (not a JSON object, or a field the guards read
// arriving with the wrong JSON type, see decodePayload) denies with
// belt[payload]. A config belt cannot read denies with belt[config]: every
// guard's rules live in that file, so continuing would run
// write-internal-names with no names and the commit guards with no rules.
// Both denies are loud on purpose: a wedged session with a clear reason
// beats a session that quietly stopped being guarded.
func Run(event string, load Load, r io.Reader, w io.Writer) {
	runner{load: load, emit: notify.EmitEventSync}.guards(event, r, w)
}

func (rn runner) guards(event string, r io.Reader, w io.Writer) {
	p, err := decodePayload(r, guardInputFields[event])
	if err != nil {
		d := payloadDenial(err)
		rn.emit(
			"belt",
			"warn",
			"blocked a tool call (unreadable payload)",
			d.Reason,
			withSession(map[string]string{
				"guard": d.Guard, "event": event, decodeErrorTag: err.Error(),
			}, p),
		)
		writeDeny(w, d)
		return
	}
	cfg, err := rn.load()
	if err != nil {
		d := configDenial(err)
		rn.emit(
			"belt",
			"error",
			"blocked every tool call (unreadable config)",
			d.Reason,
			withSession(map[string]string{"guard": d.Guard, "event": event}, p),
		)
		writeDeny(w, d)
		return
	}
	in := toInput(event, p)
	d := guard.Run(in, cfg)
	if d == nil {
		return
	}
	rn.emit(
		"belt",
		"warn",
		fmt.Sprintf("blocked %s (%s)", p.ToolName, d.Guard),
		d.Reason,
		withSession(map[string]string{"guard": d.Guard, "event": event}, p),
	)
	writeDeny(w, d)
}

// withSession adds who made the call to an event's tags: the session id
// and, inside a subagent, the agent id and type. Empty values stay off, so
// a main-session event gains only session_id.
func withSession(tags map[string]string, p payload) map[string]string {
	p.identity().AddTo(tags)
	return tags
}

// identity is who made the call this payload describes.
func (p payload) identity() guard.Identity {
	return guard.Identity{
		SessionID: string(p.SessionID),
		AgentID:   string(p.AgentID),
		AgentType: string(p.AgentType),
	}
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
	return guard.Reasonf(
		ConfigDenyID,
		"belt cannot read its config, so every guarded tool call is blocked until it is fixed: %v. "+
			"Run `belt doctor` to see what belt resolved, then fix the file (or move it aside to fall back to the defaults).",
		err,
	)
}

// RunHint reads a hook payload from r, runs the hints for event, and writes
// any advice as additionalContext. Silence is the common case: when no hint
// fires, nothing is written at all, so the model sees no extra context. Most
// hint events ride PostToolUse; the start events ride SessionStart and
// SubagentStart, and the emitted hookEventName must match the hook that
// invoked belt or Claude Code drops the output (see hintAdviceByEvent).
//
// A payload belt cannot read (see decodePayload) runs no hint and writes
// nothing: a hint has no denial path, and the tool call it would advise on
// has already run. It leaves a warn event naming the decode error instead,
// so the silence is visible on the timeline.
func RunHint(event string, load Load, r io.Reader, w io.Writer) {
	runner{load: load, emit: notify.EmitEventSync}.hints(event, r, w)
}

func (rn runner) hints(event string, r io.Reader, w io.Writer) {
	p, err := decodePayload(r, hintInputFields[event])
	if err != nil {
		rn.emit(
			"belt",
			"warn",
			"skipped the hints (unreadable payload)",
			"belt could not read the hook payload, so no hint ran: "+err.Error(),
			withSession(map[string]string{"event": event, decodeErrorTag: err.Error()}, p),
		)
		return
	}
	cfg, err := rn.load()
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
		rn.emit(
			"belt",
			"info",
			fmt.Sprintf("hinted %s (%s)", p.ToolName, a.Hint),
			a.Text,
			withSession(map[string]string{"hint": a.Hint, "event": event}, p),
		)
	}
	writeAdvice(w, event, text)
}

// RunRecord reads a lifecycle hook payload from r and records the event to
// the events ledger. It takes no writer on purpose. On SubagentStop any
// additionalContext keeps the subagent running, and TaskCompleted,
// TeammateIdle and PreCompact treat exit 2 as a block, so the record path
// must print nothing and fail nothing; the signature makes that structural,
// the way a hint's interface has no denial path (docs/adr/0024). The config
// decides only whether and where entries post (ledger.enabled,
// ledger.source) and which model and effort a subagent-start entry carries
// (ledger.agent_types); a broken config file records with the defaults,
// because the ledger is not a guard and an entry is harmless. A missing or
// unreadable payload is still recorded, at warn level, from whatever
// fields arrived. That entry is the warn event for the payload: the record
// path can neither deny nor print, and with ledger.enabled false it posts
// nothing at all, a payload warning included. The session and agent fields
// decode leniently here too (recordPayload embeds payload), so a mistyped
// session_id or transcript_path is dropped silently and the entry stays at
// info with no warning.
func RunRecord(event string, load Load, r io.Reader) {
	recorder{load: load, build: buildEntry, emit: postEntry}.run(event, r)
}

// entryBuilder turns a decoded lifecycle payload into its ledger entry.
// declared is ledger.agent_types from the config in effect.
type entryBuilder func(
	event string, p recordPayload, decodeErr error, declared config.AgentTypes,
) notify.Event

// recorder is the record path with its effects injected, so a test can see
// the entry that would post, and prove a panic is swallowed.
type recorder struct {
	load  Load
	build entryBuilder
	emit  func(notify.Event)
}

// configTag marks an entry recorded with the defaults because the config
// could not be read, so the fallback is visible in the timeline.
const configTag = "config"

// run records one event. Its deferred recover is what makes "prints
// nothing, exits 0" hold for bugs too: an escaped panic exits 2 with a
// stack trace on stderr, which blocks TaskCompleted, TeammateIdle,
// PreCompact and SubagentStop and hands the trace to the model. The panic
// report follows the ledger settings once the config has loaded, so a
// disabled ledger posts nothing at all; before that it uses the defaults.
func (rc recorder) run(event string, r io.Reader) {
	var settings config.Ledger // the defaults until the config loads
	defer func() {
		if v := recover(); v != nil && settings.On() {
			rc.reportPanic(event, settings.SourceName(), v)
		}
	}()
	cfg, cfgErr := rc.load() // on error cfg holds the defaults: record anyway
	settings = cfg.Ledger
	if !settings.On() {
		// Read the payload anyway: a producer still writing a large one
		// (a long final message on SubagentStop) would fail on a closed pipe.
		_, _ = io.Copy(io.Discard, r)
		return
	}
	var p recordPayload
	decodeErr := json.NewDecoder(r).Decode(&p)
	ev := rc.build(event, p, decodeErr, settings.AgentTypes)
	ev.Source = settings.SourceName()
	if cfgErr != nil {
		if ev.Tags == nil {
			ev.Tags = map[string]string{}
		}
		ev.Tags[configTag] = "unreadable, recorded with the defaults"
	}
	rc.emit(ev)
}

// postEntry posts one entry to the events service within recordBudget.
func postEntry(ev notify.Event) {
	notify.EmitEventSyncWithin(recordBudget, ev.Source, ev.Level, ev.Title, ev.Message, ev.Tags)
}

// buildEntry places the event's working directory in git, under
// locateBudget, and builds the entry.
func buildEntry(
	event string, p recordPayload, decodeErr error, declared config.AgentTypes,
) notify.Event {
	in := toLedgerInput(p)
	if decodeErr != nil {
		in.PayloadError = decodeErr.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), locateBudget)
	defer cancel()
	return ledger.Build(event, in, guard.LocationAt(ctx, p.Cwd), declared)
}

// reportPanic records a swallowed panic under source, so a broken record
// path shows in the timeline instead of as missing entries. It recovers on
// its own account: a second panic from the emit is dropped as well.
func (rc recorder) reportPanic(event, source string, v any) {
	defer func() { _ = recover() }()
	rc.emit(notify.Event{
		Source:  source,
		Level:   "error",
		Title:   "belt record failed",
		Message: fmt.Sprint(v),
		Tags:    map[string]string{"event": event},
	})
}

// toLedgerInput maps a lifecycle payload onto the ledger input.
func toLedgerInput(p recordPayload) ledger.Input {
	return ledger.Input{
		SessionID:            string(p.SessionID),
		TranscriptPath:       string(p.TranscriptPath),
		AgentID:              string(p.AgentID),
		AgentType:            string(p.AgentType),
		AgentTranscriptPath:  p.AgentTranscriptPath,
		LastAssistantMessage: p.LastAssistantMessage,
		StopHookActive:       p.StopHookActive,
		TaskID:               p.TaskID,
		TaskSubject:          p.TaskSubject,
		TeammateName:         p.TeammateName,
		ToolName:             p.ToolName,
		ToolUseID:            p.ToolUseID,
		Reason:               p.Reason,
		NotificationType:     p.NotificationType,
		NotificationTitle:    p.Title,
		NotificationMessage:  p.Message,
		Trigger:              p.Trigger,
		Error:                p.Error,
		ErrorDetails:         p.ErrorDetails,
	}
}

// writeAdvice emits advice in the form the invoking hook event accepts.
func writeAdvice(w io.Writer, event, text string) {
	out := hintAdviceFor(event)
	if out.plain {
		fmt.Fprintln(w, text)
		return
	}
	_ = json.NewEncoder(w).Encode(adviceDecision{
		HookSpecificOutput: adviceOutput{
			HookEventName:     out.hookEvent,
			AdditionalContext: text,
		},
	})
}

// hintAdvice is how one hint event's advice must travel: the Claude Code
// hook the event is registered under, whose name the JSON envelope has to
// carry or Claude Code drops the output, and whether that hook reads plain
// stdout instead of the envelope.
type hintAdvice struct {
	hookEvent string
	plain     bool
}

// hintAdviceByEvent lists the hint events that do not ride PostToolUse.
// UserPromptSubmit adds plain stdout to context on exit 0; it is not in the
// hookSpecificOutput.additionalContext event family, so the JSON envelope
// would be dropped (or injected verbatim) there.
var hintAdviceByEvent = map[string]hintAdvice{
	hint.EventSessionStart:  {hookEvent: "SessionStart"},
	hint.EventSubagentStart: {hookEvent: "SubagentStart"},
	hint.EventPrompt:        {hookEvent: "UserPromptSubmit", plain: true},
}

// hintAdviceFor maps a hint event to how its advice travels; every event
// not in the table rides PostToolUse.
func hintAdviceFor(event string) hintAdvice {
	if out, ok := hintAdviceByEvent[event]; ok {
		return out
	}
	return hintAdvice{hookEvent: "PostToolUse"}
}

// toHintInput maps a PostToolUse payload onto the shared hint input. A search
// carries its repo and hit paths in the response; a bash command carries only
// what it ran; an external-text call is identified by the tool that made it.
func toHintInput(event string, p payload) hint.Input {
	in := hint.Input{
		Event:          event,
		Cwd:            p.Cwd,
		SessionID:      string(p.SessionID),
		TranscriptPath: string(p.TranscriptPath),
		AgentID:        string(p.AgentID),
		AgentType:      string(p.AgentType),
	}
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
			if s, ok := p.ToolInput["repo"].(string); ok &&
				!strings.ContainsAny(s, `.*+?[]()|\$^`) {
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
	in := guard.Input{
		Event:     event,
		Cwd:       p.Cwd,
		SessionID: string(p.SessionID),
		AgentID:   string(p.AgentID),
		AgentType: string(p.AgentType),
	}
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
