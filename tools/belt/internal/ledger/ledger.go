// Package ledger turns Claude Code lifecycle hook events into entries for
// the local events service, so a session and the subagents it spawns
// leave one timeline: which subagent started and stopped where, which task
// completed, which tool call auto mode denied, when a session asked for
// attention, compacted or ended, and which turn died on an API error after
// its retries ran out. It is belt's third concept beside guards
// (deny) and hints (advise): a ledger entry never reaches the model and
// never steers the session (docs/adr/0024). Build is pure; the hook
// entrypoint resolves the git location and posts the entry.
package ledger

import (
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// Event names match `belt record <event>`; each comment names the Claude
// Code hook it is registered under.
const (
	EventSubagentStart    = "subagent-start"    // hook: SubagentStart
	EventSubagentStop     = "subagent-stop"     // hook: SubagentStop
	EventTaskCompleted    = "task-completed"    // hook: TaskCompleted
	EventTeammateIdle     = "teammate-idle"     // hook: TeammateIdle
	EventPermissionDenied = "permission-denied" // hook: PermissionDenied
	EventNotification     = "notification"      // hook: Notification
	EventPreCompact       = "pre-compact"       // hook: PreCompact
	EventPostCompact      = "post-compact"      // hook: PostCompact
	EventSessionEnd       = "session-end"       // hook: SessionEnd
	EventStopFailure      = "stop-failure"      // hook: StopFailure
)

// Source is the events-service source entries post under unless
// ledger.source names another. It stays apart from the source belt's
// denies and hints use because the events service caps each source's
// in-memory history: a session running many subagents must not push guard
// denials out of view.
const Source = config.DefaultLedgerSource

// maxMessageBytes caps an entry's message. A subagent's final message can
// run to pages; the entry needs enough to recognize the outcome, and the
// full text stays in the transcript the entry's transcript tag names.
const maxMessageBytes = 500

// Entry levels. Every recorded event is info except a failed turn, which is
// warn or error by its error type (failureLevel). An entry whose hook
// payload could not be decoded is warn, so a broken payload shows up in the
// timeline instead of as a sparse entry nobody questions.
const (
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

// unknownError is the error type a stop-failure entry names when the
// payload carries none. Claude Code reports an error it cannot classify as
// "unknown" itself, so a missing type reads the same way.
const unknownError = "unknown"

// noDetail is a stop-failure message when the payload carries neither the
// rendered error nor its details.
const noDetail = "No detail reported."

// transientErrors are the StopFailure error types where the API was
// reachable but gave up after exhausting its retries. A turn that died on
// one is a warn entry; every other type needs the user to act (auth,
// billing, a bad request) and is an error entry.
var transientErrors = map[string]bool{
	"rate_limit":             true,
	"overloaded":             true,
	"server_error":           true,
	"cloud_credential_error": true,
}

// Input is the subset of a lifecycle hook payload an entry records. Each
// event carries only some of the fields; the rest stay empty.
type Input struct {
	SessionID      string
	TranscriptPath string // the session's transcript
	// AgentID and AgentType name the subagent the event is about; empty
	// for the main session.
	AgentID             string
	AgentType           string
	AgentTranscriptPath string // subagent-stop: the subagent's own transcript
	// LastAssistantMessage is the subagent's final text on subagent-stop
	// and the rendered API error on stop-failure ("API Error: Rate limit
	// reached"); the hooks share the key.
	LastAssistantMessage string
	// StopHookActive marks a subagent-stop that fires again after a stop
	// hook already kept the subagent running once.
	StopHookActive bool
	TaskID         string // task-completed
	TaskSubject    string // task-completed
	TeammateName   string // task-completed, teammate-idle
	ToolName       string // permission-denied: the denied tool
	ToolUseID      string // permission-denied: the denied call
	// Reason is why the session ended (session-end) or why auto mode
	// denied the call (permission-denied); the hooks share the key.
	Reason              string
	NotificationType    string // notification: which kind fired, e.g. permission_prompt
	NotificationTitle   string // notification: optional heading
	NotificationMessage string // notification: the text
	Trigger             string // pre-compact, post-compact: manual or auto
	Error               string // stop-failure: the error type, e.g. rate_limit
	ErrorDetails        string // stop-failure: e.g. 429 Too Many Requests
	// PayloadError is the decode error of a hook payload belt could not
	// read, "" when it decoded. The entry is still built from whatever
	// arrived.
	PayloadError string
}

// spec describes one event: the entry title, the payload field that
// qualifies it (shown in parentheses when set), the field that becomes the
// entry message, nil for events that carry no text, and the entry level,
// nil for info.
type spec struct {
	name      string
	title     string
	qualifier func(Input) string
	message   func(Input) string
	level     func(Input) string
}

// catalog lists every recordable event in a fixed order, which Events and
// the CLI's error text follow.
var catalog = []spec{
	{EventSubagentStart, "subagent started", agentType, nil, nil},
	{EventSubagentStop, "subagent stopped", agentType, lastMessage, nil},
	{EventTaskCompleted, "task completed", teammate, taskSubject, nil},
	{EventTeammateIdle, "teammate idle", teammate, nil, nil},
	{EventPermissionDenied, "permission denied", toolName, reason, nil},
	{EventNotification, "notification", notificationType, notificationText, nil},
	{EventPreCompact, "compaction starting", trigger, nil, nil},
	{EventPostCompact, "compaction finished", trigger, nil, nil},
	{EventSessionEnd, "session ended", reason, nil, nil},
	{EventStopFailure, "turn failed", failureType, failureText, failureLevel},
}

func agentType(in Input) string        { return in.AgentType }
func teammate(in Input) string         { return in.TeammateName }
func toolName(in Input) string         { return in.ToolName }
func notificationType(in Input) string { return in.NotificationType }
func trigger(in Input) string          { return in.Trigger }
func reason(in Input) string           { return in.Reason }
func lastMessage(in Input) string      { return in.LastAssistantMessage }
func taskSubject(in Input) string      { return in.TaskSubject }

// notificationText is the notification's message under its title, when it
// has one: "Permission needed: Claude needs your permission".
func notificationText(in Input) string {
	if in.NotificationTitle == "" {
		return in.NotificationMessage
	}
	return in.NotificationTitle + ": " + in.NotificationMessage
}

// failureType is the error type a failed turn died on, trimmed, and
// unknownError when the payload names none.
func failureType(in Input) string {
	if e := strings.TrimSpace(in.Error); e != "" {
		return e
	}
	return unknownError
}

// failureText is the rendered API error with its details in parentheses,
// "API Error: Rate limit reached (429 Too Many Requests)", either one alone
// when the other is empty, and noDetail when both are.
func failureText(in Input) string {
	rendered := strings.TrimSpace(in.LastAssistantMessage)
	details := strings.TrimSpace(in.ErrorDetails)
	switch {
	case rendered != "" && details != "":
		return rendered + " (" + details + ")"
	case rendered != "":
		return rendered
	case details != "":
		return details
	}
	return noDetail
}

// failureLevel is warn for a transient error type and error for the rest.
func failureLevel(in Input) string {
	if transientErrors[failureType(in)] {
		return levelWarn
	}
	return levelError
}

// Events lists every valid record event. CLI validation and its error text
// read from here so an event cannot be added to the catalog and then be
// rejected at the command line.
func Events() []string {
	names := make([]string, len(catalog))
	for i, s := range catalog {
		names[i] = s.name
	}
	return names
}

// Build turns one lifecycle event into its ledger entry, under Source. loc
// is where the event's working directory sits in git; its zero value
// leaves the repo, worktree, canonical and branch tags off. declared is
// ledger.agent_types: a subagent-start entry whose agent type has an entry
// there is tagged with its model and effort.
func Build(event string, in Input, loc gitloc.Location, declared config.AgentTypes) notify.Event {
	s := lookup(event)
	ev := notify.Event{
		Source:  Source,
		Level:   level(s, in),
		Title:   title(s, in),
		Message: truncate(text(s, in), maxMessageBytes),
		Tags:    tags(event, in, loc, declared),
	}
	if in.PayloadError != "" {
		ev.Level = levelWarn
		ev.Message = "hook payload unreadable: " + in.PayloadError
	}
	return ev
}

// lookup finds an event's spec. An event outside the catalog is rejected
// at the CLI; a direct caller still gets an entry titled by the event name.
func lookup(event string) spec {
	for _, s := range catalog {
		if s.name == event {
			return s
		}
	}
	return spec{name: event, title: event}
}

// title names the event and its qualifier: "subagent stopped (Explore)".
func title(s spec, in Input) string {
	if s.qualifier == nil {
		return s.title
	}
	if q := s.qualifier(in); q != "" {
		return s.title + " (" + q + ")"
	}
	return s.title
}

// level is the entry level, info for an event whose spec sets none.
func level(s spec, in Input) string {
	if s.level == nil {
		return levelInfo
	}
	return s.level(in)
}

// text is the entry body, "" for an event that carries none.
func text(s spec, in Input) string {
	if s.message == nil {
		return ""
	}
	return s.message(in)
}

// tags carries every identifying field that is set, so the timeline can be
// filtered by session, agent, repo, worktree or branch. worktree is set
// only for a linked worktree; canonical names the main checkout either way.
// model and effort come from declared, on subagent-start only; error is
// set on stop-failure only, and always there.
func tags(
	event string, in Input, loc gitloc.Location, declared config.AgentTypes,
) map[string]string {
	d := declaredTags(event, in, declared)
	pairs := [][2]string{
		{"event", event},
		{"session_id", in.SessionID},
		{"agent_id", in.AgentID},
		{"agent_type", in.AgentType},
		{"model", d.Model},
		{"effort", d.Effort},
		{"repo", loc.Repo},
		{"worktree", linkedWorktree(loc)},
		{"canonical", loc.Canonical},
		{"branch", loc.Branch},
		{"task_id", in.TaskID},
		{"teammate", in.TeammateName},
		{"tool", in.ToolName},
		{"tool_use_id", in.ToolUseID},
		{"notification_type", in.NotificationType},
		{"trigger", in.Trigger},
		{"reason", in.Reason},
		{"error", errorTag(event, in)},
		{"transcript", transcript(in)},
		{"stop_hook_active", stopHookActive(in)},
	}
	out := make(map[string]string, len(pairs))
	for _, kv := range pairs {
		if kv[1] != "" {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

// declaredTags is the model and effort ledger.agent_types declares for the
// subagent a subagent-start entry is about. Every other event, an entry
// with no agent type, and a type with no entry get none: the map restates
// what an agent definition declares, and belt never guesses a value.
func declaredTags(event string, in Input, declared config.AgentTypes) config.AgentTypeTags {
	if event != EventSubagentStart || in.AgentType == "" {
		return config.AgentTypeTags{}
	}
	return declared.For(in.AgentType)
}

// errorTag is the error type of a failed turn, so the timeline can be
// filtered by it; "" on every other event, which carries no error field.
func errorTag(event string, in Input) string {
	if event != EventStopFailure {
		return ""
	}
	return failureType(in)
}

// linkedWorktree is the working tree's root when it is a linked worktree,
// "" for the main checkout (whose root the canonical tag already names).
// The root is git's toplevel for the payload cwd, never the cwd itself, so
// a cwd in a subdirectory still names the worktree. That cwd belongs to
// the session that fired the hook: for a spawn it can be the parent's, so
// the tag names where the event fired, not where the subagent will work.
func linkedWorktree(loc gitloc.Location) string {
	if !loc.Linked {
		return ""
	}
	return loc.Toplevel
}

// transcript names the transcript the event is about: the subagent's own
// when the event carries one, else the session's.
func transcript(in Input) string {
	if in.AgentTranscriptPath != "" {
		return in.AgentTranscriptPath
	}
	return in.TranscriptPath
}

// stopHookActive renders the flag as a tag value, present only when set.
func stopHookActive(in Input) string {
	if !in.StopHookActive {
		return ""
	}
	return "true"
}

// truncate cuts s to at most limit bytes without splitting a UTF-8
// sequence: a cut that would land inside a rune backs up to its start.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
