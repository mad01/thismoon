package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
)

// assertionID is the kof assertion the fixture's hint names.
const assertionID = "01792948229235707000-1e1c"

// fakeEvents is an events service holding entries per source, newest
// first. It filters by q and before as the real one does, and fails every
// request when down is set.
func fakeEvents(t *testing.T, entries map[string][]scorecard.Event, down bool) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down || r.URL.Path != "/api/events" {
			http.Error(w, "events: unavailable", http.StatusServiceUnavailable)
			return
		}
		q := r.URL.Query()
		out := []scorecard.Event{}
		for _, e := range entries[q.Get("source")] {
			body, _ := json.Marshal(e)
			if b := q.Get("before"); b != "" && e.ID >= b {
				continue
			}
			if s := q.Get("q"); s != "" && !strings.Contains(string(body), s) {
				continue
			}
			out = append(out, e)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

// fakeKof is keeper-of-facts holding the assertion the hint names, pinned
// to a.go, and another one. It answers only the listing a scorecard asks
// for, and counts the requests.
func fakeKof(t *testing.T, requests *int) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if r.URL.Path != "/api/assertions" {
			http.NotFound(w, r)
			return
		}
		pins := []map[string]any{
			{"repo": "github.com/owner/other", "file": "a.go", "start_line": 1, "end_line": 9},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"assertions": []map[string]any{
			{"id": "01792948229235707000-ffff", "claim": "b.go does another", "pins": pins},
			{"id": assertionID, "claim": "a.go does the thing", "pins": pins},
		}})
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

// sessionEvents are the belt and belt-ledger entries of the older
// session, which runs from minute 1, with one older entry of another
// session in each source, so neither listing reaches back to its cap. Ids
// start with the unix nanoseconds, as the events service writes them.
func sessionEvents() map[string][]scorecard.Event {
	ev := func(minutes int, session, title, message string, tags map[string]string) scorecard.Event {
		when := servertest.T0.Add(time.Duration(minutes) * time.Minute)
		tags["session_id"] = session
		return scorecard.Event{
			ID: fmt.Sprintf("%020d-0001", when.UnixNano()), Time: when, Title: title,
			Message: message, Tags: tags,
		}
	}
	older := func() scorecard.Event {
		return ev(-60, "sess-gone", "hinted Bash (prefer-csl)", "", map[string]string{})
	}
	kofText := "1 stored assertion(s) for repo:owner/other\n  [fresh] a.go does the thing\n" +
		"          a.go:1-9 (kof_get id=" + assertionID + ")"
	return map[string][]scorecard.Event{
		"belt": {
			ev(2, servertest.OldSession, "hinted Edit (kof-assertions)", kofText,
				map[string]string{"hint": "kof-assertions", "event": "edit"}),
			ev(2, servertest.NewSession, "hinted Bash (prefer-csl)", "",
				map[string]string{"hint": "prefer-csl"}),
			ev(1, servertest.OldSession, "hinted Bash (prefer-csl)", "",
				map[string]string{"hint": "prefer-csl"}),
			older(),
		},
		"belt-ledger": {
			ev(3, servertest.OldSession, "", "", map[string]string{"event": "subagent-stop"}),
			ev(2, servertest.OldSession, "", "", map[string]string{"event": "subagent-start"}),
			older(),
		},
	}
}

// TestSessionScorecard: the scorecard joins the session's trace with its
// hints from the events service and their pins from keeper-of-facts.
func TestSessionScorecard(t *testing.T) {
	h := newHarness(t)
	var kofRequests int
	h.query.SetPeers(scorecard.NewHTTPPeers(
		fakeEvents(t, sessionEvents(), false), fakeKof(t, &kofRequests)))

	var sc scorecard.Scorecard
	h.getJSON(t, "/api/sessions/"+servertest.OldSession+"/scorecard", http.StatusOK, &sc)
	if sc.SessionID != servertest.OldSession || sc.Version != scorecard.Version {
		t.Errorf("scorecard = %+v", sc)
	}
	ok := scorecard.Source{Status: scorecard.StatusOK}
	if sc.Sources.Transcripts != ok || sc.Sources.Events != ok || sc.Sources.Kof != ok {
		t.Errorf("sources = %+v, want all ok", sc.Sources)
	}
	if sc.Totals.Turns != 1 || sc.Agents.Count != 1 {
		t.Errorf("totals = %+v, agents = %+v; want one turn and one agent", sc.Totals, sc.Agents)
	}
	if sc.Hints == nil || sc.Hints.Shown != 2 || len(sc.Hints.ByHint) != 2 {
		t.Fatalf("hints = %+v, want the older session's two", sc.Hints)
	}
	k := sc.Hints.Kof
	if len(k.Assertions) != 1 || k.Assertions[0].ID != assertionID || k.PinnedPaths != 1 ||
		k.Assertions[0].Used == nil || *k.Assertions[0].Used {
		t.Errorf("kof = %+v, want one assertion pinned to a.go, not used", k)
	}
	if kofRequests != 1 {
		t.Errorf("keeper-of-facts requests = %d, want the one listing", kofRequests)
	}
	want := scorecard.Ledger{SubagentStart: 1, SubagentStop: 1}
	if sc.Agents.Ledger == nil || *sc.Agents.Ledger != want {
		t.Errorf("ledger = %+v, want %+v", sc.Agents.Ledger, want)
	}
	if i := slices.IndexFunc(sc.Tools, func(ts scorecard.ToolStat) bool {
		return ts.Name == "Read"
	}); i < 0 || sc.Tools[i].AgentCalls != 1 {
		t.Errorf("tools = %+v, want the agent's Read", sc.Tools)
	}
}

// TestSessionScorecardWithoutEvents: an events service that does not
// answer leaves the hints and the ledger null and says so; the rest of the
// scorecard still comes back.
func TestSessionScorecardWithoutEvents(t *testing.T) {
	h := newHarness(t)
	var kofRequests int
	h.query.SetPeers(scorecard.NewHTTPPeers(fakeEvents(t, nil, true), fakeKof(t, &kofRequests)))

	var raw map[string]json.RawMessage
	h.getJSON(t, "/api/sessions/"+servertest.OldSession+"/scorecard", http.StatusOK, &raw)
	if string(raw["hints"]) != "null" {
		t.Errorf("hints = %s, want null", raw["hints"])
	}
	var sc scorecard.Scorecard
	h.getJSON(t, "/api/sessions/"+servertest.OldSession+"/scorecard", http.StatusOK, &sc)
	if sc.Sources.Events.Status != scorecard.StatusUnavailable || sc.Sources.Events.Detail == "" {
		t.Errorf("events source = %+v, want unavailable with a detail", sc.Sources.Events)
	}
	if sc.Sources.Kof.Status != scorecard.StatusSkipped ||
		!strings.Contains(sc.Sources.Kof.Detail, "events service did not answer") ||
		sc.Agents.Ledger != nil || kofRequests != 0 {
		t.Errorf("kof = %+v after %d requests, ledger = %+v; want skipped, saying why, "+
			"without asking, and null", sc.Sources.Kof, kofRequests, sc.Agents.Ledger)
	}
	if sc.Totals.Turns != 1 {
		t.Errorf("totals = %+v, want the trace's turn", sc.Totals)
	}
}

func TestSessionScorecardUnknownSession(t *testing.T) {
	h := newHarness(t)
	h.get(t, "/api/sessions/sess-none/scorecard", http.StatusNotFound)
}
