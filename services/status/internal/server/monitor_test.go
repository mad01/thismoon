package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/status/internal/history"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>fake-web</string><key>ProgramArguments</key><array><string>%s</string><string>serve</string><string>--port</string><string>%d</string></array><key>TManMetadata</key><dict><key>ManagedBy</key><string>t-man</string></dict></dict></plist>`

// newTestMonitor wires a Monitor at a temp agents dir containing one fake
// t-man service whose --port points at the given httptest server. The plist's
// binary doesn't exist, so no installed version is probed and drift stays off.
func newTestMonitor(t *testing.T, ts *httptest.Server) *Monitor {
	t.Helper()
	return newTestMonitorWithBinary(t, ts, "/bin/fake")
}

// newTestMonitorWithBinary is newTestMonitor with the plist's binary under
// test control, for the drift scenarios.
func newTestMonitorWithBinary(t *testing.T, ts *httptest.Server, binary string) *Monitor {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	agents := t.TempDir()
	plist := fmt.Sprintf(plistTemplate, binary, mustPort(t, u))
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

// writeFakeBinary drops a script at path that answers `version` with sha, the
// way every fleet binary does, and launches it once so a monitor cycle's
// timed probe never pays for its first launch.
func writeFakeBinary(t *testing.T, path, sha string) {
	t.Helper()
	script := "#!/bin/sh\necho " + sha + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	launchOnce(t, path)
}

// TestDriftDetection walks the stale-binary story: the running process serves
// an older sha than the binary on disk. One mismatched cycle is tolerated
// (mid-deploy), the second confirms drift, and a restart (running == installed
// again) clears it.
func TestDriftDetection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"version":"abc1234"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	bin := filepath.Join(t.TempDir(), "fake-web")
	writeFakeBinary(t, bin, "def5678")
	m := newTestMonitorWithBinary(t, ts, bin)
	ctx := context.Background()

	m.cycle(ctx)
	if s := m.Snapshot().Services[0]; s.Drift {
		t.Fatalf("drift after 1 cycle = true, want false (confirms after %d)", driftConfirmCycles)
	}

	m.cycle(ctx)
	s := m.Snapshot().Services[0]
	if !s.Drift || s.Version != "abc1234" || s.Installed != "def5678" {
		t.Fatalf("after 2 cycles: drift=%v version=%q installed=%q, want true/abc1234/def5678",
			s.Drift, s.Version, s.Installed)
	}

	// The service restarts on the new binary: /version and the on-disk sha
	// agree again. The drifted meta bypasses MetaInterval, so one cycle clears.
	writeFakeBinary(t, bin, "abc1234")
	m.cycle(ctx)
	if s := m.Snapshot().Services[0]; s.Drift || s.Installed != "abc1234" {
		t.Fatalf("after restart: drift=%v installed=%q, want false/abc1234", s.Drift, s.Installed)
	}
}

// TestBuildMetaFromVersionEndpoint checks the dashboard picks up the tag and
// build time a service reports alongside its version.
func TestBuildMetaFromVersionEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"version":"abc1234","commit":"abc1234def","tag":"fake/v1.2.3",`+
				`"build_time":"2026-08-13T09:00:00Z"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	m := newTestMonitor(t, ts)
	m.cycle(context.Background())
	s := m.Snapshot().Services[0]
	if s.Version != "abc1234" || s.Tag != "fake/v1.2.3" || s.BuildTime != "2026-08-13T09:00:00Z" {
		t.Fatalf("version=%q tag=%q build_time=%q, want abc1234/fake/v1.2.3/2026-08-13T09:00:00Z",
			s.Version, s.Tag, s.BuildTime)
	}
}

// TestBuildMetaBestEffort pins the "" semantics: a service still serving the
// older bare {"version":"..."} payload reports its version and leaves the tag
// and build time empty, so the dashboard renders neither.
func TestBuildMetaBestEffort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"version":"abc1234"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	m := newTestMonitor(t, ts)
	m.cycle(context.Background())
	s := m.Snapshot().Services[0]
	if s.Version != "abc1234" || s.Tag != "" || s.BuildTime != "" {
		t.Fatalf("version=%q tag=%q build_time=%q, want abc1234 and two empties",
			s.Version, s.Tag, s.BuildTime)
	}
}

// TestNoDriftWithoutInstalledVersion pins the guard: a binary that can't
// report a version (missing, not a fleet tool) never counts as drift.
func TestNoDriftWithoutInstalledVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"version":"abc1234"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	m := newTestMonitor(t, ts) // plist binary /bin/fake does not exist
	ctx := context.Background()

	m.cycle(ctx)
	m.cycle(ctx)
	s := m.Snapshot().Services[0]
	if s.Drift || s.Installed != "" || s.Version != "abc1234" {
		t.Fatalf(
			"drift=%v installed=%q version=%q, want false/empty/abc1234",
			s.Drift,
			s.Installed,
			s.Version,
		)
	}
}

// firstLaunchBudget bounds the untimed launch that warms a fake binary.
const firstLaunchBudget = 30 * time.Second

// launchOnce runs a newly written executable once, outside any probe
// deadline. macOS assesses a new executable on its first launch, and while a
// full make test is linking and launching dozens of fresh test binaries that
// queue made a fake script's first launch take up to 4.9 s, past the 3 s
// probe timeout; later launches of the same file took milliseconds.
func launchOnce(t *testing.T, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), firstLaunchBudget)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "version").CombinedOutput(); err != nil {
		t.Fatalf("first launch of %s: %v: %s", path, err, out)
	}
}
