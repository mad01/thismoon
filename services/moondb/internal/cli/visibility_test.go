package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/ingest"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// liveServe runs what moondb serve runs over a fresh workdir: the local
// store, an ingester draining the spool and the HTTP server. It seeds the
// store with sessions before the ingester starts, and returns a
// visibilityRun wired to them and its output.
func liveServe(t *testing.T, sessions ...store.Session) (visibilityRun, *bytes.Buffer) {
	t.Helper()
	workdir := t.TempDir()
	st, err := local.Open(workdir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, s := range sessions {
		if err := st.PutSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(workdir, moondb.SpoolDirName)
	in := &ingest.Ingester{Store: st, Spool: &spool.Reader{Dir: dir}, Logf: t.Logf}
	go func() {
		defer close(done)
		_ = in.Run(ctx)
	}()
	ts := httptest.NewServer(server.New(st, &query.Service{Store: st}, buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	var out bytes.Buffer
	return visibilityRun{
		client: client.New(ts.URL),
		spool:  &spool.Writer{Dir: dir},
		out:    &out,
		wait:   visibilityWait,
	}, &out
}

func listedIDs(t *testing.T, c *client.Client, includeHidden bool) []string {
	t.Helper()
	list, err := c.Sessions(context.Background(), client.SessionQuery{IncludeHidden: includeHidden})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	return ids
}

// TestHideAndShowRoundTrip: hide takes a session out of the listing once
// serve has applied it, --all still lists it, and show puts it back. A
// prefix names the session, and naming it twice changes it once.
func TestHideAndShowRoundTrip(t *testing.T) {
	t0 := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	v, out := liveServe(t,
		store.Session{ID: "keep-1", Title: "kept", StartedAt: t0},
		store.Session{ID: "scratch-1", Title: "scratch", StartedAt: t0.Add(time.Minute)},
	)
	ctx := context.Background()

	if err := v.set(ctx, []string{"scratch", "scratch-1"}, true); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hidden "+short("scratch-1")+"  scratch\n"; got != want {
		t.Errorf("hide printed %q, want one line for scratch-1", got)
	}
	if ids := listedIDs(t, v.client, false); !slices.Equal(ids, []string{"keep-1"}) {
		t.Errorf("listing after hide = %v, want keep-1 alone", ids)
	}
	if ids := listedIDs(t, v.client, true); len(ids) != 2 {
		t.Errorf("listing with hidden = %v, want both", ids)
	}

	out.Reset()
	if err := v.set(ctx, []string{"scratch-1"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "shown "+short("scratch-1")) {
		t.Errorf("show printed %q", out.String())
	}
	if ids := listedIDs(t, v.client, false); len(ids) != 2 {
		t.Errorf("listing after show = %v, want both", ids)
	}
}

// TestHideUnknownSessionSpoolsNothing: an id serve does not know fails
// before anything reaches the spool, so the session it was meant for is
// not touched either.
func TestHideUnknownSessionSpoolsNothing(t *testing.T) {
	v, _ := liveServe(t, store.Session{ID: "keep-1", StartedAt: time.Now()})
	err := v.set(context.Background(), []string{"keep-1", "nope"}, true)
	if err == nil || !strings.Contains(err.Error(), `no session "nope"`) {
		t.Fatalf("err = %v, want the unknown id named", err)
	}
	if ids := listedIDs(t, v.client, false); !slices.Equal(ids, []string{"keep-1"}) {
		t.Errorf("listing = %v, want keep-1 still shown", ids)
	}
}

// TestHideWithoutServeDrainingTimesOut: when nothing drains the spool the
// command says the request is still queued instead of claiming success.
func TestHideWithoutServeDrainingTimesOut(t *testing.T) {
	v, _ := liveServe(t, store.Session{ID: "keep-1", StartedAt: time.Now()})
	v.spool = &spool.Writer{Dir: t.TempDir()} // a spool no ingester reads
	v.wait = 100 * time.Millisecond
	err := v.set(context.Background(), []string{"keep-1"}, true)
	if err == nil || !strings.Contains(err.Error(), "stays in the spool") {
		t.Fatalf("err = %v, want the request reported as still queued", err)
	}
}

// TestPrintSessionsAll: a listing with hidden sessions marks each row in a
// HIDDEN column; a plain listing has none.
func TestPrintSessionsAll(t *testing.T) {
	rows := []server.SessionSummary{
		{ID: "sess-hidden", Title: "scratch", Hidden: true},
		{ID: "sess-shown", Title: "kept"},
	}
	var plain, all bytes.Buffer
	printSessions(&plain, rows, "", false)
	printSessions(&all, rows, "", true)
	if strings.Contains(plain.String(), "HIDDEN") {
		t.Errorf("plain listing has a HIDDEN column:\n%s", plain.String())
	}
	lines := strings.Split(all.String(), "\n")
	col := slices.Index(strings.Fields(lines[0]), "HIDDEN")
	if col < 0 {
		t.Fatalf("header = %q, want a HIDDEN column", lines[0])
	}
	for i, want := range []string{"yes", "-"} {
		if f := strings.Fields(lines[i+1]); len(f) <= col || f[col] != want {
			t.Errorf("row %d = %q, want HIDDEN %s", i, lines[i+1], want)
		}
	}
}
