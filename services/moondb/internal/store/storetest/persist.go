package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// recordsOutliveClose: a store reopened on the same dir answers every read
// as it did before Close, and writing carries on where it stopped: Seq
// continues in the session and a stored id is still a no-op.
func recordsOutliveClose(t *testing.T, open func(t *testing.T, dir string) store.Store) {
	ctx := context.Background()
	dir := t.TempDir()
	first := open(t, dir)
	closed := false
	t.Cleanup(func() {
		if !closed {
			closeStore(t, first)
		}
	})
	seed(t, first, history()...)
	sha, err := first.Chunks().Put(ctx, []byte("kept"))
	must(t, err)
	before := view(t, first)
	closed = true
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := open(t, dir)
	t.Cleanup(func() { closeStore(t, second) })
	if after := view(t, second); after != before {
		t.Fatalf("reopened store differs:\nbefore %s\nafter  %s", before, after)
	}
	if got, err := second.Chunks().Get(ctx, sha); err != nil || string(got) != "kept" {
		t.Errorf("Get(%s) after reopen = %q, %v; want %q", sha, got, err, "kept")
	}
	sessionOutlivesClose(t, second)

	seed(t, second, turn("t9", "s1", store.TurnOpen, at(80)),
		store.Edit{ID: "e1", SessionID: "s1", TurnID: "t9", At: at(81), Path: "replayed.go"})
	d, err := second.Session(ctx, "s1")
	must(t, err)
	if n := len(d.Turns); n == 0 || d.Turns[n-1].ID != "t9" || d.Turns[n-1].Seq != 3 {
		t.Errorf("s1 turns after reopen = %+v, want t9 last with Seq 3", d.Turns)
	}
	edits, err := second.Edits(ctx, store.EditQuery{TurnID: "t9"})
	must(t, err)
	wantIDs(t, "edits under t9 after replaying e1", ids(edits, editID))
}

// sessionOutlivesClose checks that s1's last put session fields, its first
// turn's totals and its skipped counts all read back from a reopened store.
func sessionOutlivesClose(t *testing.T, s store.Store) {
	t.Helper()
	d, err := s.Session(context.Background(), "s1")
	must(t, err)
	if d.Title != "renamed" || d.TitleSource != store.TitleNamed ||
		d.PromptLine != "fix the bug" || d.EndedAt == nil {
		t.Errorf("s1 after reopen = title %q from %q, prompt line %q, ended %v; "+
			"want the last put", d.Title, d.TitleSource, d.PromptLine, d.EndedAt)
	}
	if len(d.Turns) == 0 || d.Turns[0].Totals == nil || len(d.Turns[0].Totals.Usage) != 2 {
		t.Errorf("t1 after reopen = %+v, want its totals with two usage entries", d.Turns)
	}
	if len(d.Turns) == 0 || !sameSkipped(d.Turns[0].Skipped, historySkipped()) {
		t.Errorf("t1 after reopen = %+v, want its skipped counts %+v", d.Turns, historySkipped())
	}
}

// history is a small but complete record set: two sessions, open and
// closed turns, edits in two worktrees, and one of every append-only fact.
func history() []any {
	a, b := loc(repoA, "/wt/a"), loc(repoB, "/wt/b")
	matches := true
	ended := at(90)
	renamed := session("s1", at(0))
	renamed.Title, renamed.TitleSource = "renamed", store.TitleNamed
	renamed.PromptLine, renamed.EndedAt = "fix the bug", &ended
	closed := turn("t1", "s1", store.TurnClosed, at(1))
	closed.EndedAt = &ended
	closed.Totals = totals(1, usage("model-a", 10, 20), usage("model-b", 1, 2))
	closed.Skipped = historySkipped()
	return []any{
		session("s1", at(0)), session("s2", at(30)),
		closed,
		turn("t2", "s1", store.TurnOpen, at(10)),
		turn("u1", "s2", store.TurnOpen, at(31)),
		store.Edit{
			ID: "e1", ToolUseID: "tu1", SessionID: "s1", TurnID: "t1", Tool: "Edit", At: at(2),
			Location: a, Path: "x.go", Kind: store.EditUpdate, AfterChunk: "c1", AfterBlob: "b1",
			BeforeSource: store.BeforeOriginal, ObservedMatches: &matches, EOL: "lf",
		},
		store.Edit{ID: "e2", SessionID: "s1", TurnID: "t2", At: at(11), Location: a, Path: "y.go"},
		store.Edit{ID: "e3", SessionID: "s2", TurnID: "u1", At: at(32), Location: b, Path: "x.go"},
		store.Edit{ID: "e4", SessionID: "s2", TurnID: "u1", At: at(33), Location: a, Path: "x.go"},
		store.UnattributedDelta{
			ID: "d1", SessionID: "s1", TurnID: "t2", At: at(12), Location: a, Path: "x.go",
			Cause: store.DeltaSnapshot, Ambiguous: true,
		},
		store.Snapshot{
			ID: "p1", SessionID: "s1", TurnID: "t2", At: at(10), Phase: store.SnapshotPre,
			Location: a, MapChunk: "m1", FileCount: 2,
		},
		store.CommitLink{
			ID: "l1", Repo: repoA, Worktree: "/wt/a", Commit: "c0ffee", Path: "x.go", Blob: "b1",
			EditID: "e1", TurnID: "t1", SessionID: "s1", At: at(20),
		},
		renamed,
	}
}

// historySkipped is the skipped counts of history's closed turn: two Bash
// calls, every reason set on one of them.
func historySkipped() []store.SkippedFiles {
	return []store.SkippedFiles{
		{Tool: "Bash", ToolUseID: "tu7", At: at(3), Clean: 40, Timeout: 2, Failed: 1},
		{Tool: "Bash", ToolUseID: "tu8", At: at(4), Clean: 3},
	}
}

// sameSkipped reports whether two skipped lists hold the same counts in the
// same order, times compared as instants.
func sameSkipped(got, want []store.SkippedFiles) bool {
	return slices.EqualFunc(got, want, func(g, w store.SkippedFiles) bool {
		gAt, wAt := g.At, w.At
		g.At, w.At = time.Time{}, time.Time{}
		return g == w && gAt.Equal(wAt)
	})
}

// view renders everything the store answers about its records as one JSON
// string, so two stores can be compared whole.
func view(t *testing.T, st store.Store) string {
	t.Helper()
	ctx := context.Background()
	v := map[string]any{}
	sessions, err := st.Sessions(ctx, store.SessionFilter{})
	must(t, err)
	v["sessions"] = sessions
	for _, s := range sessions {
		d, err := st.Session(ctx, s.ID)
		must(t, err)
		v["session "+s.ID] = d
		for _, tn := range d.Turns {
			td, err := st.Turn(ctx, tn.ID)
			must(t, err)
			v["turn "+tn.ID] = td
		}
		for _, wt := range d.Worktrees {
			open, err := st.OpenTurns(ctx, wt)
			must(t, err)
			v["open "+wt] = open
		}
	}
	edits, err := st.Edits(ctx, store.EditQuery{})
	must(t, err)
	v["edits"] = edits
	for _, e := range edits {
		q := store.ChainQuery{Worktree: e.Worktree, Path: e.Path, At: e.At, ExcludeID: e.ID}
		latest, err := st.LatestAfter(ctx, q)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("LatestAfter(%s, %s): %v", e.Worktree, e.Path, err)
		}
		v["chain of "+e.ID] = latest.ID
	}
	raw, err := json.Marshal(v)
	must(t, err)
	return string(raw)
}
