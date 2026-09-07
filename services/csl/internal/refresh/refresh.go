// Package refresh runs the background index refresh inside `csl web`: a
// periodic syncer.Run (pull + reindex changed repos) plus a kick channel for
// the web UI's manual "refresh now". The loop wakes on a short heartbeat and
// compares timestamps instead of counting ticks, so a Mac waking from sleep
// catches up on the next heartbeat rather than silently skipping a cycle.
// Coordination with a manual `csl sync` comes from the shared sync lock: a
// cycle that finds the lock held is skipped and retried, never raced.
package refresh

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// heartbeat is how often the loop wakes to check whether a cycle is due. Short
// relative to the interval so a wake-from-sleep triggers a catch-up promptly
// (a time.Ticker does not advance while the Mac is asleep).
const heartbeat = time.Minute

// maxBackoff caps the retry delay after failed cycles, so an offline stretch
// settles into occasional retries instead of every-heartbeat churn.
const maxBackoff = time.Hour

// ErrBusy reports that a refresh is already running or queued; the caller
// should retry once the current one finishes.
var ErrBusy = errors.New("refresh: a refresh is already running")

// runFunc is the seam the Refresher drives one sync cycle through; production
// wires it to syncer.Run, tests inject a fake.
type runFunc func(ctx context.Context, only string) (*syncer.Report, error)

// outcome is the recorded result of the last refresh that touched one repo.
type outcome struct {
	status  string
	message string
	indexed bool
	at      time.Time
}

// Refresher owns the background refresh loop and the per-repo results the
// status page reads. One Refresher runs per `csl web` process; its enabled
// flag and interval are snapshotted from the config at construction, so a
// config change takes effect on the next web restart.
type Refresher struct {
	cfg       *config.Config
	enabled   bool
	interval  time.Duration
	heartbeat time.Duration
	run       runFunc
	now       func() time.Time
	kick      chan string

	mu          sync.Mutex
	running     bool
	lastRun     time.Time // last completed full cycle
	lastErr     string
	failures    int
	nextAttempt time.Time // earliest retry after a failure; zero = no backoff
	results     map[string]outcome
}

// New returns a Refresher for the given config. Run starts the loop; Kick and
// Status work regardless of whether background refresh is enabled.
func New(cfg *config.Config) *Refresher {
	return &Refresher{
		cfg:       cfg,
		enabled:   cfg.RefreshEnabled(),
		interval:  cfg.RefreshInterval(),
		heartbeat: heartbeat,
		run: func(ctx context.Context, only string) (*syncer.Report, error) {
			return syncer.Run(ctx, cfg, syncer.Options{Only: only})
		},
		now:     time.Now,
		kick:    make(chan string, 1),
		results: map[string]outcome{},
	}
}

// Run drives the refresh loop until ctx is cancelled. When background refresh
// is enabled it cycles immediately (catch-up on startup) and then whenever the
// last full cycle is older than the interval; manual kicks are served either
// way.
func (r *Refresher) Run(ctx context.Context) {
	if r.enabled {
		r.cycle(ctx, "")
	}
	t := time.NewTicker(r.heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case only := <-r.kick:
			r.cycle(ctx, only)
		case <-t.C:
			if r.due() {
				r.cycle(ctx, "")
			}
		}
	}
}

// Kick queues a manual refresh — of one repo (org/repo name) or, with an empty
// string, of everything. It returns ErrBusy while a refresh is running or
// already queued.
func (r *Refresher) Kick(only string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return ErrBusy
	}
	select {
	case r.kick <- only:
		return nil
	default:
		return ErrBusy
	}
}

// due reports whether a periodic cycle should run now: enabled, not backing
// off after a failure, and the last full cycle older than the interval.
func (r *Refresher) due() bool {
	if !r.enabled {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if now.Before(r.nextAttempt) {
		return false
	}
	return r.lastRun.IsZero() || now.Sub(r.lastRun) >= r.interval
}

// cycle runs one sync and records the result. A cycle skipped because another
// sync holds the lock is not a failure: it retries on the next heartbeat
// without backoff. Real failures back off exponentially, resetting on success.
func (r *Refresher) cycle(ctx context.Context, only string) {
	r.mu.Lock()
	r.running = true
	r.mu.Unlock()

	start := r.now()
	rep, err := r.run(ctx, only)
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false

	switch {
	case errors.Is(err, syncer.ErrLocked):
		r.lastErr = "skipped: another sync is running"
		log.Printf("csl web: refresh skipped, sync lock held elsewhere")
	case err != nil:
		r.failures++
		delay := backoffDelay(r.heartbeat, r.failures)
		r.nextAttempt = now.Add(delay)
		r.lastErr = err.Error()
		log.Printf(
			"csl web: refresh cycle failed (attempt %d), backing off %s: %v",
			r.failures, delay, err,
		)
	default:
		r.failures = 0
		r.nextAttempt = time.Time{}
		r.lastErr = ""
		if only == "" {
			r.lastRun = now
		}
		indexed := make(map[string]bool, len(rep.Indexed))
		for _, repo := range rep.Indexed {
			indexed[repo.Path] = true
		}
		for _, pr := range rep.Results {
			r.results[pr.Repo.Path] = outcome{
				status:  pr.Status,
				message: pr.Message,
				indexed: indexed[pr.Repo.Path],
				at:      now,
			}
		}
		log.Printf(
			"csl web: refresh complete: %d repos, %d indexed (%s)",
			len(rep.Results), len(rep.Indexed), now.Sub(start).Round(time.Second),
		)
	}
}

// backoffDelay grows the retry delay exponentially from one heartbeat, capped
// at maxBackoff: heartbeat, 2x, 4x, ...
func backoffDelay(base time.Duration, failures int) time.Duration {
	delay := base << (failures - 1)
	if delay <= 0 || delay > maxBackoff { // <=0 guards against shift overflow
		return maxBackoff
	}
	return delay
}

// Status is the /api/refresh_status payload: the loop state plus one row per
// discovered repo. Times are RFC3339 strings, empty when not applicable.
type Status struct {
	Enabled         bool         `json:"enabled"`
	IntervalMinutes int          `json:"interval_minutes"`
	Running         bool         `json:"running"`
	LastRun         string       `json:"last_run,omitempty"`
	NextRun         string       `json:"next_run,omitempty"`
	LastError       string       `json:"last_error,omitempty"`
	Error           string       `json:"error,omitempty"`
	Repos           []RepoStatus `json:"repos"`
}

// RepoStatus is one repo's row on the refresh page: the last refresh outcome
// that touched it plus the index timestamp from state.json.
type RepoStatus struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	Indexed     bool   `json:"indexed"`
	RefreshedAt string `json:"refreshed_at,omitempty"`
	IndexedAt   string `json:"indexed_at,omitempty"`
}

// Status snapshots the loop state and joins the discovered repos with their
// last refresh outcomes and indexed_at from state.json. Discovery or state
// errors land in the Error field; the loop state is always reported.
func (r *Refresher) Status() Status {
	r.mu.Lock()
	st := Status{
		Enabled:         r.enabled,
		IntervalMinutes: int(r.interval / time.Minute),
		Running:         r.running,
		LastRun:         rfc3339(r.lastRun),
		LastError:       r.lastErr,
		Repos:           []RepoStatus{},
	}
	if r.enabled && !r.lastRun.IsZero() {
		next := r.lastRun.Add(r.interval)
		if next.Before(r.nextAttempt) {
			next = r.nextAttempt
		}
		st.NextRun = rfc3339(next)
	}
	outcomes := maps.Clone(r.results)
	r.mu.Unlock()

	targets, err := r.cfg.DiscoverRepos()
	if err != nil {
		st.Error = fmt.Sprintf("discover repos: %v", err)
		return st
	}

	var state *search.IndexState
	if indexDir, dirErr := search.DefaultIndexDir(); dirErr == nil {
		if s, stateErr := search.LoadState(indexDir); stateErr == nil {
			state = s
		}
	}

	for _, repo := range targets {
		row := RepoStatus{Name: repo.Name, Path: repo.Path}
		if o, ok := outcomes[repo.Path]; ok {
			row.Status = o.status
			row.Message = o.message
			row.Indexed = o.indexed
			row.RefreshedAt = rfc3339(o.at)
		}
		if state != nil {
			if rs, ok := state.GetRepo(repo.Path); ok {
				row.IndexedAt = rfc3339(rs.IndexedAt)
			}
		}
		st.Repos = append(st.Repos, row)
	}
	sort.Slice(st.Repos, func(i, j int) bool { return st.Repos[i].Name < st.Repos[j].Name })
	return st
}

// rfc3339 formats t for the JSON payload, mapping the zero time to "".
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
