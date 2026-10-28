package server

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// sessionList fetches GET /api/sessions with query.
func (h harness) sessionList(t *testing.T, query string) SessionList {
	t.Helper()
	var got SessionList
	h.getJSON(t, "/api/sessions"+query, http.StatusOK, &got)
	return got
}

// oldCost is what the older session's trace says it cost.
func (h harness) oldCost(t *testing.T) float64 {
	t.Helper()
	var tr trace.SessionTrace
	h.getJSON(t, "/api/sessions/"+servertest.OldSession+"/trace", http.StatusOK, &tr)
	if tr.Totals.Cost == nil {
		t.Fatal("the older session's trace has no cost")
	}
	return *tr.Totals.Cost
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// sameRepoCount compares two repo counts by repository and sessions,
// leaving their cost to the tests that check it.
func sameRepoCount(a, b RepoCount) bool {
	return a.Repo == b.Repo && a.Name == b.Name && a.Sessions == b.Sessions
}

// TestSessionRows: a row names the session by the title rule, says
// whether it runs, counts what it touched, and prices the tokens its
// turns keep to what its trace computes from the transcript.
func TestSessionRows(t *testing.T) {
	h := newHarness(t)
	got := h.sessionList(t, "").Sessions
	if len(got) != 2 {
		t.Fatalf("sessions = %d, want 2", len(got))
	}
	newer, older := got[0], got[1]
	if newer.Status != StatusRunning || older.Status != StatusEnded {
		t.Errorf("status = %s, %s; want the open turn running and the closed one ended",
			newer.Status, older.Status)
	}
	if older.Title != servertest.OldPromptLine {
		t.Errorf("untitled session's title = %q, want its prompt line", older.Title)
	}
	if !slices.Equal(newer.RepoNames, []string{"owner/thismoon"}) {
		t.Errorf("repo names = %v, want the last two segments", newer.RepoNames)
	}
	if newer.FileCount != 2 || newer.AgentCount != 1 || older.FileCount != 1 {
		t.Errorf("files %d, agents %d, older files %d; want 2, 1, 1",
			newer.FileCount, newer.AgentCount, older.FileCount)
	}
	if !newer.LastActivityAt.Equal(servertest.T0.Add(63 * time.Minute)) {
		t.Errorf("last activity = %v, want the newest edit", newer.LastActivityAt)
	}
	if newer.EndedAt != nil || older.EndedAt == nil ||
		!older.EndedAt.Equal(servertest.T0.Add(3*time.Minute)) {
		t.Errorf("ended at %v, %v; want none while running and the last turn's end once ended",
			newer.EndedAt, older.EndedAt)
	}
	if older.TranscriptPath != servertest.OldTranscriptPath || newer.TranscriptPath != "" {
		t.Errorf("transcript paths %q, %q; want the session records'",
			older.TranscriptPath, newer.TranscriptPath)
	}
	if newer.Cost != nil || newer.Tokens.Total() != 0 {
		t.Errorf("newer cost %v, tokens %+v; want none while its turn is open",
			newer.Cost, newer.Tokens)
	}
	if older.Cost == nil || !near(*older.Cost, h.oldCost(t)) || older.Tokens.Total() != 3780 {
		t.Errorf("older cost %v, tokens %d; want the trace's cost and 3780 tokens",
			older.Cost, older.Tokens.Total())
	}
	if older.CostMain == nil || older.CostAgents == nil ||
		!near(*older.CostMain+*older.CostAgents, *older.Cost) ||
		!slices.Equal(older.Agents, []string{servertest.AgentName}) || older.AgentCount != 2 {
		t.Errorf("older main %v, agents %v named %v, count %d; want the cost split and the "+
			"reviewer beside main", older.CostMain, older.CostAgents, older.Agents, older.AgentCount)
	}
}

// TestSessionsWindow: the window adds up every session the filters keep,
// past the limit, and the repos come from the time window whatever the
// repo filter says. The time window keeps the sessions active in it.
func TestSessionsWindow(t *testing.T) {
	h := newHarness(t)
	cost := h.oldCost(t)
	wantRepos := []RepoCount{
		{Repo: servertest.OtherRepo, Name: "owner/other", Sessions: 1},
		{Repo: servertest.Repo, Name: "owner/thismoon", Sessions: 1},
	}
	// The newer session started 30 minutes before now and was last active
	// 27 minutes before it; the older one ended 87 minutes before it.
	newerOnly := WindowTotals{Sessions: 1, Turns: 1, Edits: 2, Files: 2}
	tests := []struct {
		name, query string
		rows        int
		want        WindowTotals
		// timed is set when the time window leaves the older session out.
		timed bool
	}{
		{"all", "", 2, WindowTotals{Sessions: 2, Turns: 2, Edits: 3, Files: 3}, false},
		{"limit", "?limit=1", 1, WindowTotals{Sessions: 2, Turns: 2, Edits: 3, Files: 3}, false},
		{
			"repo", "?repo=" + servertest.OtherRepo, 1,
			WindowTotals{Sessions: 1, Turns: 1, Edits: 1, Files: 1},
			false,
		},
		{"since", "?since=45m", 1, newerOnly, true},
		{"since after the newer start", "?since=29m", 1, newerOnly, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.sessionList(t, tc.query)
			w := got.Window
			if len(got.Sessions) != tc.rows || w.Sessions != tc.want.Sessions ||
				w.Turns != tc.want.Turns || w.Edits != tc.want.Edits || w.Files != tc.want.Files {
				t.Errorf(
					"%d rows, window %+v; want %d rows, %+v",
					len(got.Sessions),
					w,
					tc.rows,
					tc.want,
				)
			}
			if !tc.timed && (w.Cost == nil || !near(*w.Cost, cost) || w.CostMain == nil ||
				w.CostAgents == nil || !near(*w.CostMain+*w.CostAgents, cost)) {
				t.Errorf("window cost = %v (main %v, agents %v), want the older session's %v split",
					w.Cost, w.CostMain, w.CostAgents, cost)
			}
			if tc.timed && w.Cost != nil {
				t.Errorf("window cost = %v, want none: no session in it has tokens", *w.Cost)
			}
			if !tc.timed && !slices.EqualFunc(got.Repos, wantRepos, sameRepoCount) {
				t.Errorf("repos = %+v, want %+v", got.Repos, wantRepos)
			}
		})
	}
}

// TestSessionsCarryActiveTime: every row and the window answer both
// times, active_ms and duration_ms. The older session's closed turn gives
// its totals' wall less user time, the newer one's open turn runs from its
// start to its last edit for both times, and the window sums the rows'
// active time.
func TestSessionsCarryActiveTime(t *testing.T) {
	h := newHarness(t)
	var raw struct {
		Sessions []map[string]json.RawMessage `json:"sessions"`
		Window   map[string]json.RawMessage   `json:"window"`
	}
	h.getJSON(t, "/api/sessions", http.StatusOK, &raw)
	for i, row := range raw.Sessions {
		for _, key := range []string{"active_ms", "duration_ms"} {
			if _, ok := row[key]; !ok {
				t.Errorf("row %d has no %s", i, key)
			}
		}
	}
	if _, ok := raw.Window["active_ms"]; !ok {
		t.Error("window has no active_ms")
	}

	var view SessionView
	h.getJSON(t, "/api/sessions/"+servertest.OldSession, http.StatusOK, &view)
	if len(view.Turns) != 1 || view.Turns[0].Totals == nil {
		t.Fatalf("older session turns = %+v, want one with totals", view.Turns)
	}
	tot := view.Turns[0].Totals
	wantOld := tot.WallMS - tot.UserMS
	wantNew := (2 * time.Minute).Milliseconds()
	list := h.sessionList(t, "")
	if len(list.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list.Sessions))
	}
	newer, older := list.Sessions[0], list.Sessions[1]
	if older.ActiveMS != wantOld || older.DurationMS != 120_000 {
		t.Errorf("older active %d, wall %d; want %d and 120000", older.ActiveMS, older.DurationMS,
			wantOld)
	}
	if newer.ActiveMS != wantNew || newer.DurationMS != wantNew {
		t.Errorf("newer active %d, wall %d; want both %d, to its last edit",
			newer.ActiveMS, newer.DurationMS, wantNew)
	}
	if list.Window.ActiveMS != wantOld+wantNew {
		t.Errorf("window active = %d, want the rows' sum %d", list.Window.ActiveMS, wantOld+wantNew)
	}
	if view.Summary.ActiveMS != wantOld || view.ActiveMS != wantOld {
		t.Errorf("session view active %d, row %d; want %d", view.ActiveMS, view.Summary.ActiveMS,
			wantOld)
	}
}

// TestSessionViewCarriesRow: one session answers with its turns and the
// listing row, title and status included.
func TestSessionViewCarriesRow(t *testing.T) {
	h := newHarness(t)
	var got SessionView
	h.getJSON(t, "/api/sessions/"+servertest.OldSession, http.StatusOK, &got)
	if got.Summary.Title != servertest.OldPromptLine || got.Summary.Status != StatusEnded ||
		len(got.Turns) != 1 || got.Summary.TurnCount != 1 {
		t.Errorf("view = summary %+v, %d turns", got.Summary, len(got.Turns))
	}
}

func TestSessionStatus(t *testing.T) {
	now := servertest.T0
	tests := []struct {
		name     string
		idle     time.Duration
		lastOpen bool
		want     string
	}{
		{"recent activity", moondb.SessionIdleAfter - time.Second, false, StatusRunning},
		{"idle", moondb.SessionIdleAfter, false, StatusEnded},
		{"open last turn", moondb.OpenTurnStaleAfter - time.Second, true, StatusRunning},
		{"stale open turn", moondb.OpenTurnStaleAfter, true, StatusEnded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum := store.SessionSummary{
				LastActivityAt: now.Add(-tc.idle),
				LastTurnOpen:   tc.lastOpen,
			}
			if got := sessionStatus(sum, now); got != tc.want {
				t.Errorf("status = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSessionTitle(t *testing.T) {
	tests := []struct {
		name string
		s    store.Session
		want string
	}{
		{"harness title", store.Session{Title: "t", PromptLine: "p", Cwd: "/a/b"}, "t"},
		{"prompt line", store.Session{PromptLine: "p", Cwd: "/a/b"}, "p"},
		{"cwd segment", store.Session{Cwd: "/a/b/"}, "b"},
		{"id", store.Session{ID: "sess", Cwd: "/"}, "sess"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionTitle(tc.s); got != tc.want {
				t.Errorf("sessionTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

// rowByID is the listed row of session id; it fails t without one.
func rowByID(t *testing.T, list SessionList, id string) SessionSummary {
	t.Helper()
	i := slices.IndexFunc(list.Sessions, func(r SessionSummary) bool { return r.ID == id })
	if i < 0 {
		t.Fatalf("no row for %s among %d rows", id, len(list.Sessions))
	}
	return list.Sessions[i]
}

// sameRepoCost compares two attributed parts by repository and cost.
func sameRepoCost(a, b RepoCost) bool {
	return a.Repo == b.Repo && (a.Cost == nil) == (b.Cost == nil) &&
		(a.Cost == nil || near(*a.Cost, *b.Cost))
}

// TestSessionDays: a row lists the local days the session was active on,
// each with that day's slice. Every fixture record falls on one day, so
// each row has one day whose figures are the row's own, priced the same,
// and under the repo filter the day carries its attributed part too.
func TestSessionDays(t *testing.T) {
	h := newHarness(t)
	got := h.sessionList(t, "").Sessions
	if len(got) != 2 {
		t.Fatalf("sessions = %d, want 2", len(got))
	}
	for _, row := range got {
		if len(row.Days) != 1 {
			t.Errorf("%s has %d days, want 1", row.ID, len(row.Days))
			continue
		}
		d := row.Days[0]
		if d.Date != store.LocalDay(row.StartedAt) || d.Turns != row.TurnCount ||
			d.Edits != row.EditCount || d.Files != row.FileCount || d.Agents != row.AgentCount ||
			d.ActiveMS != row.ActiveMS || !d.LastAt.Equal(row.LastActivityAt) ||
			(d.Cost == nil) != (row.Cost == nil) || (d.Cost != nil && !near(*d.Cost, *row.Cost)) ||
			!slices.EqualFunc(d.RepoCosts, row.RepoCosts, sameRepoCost) {
			t.Errorf("%s day = %+v, want the row's own figures: %+v", row.ID, d, row)
		}
	}
	filtered := h.sessionList(t, "?repo="+servertest.OtherRepo).Sessions
	if len(filtered) != 1 || len(filtered[0].Days) != 1 {
		t.Fatalf("filtered = %+v, want the older session with its one day", filtered)
	}
	row, d := filtered[0], filtered[0].Days[0]
	if row.RepoCost == nil || d.RepoCost == nil || !near(*d.RepoCost, *row.RepoCost) {
		t.Errorf("day repo_cost %v, row %v; want the day's attributed part, the row's",
			d.RepoCost, row.RepoCost)
	}
}

// TestSessionsDirs: the sessions that count toward no repository are
// grouped by working directory, trailing slash dropped, each with its
// cost, and the cwd filter keeps one directory's sessions.
func TestSessionsDirs(t *testing.T) {
	h := newHarness(t)
	const scratch = "sess-scratch"
	start := servertest.T0.Add(70 * time.Minute)
	h.store.Add(
		store.Session{
			ID:        scratch,
			Harness:   "test-harness",
			Cwd:       "/scratch/probe/",
			StartedAt: start,
		},
		[]store.Turn{{
			ID: "turn-scratch", SessionID: scratch, Seq: 1, Status: store.TurnOpen,
			StartedAt: start,
		}},
		nil,
		nil,
	)
	list := h.sessionList(t, "")
	want := []DirCount{{Dir: "/scratch/probe", Name: "scratch/probe", Sessions: 1}}
	if !slices.EqualFunc(list.Dirs, want, func(a, b DirCount) bool {
		return a.Dir == b.Dir && a.Name == b.Name && a.Sessions == b.Sessions && a.Cost == nil
	}) {
		t.Errorf("dirs = %+v, want %+v: the fixture's other sessions have repos", list.Dirs, want)
	}
	kept := h.sessionList(t, "?cwd=/scratch/probe")
	if len(kept.Sessions) != 1 || kept.Sessions[0].ID != scratch || kept.Window.Sessions != 1 ||
		kept.Sessions[0].CwdName != "scratch/probe" {
		t.Errorf("cwd filter kept %+v (window %d), want the scratch session alone, its cwd named",
			kept.Sessions, kept.Window.Sessions)
	}
	if row := rowByID(t, list, servertest.NewSession); row.CwdName != "" {
		t.Errorf("a session with a repo has cwd_name %q, want none", row.CwdName)
	}
	if none := h.sessionList(t, "?cwd=/nowhere"); len(none.Sessions) != 0 {
		t.Errorf("unknown cwd kept %d sessions, want none", len(none.Sessions))
	}
}

func TestDirName(t *testing.T) {
	for dir, want := range map[string]string{
		"/private/tmp/mad494":  "tmp/mad494",
		"/home/u/.tmp/scratch": ".tmp/scratch",
		"/scratch/probe":       "scratch/probe",
		"/x":                   "x",
		tempDir:                "tmp",
		tempDir + "/eval-1":    "tmp/eval-1",
	} {
		if got := dirName(dir); got != want {
			t.Errorf("dirName(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestRepoName(t *testing.T) {
	for repo, want := range map[string]string{
		"github.com/mad01/thismoon": "mad01/thismoon",
		"git.example.com/a/b/c":     "b/c",
		"owner/repo":                "owner/repo",
		"local":                     "local",
	} {
		if got := repoName(repo); got != want {
			t.Errorf("repoName(%q) = %q, want %q", repo, got, want)
		}
	}
}

// Fixture records of the session addOrchestration adds.
const (
	orchSession = "sess-orch"
	orchTurn    = "turn-orch"
	orchAgent   = "agent-cwd"
)

// addOrchestration adds a priced session whose main conversation edited
// nothing, whose one subagent with a record edited nothing either and
// worked in OtherRepo's checkout, and whose one edit came from a subagent
// known only from that edit, in Repo.
func addOrchestration(h harness) {
	other := store.Location{Repo: servertest.OtherRepo, Worktree: "/work/other"}
	here := store.Location{Repo: servertest.Repo, Worktree: servertest.Worktree}
	h.store.Add(
		store.Session{
			ID:        orchSession,
			Harness:   "test-harness",
			Cwd:       "/work",
			StartedAt: servertest.T0,
		},
		[]store.Turn{servertest.PricedTurn(orchTurn, orchSession)},
		[]store.Agent{servertest.PricedAgent(orchAgent, orchSession, orchTurn, other)},
		[]store.Edit{servertest.Edit("toolu_ghost:0", orchSession, orchTurn, "agent-ghost", 2, here,
			servertest.PathA)},
	)
}

// costsByRepo maps each attributed entry to its cost, nil when unpriced.
func costsByRepo(costs []RepoCost) map[string]*float64 {
	out := map[string]*float64{}
	for _, c := range costs {
		out[c.Repo] = c.Cost
	}
	return out
}

// wantCosts checks that got holds exactly the entries of want, in order,
// with the costs want gives.
func wantCosts(t *testing.T, what string, got []RepoCost, want []string, cost map[string]float64) {
	t.Helper()
	repos := make([]string, 0, len(got))
	for _, c := range got {
		repos = append(repos, c.Repo)
		if c.Cost == nil || !near(*c.Cost, cost[c.Repo]) {
			t.Errorf("%s %s cost = %v, want %v", what, c.Repo, c.Cost, cost[c.Repo])
		}
	}
	if !slices.Equal(repos, want) {
		t.Errorf("%s entries = %v, want %v", what, repos, want)
	}
}

// TestSessionsAttributeCost: the listing splits cost by the attribution
// rule through the HTTP API. A main conversation's cost goes to the
// repository of its edits or to orchestration, a subagent's to its edits'
// repository, else its cwd's, else no_repo. The window adds the parts up
// across sessions. A repo chip and the repo filter count every session
// that edited the repository or sends it a part, and a repo-filtered row
// says what it attributes to that repository: a real zero when the rule
// sends nothing there, none when the session has no priced cost.
func TestSessionsAttributeCost(t *testing.T) {
	h := newHarness(t)
	addOrchestration(h)
	list := h.sessionList(t, "")
	if len(list.Sessions) != 3 {
		t.Fatalf("sessions = %d, want 3", len(list.Sessions))
	}
	newer, orch, older := rowByID(t, list, servertest.NewSession), rowByID(t, list, orchSession),
		rowByID(t, list, servertest.OldSession)
	if older.CostMain == nil || older.CostAgents == nil {
		t.Fatalf("older row = %+v, want it priced", older)
	}
	turnCost, agentCost := *older.CostMain, *older.CostAgents

	wantCosts(t, "older row", older.RepoCosts,
		[]string{servertest.OtherRepo, store.CostNoRepo},
		map[string]float64{servertest.OtherRepo: turnCost, store.CostNoRepo: agentCost})
	wantCosts(t, "orchestration row", orch.RepoCosts,
		[]string{servertest.OtherRepo, store.CostOrchestration},
		map[string]float64{servertest.OtherRepo: agentCost, store.CostOrchestration: turnCost})
	if len(newer.RepoCosts) != 0 {
		t.Errorf("newer row repo_costs = %+v, want none: nothing in it is priced", newer.RepoCosts)
	}
	wantCosts(t, "window", list.Window.RepoCosts,
		[]string{servertest.OtherRepo, store.CostOrchestration, store.CostNoRepo},
		map[string]float64{
			servertest.OtherRepo:    turnCost + agentCost,
			store.CostOrchestration: turnCost,
			store.CostNoRepo:        agentCost,
		})

	parts := 0.0
	for _, c := range list.Window.RepoCosts {
		if c.Cost != nil {
			parts += *c.Cost
		}
	}
	if list.Window.Cost == nil || !near(parts, *list.Window.Cost) {
		t.Errorf("window parts add up to %v, want the window cost %v", parts, list.Window.Cost)
	}

	chips := map[string]RepoCount{}
	for _, r := range list.Repos {
		chips[r.Repo] = r
	}
	if c := chips[servertest.OtherRepo]; c.Sessions != 2 || c.Cost == nil ||
		!near(*c.Cost, turnCost+agentCost) {
		t.Errorf("other repo chip = %+v, want 2 sessions and both sessions' parts", c)
	}
	if c := chips[servertest.Repo]; c.Sessions != 2 || c.Cost != nil {
		t.Errorf("thismoon chip = %+v, want 2 sessions and no cost: no priced part goes there", c)
	}

	filtered := h.sessionList(t, "?repo="+servertest.Repo).Sessions
	got := map[string]*float64{}
	for _, row := range filtered {
		got[row.ID] = row.RepoCost
	}
	if len(filtered) != 2 || got[servertest.NewSession] != nil || got[orchSession] == nil ||
		*got[orchSession] != 0 {
		t.Errorf("thismoon rows' repo_cost = %v, want none for the unpriced one and 0 for "+
			"the one that sends nothing there", got)
	}
	filtered = h.sessionList(t, "?repo="+servertest.OtherRepo).Sessions
	clear(got)
	for _, row := range filtered {
		got[row.ID] = row.RepoCost
	}
	if len(filtered) != 2 || got[orchSession] == nil || !near(*got[orchSession], agentCost) ||
		got[servertest.OldSession] == nil || !near(*got[servertest.OldSession], turnCost) {
		t.Errorf("other repo rows' repo_cost = %v, want the subagent's part of the cwd-only "+
			"session and the main conversation's part of the older one", got)
	}
	if older.RepoCost != nil {
		t.Errorf("unfiltered row repo_cost = %v, want none", *older.RepoCost)
	}
}

// TestSessionAgentsCarryCostRepo: a session's agent rows name where the
// attribution rule sends their cost, and the row's repo_costs add up the
// priced rows by that name.
func TestSessionAgentsCarryCostRepo(t *testing.T) {
	h := newHarness(t)
	addOrchestration(h)
	var view SessionView
	h.getJSON(t, "/api/sessions/"+orchSession, http.StatusOK, &view)
	want := map[string]string{
		"": store.CostOrchestration, orchAgent: servertest.OtherRepo,
		"agent-ghost": "",
	}
	sums := map[string]float64{}
	for _, r := range view.Agents {
		if r.CostRepo != want[r.ID] {
			t.Errorf("agent %q cost_repo = %q, want %q", r.ID, r.CostRepo, want[r.ID])
		}
		if r.Cost != nil {
			sums[r.CostRepo] += *r.Cost
		}
	}
	got := costsByRepo(view.Summary.RepoCosts)
	if len(got) != len(sums) {
		t.Errorf("repo_costs %v, agent sums %v; want one entry per priced cost_repo", got, sums)
	}
	for repo, sum := range sums {
		if got[repo] == nil || !near(*got[repo], sum) {
			t.Errorf("repo_costs[%s] = %v, want the agent rows' %v", repo, got[repo], sum)
		}
	}
}

// TestRepoFilterFollowsAttribution: a session whose only link to a
// repository is a subagent's cwd there appears under that repo filter and
// gets a chip. For every repository, the chip's sessions are the filtered
// rows, and its cost equals both the filtered window's part and the sum
// of the filtered rows' repo_cost.
func TestRepoFilterFollowsAttribution(t *testing.T) {
	h := newHarness(t)
	addOrchestration(h)
	const (
		cwdRepo    = "example.com/owner/cwd-only"
		cwdSession = "sess-cwd"
		cwdTurn    = "turn-cwd"
	)
	cwd := store.Location{Repo: cwdRepo, Worktree: "/work/cwd-only"}
	h.store.Add(
		store.Session{
			ID:        cwdSession,
			Harness:   "test-harness",
			Cwd:       "/work",
			StartedAt: servertest.T0,
		},
		[]store.Turn{servertest.PricedTurn(cwdTurn, cwdSession)},
		[]store.Agent{servertest.PricedAgent("agent-cwd-only", cwdSession, cwdTurn, cwd)},
		nil,
	)
	chips := map[string]RepoCount{}
	for _, r := range h.sessionList(t, "").Repos {
		chips[r.Repo] = r
	}
	if c, ok := chips[cwdRepo]; !ok || c.Sessions != 1 || c.Cost == nil {
		t.Fatalf("cwd-only chip = %+v (listed %v), want 1 session with a priced cost", c, ok)
	}
	for _, repo := range []string{cwdRepo, servertest.OtherRepo, servertest.Repo} {
		chip := chips[repo]
		list := h.sessionList(t, "?repo="+repo)
		ids, sum := make([]string, 0, len(list.Sessions)), 0.0
		for _, row := range list.Sessions {
			ids = append(ids, row.ID)
			if row.RepoCost != nil {
				sum += *row.RepoCost
			}
		}
		if len(ids) != chip.Sessions {
			t.Errorf("%s: chip counts %d sessions, filtered rows %v", repo, chip.Sessions, ids)
		}
		if repo == cwdRepo && !slices.Contains(ids, cwdSession) {
			t.Errorf("%s: filtered rows %v, want %s", repo, ids, cwdSession)
		}
		part := costsByRepo(list.Window.RepoCosts)[repo]
		if chip.Cost == nil {
			if sum != 0 || part != nil {
				t.Errorf("%s: unpriced chip, rows add %v and window part %v", repo, sum, part)
			}
			continue
		}
		if !near(sum, *chip.Cost) || part == nil || !near(*part, *chip.Cost) {
			t.Errorf("%s: chip cost %v, filtered rows add %v, window part %v; want one number",
				repo, *chip.Cost, sum, part)
		}
	}
}

// TestCwdReposCount: a row lists the repositories its session worked in
// without editing them, its own cwd's and its subagents' cwds', and names
// its entrypoint and tags. A session counts toward each of them as toward
// the ones it edited or sends cost to: the repo filter keeps it, the chip
// counts it once, and the chip's cost and the rows' repo_cost stay the
// attributed cost.
func TestCwdReposCount(t *testing.T) {
	h := newHarness(t)
	const (
		thirdRepo   = "example.com/owner/third"
		fourthRepo  = "example.com/owner/fourth"
		quietID     = "sess-quiet"
		editorID    = "sess-editor"
		scratchID   = "sess-scratch"
		scoutID     = "agent-scout"
		headlessCLI = "sdk-cli"
	)
	here := store.Location{Repo: servertest.Repo, Worktree: servertest.Worktree}
	fourth := store.Location{Repo: fourthRepo, Worktree: "/home/.worktrees/fourth/scout"}
	// quiet's cwd sits in Repo and it edits nothing.
	h.store.Add(
		store.Session{
			ID: quietID, Harness: "test-harness", Cwd: servertest.Worktree, Location: here,
			Entrypoint: headlessCLI, Tags: []string{"eval"}, StartedAt: servertest.T0,
		},
		[]store.Turn{servertest.PricedTurn("turn-quiet", quietID)}, nil, nil,
	)
	// editor's cwd sits in third, and it edits Repo.
	h.store.Add(
		store.Session{
			ID: editorID, Harness: "test-harness", Cwd: "/work/third",
			Location:  store.Location{Repo: thirdRepo, Worktree: "/work/third"},
			StartedAt: servertest.T0,
		},
		[]store.Turn{servertest.PricedTurn("turn-editor", editorID)},
		nil,
		[]store.Edit{
			servertest.Edit("toolu_ed:0", editorID, "turn-editor", "", 2, here, servertest.PathA),
		},
	)
	// scratch starts outside any repository and edits nothing. One
	// subagent reads in a worktree of fourth, one runs outside any
	// repository.
	h.store.Add(
		store.Session{
			ID: scratchID, Harness: "test-harness", Cwd: "/home/.tmp/scratch",
			StartedAt: servertest.T0,
		},
		[]store.Turn{servertest.PricedTurn("turn-scratch", scratchID)},
		[]store.Agent{
			servertest.PricedAgent(scoutID, scratchID, "turn-scratch", fourth),
			servertest.PricedAgent("agent-drifter", scratchID, "turn-scratch", store.Location{}),
		},
		nil,
	)

	all := h.sessionList(t, "")
	rows := map[string]SessionSummary{}
	for _, r := range all.Sessions {
		rows[r.ID] = r
	}
	wantRow := map[string][2][]string{
		quietID:   {{servertest.Repo}, {"owner/thismoon"}},
		editorID:  {{thirdRepo}, {"owner/third"}},
		scratchID: {{fourthRepo}, {"owner/fourth"}},
	}
	for id, want := range wantRow {
		r := rows[id]
		if !slices.Equal(r.CwdRepos, want[0]) || !slices.Equal(r.CwdRepoNames, want[1]) {
			t.Errorf(
				"%s: cwd_repos %v (%v), want %v (%v)",
				id,
				r.CwdRepos,
				r.CwdRepoNames,
				want[0],
				want[1],
			)
		}
	}
	if quiet := rows[quietID]; len(quiet.Repos) != 0 || quiet.Entrypoint != headlessCLI ||
		!slices.Equal(quiet.Tags, []string{"eval"}) {
		t.Errorf(
			"quiet row repos %v entrypoint %q tags %v",
			quiet.Repos,
			quiet.Entrypoint,
			quiet.Tags,
		)
	}
	if old := rows[servertest.OldSession]; old.CwdRepos != nil || old.CwdRepoNames != nil ||
		old.Entrypoint != "" || old.Tags != nil {
		t.Errorf("a session without an origin has cwd repos %v (%v) entrypoint %q tags %v",
			old.CwdRepos, old.CwdRepoNames, old.Entrypoint, old.Tags)
	}

	var view SessionView
	h.getJSON(t, "/api/sessions/"+scratchID, http.StatusOK, &view)
	var scoutCost *float64
	for _, a := range view.Agents {
		if a.ID == scoutID && a.CostRepo == fourthRepo {
			scoutCost = a.Cost
		}
	}
	if scoutCost == nil {
		t.Fatalf(
			"scout agent row %+v: want a priced cost attributed to %s",
			view.Agents,
			fourthRepo,
		)
	}

	chips := map[string]RepoCount{}
	for _, r := range all.Repos {
		chips[r.Repo] = r
	}
	// parts maps each filter's rows to their repo_cost.
	parts := map[string]map[string]*float64{}
	for repo, want := range map[string][]string{
		servertest.Repo: {editorID, servertest.NewSession, quietID},
		thirdRepo:       {editorID},
		fourthRepo:      {scratchID},
	} {
		chip := chips[repo]
		ids, sum := []string{}, 0.0
		parts[repo] = map[string]*float64{}
		for _, row := range h.sessionList(t, "?repo="+repo).Sessions {
			ids = append(ids, row.ID)
			parts[repo][row.ID] = row.RepoCost
			if row.RepoCost != nil {
				sum += *row.RepoCost
			}
		}
		slices.Sort(ids)
		if !slices.Equal(ids, want) || chip.Sessions != len(want) {
			t.Errorf(
				"%s: filtered rows %v, chip counts %d; want %v",
				repo,
				ids,
				chip.Sessions,
				want,
			)
		}
		if (chip.Cost == nil && sum != 0) || (chip.Cost != nil && !near(sum, *chip.Cost)) {
			t.Errorf(
				"%s: chip cost %v, filtered rows add %v; want one number",
				repo,
				chip.Cost,
				sum,
			)
		}
	}
	// quiet's and editor's main conversations send their cost elsewhere, so
	// the repository they only worked in gets a real zero.
	if p := parts[servertest.Repo][quietID]; p == nil || *p != 0 {
		t.Errorf("quiet session's repo_cost %v, want a real zero", p)
	}
	if p := parts[thirdRepo][editorID]; p == nil || *p != 0 {
		t.Errorf("editor session's repo_cost for %s %v, want a real zero", thirdRepo, p)
	}
	if p := parts[fourthRepo][scratchID]; p == nil || !near(*p, *scoutCost) {
		t.Errorf(
			"scratch session's repo_cost for %s %v, want the scout's %v",
			fourthRepo,
			p,
			*scoutCost,
		)
	}
}

// TestUntotalledAgentCountsItsRepo: a session whose only link to a
// repository is a subagent with its cwd there and no totals yet counts
// toward that repository before the totals backfill reaches the agent.
// The repo filter keeps it and the chip counts it, but it adds no cost
// there: its repo_cost is a real zero when the row is priced and nil when
// not, and the chip's cost leaves it out.
func TestUntotalledAgentCountsItsRepo(t *testing.T) {
	h := newHarness(t)
	const (
		fifthRepo  = "example.com/owner/fifth"
		pricedID   = "sess-waiting-priced"
		unpricedID = "sess-waiting-unpriced"
	)
	fifth := store.Location{Repo: fifthRepo, Worktree: "/home/.worktrees/fifth/scout"}
	// waiting adds a session that starts outside any repository, edits
	// nothing and runs one subagent in fifth that has no totals yet. Its
	// one turn has totals when priced.
	waiting := func(id string, priced bool) {
		turn := servertest.PricedTurn(id+"-turn", id)
		if !priced {
			turn.Totals = nil
		}
		agent := servertest.PricedAgent(id+"-agent", id, turn.ID, fifth)
		agent.Totals = nil
		h.store.Add(
			store.Session{
				ID: id, Harness: "test-harness", Cwd: "/home/.tmp/scratch",
				StartedAt: servertest.T0,
			},
			[]store.Turn{turn}, []store.Agent{agent}, nil,
		)
	}
	waiting(pricedID, true)
	waiting(unpricedID, false)

	var chip RepoCount
	for _, r := range h.sessionList(t, "").Repos {
		if r.Repo == fifthRepo {
			chip = r
		}
	}
	rows := map[string]SessionSummary{}
	ids := []string{}
	for _, r := range h.sessionList(t, "?repo="+fifthRepo).Sessions {
		rows[r.ID] = r
		ids = append(ids, r.ID)
	}
	slices.Sort(ids)
	if want := []string{pricedID, unpricedID}; !slices.Equal(ids, want) ||
		chip.Sessions != len(want) {
		t.Errorf("filtered rows %v, chip counts %d; want %v", ids, chip.Sessions, want)
	}
	if r := rows[pricedID]; r.Cost == nil || r.RepoCost == nil || *r.RepoCost != 0 {
		t.Errorf("priced session's cost %v, repo_cost %v; want a cost and a real zero",
			r.Cost, r.RepoCost)
	}
	if r := rows[unpricedID]; r.Cost != nil || r.RepoCost != nil {
		t.Errorf("unpriced session's cost %v, repo_cost %v; want both nil", r.Cost, r.RepoCost)
	}
	if chip.Cost != nil || chip.Unpriced != nil {
		t.Errorf("chip cost %v (unpriced %v), want no part for agents without totals",
			chip.Cost, chip.Unpriced)
	}
}
