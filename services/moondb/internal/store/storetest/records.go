package storetest

import (
	"context"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// putSessionUpserts: a second Put replaces the session whole, and an
// EndedAt set by a later Put persists.
func putSessionUpserts(t *testing.T, st store.Store) {
	ctx := context.Background()
	s := session("s1", at(0))
	s.Title = "first"
	seed(t, st, s)
	s.Title = "second"
	seed(t, st, s)
	ended := at(30)
	s.EndedAt = &ended
	seed(t, st, s)

	list, err := st.Sessions(ctx, store.SessionFilter{})
	must(t, err)
	if len(list) != 1 || list[0].Title != "second" {
		t.Fatalf("Sessions after three puts of one id = %+v, want one titled %q", list, "second")
	}
	d, err := st.Session(ctx, "s1")
	must(t, err)
	if d.Title != "second" || d.EndedAt == nil || !d.EndedAt.Equal(ended) {
		t.Errorf(
			"Session(s1) = title %q, ended %v; want %q, %v",
			d.Title,
			d.EndedAt,
			"second",
			ended,
		)
	}
}

// putTurnAssignsSeq: Seq 0 takes the next Seq in the session, counted per
// session, and a later Put of a known turn keeps its Seq whatever it says.
func putTurnAssignsSeq(t *testing.T, st store.Store) {
	ctx := context.Background()
	seed(t, st,
		session("s1", at(0)), session("s2", at(0)),
		turn("t1", "s1", store.TurnOpen, at(1)),
		turn("t2", "s1", store.TurnOpen, at(2)),
		turn("u1", "s2", store.TurnOpen, at(3)),
	)
	closed := turn("t1", "s1", store.TurnClosed, at(1))
	closed.Seq = 7
	seed(t, st, closed)

	d, err := st.Session(ctx, "s1")
	must(t, err)
	wantIDs(t, "s1 turns", ids(d.Turns, turnID), "t1", "t2")
	if len(d.Turns) == 2 && (d.Turns[0].Seq != 1 || d.Turns[1].Seq != 2) {
		t.Errorf("s1 seqs = %d, %d; want 1, 2", d.Turns[0].Seq, d.Turns[1].Seq)
	}
	if len(d.Turns) > 0 && d.Turns[0].Status != store.TurnClosed {
		t.Errorf(
			"t1 status = %q after the closing put, want %q",
			d.Turns[0].Status,
			store.TurnClosed,
		)
	}
	d2, err := st.Session(ctx, "s2")
	must(t, err)
	if len(d2.Turns) != 1 || d2.Turns[0].Seq != 1 {
		t.Errorf("s2 turns = %+v, want u1 with Seq 1", d2.Turns)
	}
}

// appendsAreIdempotent: appending a record whose id is already stored
// changes nothing, even when the replay carries different fields.
func appendsAreIdempotent(t *testing.T, st store.Store) {
	ctx := context.Background()
	e := edit("e1", "s1", "t1", 1, loc(repoA, "/wt/a"), "a.go")
	delta := store.UnattributedDelta{ID: "d1", SessionID: "s1", TurnID: "t1", Path: "a.go"}
	snap := store.Snapshot{ID: "p1", SessionID: "s1", TurnID: "t1", Phase: store.SnapshotPre}
	link := store.CommitLink{ID: "l1", TurnID: "t1", SessionID: "s1", Commit: "c1", At: at(5)}
	seed(t, st, session("s1", at(0)), turn("t1", "s1", store.TurnOpen, at(0)),
		e, e, delta, delta, snap, snap, link, link)

	e.Path, delta.Path, snap.Phase, link.Commit = "b.go", "b.go", store.SnapshotPost, "c2"
	seed(t, st, e, delta, snap, link)

	d, err := st.Turn(ctx, "t1")
	must(t, err)
	wantIDs(t, "edits", ids(d.Edits, editID), "e1")
	wantIDs(t, "deltas", ids(d.Deltas, deltaID), "d1")
	wantIDs(t, "snapshots", ids(d.Snapshots, snapshotID), "p1")
	wantIDs(t, "commit links", ids(d.CommitLinks, linkID), "l1")
	if len(d.Edits) == 1 && d.Edits[0].Path != "a.go" {
		t.Errorf("edit path = %q after a replay, want the first %q", d.Edits[0].Path, "a.go")
	}
	if len(d.Deltas) == 1 && d.Deltas[0].Path != "a.go" {
		t.Errorf("delta path = %q after a replay, want the first %q", d.Deltas[0].Path, "a.go")
	}
	if len(d.Snapshots) == 1 && d.Snapshots[0].Phase != store.SnapshotPre {
		t.Errorf(
			"snapshot phase = %q after a replay, want %q",
			d.Snapshots[0].Phase,
			store.SnapshotPre,
		)
	}
	if len(d.CommitLinks) == 1 && d.CommitLinks[0].Commit != "c1" {
		t.Errorf("link commit = %q after a replay, want %q", d.CommitLinks[0].Commit, "c1")
	}
	edits, err := st.Edits(ctx, store.EditQuery{})
	must(t, err)
	wantIDs(t, "all edits", ids(edits, editID), "e1")
}
