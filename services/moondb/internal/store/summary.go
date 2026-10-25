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
	// LastActivityAt is the latest turn start, turn end or edit, or the
	// session's start when it has none of them.
	LastActivityAt time.Time `json:"last_activity_at"`
	// DurationMS runs from the first turn's start to the last turn's end,
	// an open turn ending where it started. It reads the turn records
	// only, no transcript lines, so it can fall short of the session
	// trace's wall time, whose turn spans stretch to a line written after
	// the recorded end.
	DurationMS int64 `json:"duration_ms"`
	// LastTurnEnd is where DurationMS ends: the latest turn end, an open
	// turn ending where it started; zero without turns. It stays out of
	// the JSON, where a listing row says it as ended_at.
	LastTurnEnd time.Time `json:"-"`
	// Usage merges the Usage of every turn with totals (MergeUsage).
	Usage []ModelUsage `json:"usage,omitempty"`
}

// FileKey names one edited file: its worktree and its path there, or ""
// and an absolute path for a file outside any repository.
type FileKey struct {
	Worktree string
	Path     string
}

// Summarize adds up a session's turns and edits. edits may come in any
// order; Summarize only reads them.
func Summarize(s Session, turns []Turn, edits iter.Seq[*Edit]) SessionSummary {
	sum := SessionSummary{
		Session:        s,
		Repos:          []string{},
		Worktrees:      []string{},
		Files:          []FileKey{},
		TurnCount:      len(turns),
		LastActivityAt: s.StartedAt,
	}
	addTurns(&sum, turns)
	agents := map[string]bool{}
	files := map[FileKey]bool{}
	for e := range edits {
		sum.EditCount++
		agents[e.AgentID] = true
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
	sum.AgentCount = len(agents)
	return sum
}

// addTurns folds the turns into sum: the duration, the last activity, the
// last turn's status and the merged usage.
func addTurns(sum *SessionSummary, turns []Turn) {
	var first, last time.Time
	lastSeq := 0
	var usage [][]ModelUsage
	for _, t := range turns {
		end := t.StartedAt
		if t.EndedAt != nil {
			end = later(end, *t.EndedAt)
		}
		if first.IsZero() || t.StartedAt.Before(first) {
			first = t.StartedAt
		}
		last = later(last, end)
		if t.Seq >= lastSeq {
			lastSeq, sum.LastTurnOpen = t.Seq, t.Status == TurnOpen
		}
		if t.Totals != nil {
			usage = append(usage, t.Totals.Usage)
		}
	}
	sum.LastActivityAt = later(sum.LastActivityAt, last)
	sum.LastTurnEnd = last
	sum.DurationMS = last.Sub(first).Milliseconds()
	sum.Usage = MergeUsage(usage...)
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
