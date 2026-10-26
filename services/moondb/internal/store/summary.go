package store

import (
	"cmp"
	"iter"
	"slices"
	"time"
)

// SessionSummary is a session with what its records add up to: the counts,
// times and tokens a listing shows. Every backend builds it with
// Summarize, from records alone, so a listing never reads a chunk and the
// counts mean the same in every backend.
type SessionSummary struct {
	Session
	// Repos are the repository identities the session edited, sorted.
	Repos []string `json:"repos"`
	// Worktrees are the checkout roots the session edited, sorted. An edit
	// outside any repository adds none.
	Worktrees []string `json:"worktrees"`
	TurnCount int      `json:"turn_count"`
	EditCount int      `json:"edit_count"`
	// FileCount counts the distinct files the session edited, which Files
	// names. Files stays out of the JSON: a listing wants the count, and a
	// reader that sums several sessions wants the names to count a file
	// once.
	FileCount int       `json:"file_count"`
	Files     []FileKey `json:"-"`
	// AgentCount counts the agents that made an edit, the main
	// conversation among them.
	AgentCount int `json:"agent_count"`
	// LastTurnOpen is set when the session's last turn by Seq is open.
	LastTurnOpen bool `json:"last_turn_open,omitempty"`
	// LastActivityAt is the latest turn start, turn end, edit or agent
	// stop, or the session's start when it has none of them.
	LastActivityAt time.Time `json:"last_activity_at"`
	// DurationMS is the session's wall span, the pauses between turns
	// included: from the first turn's start to the last turn's end. An
	// open turn ends at its last activity, the latest edit or agent stop
	// at or after its start and before the next turn starts. It reads the
	// turn records only, no transcript lines, so it can fall short of the
	// session trace's wall time, whose turn spans stretch to a line
	// written after the recorded end.
	DurationMS int64 `json:"duration_ms"`
	// ActiveMS is the time inside the session's turns that did not wait
	// for input, the session's active time. A turn with totals gives its
	// wall time less its user time; one without gives its recorded span.
	// An open turn ends where DurationMS ends it, at its last activity.
	// A turn never counts below zero, and the gaps between turns never
	// count, so a session left open overnight adds nothing for the night.
	// It never exceeds DurationMS: turns that overlap, or totals whose
	// wall outlasts the recorded span, are capped at the wall span.
	ActiveMS int64 `json:"active_ms"`
	// LastTurnEnd is where DurationMS ends: the latest turn end, an open
	// turn ending at its last activity; zero without turns. It stays out
	// of the JSON, where a listing row says it as ended_at.
	LastTurnEnd time.Time `json:"-"`
	// Usage merges the usage of every turn and every agent with totals
	// (MergeUsage): MainUsage and AgentUsage together.
	Usage []ModelUsage `json:"usage,omitempty"`
	// MainUsage merges the usage of the turns, the main conversation's
	// model calls, and AgentUsage that of the Agent records. They stay out
	// of the JSON, where a listing row prices them as cost_main and
	// cost_agents.
	MainUsage  []ModelUsage `json:"-"`
	AgentUsage []ModelUsage `json:"-"`
	// AgentLabels names the subagents that ran, each label once: the
	// Agent records in SortAgents order (Agent.Label), then the agent
	// type of each agent known only from its edits.
	AgentLabels []string `json:"-"`
}

// FileKey names one edited file: its worktree and its path there, or ""
// and an absolute path for a file outside any repository.
type FileKey struct {
	Worktree string
	Path     string
}

// Summarize adds up a session's turns, agents and edits. agents come in
// SortAgents order; edits may come in any order. Summarize only reads
// them. AgentCount counts every agent an Agent record or an edit names,
// the main conversation among them when it edited.
func Summarize(s Session, turns []Turn, agents []Agent, edits iter.Seq[*Edit]) SessionSummary {
	sum := SessionSummary{
		Session:        s,
		Repos:          []string{},
		Worktrees:      []string{},
		Files:          []FileKey{},
		TurnCount:      len(turns),
		LastActivityAt: s.StartedAt,
	}
	addTurns(&sum, turns)
	seen := addAgents(&sum, agents)
	times := newTurnTimes(turns)
	for _, a := range agents {
		times.see(a.EndedAt)
	}
	files := map[FileKey]bool{}
	for e := range edits {
		times.see(e.At)
		sum.EditCount++
		if !seen[e.AgentID] && e.AgentID != "" {
			sum.AgentLabels = appendNew(sum.AgentLabels, cmp.Or(e.AgentType, "subagent"))
		}
		seen[e.AgentID] = true
		files[FileKey{Worktree: e.Worktree, Path: e.Path}] = true
		sum.Repos = appendNew(sum.Repos, e.Repo)
		sum.Worktrees = appendNew(sum.Worktrees, e.Worktree)
		sum.LastActivityAt = later(sum.LastActivityAt, e.At)
	}
	slices.Sort(sum.Repos)
	slices.Sort(sum.Worktrees)
	for f := range files {
		sum.Files = append(sum.Files, f)
	}
	slices.SortFunc(sum.Files, func(a, b FileKey) int {
		return cmp.Or(cmp.Compare(a.Worktree, b.Worktree), cmp.Compare(a.Path, b.Path))
	})
	sum.FileCount = len(sum.Files)
	sum.AgentCount = len(seen)
	times.apply(&sum)
	sum.Usage = MergeUsage(sum.MainUsage, sum.AgentUsage)
	return sum
}

// turnTimes adds up a session's wall span and active time from its turns.
// A turn that is not open counts when it is made; an open turn's end waits
// for the edits and agent stops Summarize reads after the turns.
type turnTimes struct {
	// first is the earliest turn start and end the latest turn start or
	// end, open turns counted at their start until apply.
	first, end time.Time
	// closedMS is the active time of the turns that are not open.
	closedMS int64
	open     []openTurn
}

// openTurn is a turn still open: it runs from start to last, the latest
// activity seen at or after start and before next, the start of the turn
// after it (zero for the last turn).
type openTurn struct {
	start, next, last time.Time
}

func newTurnTimes(turns []Turn) turnTimes {
	var tt turnTimes
	for _, t := range turns {
		if tt.first.IsZero() || t.StartedAt.Before(tt.first) {
			tt.first = t.StartedAt
		}
		tt.end = later(tt.end, t.StartedAt)
		if t.EndedAt != nil {
			tt.end = later(tt.end, *t.EndedAt)
		}
		switch {
		case t.Totals != nil:
			tt.closedMS += max(t.Totals.WallMS-t.Totals.UserMS, 0)
		case t.Status == TurnOpen:
			tt.open = append(tt.open, openTurn{
				start: t.StartedAt, next: nextStart(turns, t.StartedAt), last: t.StartedAt,
			})
		case t.EndedAt != nil:
			tt.closedMS += max(t.EndedAt.Sub(t.StartedAt).Milliseconds(), 0)
		}
	}
	return tt
}

// see records an edit or an agent stop at t, which ends every open turn
// whose stretch holds it at t or later.
func (tt *turnTimes) see(t time.Time) {
	for i := range tt.open {
		o := &tt.open[i]
		if !t.Before(o.start) && (o.next.IsZero() || t.Before(o.next)) {
			o.last = later(o.last, t)
		}
	}
}

// apply sets sum's LastTurnEnd, DurationMS and ActiveMS, every open turn
// ending at its last activity for both times, and moves sum's last
// activity up to LastTurnEnd.
func (tt *turnTimes) apply(sum *SessionSummary) {
	end, active := tt.end, tt.closedMS
	for _, o := range tt.open {
		end = later(end, o.last)
		active += o.last.Sub(o.start).Milliseconds()
	}
	sum.LastTurnEnd = end
	sum.DurationMS = end.Sub(tt.first).Milliseconds()
	sum.ActiveMS = min(active, sum.DurationMS)
	sum.LastActivityAt = later(sum.LastActivityAt, end)
}

// nextStart is the earliest turn start after start, or zero when no turn
// starts later.
func nextStart(turns []Turn, start time.Time) time.Time {
	var next time.Time
	for _, t := range turns {
		if t.StartedAt.After(start) && (next.IsZero() || t.StartedAt.Before(next)) {
			next = t.StartedAt
		}
	}
	return next
}

// addAgents folds the Agent records into sum: their labels, their merged
// usage and their stop times as activity. It returns the ids it saw.
func addAgents(sum *SessionSummary, agents []Agent) map[string]bool {
	seen := map[string]bool{}
	usage := make([][]ModelUsage, 0, len(agents))
	for _, a := range agents {
		seen[a.ID] = true
		sum.AgentLabels = appendNew(sum.AgentLabels, a.Label())
		sum.LastActivityAt = later(sum.LastActivityAt, a.EndedAt)
		if a.Totals != nil {
			usage = append(usage, a.Totals.Usage)
		}
	}
	sum.AgentUsage = MergeUsage(usage...)
	return seen
}

// addTurns folds the turns into sum: the last turn's status and the main
// conversation's merged usage. turnTimes adds up their times.
func addTurns(sum *SessionSummary, turns []Turn) {
	lastSeq := 0
	var usage [][]ModelUsage
	for _, t := range turns {
		if t.Seq >= lastSeq {
			lastSeq, sum.LastTurnOpen = t.Seq, t.Status == TurnOpen
		}
		if t.Totals != nil {
			usage = append(usage, t.Totals.Usage)
		}
	}
	sum.MainUsage = MergeUsage(usage...)
}

// appendNew appends v to list unless it is empty or already there.
func appendNew(list []string, v string) []string {
	if v == "" || slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
