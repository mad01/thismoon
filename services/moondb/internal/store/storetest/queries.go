package storetest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const (
	repoA = "github.com/o/a"
	repoB = "github.com/o/b"
)

// sessionsFilter: Sessions lists newest first by start, and each filter
// field narrows on its own and together with the others.
func sessionsFilter(t *testing.T, st store.Store) {
	seed(t, st,
		session("s1", at(0)), session("s2", at(60)), session("s3", at(120)),
		edit("e1", "s1", "t1", 1, loc(repoA, "/wt/a"), "x.go"),
		edit("e2", "s2", "t2", 61, loc(repoB, "/wt/b"), "x.go"),
	)
	cases := []struct {
		name string
		f    store.SessionFilter
		want string // ids in order, space-separated
	}{
		{"all", store.SessionFilter{}, "s3 s2 s1"},
		{"repo", store.SessionFilter{Repo: repoA}, "s1"},
		{"since", store.SessionFilter{Since: at(60)}, "s3 s2"},
		{"limit", store.SessionFilter{Limit: 2}, "s3 s2"},
		{"repo and since", store.SessionFilter{Repo: repoA, Since: at(60)}, ""},
		{"unknown repo", store.SessionFilter{Repo: "github.com/o/x"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.Sessions(context.Background(), c.f)
			must(t, err)
			wantIDs(t, "Sessions", ids(got, sessionID), strings.Fields(c.want)...)
		})
	}
}

// sessionDetail: turns come in Seq order whatever order they arrived in,
// Worktrees lists each edited worktree once, and EditCount counts the
// session's edits only.
func sessionDetail(t *testing.T, st store.Store) {
	ctx := context.Background()
	second := turn("tb", "s1", store.TurnClosed, at(2))
	second.Seq = 2
	first := turn("ta", "s1", store.TurnClosed, at(1))
	first.Seq = 1
	seed(t, st,
		session("s1", at(0)), session("s2", at(0)),
		second, first, turn("tc", "s1", store.TurnOpen, at(3)),
		edit("e1", "s1", "ta", 1, loc(repoA, "/wt/a"), "x.go"),
		edit("e2", "s1", "tb", 2, loc(repoA, "/wt/a"), "y.go"),
		edit("e3", "s1", "tc", 3, loc(repoA, "/wt/a2"), "x.go"),
		edit("e4", "s2", "u1", 4, loc(repoB, "/wt/b"), "x.go"),
	)
	d, err := st.Session(ctx, "s1")
	must(t, err)
	wantIDs(t, "turns", ids(d.Turns, turnID), "ta", "tb", "tc")
	if len(d.Turns) == 3 && d.Turns[2].Seq != 3 {
		t.Errorf("tc Seq = %d, want 3 (next after the highest)", d.Turns[2].Seq)
	}
	wantIDs(t, "worktrees", slices.Sorted(slices.Values(d.Worktrees)), "/wt/a", "/wt/a2")
	if d.EditCount != 3 {
		t.Errorf("EditCount = %d, want 3", d.EditCount)
	}
	if _, err := st.Session(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Session(nope): err = %v, want ErrNotFound", err)
	}
}

// turnDetail: everything recorded under a turn comes back newest first by
// its own time, even when it arrived out of order, and nothing from another
// turn leaks in.
func turnDetail(t *testing.T, st store.Store) {
	ctx := context.Background()
	wt := loc(repoA, "/wt/a")
	seed(t, st,
		session("s1", at(0)),
		turn("t1", "s1", store.TurnOpen, at(0)), turn("t2", "s1", store.TurnOpen, at(20)),
		edit("e1", "s1", "t1", 1, wt, "x.go"),
		edit("e2", "s1", "t1", 3, wt, "x.go"),
		edit("e0", "s1", "t1", 2, wt, "y.go"),
		edit("e9", "s1", "t2", 21, wt, "x.go"),
		store.UnattributedDelta{ID: "d2", SessionID: "s1", TurnID: "t1", At: at(4)},
		store.UnattributedDelta{ID: "d1", SessionID: "s1", TurnID: "t1", At: at(2)},
		store.UnattributedDelta{ID: "d9", SessionID: "s1", TurnID: "t2", At: at(22)},
		store.Snapshot{ID: "p1", SessionID: "s1", TurnID: "t1", At: at(0)},
		store.Snapshot{ID: "p2", SessionID: "s1", TurnID: "t1", At: at(5)},
		store.CommitLink{ID: "l1", SessionID: "s1", TurnID: "t1", At: at(10)},
		store.CommitLink{ID: "l2", SessionID: "s1", TurnID: "t1", At: at(11)},
	)
	d, err := st.Turn(ctx, "t1")
	must(t, err)
	if d.ID != "t1" || d.SessionID != "s1" || d.Seq != 1 {
		t.Errorf("Turn(t1) = id %q session %q seq %d", d.ID, d.SessionID, d.Seq)
	}
	wantIDs(t, "edits", ids(d.Edits, editID), "e2", "e0", "e1")
	wantIDs(t, "deltas", ids(d.Deltas, deltaID), "d2", "d1")
	wantIDs(t, "snapshots", ids(d.Snapshots, snapshotID), "p2", "p1")
	wantIDs(t, "commit links", ids(d.CommitLinks, linkID), "l2", "l1")
	if _, err := st.Turn(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Turn(nope): err = %v, want ErrNotFound", err)
	}
}

// editsQuery: every EditQuery field narrows on its own and combined, and
// results come newest first by At, not by arrival (e6 arrives last but is
// the oldest).
func editsQuery(t *testing.T, st store.Store) {
	a, a2, b := loc(repoA, "/wt/a"), loc(repoA, "/wt/a2"), loc(repoB, "/wt/b")
	seed(t, st,
		withBlob(edit("e1", "s1", "t1", 1, a, "x.go"), "b1"),
		withBlob(edit("e2", "s1", "t1", 2, a, "y.go"), "b2"),
		withBlob(edit("e3", "s1", "t2", 3, a2, "x.go"), "b3"),
		withBlob(edit("e4", "s2", "t3", 4, b, "x.go"), "b1"),
		edit("e5", "s2", "t3", 5, a, "x.go"),
		edit("e6", "s1", "t1", 0, a, "x.go"),
	)
	cases := []struct {
		name string
		q    store.EditQuery
		want string // ids in order, space-separated
	}{
		{"all", store.EditQuery{}, "e5 e4 e3 e2 e1 e6"},
		{"repo", store.EditQuery{Repo: repoA}, "e5 e3 e2 e1 e6"},
		{"path", store.EditQuery{Path: "x.go"}, "e5 e4 e3 e1 e6"},
		{"repo and path", store.EditQuery{Repo: repoA, Path: "x.go"}, "e5 e3 e1 e6"},
		{"worktree", store.EditQuery{Worktree: "/wt/a"}, "e5 e2 e1 e6"},
		{"worktree and path", store.EditQuery{Worktree: "/wt/a", Path: "x.go"}, "e5 e1 e6"},
		{"session", store.EditQuery{SessionID: "s2"}, "e5 e4"},
		{"turn", store.EditQuery{TurnID: "t1"}, "e2 e1 e6"},
		{"after blob", store.EditQuery{AfterBlob: "b1"}, "e4 e1"},
		{"since", store.EditQuery{Since: at(3)}, "e5 e4 e3"},
		{"limit", store.EditQuery{Limit: 2}, "e5 e4"},
		{"repo since limit", store.EditQuery{Repo: repoA, Since: at(2), Limit: 2}, "e5 e3"},
		{"turn and session disagree", store.EditQuery{TurnID: "t1", SessionID: "s2"}, ""},
		{"no match", store.EditQuery{Repo: "github.com/o/x"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.Edits(context.Background(), c.q)
			must(t, err)
			wantIDs(t, "Edits", ids(got, editID), strings.Fields(c.want)...)
		})
	}
}

// latestAfter: the newest edit by At of (worktree, path) that has an
// AfterChunk; edits without one, and other worktrees, do not count.
func latestAfter(t *testing.T, st store.Store) {
	a, b := loc(repoA, "/wt/a"), loc(repoA, "/wt/b")
	seed(t, st,
		withAfter(edit("e1", "s1", "t1", 1, a, "x.go"), "c1"),
		withAfter(edit("e2", "s1", "t1", 2, a, "x.go"), "c2"),
		edit("e3", "s1", "t1", 3, a, "x.go"),
		withAfter(edit("e4", "s1", "t1", 4, b, "x.go"), "c4"),
		edit("e5", "s1", "t1", 5, a, "y.go"),
		withAfter(edit("e6", "s1", "t1", 0, a, "x.go"), "c6"),
	)
	cases := []struct {
		worktree, path string
		want           string // "" means ErrNotFound
	}{
		{"/wt/a", "x.go", "e2"},
		{"/wt/b", "x.go", "e4"},
		{"/wt/a", "y.go", ""},
		{"/wt/a", "z.go", ""},
		{"/wt/c", "x.go", ""},
	}
	for _, c := range cases {
		got, err := st.LatestAfter(context.Background(), c.worktree, c.path)
		switch {
		case c.want == "" && !errors.Is(err, store.ErrNotFound):
			t.Errorf("LatestAfter(%s, %s) = %q, %v; want ErrNotFound",
				c.worktree, c.path, got.ID, err)
		case c.want != "" && (err != nil || got.ID != c.want):
			t.Errorf("LatestAfter(%s, %s) = %q, %v; want %q",
				c.worktree, c.path, got.ID, err, c.want)
		}
	}
}

// openTurns: the open turns with an edit in the worktree, across sessions,
// newest first. Closing or failing a turn drops it, and reopening brings it
// back; a turn whose edit arrived before the turn itself still counts.
func openTurns(t *testing.T, st store.Store) {
	x, y := loc(repoA, "/wt/x"), loc(repoA, "/wt/y")
	seed(t, st,
		edit("ef", "sD", "f1", 6, x, "p"),
		turn("a1", "sA", store.TurnOpen, at(1)),
		turn("b1", "sB", store.TurnOpen, at(2)),
		turn("b2", "sB", store.TurnClosed, at(3)),
		turn("c1", "sC", store.TurnOpen, at(4)),
		turn("d1", "sD", store.TurnOpen, at(5)),
		turn("f1", "sD", store.TurnOpen, at(6)),
		edit("ea", "sA", "a1", 1, x, "p"),
		edit("eb1", "sB", "b1", 2, x, "p"),
		edit("eb2", "sB", "b2", 3, x, "p"),
		edit("ec", "sC", "c1", 4, y, "p"),
	)
	check := func(step, worktree string, want ...string) {
		t.Helper()
		got, err := st.OpenTurns(context.Background(), worktree)
		must(t, err)
		wantIDs(t, step+": OpenTurns("+worktree+")", ids(got, turnID), want...)
	}
	check("seeded", "/wt/x", "f1", "b1", "a1")
	check("seeded", "/wt/y", "c1")
	check("seeded", "/wt/none")

	seed(t, st,
		turn("b1", "sB", store.TurnClosed, at(2)),
		turn("a1", "sA", store.TurnFailed, at(1)),
	)
	check("after closing b1 and failing a1", "/wt/x", "f1")
	seed(t, st, turn("a1", "sA", store.TurnOpen, at(1)))
	check("after reopening a1", "/wt/x", "f1", "a1")
}
