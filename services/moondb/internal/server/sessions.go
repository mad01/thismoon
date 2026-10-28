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
	// turn ending at its last edit or agent stop; absent while it runs or
	// before it has a turn.
	EndedAt *time.Time `json:"ended_at,omitempty"`
	// DurationMS is the wall span: from the first turn's start to the last
	// turn's end, an open turn ending at its last edit or agent stop, the
	// pauses between turns included. It reads the turn records only, so it
	// can fall short of the session trace's wall time.
	DurationMS int64 `json:"duration_ms"               jsonschema:"wall time in milliseconds: from the first prompt to the last turn's end, pauses between turns included"`
	// ActiveMS is the active time: the time inside the turns, less the
	// time a turn waited for input or sat idle, never above DurationMS
	// (store.SessionSummary.ActiveMS).
	ActiveMS  int64 `json:"active_ms"                 jsonschema:"active time in milliseconds: the time inside the turns, less time waiting for input or idle, never above duration_ms; pauses between turns never count"`
	TurnCount int   `json:"turn_count"`
	EditCount int   `json:"edit_count"`
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
	Cost       *float64 `json:"cost,omitempty"            jsonschema:"USD of the whole session at list prices: the main conversation and every subagent with totals; absent when no model is priced"`
	CostMain   *float64 `json:"cost_main,omitempty"`
	CostAgents *float64 `json:"cost_agents,omitempty"`
	Unpriced   []string `json:"unpriced,omitempty"`
	// RepoCosts splits Cost by the attribution rule (store.MainCostRepo,
	// store.AgentCostRepo): one entry per repository that took a part,
	// then orchestration and no_repo when they took one.
	RepoCosts []RepoCost `json:"repo_costs"                jsonschema:"the session's attributed cost: each subagent's cost goes to the repository most of its edits touched, else its cwd's repository, else no_repo; the main conversation's to the repository most of its own edits touched, else orchestration"`
	// RepoCost is set only when the listing filters by repository: the
	// part of Cost attributed to that repository (attributedCost).
	RepoCost *float64 `json:"repo_cost,omitempty"       jsonschema:"only with the repo filter: the part of cost attributed to that repository; absent when no price covers that part (its repo_costs entry names the unpriced models) or the session has no priced cost, else 0 when the attribution rule sends it nothing"`
	// Repos are the repository identities the session edited, sorted, and
	// RepoNames their short names (the last two path segments) in the same
	// order.
	Repos     []string `json:"repos"`
	RepoNames []string `json:"repo_names"`
	// Worktrees are the checkout roots the session edited, sorted.
	Worktrees []string `json:"worktrees"`
	// CwdRepos are the repositories the session worked in without editing
	// them (store.SessionSummary.CwdRepos): its own cwd's and its recorded
	// subagents' cwds', less Repos, sorted. CwdRepoNames are their short
	// names in the same order. Both absent when there are none.
	CwdRepos     []string `json:"cwd_repos,omitempty"       jsonschema:"repositories the session worked in without editing them: the one its cwd sits in and each recorded subagent's cwd repository, less the repos it edited, sorted; absent when there are none. A session recorded before sessions kept a location lacks its own cwd's"`
	CwdRepoNames []string `json:"cwd_repo_names,omitempty"`
	// Entrypoint is how the harness was started: cli for an interactive
	// session, sdk-cli for `claude -p`.
	Entrypoint string `json:"entrypoint,omitempty"      jsonschema:"how the harness was started, as Claude Code names it: cli for an interactive terminal session, sdk-cli for claude -p (headless); absent when not recorded"`
	// Tags are the session tags its launcher set, such as eval.
	Tags []string `json:"tags,omitempty"            jsonschema:"short tokens the session's launcher set in MOONDB_SESSION_TAGS, such as eval for an eval run"`
}

// SessionList is what GET /api/sessions answers: one page of rows, the
// totals of every session the filters keep, and the repositories of the
// time window.
type SessionList struct {
	Sessions []SessionSummary `json:"sessions"`
	// Window adds up every session the filters keep, past the limit.
	Window WindowTotals `json:"window"`
	// Repos are the repositories the sessions in the time window count
	// toward (chipRepos), the repo filter left out, so a page can offer
	// each one.
	Repos []RepoCount `json:"repos"`
}

// WindowTotals adds up a set of sessions. A file edited in two of them
// counts once. RepoCosts splits Cost by the attribution rule, as a row's
// repo_costs does.
type WindowTotals struct {
	Sessions int `json:"sessions"`
	// ActiveMS sums the sessions' active time.
	ActiveMS int64        `json:"active_ms"`
	Turns    int          `json:"turns"`
	Edits    int          `json:"edits"`
	Files    int          `json:"files"`
	Tokens   store.Tokens `json:"tokens"`
	// Cost covers the main conversations and their subagents, as a row's
	// does; CostMain and CostAgents split it.
	Cost       *float64   `json:"cost,omitempty"`
	CostMain   *float64   `json:"cost_main,omitempty"`
	CostAgents *float64   `json:"cost_agents,omitempty"`
	Unpriced   []string   `json:"unpriced,omitempty"`
	RepoCosts  []RepoCost `json:"repo_costs"`
}

// RepoCount is one repository with how many sessions count toward it
// (chipRepos), and the cost the attribution rule gives it across the time
// window: absent when no priced part goes there.
type RepoCount struct {
	Repo     string   `json:"repo"`
	Name     string   `json:"name"`
	Sessions int      `json:"sessions"`
	Cost     *float64 `json:"cost,omitempty"`
	Unpriced []string `json:"unpriced,omitempty"`
}

// RepoCost is the attributed cost of one repository, or of the
// orchestration or no_repo label (store.CostOrchestration,
// store.CostNoRepo). Name is the repository's short name, or the label as
// a person reads it. Cost is nil when none of the part's models is
// priced, and Unpriced names the models it leaves out.
type RepoCost struct {
	Repo     string   `json:"repo"               jsonschema:"repository identity host/owner/repo, or orchestration or no_repo"`
	Name     string   `json:"name"`
	Cost     *float64 `json:"cost,omitempty"`
	Unpriced []string `json:"unpriced,omitempty"`
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
	kept, repo := all, q.Get("repo")
	if repo != "" {
		kept = slices.DeleteFunc(slices.Clone(all), func(sum store.SessionSummary) bool {
			return !slices.Contains(chipRepos(sum), repo)
		})
	}
	now := s.now()
	rows := make([]SessionSummary, 0, min(limit, len(kept)))
	for _, sum := range kept[:min(limit, len(kept))] {
		row := s.row(sum, now)
		if repo != "" {
			row.RepoCost = attributedCost(row, repo)
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, SessionList{
		Sessions: rows,
		Window:   s.window(kept),
		Repos:    s.repoCounts(all),
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
	names := repoNames(sum.Repos)
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
		ActiveMS:       sum.ActiveMS,
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
		RepoCosts:      s.repoCosts(sum.RepoUsage),
		Repos:          sum.Repos,
		RepoNames:      names,
		Worktrees:      sum.Worktrees,
		CwdRepos:       sum.CwdRepos,
		CwdRepoNames:   repoNames(sum.CwdRepos),
		Entrypoint:     sum.Entrypoint,
		Tags:           sum.Tags,
	}
}

// window adds up the sessions: their counts, their active time, their
// distinct files, and their merged usage priced once.
func (s *Server) window(sessions []store.SessionSummary) WindowTotals {
	t := WindowTotals{Sessions: len(sessions)}
	files := map[store.FileKey]bool{}
	var main, agents [][]store.ModelUsage
	var parts [][]store.RepoUsage
	for _, sum := range sessions {
		t.ActiveMS += sum.ActiveMS
		t.Turns += sum.TurnCount
		t.Edits += sum.EditCount
		for _, f := range sum.Files {
			files[f] = true
		}
		main = append(main, sum.MainUsage)
		agents = append(agents, sum.AgentUsage)
		parts = append(parts, sum.RepoUsage)
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
	t.RepoCosts = s.repoCosts(store.MergeRepoUsage(parts...))
	return t
}

// repoCounts lists the repositories of the sessions' chipRepos, most
// sessions first, then by name, each with the cost the sessions attribute
// to it. A chip and the repo filter go by the same rule, so a chip's
// sessions and cost are what its filtered listing holds.
func (s *Server) repoCounts(sessions []store.SessionSummary) []RepoCount {
	counts := map[string]int{}
	parts := make([][]store.RepoUsage, 0, len(sessions))
	for _, sum := range sessions {
		for _, r := range chipRepos(sum) {
			counts[r]++
		}
		parts = append(parts, sum.RepoUsage)
	}
	usage := map[string][]store.ModelUsage{}
	for _, p := range store.MergeRepoUsage(parts...) {
		usage[p.Repo] = p.Usage
	}
	out := make([]RepoCount, 0, len(counts))
	for r, n := range counts {
		c := RepoCount{Repo: r, Name: repoName(r), Sessions: n}
		c.Cost, c.Unpriced = s.query.UsageCost(usage[r])
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b RepoCount) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.Repo, b.Repo))
	})
	return out
}

// chipRepos lists the repositories a session counts toward, each once: the
// ones it edited (Repos), the ones it worked in without editing them, its
// own or a subagent's cwd repository (CwdRepos), and any other the
// attribution rule sends a part of its cost to. So every repository its
// row shows a badge for is here. The repo filter keeps a session by them
// and a repo chip counts it by them.
func chipRepos(sum store.SessionSummary) []string {
	repos := slices.Concat(sum.Repos, sum.CwdRepos)
	for _, p := range sum.RepoUsage {
		if !isCostLabel(p.Repo) && !slices.Contains(repos, p.Repo) {
			repos = append(repos, p.Repo)
		}
	}
	return repos
}

// repoCosts prices attributed usage: the repositories costliest first,
// those without a priced part last, then orchestration and no_repo.
func (s *Server) repoCosts(parts []store.RepoUsage) []RepoCost {
	out := make([]RepoCost, 0, len(parts))
	for _, p := range parts {
		c := RepoCost{Repo: p.Repo, Name: costName(p.Repo)}
		c.Cost, c.Unpriced = s.query.UsageCost(p.Usage)
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b RepoCost) int {
		return cmp.Or(
			cmp.Compare(labelRank(a.Repo), labelRank(b.Repo)),
			cmp.Compare(costRank(b.Cost), costRank(a.Cost)),
			cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.Repo, b.Repo),
		)
	})
	return out
}

// attributedCost is the part of row's cost attributed to repo. With an
// entry for repo it is that entry's cost, nil when none of its models is
// priced. Without one it is a real zero when the row is priced, nil when
// the row has no priced cost.
func attributedCost(row SessionSummary, repo string) *float64 {
	if i := slices.IndexFunc(row.RepoCosts, func(c RepoCost) bool { return c.Repo == repo }); i >= 0 {
		return row.RepoCosts[i].Cost
	}
	if row.Cost == nil {
		return nil
	}
	return new(0.0)
}

// costName is how a person reads an attributed cost's repository or label.
// The no_repo label reads "no repo (cwd)", apart from a row's "no repo"
// text, which says the session counts toward no repository.
func costName(repo string) string {
	switch repo {
	case store.CostOrchestration:
		return "orchestration"
	case store.CostNoRepo:
		return "no repo (cwd)"
	}
	return repoName(repo)
}

// isCostLabel reports whether repo is a label that takes attributed cost
// no repository takes, not a repository.
func isCostLabel(repo string) bool { return labelRank(repo) > 0 }

// labelRank puts the repositories first, then orchestration, then no_repo.
func labelRank(repo string) int {
	switch repo {
	case store.CostOrchestration:
		return 1
	case store.CostNoRepo:
		return 2
	}
	return 0
}

// costRank orders costs, an unpriced one below every priced one.
func costRank(c *float64) float64 {
	if c == nil {
		return -1
	}
	return *c
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

// repoNames is each repository's repoName, in the same order.
func repoNames(repos []string) []string {
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, repoName(r))
	}
	return names
}

func tokensOf(usage []store.ModelUsage) store.Tokens {
	var t store.Tokens
	for _, u := range usage {
		t = t.Add(u.Tokens)
	}
	return t
}
