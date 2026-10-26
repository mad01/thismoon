package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// The in-flight fixture: one checkout where "other" made an edit no commit
// took (a.go) and one a commit took (c.go), the asking session "me" made
// one (b.go), and "old" made one four days back (d.go). Another checkout
// of the same repository holds an edit of a.go too.
const (
	inflightWT    = "/work/repo"
	inflightOther = "other"
	inflightTitle = "fix the parser"
)

// inflightHarness serves a local store seeded with the fixture and any
// extra seeds.
func inflightHarness(t *testing.T, extra ...func(*testing.T, store.Store)) harness {
	t.Helper()
	st, err := local.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	seedInflight(t, st)
	for _, seed := range extra {
		seed(t, st)
	}
	s := New(st, nil, testInfo)
	s.now = func() time.Time { return fixedNow }
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return harness{ts: ts}
}

func seedInflight(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	loc := store.Location{Repo: "example.com/o/repo", Worktree: inflightWT, Canonical: inflightWT}
	elsewhere := loc
	elsewhere.Worktree = "/work/repo-wt"
	edit := func(id, session, path string, at time.Time, l store.Location) store.Edit {
		return store.Edit{
			ID: id, SessionID: session, TurnID: "turn-" + session, Tool: "Edit", At: at,
			Location: l, Path: path, Kind: store.EditUpdate, AfterBlob: "blob-" + id,
		}
	}
	ago := func(d time.Duration) time.Time { return fixedNow.Add(-d) }
	records := []error{
		st.PutSession(
			ctx,
			store.Session{ID: inflightOther, Title: inflightTitle, StartedAt: ago(time.Hour)},
		),
		st.PutSession(ctx, store.Session{ID: "me", StartedAt: ago(time.Hour)}),
		st.PutTurn(
			ctx,
			store.Turn{ID: "turn-other", SessionID: inflightOther, Status: store.TurnClosed},
		),
		st.PutTurn(ctx, store.Turn{ID: "turn-me", SessionID: "me", Status: store.TurnOpen}),
		st.AppendEdit(ctx, edit("e-a", inflightOther, "a.go", ago(50*time.Minute), loc)),
		st.AppendEdit(ctx, edit("e-b", "me", "b.go", ago(40*time.Minute), loc)),
		st.AppendEdit(ctx, edit("e-c", inflightOther, "c.go", ago(30*time.Minute), loc)),
		st.AppendEdit(ctx, edit("e-d", "old", "d.go", ago(96*time.Hour), loc)),
		st.AppendEdit(ctx, edit("e-x", inflightOther, "a.go", ago(10*time.Minute), elsewhere)),
		st.AppendCommitLink(ctx, store.CommitLink{
			ID: "c1:c.go", Repo: loc.Repo, Worktree: inflightWT, Commit: "c1", Path: "c.go",
			Blob: "blob-e-c", EditID: "e-c", TurnID: "turn-other", SessionID: inflightOther,
			At: ago(20 * time.Minute),
		}),
	}
	for _, err := range records {
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// seedUnlinkable adds edits in the fixture checkout that no link the
// endpoint reads covers: a delete a commit took (no blob to link), an edit
// whose link comes from a commit in another worktree, and an edit whose
// link sits under a turn with no record.
func seedUnlinkable(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	loc := store.Location{Repo: "example.com/o/repo", Worktree: inflightWT, Canonical: inflightWT}
	ago := func(d time.Duration) time.Time { return fixedNow.Add(-d) }
	gone := store.Edit{
		ID: "e-gone", SessionID: inflightOther, TurnID: "turn-other", Tool: "Bash",
		At: ago(45 * time.Minute), Location: loc, Path: "gone.go", Kind: store.EditDelete,
	}
	elsewhere := store.Edit{
		ID: "e-w2", SessionID: inflightOther, TurnID: "turn-other", Tool: "Edit",
		At: ago(35 * time.Minute), Location: loc, Path: "w2.go", Kind: store.EditUpdate,
		AfterBlob: "blob-shared",
	}
	orphan := store.Edit{
		ID: "e-orphan", SessionID: "ghost", TurnID: "turn-ghost", Tool: "Edit",
		At: ago(25 * time.Minute), Location: loc, Path: "orphan.go", Kind: store.EditUpdate,
		AfterBlob: "blob-orphan",
	}
	link := func(id, worktree string, e store.Edit) store.CommitLink {
		return store.CommitLink{
			ID: id, Repo: loc.Repo, Worktree: worktree, Commit: "c-" + id, Path: e.Path,
			Blob: e.AfterBlob, EditID: e.ID, TurnID: e.TurnID, SessionID: e.SessionID,
			At: ago(20 * time.Minute),
		}
	}
	for _, err := range []error{
		st.AppendEdit(ctx, gone),
		st.AppendEdit(ctx, elsewhere),
		st.AppendEdit(ctx, orphan),
		st.AppendCommitLink(ctx, link("w2", "/work/repo-wt", elsewhere)),
		st.AppendCommitLink(ctx, link("orphan", inflightWT, orphan)),
	} {
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// TestInflightListsWhatNoLinkCovers pins what the endpoint cannot see past:
// a delete, a link from a commit in another worktree, and a link filed
// under a turn with no record all leave the file listed. belt's git status
// check is what drops such a file once git reports it clean.
func TestInflightListsWhatNoLinkCovers(t *testing.T) {
	h := inflightHarness(t, seedUnlinkable)
	got, paths := inflightPaths(t, h, url.Values{"exclude_session": {"me"}})
	want := []string{"orphan.go", "w2.go", "gone.go", "a.go"}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for _, f := range got.Files {
		if f.LinkedCommit != nil {
			t.Errorf("%s linked_commit = %s, want null", f.Path, *f.LinkedCommit)
		}
	}
	if s := got.Files[0].Sessions; len(s) != 1 || s[0].Title != "ghost" {
		t.Errorf("orphan sessions = %+v, want the session named by its id", s)
	}
}

// inflightPaths asks for the in-flight files of the fixture checkout with
// extra query parameters and returns the answer and its paths.
func inflightPaths(t *testing.T, h harness, extra url.Values) (InflightView, []string) {
	t.Helper()
	q := url.Values{"worktree": {inflightWT}}
	for k, v := range extra {
		q[k] = v
	}
	var got InflightView
	h.getJSON(t, "/api/inflight?"+q.Encode(), http.StatusOK, &got)
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	return got, paths
}

func TestInflightKeepsOtherSessionsUncommittedEdits(t *testing.T) {
	h := inflightHarness(t)
	tests := []struct {
		name  string
		query url.Values
		want  []string
	}{
		{
			name:  "another session's edit shows; mine, a linked one and an old one do not",
			query: url.Values{"exclude_session": {"me"}},
			want:  []string{"a.go"},
		},
		{
			name:  "without an asking session every session counts",
			query: url.Values{},
			want:  []string{"b.go", "a.go"},
		},
		{
			name:  "a wider since window reaches the older edit",
			query: url.Values{"exclude_session": {"me"}, "since": {"7d"}},
			want:  []string{"a.go", "d.go"},
		},
		{
			name:  "limit keeps the newest files",
			query: url.Values{"exclude_session": {"me"}, "since": {"7d"}, "limit": {"1"}},
			want:  []string{"a.go"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, paths := inflightPaths(t, h, tc.query)
			if !slices.Equal(paths, tc.want) {
				t.Errorf("paths = %v, want %v", paths, tc.want)
			}
		})
	}
}

func TestInflightNamesTheSessionAndItsEdits(t *testing.T) {
	h := inflightHarness(t)
	got, _ := inflightPaths(t, h, url.Values{"exclude_session": {"me"}})
	if got.Worktree != inflightWT || len(got.Files) != 1 {
		t.Fatalf("answer = %+v, want one file in %s", got, inflightWT)
	}
	f := got.Files[0]
	want := store.InFlightSession{
		ID: inflightOther, Title: inflightTitle, LastEditAt: fixedNow.Add(-50 * time.Minute), Edits: 1,
	}
	if len(f.Sessions) != 1 || f.Sessions[0] != want {
		t.Errorf("sessions = %+v, want [%+v]", f.Sessions, want)
	}
	if !f.LastEditAt.Equal(want.LastEditAt) || f.LinkedCommit != nil {
		t.Errorf("file = %+v, want last edit %s and no linked commit", f, want.LastEditAt)
	}
}

func TestInflightAnswersAnEmptyListAsJSON(t *testing.T) {
	h := inflightHarness(t)
	_, body := h.get(t, "/api/inflight?worktree=/nowhere", http.StatusOK)
	if want := `{"worktree":"/nowhere","files":[]}` + "\n"; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func TestInflightRejectsABadQuery(t *testing.T) {
	h := inflightHarness(t)
	for path, want := range map[string]string{
		"/api/inflight":                               "worktree is required",
		"/api/inflight?worktree=work/repo":            "absolute checkout root",
		"/api/inflight?worktree=/work/repo&since=-1h": "since must be",
		"/api/inflight?worktree=/work/repo&limit=0":   "limit must be",
	} {
		h.wantAPIError(t, path, http.StatusBadRequest, want)
	}
}
