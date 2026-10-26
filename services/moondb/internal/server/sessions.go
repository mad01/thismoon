package server

import (
	"cmp"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Session statuses a listing row reports.
const (
	// StatusRunning: the session's last activity is recent, or its last
	// turn is open and not stale (moondb.SessionIdleAfter,
	// moondb.OpenTurnStaleAfter).
	StatusRunning = "running"
	// StatusEnded: anything else.
	StatusEnded = "ended"
)

// SessionSummary is one row of the session listing. Everything in it comes
// from the store's records, never a chunk: the counts and times from the
// session's turns and edits, the tokens from the totals each turn keeps,
// and the cost priced from those tokens on read, so a price change
// reprices every row.
type SessionSummary struct {
	ID string `json:"id"`
	// Title is the session's harness title, else the first line of its
	// first prompt, else the last segment of its cwd, else its id: never
	// empty.
	Title   string `json:"title"`
	Harness string `json:"harness"`
	Cwd     string `json:"cwd"`
	Model   string `json:"model,omitempty"`
	// TranscriptPath is the harness transcript file the session record
	// keeps.
	TranscriptPath string `json:"transcript_path,omitempty"`
	// Status is StatusRunning or StatusEnded.
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	// EndedAt is the last turn's end once the session is ended, an open
	// turn ending where it started; absent while it runs or before it has
	// a turn.
	EndedAt *time.Time `json:"ended_at,omitempty"`
	// DurationMS runs from the first turn's start to the last turn's end,
	// an open turn ending where it started. It reads the turn records
	// only, so it can fall short of the session trace's wall time.
	DurationMS int64 `json:"duration_ms"`
	TurnCount  int   `json:"turn_count"`
	EditCount  int   `json:"edit_count"`
	// FileCount counts the distinct files the session edited.
	FileCount int `json:"file_count"`
	// AgentCount counts the agents that ran: the subagents with a record,
	// the ones known only from an edit, and the main conversation when it
	// edited. Agents names the subagents, each label once.
	AgentCount int      `json:"agent_count"`
	Agents     []string `json:"agents"`
	// Tokens sums the tokens of the turns and the subagents that have
	// totals: the main conversation and every agent run.
	Tokens store.Tokens `json:"tokens"`
	// Cost is the USD of the priced tokens, nil when none is priced.
	// Unpriced names the models it leaves out. CostMain and CostAgents
	// split it between the main conversation and the subagents; CostAgents
	// is absent when no subagent with a record has totals.
	Cost       *float64 `json:"cost,omitempty"`
	CostMain   *float64 `json:"cost_main,omitempty"`
	CostAgents *float64 `json:"cost_agents,omitempty"`
	Unpriced   []string `json:"unpriced,omitempty"`
	// Repos are the repository identities the session edited, sorted, and
	// RepoNames their short names (the last two path segments) in the same
	// order.
	Repos     []string `json:"repos"`
	RepoNames []string `json:"repo_names"`
	// Worktrees are the checkout roots the session edited, sorted.
	Worktrees []string `json:"worktrees"`
}

// SessionList is what GET /api/sessions answers: one page of rows, the
// totals of every session the filters keep, and the repositories of the
// time window.
type SessionList struct {
	Sessions []SessionSummary `json:"sessions"`
	// Window adds up every session the filters keep, past the limit.
	Window WindowTotals `json:"window"`
	// Repos are the repositories the sessions in the time window edited,
	// the repo filter left out, so a page can offer each one.
	Repos []RepoCount `json:"repos"`
}

// WindowTotals adds up a set of sessions. A file edited in two of them
// counts once.
type WindowTotals struct {
	Sessions int          `json:"sessions"`
	Turns    int          `json:"turns"`
	Edits    int          `json:"edits"`
	Files    int          `json:"files"`
	Tokens   store.Tokens `json:"tokens"`
	// Cost covers the main conversations and their subagents, as a row's
	// does; CostMain and CostAgents split it.
	Cost       *float64 `json:"cost,omitempty"`
	CostMain   *float64 `json:"cost_main,omitempty"`
	CostAgents *float64 `json:"cost_agents,omitempty"`
	Unpriced   []string `json:"unpriced,omitempty"`
}

// RepoCount is one repository with how many sessions edited it.
type RepoCount struct {
	Repo     string `json:"repo"`
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
}

// SessionView is a session as GET /api/sessions/{id} answers it: the
// session with its turns, plus its listing row, whose title and status
// follow the listing's rules, and its agents: the main conversation first,
// then every subagent. The rows take the place of the store's Agent
// records (store.SessionDetail), so Agents is the one agents field.
type SessionView struct {
	store.SessionSummary
	Turns   []store.Turn   `json:"turns"`
	Summary SessionSummary `json:"summary"`
	Agents  []AgentRow     `json:"agents"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, limit, err := s.parseWindow(q)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	all, err := s.store.Sessions(r.Context(), store.SessionFilter{Since: since})
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	kept := all
	if repo := q.Get("repo"); repo != "" {
		kept = slices.DeleteFunc(slices.Clone(all), func(sum store.SessionSummary) bool {
			return !slices.Contains(sum.Repos, repo)
		})
	}
	now := s.now()
	rows := make([]SessionSummary, 0, min(limit, len(kept)))
	for _, sum := range kept[:min(limit, len(kept))] {
		rows = append(rows, s.row(sum, now))
	}
	writeJSON(w, http.StatusOK, SessionList{
		Sessions: rows,
		Window:   s.window(kept),
		Repos:    repoCounts(all),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Session(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	agents, err := s.sessionAgents(r.Context(), d)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SessionView{
		SessionSummary: d.SessionSummary,
		Turns:          d.Turns,
		Summary:        s.row(d.SessionSummary, s.now()),
		Agents:         agents,
	})
}

// row turns a store summary into a listing row at time now.
func (s *Server) row(sum store.SessionSummary, now time.Time) SessionSummary {
	cost, unpriced := s.query.UsageCost(sum.Usage)
	costMain, _ := s.query.UsageCost(sum.MainUsage)
	var costAgents *float64
	if len(sum.AgentUsage) > 0 {
		costAgents, _ = s.query.UsageCost(sum.AgentUsage)
	}
	names := make([]string, 0, len(sum.Repos))
	for _, r := range sum.Repos {
		names = append(names, repoName(r))
	}
	status := sessionStatus(sum, now)
	var ended *time.Time
	if status == StatusEnded && !sum.LastTurnEnd.IsZero() {
		ended = &sum.LastTurnEnd
	}
	return SessionSummary{
		ID:             sum.ID,
		Title:          sessionTitle(sum.Session),
		Harness:        sum.Harness,
		Cwd:            sum.Cwd,
		Model:          sum.Model,
		TranscriptPath: sum.TranscriptPath,
		Status:         status,
		StartedAt:      sum.StartedAt,
		LastActivityAt: sum.LastActivityAt,
		EndedAt:        ended,
		DurationMS:     sum.DurationMS,
		TurnCount:      sum.TurnCount,
		EditCount:      sum.EditCount,
		FileCount:      sum.FileCount,
		AgentCount:     sum.AgentCount,
		Agents:         orEmpty(sum.AgentLabels),
		Tokens:         tokensOf(sum.Usage),
		Cost:           cost,
		CostMain:       costMain,
		CostAgents:     costAgents,
		Unpriced:       unpriced,
		Repos:          sum.Repos,
		RepoNames:      names,
		Worktrees:      sum.Worktrees,
	}
}

// window adds up the sessions: their counts, their distinct files, and
// their merged usage priced once.
func (s *Server) window(sessions []store.SessionSummary) WindowTotals {
	t := WindowTotals{Sessions: len(sessions)}
	files := map[store.FileKey]bool{}
	var main, agents [][]store.ModelUsage
	for _, sum := range sessions {
		t.Turns += sum.TurnCount
		t.Edits += sum.EditCount
		for _, f := range sum.Files {
			files[f] = true
		}
		main = append(main, sum.MainUsage)
		agents = append(agents, sum.AgentUsage)
	}
	t.Files = len(files)
	mainUsage, agentUsage := store.MergeUsage(main...), store.MergeUsage(agents...)
	merged := store.MergeUsage(mainUsage, agentUsage)
	t.Tokens = tokensOf(merged)
	t.Cost, t.Unpriced = s.query.UsageCost(merged)
	t.CostMain, _ = s.query.UsageCost(mainUsage)
	if len(agentUsage) > 0 {
		t.CostAgents, _ = s.query.UsageCost(agentUsage)
	}
	return t
}

// repoCounts lists the repositories the sessions edited, most sessions
// first, then by name.
func repoCounts(sessions []store.SessionSummary) []RepoCount {
	counts := map[string]int{}
	for _, sum := range sessions {
		for _, r := range sum.Repos {
			counts[r]++
		}
	}
	out := make([]RepoCount, 0, len(counts))
	for r, n := range counts {
		out = append(out, RepoCount{Repo: r, Name: repoName(r), Sessions: n})
	}
	slices.SortFunc(out, func(a, b RepoCount) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.Repo, b.Repo))
	})
	return out
}

// sessionTitle names a session: its harness title, else its first prompt's
// first line, else the last segment of its cwd, else its id.
func sessionTitle(s store.Session) string {
	if cwd := strings.TrimRight(s.Cwd, "/"); cwd != "" {
		return cmp.Or(s.Title, s.PromptLine, path.Base(cwd))
	}
	return cmp.Or(s.Title, s.PromptLine, s.ID)
}

// sessionStatus is StatusRunning while the session's last activity is
// within moondb.SessionIdleAfter of now, or its last turn is open and the
// activity within moondb.OpenTurnStaleAfter: a long tool call records
// nothing until it ends, and an interrupted turn never closes.
func sessionStatus(sum store.SessionSummary, now time.Time) string {
	idle := now.Sub(sum.LastActivityAt)
	if idle < moondb.SessionIdleAfter || (sum.LastTurnOpen && idle < moondb.OpenTurnStaleAfter) {
		return StatusRunning
	}
	return StatusEnded
}

// repoName is a repository identity's last two path segments, the owner
// and the name: "github.com/mad01/thismoon" is "mad01/thismoon".
func repoName(repo string) string {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) <= 2 {
		return repo
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func tokensOf(usage []store.ModelUsage) store.Tokens {
	var t store.Tokens
	for _, u := range usage {
		t = t.Add(u.Tokens)
	}
	return t
}
