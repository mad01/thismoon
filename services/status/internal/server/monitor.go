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
	Version   string        `json:"version,omitempty"`
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
	version string
	webkit  string
	fetched time.Time
}

// Monitor discovers, probes, and records services on an interval and keeps
// the latest Snapshot for the handlers.
type Monitor struct {
	opts  Options
	store *history.Store
	loops *crashloop.Tracker

	mu   sync.RWMutex
	snap Snapshot
	meta map[string]meta
}

func newMonitor(opts Options, store *history.Store) *Monitor {
	return &Monitor{
		opts:  opts,
		store: store,
		loops: crashloop.New(opts.RestartWindow, opts.RestartThreshold, opts.RestartCooldown),
		meta:  map[string]meta{},
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

	statuses := make([]ServiceStatus, len(services))
	down := 0
	m.mu.RLock()
	// Capture the prior up/down state per label so we can emit an event only on
	// a transition. A label absent here (first cycle, or a freshly discovered
	// service) is left out so startup doesn't emit a burst of spurious events.
	prevUp := make(map[string]bool, len(m.snap.Services))
	for _, st := range m.snap.Services {
		prevUp[st.Label] = st.Up
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
			st.Version, st.Webkit = md.version, md.webkit
		}
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
	m.detectCrashLoops(services, runs, runsOK, now)
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
		notify.EmitEvent("status", "error", a.Label+" crash-looping", lines[i],
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
			notify.EmitEvent("status", "info", st.Label+" recovered", "",
				map[string]string{"service": st.Label})
		} else {
			notify.EmitEvent("status", "warn", st.Label+" down", st.Detail,
				map[string]string{"service": st.Label})
		}
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
// slower cadence than the checks. Best-effort: services without the endpoints
// simply show no version.
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
		if ok && now.Sub(md.fetched) < m.opts.MetaInterval {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			base := fmt.Sprintf("http://127.0.0.1:%d", svc.Port)
			md := meta{
				version: fetchVersion(ctx, base+"/version"),
				webkit:  fetchVersion(ctx, base+"/webkit/version"),
				fetched: now,
			}
			m.mu.Lock()
			m.meta[svc.Label] = md
			m.mu.Unlock()
		}()
	}
	wg.Wait()
}

var metaClient = &http.Client{Timeout: 3 * time.Second}

// fetchVersion reads a {"version":"..."} payload (the cross-tool convention);
// any failure returns "".
func fetchVersion(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := metaClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Version
}
