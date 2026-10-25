// Package storetest holds the store.Store contract as a test suite every
// backend runs against itself. It is a plain package so each backend's
// tests can import it. The suite asserts behavior through the interface
// only, never a backend's layout.
package storetest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Run exercises a backend against the Store contract. open returns the
// backend's store for dir and fails t when it cannot. The suite hands it a
// fresh t.TempDir for every case, and the same dir twice to check that
// records outlive Close; a backend that does not live on disk treats dir as
// a namespace, where two opens of one dir reach the same records. The suite
// closes every store it opens.
func Run(t *testing.T, open func(t *testing.T, dir string) store.Store) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, st store.Store)
	}{
		{"PutSessionUpserts", putSessionUpserts},
		{"PutTurnAssignsSeq", putTurnAssignsSeq},
		{"AppendsAreIdempotent", appendsAreIdempotent},
		{"SessionsFilter", sessionsFilter},
		{"SessionDetail", sessionDetail},
		{"TurnDetail", turnDetail},
		{"EditsQuery", editsQuery},
		{"LatestAfter", latestAfter},
		{"OpenTurns", openTurns},
		{"ChunksRoundTrip", chunksRoundTrip},
		{"Ping", ping},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := open(t, t.TempDir())
			t.Cleanup(func() { closeStore(t, st) })
			c.run(t, st)
		})
	}
	t.Run("RecordsOutliveClose", func(t *testing.T) { recordsOutliveClose(t, open) })
}

func closeStore(t *testing.T, st store.Store) {
	t.Helper()
	if err := st.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// t0 anchors every fixture time; at gives a time minutes after it.
var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

func session(id string, started time.Time) store.Session {
	return store.Session{ID: id, Harness: "claude-code", Cwd: "/w/" + id, StartedAt: started}
}

func turn(id, sessionID string, status store.TurnStatus, started time.Time) store.Turn {
	return store.Turn{ID: id, SessionID: sessionID, Status: status, StartedAt: started}
}

func loc(repo, worktree string) store.Location {
	return store.Location{Repo: repo, Worktree: worktree, Canonical: worktree, Branch: "main"}
}

// edit is a fixture edit: one row per edit keeps a fixture readable beside
// the expectations it drives.
func edit(id, sessionID, turnID string, minute int, l store.Location, path string) store.Edit {
	return store.Edit{
		ID: id, SessionID: sessionID, TurnID: turnID, At: at(minute), Location: l, Path: path,
	}
}

// withBlob sets the edit's after-blob; withAfter its after-chunk.
func withBlob(e store.Edit, blob string) store.Edit   { e.AfterBlob = blob; return e }
func withAfter(e store.Edit, chunk string) store.Edit { e.AfterChunk = chunk; return e }

// seed writes each record through the method its type calls for.
func seed(t *testing.T, st store.Store, recs ...any) {
	t.Helper()
	ctx := context.Background()
	for _, r := range recs {
		var err error
		switch v := r.(type) {
		case store.Session:
			err = st.PutSession(ctx, v)
		case store.Turn:
			err = st.PutTurn(ctx, v)
		case store.Edit:
			err = st.AppendEdit(ctx, v)
		case store.UnattributedDelta:
			err = st.AppendDelta(ctx, v)
		case store.Snapshot:
			err = st.AppendSnapshot(ctx, v)
		case store.CommitLink:
			err = st.AppendCommitLink(ctx, v)
		default:
			t.Fatalf("seed: unsupported record %T", r)
		}
		if err != nil {
			t.Fatalf("seed %T: %v", r, err)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ids[T any](xs []T, id func(T) string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, id(x))
	}
	return out
}

func sessionID(s store.Session) string         { return s.ID }
func turnID(t store.Turn) string               { return t.ID }
func editID(e store.Edit) string               { return e.ID }
func deltaID(d store.UnattributedDelta) string { return d.ID }
func snapshotID(s store.Snapshot) string       { return s.ID }
func linkID(l store.CommitLink) string         { return l.ID }

// wantIDs fails t unless got lists exactly want, in order.
func wantIDs(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}
