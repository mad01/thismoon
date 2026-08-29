package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	kitnotify "github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/status/internal/check"
	"github.com/mad01/thismoon/services/status/internal/crashloop"
	"github.com/mad01/thismoon/services/status/internal/discover"
	"github.com/mad01/thismoon/services/status/internal/history"
	"github.com/mad01/thismoon/services/status/internal/notify"
)

// ServiceStatus is one service's current state plus its uptime window.
type ServiceStatus struct {
	discover.Service
	Up        bool          `json:"up"`
	Known     bool          `json:"known"` // false until the first check completes
	Detail    string        `json:"detail,omitempty"`
	CheckedAt time.Time     `json:"checked_at"`
	Version   string        `json:"version,omitempty"`    // running process, from GET /version
	Tag       string        `json:"tag,omitempty"`        // release tag the running build came from
	BuildTime string        `json:"build_time,omitempty"` // when the running build was linked, RFC 3339
	Installed string        `json:"installed,omitempty"`  // binary on disk, from `<binary> version`
	Drift     bool          `json:"drift,omitempty"`      // running != installed, confirmed over consecutive cycles
	Webkit    string        `json:"webkit,omitempty"`
	Uptime    float64       `json:"uptime_pct"`
	HasUptime bool          `json:"has_uptime"`
	Days      []history.Day `json:"days"`
}

// Snapshot is the full dashboard state, also served at /api/status.
type Snapshot struct {
	GeneratedAt time.Time       `json:"generated_at"`
	WindowDays  int             `json:"window_days"`
	Down        int             `json:"down"`
	Services    []ServiceStatus `json:"services"`
}

type meta struct {
	version   string // running process, from GET /version
	tag       string // release tag the running build came from, "" if untagged
	buildTime string // when the running build was linked, "" if the service predates the field
	installed string // binary on disk, from `<binary> version`
	webkit    string
	fetched   time.Time
}

// drifted reports whether the running process and the binary on disk disagree
// about their build sha. Both sides must be known: a service without the
// endpoint or a binary without a version command can't drift.
func (m meta) drifted() bool {
	return m.version != "" && m.installed != "" && m.version != m.installed
}

// driftConfirmCycles is how many consecutive cycles a version mismatch must
// persist before it counts as drift. A mismatch observed mid-`ralph up`
// (binary replaced, restart a moment away) clears on the next cycle instead
// of alerting.
const driftConfirmCycles = 2

// Monitor discovers, probes, and records services on an interval and keeps
// the latest Snapshot for the handlers.
type Monitor struct {
	opts  Options
	store *history.Store
	loops *crashloop.Tracker

	mu   sync.RWMutex
	snap Snapshot
	meta map[string]meta

	// driftRuns counts consecutive cycles a service's meta has shown a version
	// mismatch. Only touched from the single cycle goroutine.
	driftRuns map[string]int
}

func newMonitor(opts Options, store *history.Store) *Monitor {
	return &Monitor{
		opts:      opts,
		store:     store,
		loops:     crashloop.New(opts.RestartWindow, opts.RestartThreshold, opts.RestartCooldown),
		meta:      map[string]meta{},
		driftRuns: map[string]int{},
	}
}

// Snapshot returns the latest poll result.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snap
}

func (m *Monitor) run(ctx context.Context) {
	m.cycle(ctx)
	tick := time.NewTicker(m.opts.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.cycle(ctx)
		}
	}
}

func (m *Monitor) cycle(ctx context.Context) {
	services, err := discover.Scan(m.opts.AgentsDir, m.opts.DaemonsDir, m.opts.RoutesPath)
	if err != nil {
		log.Printf("status: discover: %v", err)
		return
	}

	now := time.Now()
	results := make([]check.Result, len(services))
	runs := make([]int, len(services))
	runsOK := make([]bool, len(services))
	var wg sync.WaitGroup
	for i, svc := range services {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = check.Service(ctx, svc)
			runs[i], runsOK[i] = check.Runs(ctx, svc)
		}()
	}
	wg.Wait()

	for i, svc := range services {
		m.store.Record(svc.Label, now, results[i].Up)
	}
	m.store.Prune(now, m.opts.KeepDays)
	if err := m.store.Save(); err != nil {
		log.Printf("status: save history: %v", err)
	}

	m.refreshMeta(ctx, services, results, now)
	m.countDrift(services)

	statuses := make([]ServiceStatus, len(services))
	down := 0
	m.mu.RLock()
	// Capture the prior up/down and drift state per label so we can emit an
	// event only on a transition. A label absent here (first cycle, or a
	// freshly discovered service) is left out so startup doesn't emit a burst
	// of spurious events.
	prevUp := make(map[string]bool, len(m.snap.Services))
	prevDrift := make(map[string]bool, len(m.snap.Services))
	for _, st := range m.snap.Services {
		prevUp[st.Label] = st.Up
		prevDrift[st.Label] = st.Drift
	}
	for i, svc := range services {
		st := ServiceStatus{
			Service:   svc,
			Up:        results[i].Up,
			Known:     true,
			Detail:    results[i].Detail,
			CheckedAt: now,
			Days:      m.store.Days(svc.Label, now, m.opts.HistoryDays),
		}
		st.Uptime, st.HasUptime = m.store.Uptime(svc.Label, now, m.opts.HistoryDays)
		if md, ok := m.meta[svc.Label]; ok {
			st.Version, st.Webkit, st.Installed = md.version, md.webkit, md.installed
			st.Tag, st.BuildTime = md.tag, md.buildTime
		}
		st.Drift = m.driftRuns[svc.Label] >= driftConfirmCycles
		if !st.Up {
			down++
		}
		statuses[i] = st
	}
	m.mu.RUnlock()

	// Routed web services first, then other HTTP services, then PID-checked
	// jobs; alphabetical within each group.
	sort.SliceStable(statuses, func(i, j int) bool {
		gi, gj := group(statuses[i]), group(statuses[j])
		if gi != gj {
			return gi < gj
		}
		return statuses[i].Label < statuses[j].Label
	})

	m.mu.Lock()
	m.snap = Snapshot{
		GeneratedAt: now,
		WindowDays:  m.opts.HistoryDays,
		Down:        down,
		Services:    statuses,
	}
	m.mu.Unlock()

	emitTransitions(statuses, prevUp)
	emitDriftTransitions(statuses, prevDrift)
	m.detectCrashLoops(services, runs, runsOK, now)
}

// countDrift updates the consecutive-mismatch counter per service from the
// freshly refreshed meta. Runs on the cycle goroutine; only meta reads need
// the lock.
func (m *Monitor) countDrift(services []discover.Service) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, svc := range services {
		if m.meta[svc.Label].drifted() {
			m.driftRuns[svc.Label]++
		} else {
			delete(m.driftRuns, svc.Label)
		}
	}
}

// detectCrashLoops feeds launchd run counters into the tracker and, for each
// service that crossed the restart threshold, records an error event and posts
// one coalesced macOS banner (a single alert even when several services loop
// at once).
func (m *Monitor) detectCrashLoops(
	services []discover.Service,
	runs []int,
	runsOK []bool,
	now time.Time,
) {
	active := make(map[string]bool, len(services))
	var alerts []crashloop.Alert
	for i, svc := range services {
		active[svc.Label] = true
		if !runsOK[i] {
			continue
		}
		if a, fire := m.loops.Observe(svc.Label, now, runs[i]); fire {
			alerts = append(alerts, a)
		}
	}
	m.loops.Forget(active)

	if len(alerts) == 0 {
		return
	}
	lines := make([]string, len(alerts))
	for i, a := range alerts {
		lines[i] = fmt.Sprintf("%s: %d restarts in the last %s", a.Label, a.Restarts, a.Window)
		kitnotify.EmitEvent("status", "error", a.Label+" crash-looping", lines[i],
			map[string]string{
				"service":  a.Label,
				"restarts": strconv.Itoa(a.Restarts),
			})
	}
	notify.Banner("status: crash loop detected", strings.Join(lines, "\n"))
}

// emitTransitions records an event for each service whose up/down state flipped
// since the previous cycle. Services with no prior known state are skipped.
func emitTransitions(statuses []ServiceStatus, prevUp map[string]bool) {
	for _, st := range statuses {
		was, known := prevUp[st.Label]
		if !known || was == st.Up {
			continue
		}
		if st.Up {
			kitnotify.EmitEvent("status", "info", st.Label+" recovered", "",
				map[string]string{"service": st.Label})
		} else {
			kitnotify.EmitEvent("status", "warn", st.Label+" down", st.Detail,
				map[string]string{"service": st.Label})
		}
	}
}

// emitDriftTransitions records an event for each service whose drift state
// flipped since the previous cycle, plus one coalesced macOS banner when new
// drifts appear. This is the "ralph reported ok but the old binary kept
// running" signal: the process serves an older sha than the binary on disk.
func emitDriftTransitions(statuses []ServiceStatus, prevDrift map[string]bool) {
	var lines []string
	for _, st := range statuses {
		was, known := prevDrift[st.Label]
		if !known || was == st.Drift {
			continue
		}
		if st.Drift {
			line := fmt.Sprintf("%s: running %s, installed %s", st.Label, st.Version, st.Installed)
			lines = append(lines, line)
			kitnotify.EmitEvent("status", "warn", st.Label+" running stale binary", line,
				map[string]string{
					"service":   st.Label,
					"running":   st.Version,
					"installed": st.Installed,
				})
		} else {
			kitnotify.EmitEvent("status", "info", st.Label+" binary current", "",
				map[string]string{"service": st.Label})
		}
	}
	if len(lines) > 0 {
		notify.Banner("status: stale binary detected", strings.Join(lines, "\n"))
	}
}

func group(s ServiceStatus) int {
	switch {
	case s.Link != "":
		return 0
	case s.HTTP():
		return 1
	default:
		return 2
	}
}

// refreshMeta fetches /version and /webkit/version from up HTTP services at a
// slower cadence than the checks, plus the on-disk binary's own sha via
// `<binary> version`. Best-effort: services without the endpoints simply show
// no version. While running and installed disagree the cadence is ignored and
// the service is re-probed every cycle, so a mismatch confirms as drift (or
// clears after a restart) within a couple of cycles instead of MetaInterval.
func (m *Monitor) refreshMeta(
	ctx context.Context,
	services []discover.Service,
	results []check.Result,
	now time.Time,
) {
	var wg sync.WaitGroup
	for i, svc := range services {
		if !svc.HTTP() || !results[i].Up {
			continue
		}
		m.mu.RLock()
		md, ok := m.meta[svc.Label]
		m.mu.RUnlock()
		if ok && now.Sub(md.fetched) < m.opts.MetaInterval && !md.drifted() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			base := fmt.Sprintf("http://127.0.0.1:%d", svc.Port)
			info := fetchBuildInfo(ctx, base+"/version")
			md := meta{
				version:   info.Version,
				tag:       info.Tag,
				buildTime: info.BuildTime,
				installed: check.BinaryVersion(ctx, svc.Binary),
				webkit:    fetchBuildInfo(ctx, base+"/webkit/version").Version,
				fetched:   now,
			}
			m.mu.Lock()
			m.meta[svc.Label] = md
			m.mu.Unlock()
		}()
	}
	wg.Wait()
}

var metaClient = &http.Client{Timeout: 3 * time.Second}

// fetchBuildInfo reads the build metadata object a component serves at
// GET /version (the cross-tool convention). Keys the payload omits stay "", so
// a service still serving the older bare {"version":"..."} shape reports its
// version and nothing else; any failure returns the zero Info.
func fetchBuildInfo(ctx context.Context, url string) buildinfo.Info {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return buildinfo.Info{}
	}
	resp, err := metaClient.Do(req)
	if err != nil {
		return buildinfo.Info{}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return buildinfo.Info{}
	}
	var info buildinfo.Info
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return buildinfo.Info{}
	}
	return info
}
