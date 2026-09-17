// Package syncer pulls the discovered repos and batch-reindexes the ones that
// changed — the shared engine behind the `csl sync` command and the background
// refresh loop in `csl web`. Both invocations run the same phases (discover,
// pull, queue drain, index, optional semantic re-embed) under the same lock,
// so a manual sync and a background refresh can never double-pull or race on
// state.json.
package syncer

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/csl/internal/queue"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// PullResult is the outcome of the pull phase for one repo.
type PullResult struct {
	Repo    finder.Repo
	Status  string // ok, updated, dirty, fail, detach, skip, noremote, notgit
	Message string
}

// Report summarizes one completed sync run.
type Report struct {
	Results []PullResult  // one per evaluated repo, sorted by name
	Indexed []finder.Repo // repos reindexed this run
}

// Options tunes one sync run. The zero value is a full, quiet, non-TTY run
// with the config-default concurrency.
type Options struct {
	// Concurrency is the number of parallel pull workers; zero or negative
	// means the config default (sync.concurrency, fallback 8).
	Concurrency int
	// DryRun evaluates every repo without pulling, indexing, or locking.
	DryRun bool
	// Only restricts the run to the single repo whose org/repo name or
	// absolute path matches. Empty means every discovered repo.
	Only string
	// Out receives the per-repo result lines and summary; nil discards them.
	Out io.Writer
	// Err receives progress and diagnostics; nil discards them.
	Err io.Writer
	// TTY marks Err as an interactive terminal, enabling in-place progress.
	TTY bool
}

// Run executes one sync: discover repos, pull them in parallel, drain the
// reindex queue, reindex what changed, and (when semantic.sync is on) re-embed
// the same repos. It holds the sync lock for the pull and index phases and
// returns ErrLocked without touching anything when another run already holds
// it.
func Run(ctx context.Context, cfg *config.Config, opts Options) (*Report, error) {
	start := time.Now()
	out, errw := opts.Out, opts.Err
	if out == nil {
		out = io.Discard
	}
	if errw == nil {
		errw = io.Discard
	}

	fmt.Fprintf(errw, "Discovering repos...")

	targets, err := cfg.DiscoverRepos()
	if err != nil {
		fmt.Fprintln(errw)
		return nil, err
	}

	if opts.Only != "" {
		targets = filterOnly(targets, opts.Only)
		if len(targets) == 0 {
			fmt.Fprintln(errw)
			return nil, fmt.Errorf("syncer: no repo matching %q", opts.Only)
		}
	}

	fmt.Fprintf(errw, " %d found\n", len(targets))

	concurrency := cfg.Sync.EffectiveConcurrency()
	if opts.Concurrency > 0 {
		concurrency = opts.Concurrency
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, err
	}

	// The lock covers the pull phase too, not just indexing: two concurrent
	// runs pulling the same working trees is exactly the collision the
	// background refresher and a manual `csl sync` must not have.
	if !opts.DryRun {
		unlock, lockErr := Lock(indexDir)
		if lockErr != nil {
			return nil, lockErr
		}
		defer unlock()
	}

	results := runPullPhase(ctx, targets, concurrency, opts, errw)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Repo.Name < results[j].Repo.Name
	})

	counts := countStatuses(results)
	for _, r := range results {
		msg := ""
		if r.Message != "" {
			msg = "  " + r.Message
		}
		fmt.Fprintf(out, "[%-7s] %s%s\n", r.Status, r.Repo.Name, msg)
	}

	report := &Report{Results: results}

	if opts.DryRun {
		fmt.Fprintf(out, "\ndry-run: %d repos evaluated\n", len(results))
		return report, nil
	}

	// Drain any queued repos from ad-hoc pulls.
	queuePath, _ := queue.DefaultPath()
	var queuedRepos []finder.Repo
	if queuePath != "" {
		claimedPath, queuedPaths, claimErr := queue.Claim(queuePath)
		if claimErr == nil && len(queuedPaths) > 0 {
			fmt.Fprintf(errw, "\nDraining %d queued repo(s)...\n", len(queuedPaths))
			for _, p := range queuedPaths {
				repo, err := finder.Inspect(p)
				if err != nil {
					continue
				}
				queuedRepos = append(queuedRepos, repo)
			}
			defer func() { _ = queue.Release(claimedPath) }()
		}
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return nil, err
	}

	updated := reposToIndex(results, state, queuedRepos)

	if len(updated) == 0 {
		fmt.Fprintf(out, "\n%s\n", syncSummary(counts))
		printAttention(out, results)
		emitSyncEvent(start, counts, len(results), 0)
		return report, nil
	}

	fmt.Fprintf(errw, "\nIndexing %d repo(s)...\n", len(updated))

	if err := search.IndexRepos(indexDir, updated, cfg.AllowedHiddenDirs(), func(i, total int, repo finder.Repo) {
		fmt.Fprintf(errw, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}); err != nil {
		return nil, fmt.Errorf("indexing failed: %w", err)
	}

	for _, repo := range updated {
		fp, fpErr := search.Fingerprint(repo.Path)
		if fpErr == nil {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	if err := state.Save(indexDir); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}
	report.Indexed = updated

	fmt.Fprintf(out, "indexed %d repo(s)\n", len(updated))

	// Semantic reindex of the same changed repos, opt-in via semantic.sync and
	// fully decoupled: the lexical reindex above has already succeeded, so any
	// failure here is reported but never fails the sync.
	if cfg.SemanticSyncEnabled() {
		runSemanticSync(ctx, errw, updated, opts.TTY)
	}

	fmt.Fprintf(out, "\n%s\n", syncSummary(counts))
	printAttention(out, results)
	emitSyncEvent(start, counts, len(results), len(updated))

	return report, nil
}

// runPullPhase pulls the targets in parallel, driving in-place TTY progress on
// errw when opts.TTY is set.
func runPullPhase(
	ctx context.Context,
	targets []finder.Repo,
	concurrency int,
	opts Options,
	errw io.Writer,
) []PullResult {
	total := len(targets)
	var completed atomic.Int64
	var onDone func()
	if opts.TTY && total > 0 {
		width := len(fmt.Sprint(total))
		fmt.Fprintf(errw, "Pulling [%*d/%d]", width, 0, total)
		onDone = func() {
			n := completed.Add(1)
			fmt.Fprintf(errw, "\rPulling [%*d/%d]", width, n, total)
		}
	} else if total > 0 {
		fmt.Fprintf(errw, "Pulling %d repos...\n", total)
	}

	results := pullAll(ctx, targets, concurrency, opts.DryRun, onDone)

	if opts.TTY && total > 0 {
		fmt.Fprintln(errw)
	}
	return results
}

// filterOnly keeps the repos whose org/repo name or absolute path equals only.
func filterOnly(repos []finder.Repo, only string) []finder.Repo {
	var out []finder.Repo
	for _, r := range repos {
		if r.Name == only || r.Path == only {
			out = append(out, r)
		}
	}
	return out
}

func countStatuses(results []PullResult) map[string]int {
	counts := make(map[string]int, 8)
	for _, r := range results {
		counts[r.Status]++
	}
	return counts
}

func syncSummary(counts map[string]int) string {
	return fmt.Sprintf(
		"sync: %d ok, %d updated, %d dirty, %d fail, %d detach, %d skip, %d noremote, %d notgit",
		counts["ok"],
		counts["updated"],
		counts["dirty"],
		counts["fail"],
		counts["detach"],
		counts["skip"],
		counts["noremote"],
		counts["notgit"],
	)
}

// emitSyncEvent archives one summary event per completed (non-dry-run) sync
// run to the local events service; best-effort, dropped if serve is down.
func emitSyncEvent(start time.Time, counts map[string]int, total, indexed int) {
	level := "info"
	if counts["fail"] > 0 {
		level = "warn"
	}
	notify.EmitEventSync(
		"csl",
		level,
		fmt.Sprintf(
			"sync complete: %d repos, %d updated, %d indexed",
			total,
			counts["updated"],
			indexed,
		),
		syncSummary(counts),
		map[string]string{
			"repos":    fmt.Sprintf("%d", total),
			"updated":  fmt.Sprintf("%d", counts["updated"]),
			"indexed":  fmt.Sprintf("%d", indexed),
			"failed":   fmt.Sprintf("%d", counts["fail"]),
			"duration": time.Since(start).Round(time.Second).String(),
		},
	)
}

// reposToIndex selects which repos need (re-)indexing after a pull phase.
// Includes repos updated by pull, newly discovered repos absent from state,
// and queued repos from ad-hoc pulls (deduped).
func reposToIndex(
	results []PullResult,
	state *search.IndexState,
	queued []finder.Repo,
) []finder.Repo {
	var out []finder.Repo
	for _, r := range results {
		if r.Status == "updated" {
			out = append(out, r.Repo)
		}
	}

	for _, r := range results {
		if r.Status != "ok" {
			continue
		}
		if _, known := state.GetRepo(r.Repo.Path); !known {
			out = append(out, r.Repo)
		}
	}

	seen := make(map[string]bool, len(out))
	for _, r := range out {
		seen[r.Path] = true
	}
	for _, r := range queued {
		if !seen[r.Path] {
			seen[r.Path] = true
			out = append(out, r)
		}
	}
	return out
}

func printAttention(w io.Writer, results []PullResult) {
	var attention []PullResult
	for _, r := range results {
		if r.Status == "dirty" || r.Status == "fail" {
			attention = append(attention, r)
		}
	}
	if len(attention) == 0 {
		return
	}
	fmt.Fprintf(w, "\nNeeds attention (%d):\n", len(attention))
	for _, r := range attention {
		name := r.Repo.Name
		if name == "" {
			name = r.Repo.Path
		}
		if name == "" {
			name = "(unknown repo)"
		}
		msg := ""
		if r.Message != "" {
			msg = " — " + r.Message
		}
		fmt.Fprintf(w, "  [%-7s] %s%s\n", r.Status, name, msg)
	}
}
