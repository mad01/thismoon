// Package scanner orchestrates one scan: discover dependencies across the
// registered repos, check each against OSV, persist the result, and (for serve)
// notify on advisories not seen before. It is the functional core wiring
// discover + osv + store + notify together; the CLI and serve loop call in here.
package scanner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/notify"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// Checker is the OSV surface the scanner needs (real *osv.Client or a fake).
type Checker interface {
	Check(ctx context.Context, queries []osv.Query) ([][]store.Advisory, error)
}

// Discover enumerates dependencies across repos without checking advisories.
// Returned dependencies have no Advisories attached. Discovery errors are
// returned alongside whatever was found, so a single bad repo doesn't lose the
// rest.
func Discover(
	repos []string,
	ecosystems []discover.Ecosystem,
	opts discover.Options,
) ([]store.Dependency, error) {
	return discover.Repos(repos, ecosystems, opts)
}

// Check attaches OSV advisories to each dependency in place and returns the
// annotated set. Dependencies with no advisory come back unchanged.
func Check(
	ctx context.Context,
	deps []store.Dependency,
	checker Checker,
) ([]store.Dependency, error) {
	if len(deps) == 0 {
		return deps, nil
	}
	queries := make([]osv.Query, len(deps))
	for i, d := range deps {
		queries[i] = osv.Query{Name: d.Name, Ecosystem: d.Ecosystem, Version: d.Version}
	}
	advisories, err := checker.Check(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("osv check: %w", err)
	}
	for i := range deps {
		if i < len(advisories) {
			deps[i].Advisories = advisories[i]
		}
	}
	return deps, nil
}

// Engine binds the scan pieces to a concrete store and repo set. The serve loop
// and the HTTP handlers share one Engine, so a scheduled scan and an on-demand
// one take exactly the same path.
type Engine struct {
	// Prepare resolves, per scan, the repos to walk and the path-level
	// exclusions to apply — both derived from the discovery config, loaded once
	// here so editing the config takes effect next cycle without a restart.
	Prepare    func() ([]string, discover.Options, error)
	Ecosystems []discover.Ecosystem
	Checker    Checker
	Notifier   notify.Notifier
	Store      *store.Store
}

// Scan discovers dependencies (no advisory check) and persists them. Discovery
// problems are logged, not fatal, so a transient bad repo still yields a scan.
func (e *Engine) Scan() ([]store.Dependency, error) {
	repos, opts, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	deps, derr := Discover(repos, e.Ecosystems, opts)
	if derr != nil {
		log.Printf("deps: %v", derr)
	}
	if err := e.Store.Save(deps); err != nil {
		return nil, err
	}
	return deps, nil
}

// Check discovers dependencies, checks them against OSV, and persists the
// annotated set.
func (e *Engine) Check(ctx context.Context) ([]store.Dependency, error) {
	start := time.Now()
	repos, opts, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	deps, derr := Discover(repos, e.Ecosystems, opts)
	if derr != nil {
		log.Printf("deps: %v", derr)
	}
	deps, err = Check(ctx, deps, e.Checker)
	if err != nil {
		return nil, err
	}
	if err := e.Store.Save(deps); err != nil {
		return nil, err
	}
	flagged := 0
	for _, d := range deps {
		if d.Flagged() {
			flagged++
		}
	}
	level := "info"
	if flagged > 0 {
		level = "warn"
	}
	notify.EmitEvent("deps", level,
		fmt.Sprintf("scan complete: %d deps, %d flagged", len(deps), flagged),
		"",
		map[string]string{
			"repos":    fmt.Sprintf("%d", len(repos)),
			"deps":     fmt.Sprintf("%d", len(deps)),
			"flagged":  fmt.Sprintf("%d", flagged),
			"duration": time.Since(start).Round(time.Second).String(),
		})
	return deps, nil
}

// CheckRepo rescans a single repo (matched by absolute path or basename),
// checks it against OSV, and merges the result into the store without touching
// the other repos or the last full-scan time. It is the per-repo "Rescan" path.
func (e *Engine) CheckRepo(ctx context.Context, repoID string) ([]store.Dependency, error) {
	repos, opts, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	repo, ok := matchRepo(repos, repoID)
	if !ok {
		return nil, fmt.Errorf("repo %q is not among the scanned repos", repoID)
	}
	deps, derr := Discover([]string{repo}, e.Ecosystems, opts)
	if derr != nil {
		log.Printf("deps: %v", derr)
	}
	deps, err = Check(ctx, deps, e.Checker)
	if err != nil {
		return nil, err
	}
	if err := e.Store.SaveRepo(repo, deps); err != nil {
		return nil, err
	}
	return deps, nil
}

// matchRepo resolves a user-supplied repo id (an absolute path or a basename)
// to one of the scanned repo roots.
func matchRepo(repos []string, id string) (string, bool) {
	for _, r := range repos {
		if r == id || filepath.Base(r) == id {
			return r, true
		}
	}
	return "", false
}

// Notify fires the notifier for flagged advisories not yet delivered.
func (e *Engine) Notify() (int, error) {
	return Notify(e.Store, e.Notifier)
}

// CheckAndNotify is one full cycle for the background loop: re-check every dep
// against OSV, then notify on anything newly flagged.
func (e *Engine) CheckAndNotify(ctx context.Context) error {
	if _, err := e.Check(ctx); err != nil {
		return err
	}
	_, err := e.Notify()
	return err
}

// Notify fires ONE coalesced macOS notification summarizing every flagged
// advisory not yet notified, then records them all as notified. Coalescing is
// deliberate: a fresh scan can surface dozens of advisories across many
// transitive deps, and firing one banner each would flood NotificationCenter
// (the sandbox-watch script batches denials for the same reason). A delivery
// failure leaves the flags pending so the next cycle retries. Returns the number
// of advisories covered by the notification (0 if nothing new).
func Notify(st *store.Store, n notify.Notifier) (int, error) {
	pending := st.PendingFlags()
	if len(pending) == 0 {
		return 0, nil
	}
	title, body := summarize(pending)
	if err := n.Notify(title, body); err != nil {
		log.Printf("deps: notify failed, will retry: %v", err)
		return 0, nil
	}
	notify.EmitEvent("deps", "warn", title, body, nil)
	if err := st.MarkNotified(pending); err != nil {
		return len(pending), err
	}
	return len(pending), nil
}

// summarize builds the coalesced banner: a count in the title and, in the body,
// the distinct affected packages (capped) so the banner is actionable at a
// glance. Full detail lives on the web page and in deps_check.
func summarize(pending []store.Flag) (title, body string) {
	const maxNamed = 4
	seen := map[string]struct{}{}
	var names []string
	for _, f := range pending {
		key := f.Dep.Name + "@" + f.Dep.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, f.Dep.Name)
	}
	title = fmt.Sprintf("deps: %d new advisory(ies) across %d package(s)", len(pending), len(names))
	shown := names
	if len(shown) > maxNamed {
		shown = shown[:maxNamed]
	}
	body = strings.Join(shown, ", ")
	if len(names) > len(shown) {
		body += fmt.Sprintf(", +%d more", len(names)-len(shown))
	}
	body += " — see deps.this"
	return title, body
}
