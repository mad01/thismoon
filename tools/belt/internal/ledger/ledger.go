// Package ledger turns Claude Code lifecycle hook events into entries for
// the local events service, so an orchestrator session and its workers
// leave one timeline: which subagent started and stopped where, which task
// completed, when a session compacted or ended. It is belt's third concept
// beside guards (deny) and hints (advise): a ledger entry never reaches the
// model and never steers the session. Build is pure; the hook entrypoint
// resolves the git location and posts the entry.
package ledger

import (
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// Event names match `belt record <event>`; each comment names the Claude
// Code hook it is registered under.
const (
	EventSubagentStart = "subagent-start" // hook: SubagentStart
	EventSubagentStop  = "subagent-stop"  // hook: SubagentStop
	EventTaskCompleted = "task-completed" // hook: TaskCompleted
	EventTeammateIdle  = "teammate-idle"  // hook: TeammateIdle
	EventPreCompact    = "pre-compact"    // hook: PreCompact
	EventPostCompact   = "post-compact"   // hook: PostCompact
	EventSessionEnd    = "session-end"    // hook: SessionEnd
)

// Source is the events-service source every entry posts under. It stays
// apart from the source belt's denies and hints use because the events
// service caps each source's in-memory history: a busy factory must not
// push guard denials out of view.
const Source = "belt-ledger"

// maxMessageBytes caps an entry's message. A subagent's final message can
// run to pages; the entry needs enough to recognize the outcome, and the
// full text stays in the transcript the entry's transcript tag names.
const maxMessageBytes = 500

// Entry levels. Every recorded event is info; an entry whose hook payload
// could not be decoded is warn, so a broken payload shows up in the
// timeline instead of as a sparse entry nobody questions.
const (
	levelInfo = "info"
	levelWarn = "warn"
)

// Input is the subset of a lifecycle hook payload an entry records. Each
// event carries only some of the fields; the rest stay empty.
type Input struct {
	SessionID      string
	TranscriptPath string // the session's transcript
	// AgentID and AgentType name the subagent the event is about; empty
	// for the main session.
	AgentID              string
	AgentType            string
	AgentTranscriptPath  string // subagent-stop: the subagent's own transcript
	LastAssistantMessage string // subagent-stop: the subagent's final text
	// StopHookActive marks a subagent-stop that fires again after a stop
	// hook already kept the subagent running once.
	StopHookActive bool
	TaskID         string // task-completed
	TaskSubject    string // task-completed
	TeammateName   string // task-completed, teammate-idle
	Trigger        string // pre-compact, post-compact: manual or auto
	Reason         string // session-end: why the session ended
	// PayloadError is the decode error of a hook payload belt could not
	// read, "" when it decoded. The entry is still built from whatever
	// arrived.
	PayloadError string
}

// spec describes one event: the entry title and the payload field that
// qualifies it, shown in parentheses when set.
type spec struct {
	name      string
	title     string
	qualifier func(Input) string
}

// catalog lists every recordable event in a fixed order, which Events and
// the CLI's error text follow.
var catalog = []spec{
	{EventSubagentStart, "subagent started", func(in Input) string { return in.AgentType }},
	{EventSubagentStop, "subagent stopped", func(in Input) string { return in.AgentType }},
	{EventTaskCompleted, "task completed", func(in Input) string { return in.TeammateName }},
	{EventTeammateIdle, "teammate idle", func(in Input) string { return in.TeammateName }},
	{EventPreCompact, "compaction starting", func(in Input) string { return in.Trigger }},
	{EventPostCompact, "compaction finished", func(in Input) string { return in.Trigger }},
	{EventSessionEnd, "session ended", func(in Input) string { return in.Reason }},
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

// Build turns one lifecycle event into its ledger entry. loc is where the
// event's working directory sits in git; its zero value leaves the repo,
// worktree, canonical and branch tags off.
func Build(event string, in Input, loc guard.Location) notify.Event {
	ev := notify.Event{
		Source:  Source,
		Level:   levelInfo,
		Title:   title(event, in),
		Message: truncate(message(in), maxMessageBytes),
		Tags:    tags(event, in, loc),
	}
	if in.PayloadError != "" {
		ev.Level = levelWarn
		ev.Message = "hook payload unreadable: " + in.PayloadError
	}
	return ev
}

// title names the event and its qualifier: "subagent stopped (Explore)".
// An event outside the catalog is rejected at the CLI; a direct caller
// still gets the event name as the title.
func title(event string, in Input) string {
	for _, s := range catalog {
		if s.name != event {
			continue
		}
		if q := s.qualifier(in); q != "" {
			return s.title + " (" + q + ")"
		}
		return s.title
	}
	return event
}

// message is the entry body: the subagent's final text when the event
// carries one, else the completed task's subject.
func message(in Input) string {
	if in.LastAssistantMessage != "" {
		return in.LastAssistantMessage
	}
	return in.TaskSubject
}

// tags carries every identifying field that is set, so the timeline can be
// filtered by session, agent, repo, worktree or branch. worktree is set
// only for a linked worktree; canonical names the main checkout either way.
func tags(event string, in Input, loc guard.Location) map[string]string {
	pairs := [][2]string{
		{"event", event},
		{"session_id", in.SessionID},
		{"agent_id", in.AgentID},
		{"agent_type", in.AgentType},
		{"repo", loc.Repo},
		{"worktree", linkedWorktree(loc)},
		{"canonical", loc.Canonical},
		{"branch", loc.Branch},
		{"task_id", in.TaskID},
		{"teammate", in.TeammateName},
		{"trigger", in.Trigger},
		{"reason", in.Reason},
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

// linkedWorktree is the working tree's root when it is a linked worktree,
// "" for the main checkout (whose root the canonical tag already names).
func linkedWorktree(loc guard.Location) string {
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
