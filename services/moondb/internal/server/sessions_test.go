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
			if !tc.timed && !slices.Equal(got.Repos, wantRepos) {
				t.Errorf("repos = %+v, want %+v", got.Repos, wantRepos)
			}
		})
	}
}

// TestSessionsCarryActiveTime: every row and the window answer both
// times, active_ms and duration_ms. The older session's closed turn gives
// its totals' wall less user time, the newer one's open turn runs from its
// start to its last edit, and the window sums the rows' active time.
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
	if newer.ActiveMS != wantNew || newer.DurationMS != 0 {
		t.Errorf("newer active %d, wall %d; want %d to its last edit and 0 while open",
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
