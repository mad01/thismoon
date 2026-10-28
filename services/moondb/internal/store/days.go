package store

import (
	"cmp"
	"slices"
	"time"
)

// DayActivity is one local calendar day a session was active on, with the
// session's slice of it, so a listing can show the session under every
// day it worked and give each day its own figures. A day is active when
// it holds the session's start, a turn start or end, an edit, or an
// agent start or stop. A turn's count, active time and usage go to the
// day it started, an edit's to the day it was made, an agent's usage to
// the day it started.
type DayActivity struct {
	// Date is the day as LocalDay gives it, YYYY-MM-DD.
	Date string `json:"date"`
	// FirstAt and LastAt bound the activity seen that day.
	FirstAt time.Time `json:"first_at"`
	LastAt  time.Time `json:"last_at"`
	// Turns counts the turns that started that day, Edits the edits made
	// that day and Files the distinct files they touched. Agents counts
	// the agents that started or edited that day, the main conversation
	// among them when it edited.
	Turns  int `json:"turns"`
	Edits  int `json:"edits"`
	Files  int `json:"files"`
	Agents int `json:"agents"`
	// ActiveMS is the active time of the turns that started that day, by
	// the SessionSummary.ActiveMS rule for each turn. It is not capped at
	// the day's wall span.
	ActiveMS int64 `json:"active_ms"`
	// Usage merges the usage of the turns and the agents that started
	// that day, and RepoUsage splits it by the attribution rule over the
	// day's edits. Both stay out of the JSON, where a listing row prices
	// them.
	Usage     []ModelUsage `json:"-"`
	RepoUsage []RepoUsage  `json:"-"`
}

// LocalDay is the local calendar day of t, the key a listing groups
// sessions by. serve and the page that reads it run on one machine, so
// the server's zone is the reader's.
func LocalDay(t time.Time) string { return t.In(time.Local).Format(time.DateOnly) }

// dayBuilder gathers a session's records into days.
type dayBuilder map[string]*dayAcc

// dayAcc is one day under construction: its DayActivity plus the sets and
// usage lists that give its counts and usage at finish.
type dayAcc struct {
	DayActivity
	files   map[FileKey]bool
	agents  map[string]bool
	main    [][]ModelUsage
	agentUs [][]ModelUsage
	records []Agent
	tallies agentTallies
}

// see records activity at t and returns its day; a zero t is no activity.
func (b dayBuilder) see(t time.Time) *dayAcc {
	if t.IsZero() {
		return nil
	}
	key := LocalDay(t)
	d, ok := b[key]
	if !ok {
		d = &dayAcc{
			DayActivity: DayActivity{Date: key, FirstAt: t, LastAt: t},
			files:       map[FileKey]bool{},
			agents:      map[string]bool{},
			tallies:     agentTallies{},
		}
		b[key] = d
	}
	if t.Before(d.FirstAt) {
		d.FirstAt = t
	}
	d.LastAt = later(d.LastAt, t)
	return d
}

// turn counts t under its start day with activeMS of active time, and
// records its end as activity.
func (b dayBuilder) turn(t Turn, activeMS int64) {
	d := b.see(t.StartedAt)
	if d == nil {
		return
	}
	d.Turns++
	d.ActiveMS += activeMS
	if t.Totals != nil {
		d.main = append(d.main, t.Totals.Usage)
	}
	if t.EndedAt != nil {
		b.see(*t.EndedAt)
	}
}

// active adds ms of active time to the day a turn started at start: an
// open turn's, known once its last activity is.
func (b dayBuilder) active(start time.Time, ms int64) {
	if d := b.see(start); d != nil {
		d.ActiveMS += ms
	}
}

// edit counts e under the day it was made.
func (b dayBuilder) edit(e *Edit) {
	d := b.see(e.At)
	if d == nil {
		return
	}
	d.Edits++
	d.files[FileKey{Worktree: e.Worktree, Path: e.Path}] = true
	d.agents[e.AgentID] = true
	d.tallies.add(e)
}

// agent counts a under the day it started, its stop day active too. An
// agent whose start is unknown counts under its stop day.
func (b dayBuilder) agent(a Agent) {
	d := b.see(cmp.Or(a.StartedAt, a.EndedAt))
	if d == nil {
		return
	}
	d.agents[a.ID] = true
	d.records = append(d.records, a)
	if a.Totals != nil {
		d.agentUs = append(d.agentUs, a.Totals.Usage)
	}
	b.see(a.EndedAt)
}

// finish lists the days oldest first with their counts and usage set.
func (b dayBuilder) finish() []DayActivity {
	out := make([]DayActivity, 0, len(b))
	for _, d := range b {
		day := d.DayActivity
		day.Files = len(d.files)
		day.Agents = len(d.agents)
		main := MergeUsage(d.main...)
		day.Usage = MergeUsage(main, MergeUsage(d.agentUs...))
		day.RepoUsage = attribute(main, d.records, d.tallies)
		out = append(out, day)
	}
	slices.SortFunc(out, func(a, b DayActivity) int { return cmp.Compare(a.Date, b.Date) })
	return out
}
