package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/status/internal/history"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>fake-web</string><key>ProgramArguments</key><array><string>/bin/fake</string><string>serve</string><string>--port</string><string>%d</string></array><key>TManMetadata</key><dict><key>ManagedBy</key><string>t-man</string></dict></dict></plist>`

// newTestMonitor wires a Monitor at a temp agents dir containing one fake
// t-man service whose --port points at the given httptest server.
func newTestMonitor(t *testing.T, ts *httptest.Server) *Monitor {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	agents := t.TempDir()
	plist := fmt.Sprintf(plistTemplate, mustPort(t, u))
	if err := os.WriteFile(filepath.Join(agents, "fake-web.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := history.Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		AgentsDir:  agents,
		DaemonsDir: filepath.Join(agents, "no-daemons"),
		RoutesPath: "",
	}
	opts.defaults()
	return newMonitor(opts, store)
}

func mustPort(t *testing.T, u *url.URL) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
		t.Fatal(err)
	}
	return port
}

// TestCycleRecordsAndRecolors walks the full chain: check result -> history
// bucket -> snapshot -> rendered bar color, through an up/down/up sequence.
func TestCycleRecordsAndRecolors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	m := newTestMonitor(t, ts)
	ctx := context.Background()

	// Cycle 1: service up -> 1 ok, green day, "all operational" page.
	m.cycle(ctx)
	snap := m.Snapshot()
	if len(snap.Services) != 1 || snap.Down != 0 {
		t.Fatalf("snapshot = %d services / %d down, want 1 / 0", len(snap.Services), snap.Down)
	}
	svc := snap.Services[0]
	if !svc.Up || svc.Label != "fake-web" {
		t.Fatalf("service = %+v, want fake-web up", svc)
	}
	// Bar color and banner text now live in app.js; the Go side owns the
	// snapshot data those derive from. 1 up cycle -> 100% day, down count 0.
	today := svc.Days[len(svc.Days)-1]
	if today.OK != 1 || today.Fail != 0 || today.Pct != 100 {
		t.Fatalf("today after 1 up cycle = %+v, want 1 ok / 100%%", today)
	}

	// Cycle 2: service down -> 1 ok / 1 fail = 50%, down count 1.
	ts.Close()
	m.cycle(ctx)
	snap = m.Snapshot()
	if snap.Down != 1 || snap.Services[0].Up {
		t.Fatalf("after outage: down=%d up=%v, want 1/false", snap.Down, snap.Services[0].Up)
	}
	today = snap.Services[0].Days[len(snap.Services[0].Days)-1]
	if today.OK != 1 || today.Fail != 1 || today.Pct != 50 {
		t.Fatalf("today after outage = %+v, want 1 ok / 1 fail / 50%%", today)
	}
}

// TestTodayRecoversTowardGreen confirms today's block climbs back through the
// thresholds as green minutes accumulate after an outage.
func TestTodayRecoversTowardGreen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	m := newTestMonitor(t, ts)
	ctx := context.Background()

	// One failed check (server briefly unreachable) -> 0% day (red in app.js).
	ts.CloseClientConnections()
	ts.Close()
	m.cycle(ctx)
	day := m.Snapshot().Services[0].Days
	if d := day[len(day)-1]; !d.HasData || d.Pct >= 95 {
		t.Fatalf("after 1 fail: %+v, want recorded data below 95%%", d)
	}

	// Bring it back on the same port and accumulate ok checks. 1 fail needs
	// 19 oks to cross 95% (amber) and 199 to cross 99.5% (green); drive the
	// store directly past both thresholds and re-render via a cycle each time.
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()
	m2 := newTestMonitor(t, ts2)
	m2.store = m.store // carry the failed check over

	now := m.Snapshot().GeneratedAt
	for range 19 {
		m2.store.Record("fake-web", now, true)
	}
	m2.cycle(ctx) // adds one more ok: 21 ok / 1 fail = 95.45%
	day = m2.Snapshot().Services[0].Days
	if d := day[len(day)-1]; d.Pct < 95 || d.Pct >= 99.5 {
		t.Fatalf("past 95%%: %+v, want 95%% <= pct < 99.5%% (amber)", d)
	}

	for range 200 {
		m2.store.Record("fake-web", now, true)
	}
	m2.cycle(ctx)
	day = m2.Snapshot().Services[0].Days
	if d := day[len(day)-1]; d.Pct < 99.5 {
		t.Fatalf("past 99.5%%: %+v, want pct >= 99.5%% (green)", d)
	}
}
