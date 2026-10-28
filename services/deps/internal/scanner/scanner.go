// Package scanner orchestrates one scan: discover dependencies across the
// registered repos, check each against OSV, persist the result, and (for serve)
// notify on advisories not seen before. It is the functional core wiring
// discover + osv + store + alert together; the CLI and serve loop call in here.
package scanner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/deps/internal/alert"
	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// Checker is the OSV surface the scanner needs (real *osv.Client or a fake).
type Checker interface {
	Check(ctx context.Context, queries []osv.Query) ([][]store.Advisory, error)
}

// Discover enumerates dependencies across repos without checking advisories.
// Returned dependencies have no Advisories attached. Each repo and ecosystem
// that failed comes back as a DiscoveryError alongside whatever was found, so a
// single bad repo doesn't lose the rest. Every failure is also logged.
func Discover(
	repos []string,
	ecosystems []discover.Ecosystem,
	opts discover.Options,
) ([]store.Dependency, []store.DiscoveryError) {
	deps, errs := discover.Repos(repos, ecosystems, opts)
	for _, e := range errs {
		log.Printf("deps: discovery: %s in %s: %s", e.Ecosystem, e.Repo, e.Error)
	}
	return deps, errs
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
	Notifier   alert.Notifier
	Store      *store.Store
}

// Scan discovers dependencies (no advisory check) and persists them with the
// discovery errors. Discovery problems are saved, not fatal, so a transient bad
// repo still yields a scan.
func (e *Engine) Scan() ([]store.Dependency, error) {
	repos, opts, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	deps, discErrs := Discover(repos, e.Ecosystems, opts)
	if err := e.Store.Save(deps, discErrs); err != nil {
		return nil, err
	}
	return deps, nil
}

// Check discovers dependencies, checks them against OSV, persists the
// annotated set with the discovery errors, and emits the scan-complete event.
func (e *Engine) Check(ctx context.Context) ([]store.Dependency, error) {
	start := time.Now()
	repos, opts, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	deps, discErrs := Discover(repos, e.Ecosystems, opts)
	deps, err = Check(ctx, deps, e.Checker)
	if err != nil {
		return nil, err
	}
	if err := e.Store.Save(deps, discErrs); err != nil {
		return nil, err
	}
	ev := scanCompleteEvent(len(repos), deps, discErrs, time.Since(start))
	notify.EmitEvent(ev.Source, ev.Level, ev.Title, ev.Message, ev.Tags)
	return deps, nil
}

// scanCompleteEvent builds the events-archive record for a finished full
// check. It warns when anything is flagged or any repo and ecosystem failed
// discovery, and the message names each failure, so an unchecked ecosystem
// shows up there instead of reading as a clean scan. A dependency whose only
// findings are signals does not count as flagged: signals stay on the page.
func scanCompleteEvent(
	repos int,
	deps []store.Dependency,
	discErrs []store.DiscoveryError,
	took time.Duration,
) notify.Event {
	flagged := 0
	for _, d := range deps {
		if slices.ContainsFunc(d.Advisories, func(a store.Advisory) bool {
			return a.Kind != store.KindSignal
		}) {
			flagged++
		}
	}
	level := "info"
	if flagged > 0 || len(discErrs) > 0 {
		level = "warn"
	}
	title := fmt.Sprintf("scan complete: %d deps, %d flagged", len(deps), flagged)
	lines := make([]string, 0, len(discErrs))
	if len(discErrs) > 0 {
		title += fmt.Sprintf(", %d discovery error(s)", len(discErrs))
		for _, de := range discErrs {
			lines = append(lines, fmt.Sprintf("%s in %s: %s", de.Ecosystem, de.Repo, de.Error))
		}
	}
	return notify.Event{
		Source:  "deps",
		Level:   level,
		Title:   title,
		Message: strings.Join(lines, "\n"),
		Tags: map[string]string{
			"repos":            fmt.Sprintf("%d", repos),
			"deps":             fmt.Sprintf("%d", len(deps)),
			"flagged":          fmt.Sprintf("%d", flagged),
			"discovery_errors": fmt.Sprintf("%d", len(discErrs)),
			"duration":         took.Round(time.Second).String(),
		},
	}
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
	deps, discErrs := Discover([]string{repo}, e.Ecosystems, opts)
	deps, err = Check(ctx, deps, e.Checker)
	if err != nil {
		return nil, err
	}
	if err := e.Store.SaveRepo(repo, deps, discErrs); err != nil {
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

// Notify fires the banners for every flagged advisory not yet notified, then
// records them as notified. Malicious findings get their own banner, fired
// first and never folded into the summary. Every other finding coalesces into
// ONE summary banner. Coalescing is deliberate: a fresh scan can surface dozens
// of advisories across many transitive deps, and firing one banner each would
// flood NotificationCenter (the sandbox-watch script batches denials for the
// same reason). A delivery failure leaves that banner's flags pending so the
// next cycle retries. Returns the number of advisories the delivered banners
// covered (0 if nothing new).
func Notify(st *store.Store, n alert.Notifier) (int, error) {
	malicious, rest := splitMalicious(st.PendingFlags())
	delivered := 0
	if len(malicious) > 0 {
		title, body := maliciousBanner(malicious)
		got, err := deliver(st, n, malicious, "error", title, body)
		delivered += got
		if err != nil {
			return delivered, err
		}
	}
	if len(rest) > 0 {
		title, body := summarize(rest)
		got, err := deliver(st, n, rest, "warn", title, body)
		delivered += got
		if err != nil {
			return delivered, err
		}
	}
	return delivered, nil
}

// deliver fires one banner covering flags, archives it as an event at level,
// and marks the flags notified. A failed delivery is logged and leaves them
// pending, so the next cycle retries.
func deliver(
	st *store.Store,
	n alert.Notifier,
	flags []store.Flag,
	level, title, body string,
) (int, error) {
	if err := n.Notify(title, body); err != nil {
		log.Printf("deps: notify failed, will retry: %v", err)
		return 0, nil
	}
	notify.EmitEvent("deps", level, title, body, nil)
	if err := st.MarkNotified(flags); err != nil {
		return len(flags), err
	}
	return len(flags), nil
}

// splitMalicious separates the malicious findings from the rest, keeping the
// order of each.
func splitMalicious(flags []store.Flag) (malicious, rest []store.Flag) {
	for _, f := range flags {
		if f.Advisory.Kind == store.KindMalicious {
			malicious = append(malicious, f)
		} else {
			rest = append(rest, f)
		}
	}
	return malicious, rest
}

// maliciousBanner names each malicious package version with the repos that
// pin it, and says what to do. A malicious version may already have run its
// install scripts, so the move is to check whether it was installed and rotate
// the credentials it could reach, not to wait for a version bump.
func maliciousBanner(flags []store.Flag) (title, body string) {
	const maxNamed = 3
	var pkgs []string
	repos := map[string][]string{}
	for _, f := range flags {
		pkg := f.Dep.Name + "@" + f.Dep.Version
		if _, ok := repos[pkg]; !ok {
			pkgs = append(pkgs, pkg)
		}
		repo := filepath.Base(f.Dep.Repo)
		if !slices.Contains(repos[pkg], repo) {
			repos[pkg] = append(repos[pkg], repo)
		}
	}
	if len(pkgs) == 1 {
		title = "deps: malicious package " + pkgs[0]
	} else {
		title = fmt.Sprintf("deps: %d malicious packages", len(pkgs))
	}
	named := make([]string, 0, maxNamed)
	for _, pkg := range pkgs[:min(len(pkgs), maxNamed)] {
		named = append(named, pkg+" in "+strings.Join(repos[pkg], ", "))
	}
	body = strings.Join(named, "; ")
	if len(pkgs) > maxNamed {
		body += fmt.Sprintf("; +%d more", len(pkgs)-maxNamed)
	}
	body += ". Check whether each was installed, and rotate the credentials it could reach. See deps.this"
	return title, body
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
