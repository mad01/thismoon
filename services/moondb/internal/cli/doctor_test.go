package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/doctor"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// TestBriefsCheck: a count warns with the import that fills it, no count
// passes, and a serve without the route or without an answer steps aside
// for the checks that own those failures.
func TestBriefsCheck(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantNote string // "" means a plain pass
	}{
		{"none missing", http.StatusOK, `{"agents_to_fill":0}`, ""},
		{
			"some missing", http.StatusOK, `{"agents_to_fill":3}`,
			"3 agents without a spawn brief or later messages; run moondb import --since 30d --force",
		},
		{
			"a serve with only the old key", http.StatusOK, `{"agents_without_brief":2}`,
			"2 agents without a spawn brief or later messages; run moondb import --since 30d --force",
		},
		{"older serve", http.StatusNotFound, `{"error":"moondb: no API route"}`, "version-skew"},
		{
			"serve fails",
			http.StatusInternalServerError,
			`{"error":"moondb: boom"}`,
			"service-reachable",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/doctor" {
					t.Errorf("path = %s, want /api/doctor", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer ts.Close()
			err := briefsCheck(client.New(ts.URL))(context.Background())
			report := doctor.Collect(
				context.Background(),
				[]doctor.Check{
					{Name: "agent-briefs", Run: func(context.Context) error { return err }},
				},
			)
			got := report.Checks[0]
			if !report.OK {
				t.Fatalf("check failed: %+v", got)
			}
			if c.wantNote == "" {
				if got.Status != doctor.StatusOK {
					t.Errorf("status = %s (%s), want ok", got.Status, got.Detail)
				}
				return
			}
			if got.Status != doctor.StatusSkipped || !strings.Contains(got.Detail, c.wantNote) {
				t.Errorf("got %s %q, want skipped with %q", got.Status, got.Detail, c.wantNote)
			}
		})
	}
}

// TestCutTurnsCheck runs the check against a serve over a fixture store
// whose turns hold a few bytes each, so a turn whose next turn starts
// 5 MiB further on was cut at the old 4 MiB cap. A cut turn counts as
// repairable while its session's transcript file exists, and the window
// reaches the oldest such file; a session's last turn counts when its
// file holds more than the cap past the turn's offset.
func TestCutTurnsCheck(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	// cut has one cut turn and a tail too short to judge; tail has a tail
	// its 5 MiB file shows was cut; gone and goneTail lost their files.
	cut := cutSession{
		id: "cut", offsets: []int64{0, 5 << 20},
		path: transcriptFile(t, dir, "cut", 5<<20+100, now.Add(-84*time.Hour)),
	}
	tail := cutSession{
		id: "tail", offsets: []int64{100},
		path: transcriptFile(t, dir, "tail", 5<<20, now.Add(-24*time.Hour)),
	}
	whole := cutSession{
		id: "whole", offsets: []int64{0, 3 << 20},
		path: transcriptFile(t, dir, "whole", 3<<20+100, now.Add(-200*time.Hour)),
	}
	gone := cutSession{id: "gone", offsets: []int64{0, 6 << 20}, path: filepath.Join(dir, "gone")}
	goneTail := cutSession{id: "gone-tail", offsets: []int64{0}, path: filepath.Join(dir, "x")}
	cases := []struct {
		name     string
		sessions []cutSession
		wantNote string // "" means a plain pass
	}{
		{"none cut", []cutSession{whole}, ""},
		{
			"repairable",
			[]cutSession{whole, cut, tail},
			"2 turns keep a transcript cut at the old 4 MiB cap; 2 with the transcript file on disk: " +
				"with no session active, run moondb import --since 5d --force",
		},
		{
			"unrepairable",
			[]cutSession{gone, goneTail},
			"1 turn keeps a transcript cut at the old 4 MiB cap, with no transcript file left to repair from",
		},
		{
			"both",
			[]cutSession{cut, gone},
			"2 turns keep a transcript cut at the old 4 MiB cap; 1 with the transcript file on disk: " +
				"with no session active, run moondb import --since 5d --force; 1 without one stays cut",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := cutTurnsCheck(cutServe(t, c.sessions), func() time.Time { return now })
			got := runCheck(t, "cut-turns", check)
			if c.wantNote == "" {
				if got.Status != doctor.StatusOK {
					t.Errorf("status = %s (%s), want ok", got.Status, got.Detail)
				}
				return
			}
			if got.Status != doctor.StatusSkipped || got.Detail != c.wantNote {
				t.Errorf("got %s %q,\nwant skipped %q", got.Status, got.Detail, c.wantNote)
			}
		})
	}
}

// TestCutTurnsCheckOlderServe: a serve without the check's field, or
// without the route, steps aside for version-skew.
func TestCutTurnsCheckOlderServe(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"agents_to_fill":0}`))
		}))
		got := runCheck(t, "cut-turns", cutTurnsCheck(client.New(ts.URL), time.Now))
		ts.Close()
		if got.Status != doctor.StatusSkipped || !strings.Contains(got.Detail, "version-skew") {
			t.Errorf(
				"status %d: got %s %q, want skipped naming version-skew",
				status,
				got.Status,
				got.Detail,
			)
		}
	}
}

// cutSession is a fixture session: its transcript file and the offsets of
// its turns, each closed with a few bytes of lines.
type cutSession struct {
	id      string
	path    string
	offsets []int64
}

// cutServe serves a local store in a temp dir holding sessions and
// returns a client of it.
func cutServe(t *testing.T, sessions []cutSession) *client.Client {
	t.Helper()
	ctx := context.Background()
	st, err := local.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	chunk, err := st.Chunks().Put(ctx, []byte(`{"type":"assistant"}`+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, s := range sessions {
		err := st.PutSession(ctx, store.Session{ID: s.id, TranscriptPath: s.path, StartedAt: start})
		for i, off := range s.offsets {
			err = errors.Join(err, st.PutTurn(ctx, store.Turn{
				ID: s.id + "-" + strconv.Itoa(i), SessionID: s.id, Status: store.TurnClosed,
				StartedAt: start.Add(time.Duration(i) * time.Minute), TranscriptChunk: chunk,
				TranscriptOffset: off,
			}))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(server.New(st, &query.Service{Store: st}, buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	return client.New(ts.URL)
}

// transcriptFile writes a sparse file of size bytes under dir, last
// written at mtime, and returns its path.
func transcriptFile(t *testing.T, dir, name string, size int64, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name+".jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return path
}

// runCheck runs one check through doctor.Collect and returns its result,
// failing t when it failed.
func runCheck(t *testing.T, name string, run func(context.Context) error) doctor.Result {
	t.Helper()
	report := doctor.Collect(context.Background(), []doctor.Check{{Name: name, Run: run}})
	if !report.OK {
		t.Fatalf("check failed: %+v", report.Checks[0])
	}
	return report.Checks[0]
}
