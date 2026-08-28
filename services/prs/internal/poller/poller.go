// Package poller owns the refresh cycle: discover the locally checked-out
// repos (the same kit/repofind walk csl-style tools use), fetch each repo's
// open PRs from its GitHub host, and hand the results to the store. One
// cycle at a time; the background loop and POST /api/refresh share it.
package poller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/glob"

	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

// heartbeat is how often the loop checks whether a cycle is due — the
// deps-style staleness pattern, so wake-from-sleep never waits a full
// interval and a missed tick costs nothing.
const heartbeat = 30 * time.Second

// fetchWorkers bounds the concurrent per-repo API fetches in one cycle.
const fetchWorkers = 8

// Fetcher fetches one repo's open PRs; *github.Client satisfies it.
type Fetcher interface {
	OpenPRs(ctx context.Context, host, owner, name string) ([]store.PR, error)
}

// Summary reports one completed cycle.
type Summary struct {
	Repos    int           `json:"repos"`
	OpenPRs  int           `json:"open_prs"`
	Errors   int           `json:"errors"`
	Duration time.Duration `json:"-"`
	PolledAt time.Time     `json:"polled_at"`
}

// Poller runs refresh cycles against the store.
type Poller struct {
	cfg     config.Config
	store   *store.Store
	fetcher Fetcher
	now     func() time.Time

	mu          sync.Mutex // serializes cycles
	lastAttempt time.Time  // last cycle start, successful or not
}

// New returns a Poller. The store must already be loaded.
func New(cfg config.Config, st *store.Store, fetcher Fetcher) *Poller {
	return &Poller{
		cfg:     cfg,
		store:   st,
		fetcher: fetcher,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Run polls until ctx is cancelled: an immediate cycle when the store is
// stale (fresh start, wake from sleep), then one whenever the interval has
// passed. It runs for the life of the serve process.
func (p *Poller) Run(ctx context.Context) {
	for {
		if p.due() {
			if _, err := p.Refresh(ctx); err != nil {
				log.Printf("prs: poll cycle: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(heartbeat):
		}
	}
}

// due reports whether a new cycle should start: the interval has passed
// since the last attempt (not the last success, so a failing cycle retries
// at the poll interval instead of every heartbeat).
func (p *Poller) due() bool {
	p.mu.Lock()
	last := p.lastAttempt
	p.mu.Unlock()
	if last.IsZero() {
		last = p.store.Status().PolledAt
	}
	return p.now().Sub(last) >= p.cfg.PollInterval
}

// Refresh runs one synchronous cycle: discovery, fetch, store. Concurrent
// callers serialize; each still runs its own full cycle.
func (p *Poller) Refresh(ctx context.Context) (Summary, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	start := p.now()
	p.lastAttempt = start

	// Excludes are NOT passed to Find: the include-overrides-exclude rule
	// needs both lists in one place, pollTargets.
	repos, err := repofind.Find(p.cfg.Dirs, nil)
	if err != nil {
		return Summary{}, fmt.Errorf("discover repos: %w", err)
	}
	targets, err := pollTargets(repos, p.cfg)
	if err != nil {
		return Summary{}, err
	}

	states := p.fetchAll(ctx, targets)
	polledAt := p.now()
	for i := range states {
		states[i].FetchedAt = polledAt
	}
	if err := p.store.SetRepos(states, polledAt); err != nil {
		return Summary{}, err
	}

	sum := Summary{Repos: len(states), PolledAt: polledAt, Duration: p.now().Sub(start)}
	for _, st := range states {
		sum.OpenPRs += len(st.PRs)
		if st.Error != "" {
			sum.Errors++
		}
	}
	return sum, nil
}

// fetchAll fans the targets over the worker pool. A fetch error keeps the
// repo's previous PRs and records the error, so a transient failure never
// blanks the page.
func (p *Poller) fetchAll(ctx context.Context, targets []target) []store.RepoState {
	states := make([]store.RepoState, len(targets))
	workCh := make(chan int)
	var wg sync.WaitGroup
	for range fetchWorkers {
		wg.Go(func() {
			for i := range workCh {
				states[i] = p.fetchOne(ctx, targets[i])
			}
		})
	}
	for i := range targets {
		workCh <- i
	}
	close(workCh)
	wg.Wait()
	return states
}

func (p *Poller) fetchOne(ctx context.Context, t target) store.RepoState {
	st := store.RepoState{Host: t.Host, Repo: t.Owner + "/" + t.Name}
	prs, err := p.fetcher.OpenPRs(ctx, t.Host, t.Owner, t.Name)
	if err != nil {
		prev, _ := p.store.Repo(st.Key())
		st.PRs = prev.PRs
		st.Error = err.Error()
		return st
	}
	st.PRs = prs
	return st
}

// target is one repo to poll, its org/name split for the API.
type target struct {
	Host  string
	Owner string
	Name  string
}

// pollTargets reduces discovered repos to pollable targets: a repo needs a
// parsed host that passes the allowlist, must survive the exclude globs
// (config plus built-in defaults, with the include globs overriding both),
// and two checkouts of the same remote poll once. Deeply nested remote
// paths keep their last two segments, which is what the GitHub API
// addresses.
func pollTargets(repos []repofind.Repo, cfg config.Config) ([]target, error) {
	excludes, err := compileGlobs(append(cfg.Exclude, config.DefaultExcludes()...))
	if err != nil {
		return nil, err
	}
	includes, err := compileGlobs(cfg.Include)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var targets []target
	for _, r := range repos {
		if r.Host == "" || !cfg.HostAllowed(r.Host) {
			continue
		}
		parts := strings.Split(r.Name, "/")
		if len(parts) < 2 {
			continue
		}
		owner, name := parts[len(parts)-2], parts[len(parts)-1]
		if matchesAny(owner+"/"+name, excludes) && !matchesAny(owner+"/"+name, includes) {
			continue
		}
		key := r.Host + "/" + owner + "/" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, target{Host: r.Host, Owner: owner, Name: name})
	}
	return targets, nil
}

func compileGlobs(patterns []string) ([]glob.Glob, error) {
	compiled := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, fmt.Errorf("invalid repo pattern %q: %w", p, err)
		}
		compiled = append(compiled, g)
	}
	return compiled, nil
}

func matchesAny(name string, patterns []glob.Glob) bool {
	for _, g := range patterns {
		if g.Match(name) {
			return true
		}
	}
	return false
}
