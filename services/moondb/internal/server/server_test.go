package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "moondb/v0.0.0",
	BuildTime: "2026-10-01T09:00:00Z",
}

// fixedNow is the clock the test server parses relative since values against.
var fixedNow = servertest.T0.Add(90 * time.Minute)

type harness struct {
	ts    *httptest.Server
	store *servertest.Store
	query *servertest.Querier
}

func newHarness(t *testing.T) harness {
	t.Helper()
	st := servertest.NewStore()
	q := servertest.NewQuerier(st)
	s := New(st, q, testInfo)
	s.now = func() time.Time { return fixedNow }
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return harness{ts: ts, store: st, query: q}
}

// get fetches path and fails the test unless the status is want.
func (h harness) get(t *testing.T, path string, want int) (*http.Response, []byte) {
	t.Helper()
	res, err := http.Get(h.ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if res.StatusCode != want {
		t.Fatalf("GET %s = %d, want %d (%s)", path, res.StatusCode, want, body)
	}
	return res, body
}

// getJSON fetches path, checks the status, and decodes the body into out.
func (h harness) getJSON(t *testing.T, path string, want int, out any) {
	t.Helper()
	res, body := h.get(t, path, want)
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET %s Content-Type = %q, want application/json", path, ct)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v (%s)", path, err, body)
	}
}

// wantAPIError checks the error shape: a JSON object whose error names want.
func (h harness) wantAPIError(t *testing.T, path string, status int, want string) {
	t.Helper()
	var body map[string]string
	h.getJSON(t, path, status, &body)
	if !strings.HasPrefix(body["error"], "moondb: ") || !strings.Contains(body["error"], want) {
		t.Errorf(
			"GET %s error = %q, want a moondb: message containing %q",
			path,
			body["error"],
			want,
		)
	}
}

// TestQueryServiceIsAQuerier pins the wiring the CLI does: the real query
// service plugs into New without an adapter.
func TestQueryServiceIsAQuerier(t *testing.T) {
	var _ Querier = &query.Service{}
}

func TestSessionsListsSummariesNewestFirst(t *testing.T) {
	h := newHarness(t)
	var got struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	h.getJSON(t, "/api/sessions", http.StatusOK, &got)
	if len(got.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(got.Sessions))
	}
	newest := got.Sessions[0]
	if newest.ID != servertest.NewSession || got.Sessions[1].ID != servertest.OldSession {
		t.Errorf("order = %s, %s; want newest first", newest.ID, got.Sessions[1].ID)
	}
	if newest.EditCount != 2 || newest.TurnCount != 1 {
		t.Errorf("counts = %d edits, %d turns; want 2, 1", newest.EditCount, newest.TurnCount)
	}
	if !slices.Equal(newest.Repos, []string{servertest.Repo}) ||
		!slices.Equal(newest.Worktrees, []string{servertest.Worktree}) {
		t.Errorf("repos %v worktrees %v", newest.Repos, newest.Worktrees)
	}
	if newest.Title != "build the server" {
		t.Errorf("title = %q, want the session's harness title", newest.Title)
	}
}

func TestSessionsFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"repo", "?repo=" + servertest.OtherRepo, []string{servertest.OldSession}},
		{"limit", "?limit=1", []string{servertest.NewSession}},
		{"since rfc3339", "?since=2026-10-01T09:30:00Z", []string{servertest.NewSession}},
		{"since duration", "?since=45m", []string{servertest.NewSession}},
		{"since days", "?since=1d", []string{servertest.NewSession, servertest.OldSession}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			var got struct {
				Sessions []SessionSummary `json:"sessions"`
			}
			h.getJSON(t, "/api/sessions"+tc.query, http.StatusOK, &got)
			ids := make([]string, len(got.Sessions))
			for i, s := range got.Sessions {
				ids[i] = s.ID
			}
			if !slices.Equal(ids, tc.want) {
				t.Errorf("ids = %v, want %v", ids, tc.want)
			}
		})
	}
}

func TestListingsRejectBadWindow(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{
		"/api/sessions?limit=0",
		"/api/sessions?limit=many",
		"/api/sessions?since=yesterday",
		"/api/sessions?since=-5m",
		"/api/edits?limit=-1",
		"/api/edits?since=soon",
		"/api/history?path=a.go&limit=x",
	} {
		h.wantAPIError(t, path, http.StatusBadRequest, "must be")
	}
}

func TestSessionDetail(t *testing.T) {
	h := newHarness(t)
	var got store.SessionDetail
	h.getJSON(t, "/api/sessions/"+servertest.NewSession, http.StatusOK, &got)
	if got.ID != servertest.NewSession || len(got.Turns) != 1 || got.EditCount != 2 {
		t.Errorf("detail = %+v", got)
	}
	h.wantAPIError(t, "/api/sessions/nope", http.StatusNotFound, "not found")
}

func TestTurnDetail(t *testing.T) {
	h := newHarness(t)
	var got store.TurnDetail
	h.getJSON(t, "/api/turns/"+servertest.NewTurn, http.StatusOK, &got)
	if got.ID != servertest.NewTurn || got.PromptChunk != servertest.PromptSHA {
		t.Errorf("turn = %+v", got.Turn)
	}
	if len(got.Edits) != 2 || got.Edits[0].ID != servertest.EditA {
		t.Errorf("edits = %+v, want EditA first of 2", got.Edits)
	}
	if len(got.Snapshots) != 1 || got.Snapshots[0].ID != servertest.SnapshotID ||
		got.Snapshots[0].FileCount != 2 || got.Snapshots[0].Worktree != servertest.Worktree ||
		got.Snapshots[0].Skipped.TooLarge != 1 {
		t.Errorf("snapshots = %+v, want the fixture's one with 2 files, 1 too large", got.Snapshots)
	}
	if len(got.Deltas) != 1 || got.Deltas[0].ID != servertest.SnapshotDeltaID ||
		got.Deltas[0].Cause != store.DeltaSnapshot {
		t.Errorf("deltas = %+v, want the snapshot's one", got.Deltas)
	}
	h.wantAPIError(t, "/api/turns/nope", http.StatusNotFound, "not found")
}

func TestEditsFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"all", "", []string{servertest.EditA, servertest.EditB, servertest.EditC}},
		{"session", "?session=" + servertest.OldSession, []string{servertest.EditC}},
		{"turn and path", "?turn=" + servertest.NewTurn + "&path=b.go", []string{servertest.EditB}},
		{
			"worktree",
			"?worktree=" + servertest.Worktree,
			[]string{servertest.EditA, servertest.EditB},
		},
		{"repo", "?repo=" + servertest.OtherRepo, []string{servertest.EditC}},
		{"blob", "?blob=blob-a", []string{servertest.EditA}},
		{"limit", "?limit=1", []string{servertest.EditA}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			var got struct {
				Edits []store.Edit `json:"edits"`
			}
			h.getJSON(t, "/api/edits"+tc.query, http.StatusOK, &got)
			ids := make([]string, len(got.Edits))
			for i, e := range got.Edits {
				ids[i] = e.ID
			}
			if !slices.Equal(ids, tc.want) {
				t.Errorf("ids = %v, want %v", ids, tc.want)
			}
		})
	}
}

func TestWhyParsesAndAnswers(t *testing.T) {
	h := newHarness(t)
	var got query.WhyResult
	h.getJSON(t, "/api/why?worktree=/work/thismoon&path=a.go&line=3&text=package+a",
		http.StatusOK, &got)
	if got.Method != query.MethodContent || got.Edit == nil || got.Edit.ID != servertest.EditA {
		t.Errorf("result = %+v", got)
	}
	want := query.WhyRequest{Worktree: "/work/thismoon", Path: "a.go", Line: 3, Text: "package a"}
	if req := h.query.LastWhy(); req != want {
		t.Errorf("querier got %+v, want %+v", req, want)
	}
}

func TestWhyRejectsBadRequest(t *testing.T) {
	h := newHarness(t)
	h.wantAPIError(t, "/api/why?line=3", http.StatusBadRequest, "path is required")
	h.wantAPIError(t, "/api/why?path=a.go", http.StatusBadRequest, "line must be")
	h.wantAPIError(t, "/api/why?path=a.go&line=0", http.StatusBadRequest, "line must be")
}

func TestQueryErrorsMapToStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"invalid request", fmt.Errorf("%w: bad", query.ErrInvalidRequest), http.StatusBadRequest},
		{"not implemented", query.ErrNotImplemented, http.StatusNotImplemented},
		{"not found", fmt.Errorf("edit x: %w", store.ErrNotFound), http.StatusNotFound},
		{"other", errors.New("git exploded"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.query.SetErr(tc.err)
			h.wantAPIError(t, "/api/why?path=a.go&line=1", tc.code, tc.err.Error())
			h.wantAPIError(t, "/api/history?path=a.go", tc.code, tc.err.Error())
			h.wantAPIError(t, "/api/annotate?worktree=/w&path=a.go", tc.code, tc.err.Error())
		})
	}
}

func TestHistoryParsesAndAnswers(t *testing.T) {
	h := newHarness(t)
	var got struct {
		Edits []store.Edit `json:"edits"`
	}
	h.getJSON(t, "/api/history?repo="+servertest.Repo+"&path=a.go", http.StatusOK, &got)
	if len(got.Edits) != 1 || got.Edits[0].ID != servertest.EditA {
		t.Errorf("edits = %+v", got.Edits)
	}
	want := query.HistoryRequest{Repo: servertest.Repo, Path: "a.go", Limit: DefaultLimit}
	if req := h.query.LastHistory(); req != want {
		t.Errorf("querier got %+v, want %+v", req, want)
	}
	h.wantAPIError(t, "/api/history?repo=x", http.StatusBadRequest, "path is required")
}

func TestAnnotateParsesAndAnswers(t *testing.T) {
	h := newHarness(t)
	var got query.AnnotateResult
	h.getJSON(t, "/api/annotate?worktree="+servertest.Worktree+"&repo="+servertest.Repo+
		"&path=a.go", http.StatusOK, &got)
	if got.Worktree != servertest.Worktree || got.Path != servertest.PathA || len(got.Lines) != 1 {
		t.Fatalf("result = %+v", got)
	}
	want := query.AnnotatedLine{
		Line: 1, Text: "package a", Method: query.MethodContent, EditID: servertest.EditA,
		TurnID: servertest.NewTurn, SessionID: servertest.NewSession,
	}
	if got.Lines[0] != want {
		t.Errorf("line = %+v, want %+v", got.Lines[0], want)
	}
	wantReq := query.AnnotateRequest{
		Worktree: servertest.Worktree,
		Repo:     servertest.Repo,
		Path:     servertest.PathA,
	}
	if req := h.query.LastAnnotate(); req != wantReq {
		t.Errorf("querier got %+v, want %+v", req, wantReq)
	}
}

func TestAnnotateRejectsBadRequest(t *testing.T) {
	h := newHarness(t)
	h.wantAPIError(t, "/api/annotate?worktree=/w", http.StatusBadRequest, "path is required")
	h.wantAPIError(t, "/api/annotate?path=a.go", http.StatusBadRequest, "worktree is required")
}

func TestChunkServesImmutableBytes(t *testing.T) {
	h := newHarness(t)
	res, body := h.get(t, "/api/chunks/"+servertest.AfterSHA, http.StatusOK)
	if string(body) != servertest.After {
		t.Errorf("body = %q, want %q", body, servertest.After)
	}
	for header, want := range map[string]string{
		"Content-Type":           "application/octet-stream",
		"Cache-Control":          chunkCacheControl,
		"X-Content-Type-Options": "nosniff",
		"Content-Length":         fmt.Sprint(len(servertest.After)),
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Error("chunk responses must be cacheable as immutable")
	}
}

func TestChunkErrors(t *testing.T) {
	h := newHarness(t)
	h.wantAPIError(t, "/api/chunks/"+strings.Repeat("0", 64), http.StatusNotFound, "not found")
	h.wantAPIError(t, "/api/chunks/abc", http.StatusBadRequest, "not a lowercase hex SHA-256")
	upper := strings.ToUpper(servertest.AfterSHA)
	h.wantAPIError(t, "/api/chunks/"+upper, http.StatusBadRequest, "not a lowercase hex SHA-256")
	h.wantAPIError(t, "/api/chunks/"+strings.Repeat("g", 64), http.StatusBadRequest, "hex")
}

func TestHealthzFollowsPing(t *testing.T) {
	h := newHarness(t)
	h.get(t, "/healthz", http.StatusNoContent)
	h.store.SetPingErr(errors.New("disk gone"))
	h.wantAPIError(t, "/healthz", http.StatusServiceUnavailable, "disk gone")
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	var got buildinfo.Info
	h.getJSON(t, "/version", http.StatusOK, &got)
	if got != testInfo {
		t.Errorf("version = %+v, want %+v", got, testInfo)
	}
}

func TestUnknownAPIRouteIsJSON(t *testing.T) {
	h := newHarness(t)
	h.wantAPIError(t, "/api/edits/toolu_a:0", http.StatusNotFound, "no API route")
}

func TestPageServesShellAndApp(t *testing.T) {
	h := newHarness(t)
	res, body := h.get(t, "/", http.StatusOK)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Errorf("shell Content-Type = %q", res.Header.Get("Content-Type"))
	}
	for _, want := range []string{"<wk-header", `src="/app.js"`, "/webkit/webkit.js"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("shell missing %q", want)
		}
	}
	res, body = h.get(t, "/app.js", http.StatusOK)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/javascript") {
		t.Errorf("app.js Content-Type = %q", res.Header.Get("Content-Type"))
	}
	for _, route := range []string{
		"/api/sessions", "/api/turns/", "/api/chunks/", "/api/why", "/api/history",
		"/api/annotate", "/api/edits",
	} {
		if !strings.Contains(string(body), route) {
			t.Errorf("app.js never calls %s", route)
		}
	}
	h.get(t, "/webkit/webkit.css", http.StatusOK)
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		raw     string
		want    time.Time
		wantErr bool
	}{
		{raw: "", want: time.Time{}},
		{raw: "2026-10-24T08:00:00Z", want: time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)},
		{raw: "90m", want: now.Add(-90 * time.Minute)},
		{raw: "24h", want: now.Add(-24 * time.Hour)},
		{raw: "7d", want: now.Add(-7 * 24 * time.Hour)},
		{raw: "0s", wantErr: true},
		{raw: "-1h", wantErr: true},
		{raw: "xd", wantErr: true},
		{raw: "last week", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseSince(tc.raw, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSince(%q) = %v, want an error", tc.raw, got)
				}
				return
			}
			if err != nil || !got.Equal(tc.want) {
				t.Errorf("ParseSince(%q) = %v, %v; want %v", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestTurnCarriesTraceTotals(t *testing.T) {
	h := newHarness(t)
	var old TurnView
	h.getJSON(t, "/api/turns/"+servertest.OldTurn, http.StatusOK, &old)
	tot := old.TraceTotals
	if tot == nil {
		t.Fatal("the closed turn with a transcript has no trace_totals")
	}
	// Model 30s then 19s, tool 60s, harness 1s plus 10s to Stop.
	if tot.WallMS != 120_000 || tot.ModelMS != 49_000 || tot.ToolMS != 60_000 ||
		tot.HarnessMS != 11_000 || tot.ModelCalls != 2 || tot.ToolCalls != 1 {
		t.Errorf("trace_totals = %+v", tot)
	}
	// The main conversation's 1010 input and 520 output tokens, and the
	// reviewer's 2100 and 150, on claude-opus-5-5.
	if got := fmt.Sprintf("%.5f %.5f %.5f", deref(tot.Cost), deref(tot.CostMain),
		deref(tot.CostAgents)); got != "0.02584 0.01444 0.01140" {
		t.Errorf("cost, main, agents = %s; want the turn's calls and the reviewer's", got)
	}
	if len(old.Agents) != 2 || old.Agents[0].Type != MainAgentType || old.Agents[0].EditCount != 1 ||
		old.Agents[1].ID != servertest.AgentID || old.Agents[1].ModelCalls != 2 ||
		!old.Agents[1].Recorded ||
		old.Agents[1].DurationMS != 120_000 {
		t.Errorf("agents = %+v, want main with its edit, then the reviewer's 2 calls", old.Agents)
	}
	var open TurnView
	h.getJSON(t, "/api/turns/"+servertest.NewTurn, http.StatusOK, &open)
	if open.TraceTotals != nil {
		t.Errorf("the open turn has no transcript yet, so no totals; got %+v", open.TraceTotals)
	}
}

func TestTurnCarriesSkipped(t *testing.T) {
	// The turn page reads the skipped counts from the turn's own JSON, one
	// entry per Bash call with a count per reason.
	h := newHarness(t)
	var body map[string]json.RawMessage
	h.getJSON(t, "/api/turns/"+servertest.NewTurn, http.StatusOK, &body)
	var got []map[string]any
	if err := json.Unmarshal(body["skipped"], &got); err != nil {
		t.Fatalf("skipped = %s: %v", body["skipped"], err)
	}
	want := map[string]any{
		"tool": "Bash", "tool_use_id": "toolu_pull", "at": "2026-10-01T10:02:00Z",
		"clean": 40.0, "timeout": 2.0, "failed": 1.0,
	}
	if len(got) != 1 || fmt.Sprint(got[0]) != fmt.Sprint(want) {
		t.Errorf("skipped = %v, want [%v]", got, want)
	}
	var old map[string]json.RawMessage
	h.getJSON(t, "/api/turns/"+servertest.OldTurn, http.StatusOK, &old)
	if raw, ok := old["skipped"]; ok {
		t.Errorf("a turn with nothing skipped has skipped = %s, want the key left out", raw)
	}
}

func TestSessionTrace(t *testing.T) {
	h := newHarness(t)
	var got trace.SessionTrace
	h.getJSON(t, "/api/sessions/"+servertest.OldSession+"/trace", http.StatusOK, &got)
	if got.SessionID != servertest.OldSession || len(got.Turns) != 1 {
		t.Fatalf("trace = %+v, want the old session's one turn", got)
	}
	var kinds []trace.Kind
	for _, s := range got.Turns[0].Spans {
		kinds = append(kinds, s.Kind)
	}
	want := []trace.Kind{trace.KindModel, trace.KindTool, trace.KindSubagent, trace.KindModel}
	if !slices.Equal(kinds, want) {
		t.Errorf("span kinds = %v, want model, tool, the reviewer's subagent span, model", kinds)
	}
	if tool := got.Turns[0].Spans[1]; !slices.Equal(tool.EditIDs, []string{servertest.EditC}) {
		t.Errorf("tool span edits = %v, want EditC", tool.EditIDs)
	}
	if got.Totals.WallMS != 120_000 || got.Gaps == nil {
		t.Errorf("totals = %+v, gaps = %v", got.Totals, got.Gaps)
	}
	h.wantAPIError(t, "/api/sessions/nope/trace", http.StatusNotFound, "not found")
}

func TestSessionsCarryDuration(t *testing.T) {
	h := newHarness(t)
	var got struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	h.getJSON(t, "/api/sessions", http.StatusOK, &got)
	durations := map[string]int64{}
	for _, s := range got.Sessions {
		durations[s.ID] = s.DurationMS
	}
	if durations[servertest.OldSession] != 120_000 || durations[servertest.NewSession] != 0 {
		t.Errorf(
			"durations = %v, want 2 minutes for the old session and 0 for the open one",
			durations,
		)
	}
}

func deref(f *float64) float64 {
	if f == nil {
		return -1
	}
	return *f
}

// TestSessionAgents: a session lists the main conversation first, then
// each subagent with its calls and cost, the rows taking the place of the
// store's records.
func TestSessionAgents(t *testing.T) {
	h := newHarness(t)
	var v SessionView
	h.getJSON(t, "/api/sessions/"+servertest.OldSession, http.StatusOK, &v)
	if len(v.Agents) != 2 {
		t.Fatalf("agents = %+v, want main and the reviewer", v.Agents)
	}
	main, rev := v.Agents[0], v.Agents[1]
	if main.ID != "" || main.Type != MainAgentType || main.ModelCalls != 2 || main.EditCount != 1 ||
		fmt.Sprintf("%.5f", deref(main.Cost)) != "0.01444" ||
		!slices.Equal(main.Repos, []string{servertest.OtherRepo}) {
		t.Errorf("main row = %+v", main)
	}
	if rev.ID != servertest.AgentID || rev.Name != servertest.AgentName ||
		rev.Type != "general-purpose" || rev.ToolCalls != 1 || rev.DurationMS != 120_000 ||
		fmt.Sprintf("%.5f", deref(rev.Cost)) != "0.01140" ||
		!slices.Equal(rev.TurnIDs, []string{servertest.OldTurn}) || len(rev.Repos) != 0 {
		t.Errorf("reviewer row = %+v", rev)
	}
}

// TestAgentRoute answers one agent with its record, row and trace, and
// 404 for an unknown id.
func TestAgentRoute(t *testing.T) {
	h := newHarness(t)
	var v AgentView
	h.getJSON(t, "/api/agents/"+servertest.AgentID, http.StatusOK, &v)
	if v.ID != servertest.AgentID || v.Row.ModelCalls != 2 || len(v.Trace.Spans) != 3 ||
		len(v.Trace.Segments) != 1 || v.Trace.Segments[0].TurnID != servertest.OldTurn {
		t.Errorf("agent = %+v", v)
	}
	h.wantAPIError(t, "/api/agents/nope", http.StatusNotFound, "not found")
}
