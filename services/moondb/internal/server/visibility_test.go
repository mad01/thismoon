package server

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const (
	hiddenSession = "sess-hidden"
	hiddenCwd     = "/work/hidden"
)

// hiddenHarness is the fixture plus the most recent session, hidden, in a
// directory outside any repository.
func hiddenHarness(t *testing.T) harness {
	t.Helper()
	h := newHarness(t)
	h.store.Add(store.Session{
		ID: hiddenSession, Harness: "test-harness", Cwd: hiddenCwd, Title: "scratch",
		StartedAt: servertest.T0.Add(90 * time.Minute), Hidden: true,
	}, nil, nil, nil)
	return h
}

func listIDs(l SessionList) []string {
	var ids []string
	for _, s := range l.Sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

// TestSessionsLeaveHiddenOut: the listing, its window totals and its
// directory chips leave a hidden session out unless include_hidden asks for
// it, and then the row says it is hidden.
func TestSessionsLeaveHiddenOut(t *testing.T) {
	h := hiddenHarness(t)

	got := h.sessionList(t, "")
	if ids := listIDs(got); slices.Contains(ids, hiddenSession) || len(ids) != 2 {
		t.Errorf("default listing = %v, want the two visible sessions", ids)
	}
	if got.Window.Sessions != 2 {
		t.Errorf("window counts %d sessions, want 2", got.Window.Sessions)
	}
	if slices.ContainsFunc(got.Dirs, func(d DirCount) bool { return d.Dir == hiddenCwd }) {
		t.Errorf("dirs = %+v, want none for the hidden session's cwd", got.Dirs)
	}

	all := h.sessionList(t, "?include_hidden=true")
	if ids := listIDs(all); len(ids) != 3 || ids[0] != hiddenSession {
		t.Fatalf("include_hidden listing = %v, want the hidden session first of three", ids)
	}
	if !all.Sessions[0].Hidden || all.Sessions[1].Hidden {
		t.Errorf("hidden = %v, %v; want only the hidden session marked",
			all.Sessions[0].Hidden, all.Sessions[1].Hidden)
	}
}

// TestHiddenSessionStillLoadsByID: hiding drops a session from listings
// only; its own page still answers and says it is hidden.
func TestHiddenSessionStillLoadsByID(t *testing.T) {
	h := hiddenHarness(t)
	var got SessionView
	h.getJSON(t, "/api/sessions/"+hiddenSession, http.StatusOK, &got)
	if !got.Summary.Hidden || !got.Hidden {
		t.Errorf("session view hidden = %v, row hidden = %v; want both true",
			got.Hidden, got.Summary.Hidden)
	}
}

func TestSessionsRejectsBadIncludeHidden(t *testing.T) {
	h := newHarness(t)
	h.get(t, "/api/sessions?include_hidden=maybe", http.StatusBadRequest)
}
