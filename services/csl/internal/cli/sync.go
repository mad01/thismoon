package cli

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/notify"
	"github.com/mad01/thismoon/services/csl/internal/queue"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

const syncLockFile = ".csl-sync.lock"

// Retry tuning for transient `git pull` failures. Running many pulls in
// parallel fires enough simultaneous SSH handshakes that one occasionally
// times out (kex_exchange_identification); a couple of backed-off retries ride
// that out so concurrency can stay high without polluting "Needs attention".
const (
	pullMaxAttempts = 3
	pullBackoffBase = 1 * time.Second
)

var (
	syncConcurrencyFlag int
	syncDryRunFlag      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull all repos and batch-reindex updated ones",
	Long: `Pull all discovered repos (parallel, ff-only) and batch-reindex the ones
that changed, in a single process with one state.json write.

Hooks are suppressed during sync pulls via core.hooksPath=/dev/null. Any repos
queued by earlier ad-hoc git pulls are also drained and indexed.

Repos on non-default branches, in detached HEAD, with dirty working trees
(tracked uncommitted changes), or without a remote are skipped. Untracked
files do not count as dirty. Repos in hooks.post_merge.exclude are also
skipped.

Use --concurrency to control parallel pulls (default: from config, fallback 8).
Use --dry-run to preview what would happen without pulling or indexing.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().
		IntVar(&syncConcurrencyFlag, "concurrency", 0, "parallel pull workers (0 = use config default)")
	syncCmd.Flags().
		BoolVar(&syncDryRunFlag, "dry-run", false, "preview without pulling or indexing")
	rootCmd.AddCommand(syncCmd)
}

type pullResult struct {
	Repo    finder.Repo
	Status  string // ok, updated, dirty, fail, detach, skip, noremote, notgit
	Message string
}

func runSync(cmd *cobra.Command, args []string) error {
	start := time.Now()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	w := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()

	isTTY := false
	if f, ok := w.(*os.File); ok {
		isTTY = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}

	fmt.Fprintf(w, "Discovering repos...")

	repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		fmt.Fprintln(w)
		return err
	}

	concurrency := cfg.Sync.EffectiveConcurrency()
	if syncConcurrencyFlag > 0 {
		concurrency = syncConcurrencyFlag
	}

	// Apply exclude list (reuse hooks.post_merge.exclude).
	var targets []finder.Repo
	for _, r := range repos {
		if cfg.Hooks.PostMerge.IsExcluded(r.Path, r.Name) {
			continue
		}
		targets = append(targets, r)
	}

	fmt.Fprintf(w, " %d found\n", len(targets))

	// --- Pull phase ---
	total := len(targets)
	var completed atomic.Int64
	var onDone func()
	if isTTY && total > 0 {
		width := len(fmt.Sprint(total))
		fmt.Fprintf(w, "Pulling [%*d/%d]", width, 0, total)
		onDone = func() {
			n := completed.Add(1)
			fmt.Fprintf(w, "\rPulling [%*d/%d]", width, n, total)
		}
	} else if total > 0 {
		fmt.Fprintf(w, "Pulling %d repos...\n", total)
	}

	results := pullAll(cmd.Context(), targets, concurrency, syncDryRunFlag, onDone)

	if isTTY && total > 0 {
		fmt.Fprintln(w)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Repo.Name < results[j].Repo.Name
	})

	var counts [8]int // ok, updated, dirty, fail, detach, skip, noremote, notgit
	statusIdx := map[string]int{
		"ok":       0,
		"updated":  1,
		"dirty":    2,
		"fail":     3,
		"detach":   4,
		"skip":     5,
		"noremote": 6,
		"notgit":   7,
	}
	for _, r := range results {
		tag := r.Status
		msg := ""
		if r.Message != "" {
			msg = "  " + r.Message
		}
		fmt.Fprintf(out, "[%-7s] %s%s\n", tag, r.Repo.Name, msg)
		counts[statusIdx[tag]]++
	}

	if syncDryRunFlag {
		fmt.Fprintf(out, "\ndry-run: %d repos evaluated\n", len(results))
		return nil
	}

	// --- Drain any queued repos from ad-hoc pulls ---
	queuePath, _ := queue.DefaultPath()
	var queuedRepos []finder.Repo
	if queuePath != "" {
		claimedPath, queuedPaths, claimErr := queue.Claim(queuePath)
		if claimErr == nil && len(queuedPaths) > 0 {
			fmt.Fprintf(w, "\nDraining %d queued repo(s)...\n", len(queuedPaths))
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

	// --- Index phase ---
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return err
	}

	unlock, lockErr := acquireSyncLock(indexDir)
	if lockErr != nil {
		fmt.Fprintf(w, "warning: could not acquire sync lock: %v\n", lockErr)
	} else {
		defer unlock()
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return err
	}

	updated := reposToIndex(results, state, queuedRepos)

	if len(updated) == 0 {
		fmt.Fprintf(out, "\n%s\n", syncSummary(counts))
		printAttention(out, results)
		emitSyncEvent(start, counts, len(results), 0)
		return nil
	}

	fmt.Fprintf(w, "\nIndexing %d repo(s)...\n", len(updated))

	if err := search.IndexRepos(indexDir, updated, func(i, total int, repo finder.Repo) {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	for _, repo := range updated {
		fp, fpErr := search.Fingerprint(repo.Path)
		if fpErr == nil {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	if err := state.Save(indexDir); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Fprintf(out, "indexed %d repo(s)\n", len(updated))

	// Semantic reindex of the same changed repos, opt-in via semantic.sync and
	// fully decoupled: the lexical reindex above has already succeeded, so any
	// failure here is reported but never fails the sync.
	if cfg.SemanticSyncEnabled() {
		runSemanticSync(cmd.Context(), w, updated, isTTY)
	}

	fmt.Fprintf(out, "\n%s\n", syncSummary(counts))
	printAttention(out, results)
	emitSyncEvent(start, counts, len(results), len(updated))

	return nil
}

// emitSyncEvent archives one summary event per completed (non-dry-run) sync
// run to the local events service; best-effort, dropped if serve is down.
func emitSyncEvent(start time.Time, counts [8]int, total, indexed int) {
	level := "info"
	if counts[3] > 0 { // fail
		level = "warn"
	}
	notify.EmitEvent("csl", level,
		fmt.Sprintf("sync complete: %d repos, %d updated, %d indexed", total, counts[1], indexed),
		syncSummary(counts),
		map[string]string{
			"repos":    fmt.Sprintf("%d", total),
			"updated":  fmt.Sprintf("%d", counts[1]),
			"indexed":  fmt.Sprintf("%d", indexed),
			"failed":   fmt.Sprintf("%d", counts[3]),
			"duration": time.Since(start).Round(time.Second).String(),
		})
}

// runSemanticSync loads the embedding model and re-embeds the changed repos into
// the semantic index. It is best-effort and decoupled from the lexical reindex:
// a missing model or unavailable backend is reported but never fails the sync.
func runSemanticSync(ctx context.Context, w io.Writer, repos []finder.Repo, tty bool) {
	if len(repos) == 0 {
		return
	}
	semDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		fmt.Fprintf(w, "semantic: skipped (%v)\n", err)
		return
	}
	modelDir, err := semantic.DefaultModelDir()
	if err != nil {
		fmt.Fprintf(w, "semantic: skipped (%v)\n", err)
		return
	}
	needsDownload := semantic.NeedsModelDownload(modelDir)
	if needsDownload {
		fmt.Fprintf(w, "\nDownloading embedding model (first run, may take a moment)...")
	}
	if err := semantic.EnsureModel(ctx, modelDir); err != nil {
		if needsDownload {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "semantic: skipped (model unavailable: %v)\n", err)
		return
	}
	if needsDownload {
		fmt.Fprintln(w, " done")
	}
	fmt.Fprintf(w, "\nLoading embedding model...")
	emb, err := semantic.NewHugotEmbedder(ctx, modelDir)
	if err != nil {
		fmt.Fprintf(w, " failed\nsemantic: skipped (embedder unavailable: %v)\n", err)
		return
	}
	defer func() { _ = emb.Close() }()

	fmt.Fprintf(w, " done\nSemantically indexing %d repo(s)...\n", len(repos))
	files, chunks, failed := semanticSyncRepos(ctx, w, semDir, emb, repos, tty)
	fmt.Fprintf(w, "semantic: %d file(s) embedded, %d chunk(s)", files, chunks)
	if failed > 0 {
		fmt.Fprintf(w, ", %d repo(s) failed", failed)
	}
	fmt.Fprintln(w)

	// The running daemon loaded the semantic index at startup; bounce it so the
	// next query spawns a fresh daemon that reloads the updated index.
	_ = daemon.Shutdown(daemon.DefaultSocketPath())
}

// semanticSyncRepos re-embeds each repo into the semantic index at semDir using
// emb. Indexing is incremental (only changed files are re-embedded) and
// best-effort: a repo that fails is logged and skipped, never aborting the rest.
// Returns the totals embedded and the count of repos that failed.
func semanticSyncRepos(
	ctx context.Context,
	w io.Writer,
	semDir string,
	emb semantic.Embedder,
	repos []finder.Repo,
	tty bool,
) (files, chunks, failed int) {
	repoWidth := len(fmt.Sprint(len(repos)))
	for i, repo := range repos {
		var progOpt semantic.IndexOption
		if tty {
			progOpt = semantic.WithProgress(func(p semantic.IndexProgress) {
				fileWidth := len(fmt.Sprint(p.FilesTotal))
				fmt.Fprintf(w, "\r  [%*d/%d] %s [%*d/%d] %s\033[K",
					repoWidth, i+1, len(repos), repo.Name,
					fileWidth, p.FilesDone, p.FilesTotal, p.Current)
			})
		}

		var opts []semantic.IndexOption
		if progOpt != nil {
			opts = append(opts, progOpt)
		}
		stats, err := semantic.IndexRepoSemantic(ctx, semDir, repo, emb, opts...)

		if tty {
			fmt.Fprintf(w, "\r\033[K")
		}
		if err != nil {
			failed++
			fmt.Fprintf(w, "  semantic: %s failed: %v\n", repo.Name, err)
			continue
		}
		files += stats.FilesEmbedded
		chunks += stats.ChunksEmbedded
		fmt.Fprintf(w, "  [%*d/%d] %s — %d embedded, %d chunks\n",
			repoWidth, i+1, len(repos), repo.Name,
			stats.FilesEmbedded, stats.ChunksEmbedded)
	}
	return files, chunks, failed
}

func pullAll(
	ctx context.Context,
	repos []finder.Repo,
	concurrency int,
	dryRun bool,
	onDone func(),
) []pullResult {
	sem := semaphore.NewWeighted(int64(concurrency))
	results := make([]pullResult, len(repos))

	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r finder.Repo) {
			defer wg.Done()
			_ = sem.Acquire(ctx, 1)
			defer sem.Release(1)
			results[idx] = pullRepo(ctx, r, dryRun)
			if onDone != nil {
				onDone()
			}
		}(i, repo)
	}
	wg.Wait()

	return results
}

func pullRepo(ctx context.Context, repo finder.Repo, dryRun bool) pullResult {
	r := pullResult{Repo: repo}

	if _, err := os.Stat(filepath.Join(repo.Path, ".git")); err != nil {
		r.Status = "notgit"
		return r
	}

	if repo.Remote == "" {
		r.Status = "noremote"
		return r
	}

	branch, err := gitOut(repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		r.Status = "fail"
		r.Message = "cannot determine branch"
		return r
	}
	if branch == "HEAD" {
		r.Status = "detach"
		return r
	}

	defaultBranch := resolveDefaultBranch(repo.Path)
	if branch != defaultBranch {
		r.Status = "skip"
		r.Message = fmt.Sprintf("on %s (default: %s)", branch, defaultBranch)
		return r
	}

	status, err := gitOut(repo.Path, "status", "--porcelain", "-uno")
	if err == nil && status != "" {
		r.Status = "dirty"
		r.Message = fmt.Sprintf("on %s, uncommitted changes", branch)
		return r
	}

	if dryRun {
		r.Status = "ok"
		r.Message = "(dry-run)"
		return r
	}

	var out string
	for attempt := 1; attempt <= pullMaxAttempts; attempt++ {
		out, err = gitOut(
			repo.Path,
			"-c", "core.hooksPath=/dev/null",
			"pull", "--ff-only", "--no-rebase",
		)
		// Success, or a failure that retrying won't fix (auth, diverged,
		// missing ref): stop. Only flaky network/SSH errors are worth a retry.
		if err == nil || !isTransientPullError(out) {
			break
		}
		if attempt == pullMaxAttempts || !waitBackoff(ctx, pullBackoff(attempt)) {
			break
		}
	}
	// The pull may have changed git state; drop any cached fingerprint so the
	// next staleness check re-reads it instead of waiting out the TTL.
	search.InvalidateFingerprint(repo.Path)
	if err != nil {
		r.Status = "fail"
		r.Message = classifyPullError(out)
		return r
	}

	if strings.Contains(out, "Already up to date") {
		r.Status = "ok"
	} else {
		r.Status = "updated"
	}
	return r
}

// isTransientPullError reports whether a `git pull` failure is a flaky
// network/SSH condition worth retrying — the kind that clears on its own when
// many parallel pulls stop hammering the remote at once. Permanent failures
// (auth, diverged history, missing ref, repo not found) return false so they
// surface immediately instead of burning retries.
func isTransientPullError(out string) bool {
	lower := strings.ToLower(out)
	transient := []string{
		"kex_exchange_identification",
		"timed out",
		"timeout",
		"connection refused",
		"connection reset",
		"connection closed",
		"could not resolve hostname",
		"name or service not known",
		"temporary failure in name resolution",
		"broken pipe",
		"early eof",
		"the remote end hung up unexpectedly",
		"rpc failed",
	}
	for _, s := range transient {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// pullBackoff returns the wait before the retry following the n-th (1-based)
// failed attempt: exponential base*2^(n-1) plus up to one base of jitter. The
// jitter scatters concurrent retries so they don't re-create the simultaneous
// handshake storm that caused the timeout in the first place.
func pullBackoff(attempt int) time.Duration {
	d := pullBackoffBase << (attempt - 1)
	return d + time.Duration(rand.Int64N(int64(pullBackoffBase)))
}

// waitBackoff sleeps for d, returning false if ctx is cancelled first so the
// caller stops retrying instead of blocking a shutting-down sync.
func waitBackoff(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// classifyPullError turns raw `git pull` output into a short, actionable
// reason. git leads its output with low-level transport noise (ssh's
// kex_exchange_identification / banner exchange lines come first), so the old
// "first 80 chars" truncation buried the meaningful failure and left the
// attention list reading like SSH debug spew. Match the common cases, then
// fall back to the most informative single line.
func classifyPullError(out string) string {
	lower := strings.ToLower(out)
	switch {
	case strings.Contains(lower, "kex_exchange_identification"),
		strings.Contains(lower, "timed out"),
		strings.Contains(lower, "timeout"):
		return "remote unreachable (ssh/network timeout — VPN down?)"
	case strings.Contains(lower, "could not resolve hostname"),
		strings.Contains(lower, "name or service not known"):
		return "cannot resolve remote host (DNS/offline)"
	case strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "publickey"):
		return "ssh auth failed (no valid key for remote)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused by remote"
	case strings.Contains(lower, "repository not found"),
		strings.Contains(lower, "does not appear to be a git repository"):
		return "remote repo not found (deleted/renamed/no access)"
	case strings.Contains(lower, "couldn't find remote ref"),
		strings.Contains(lower, "no such ref"):
		return "remote branch missing"
	case strings.Contains(lower, "not possible to fast-forward"),
		strings.Contains(lower, "diverged"):
		return "diverged from remote (ff-only failed — needs rebase/merge)"
	case strings.Contains(lower, "would be overwritten"),
		strings.Contains(lower, "local changes"):
		return "local changes block fast-forward"
	}
	return firstFatalLine(out)
}

// firstFatalLine returns the most informative single line of git output: the
// first line starting with "fatal:" or "error:" if present, otherwise the last
// non-empty line. Capped so a stray verbose remote can't blow up the list.
func firstFatalLine(out string) string {
	var last string
	for ln := range strings.SplitSeq(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		low := strings.ToLower(ln)
		if strings.HasPrefix(low, "fatal:") || strings.HasPrefix(low, "error:") {
			return truncateMessage(ln, 100)
		}
		last = ln
	}
	if last == "" {
		return "pull failed (no output)"
	}
	return truncateMessage(last, 100)
}

func truncateMessage(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func resolveDefaultBranch(repoPath string) string {
	ref, err := gitOut(repoPath, "rev-parse", "--abbrev-ref", "origin/HEAD")
	if err == nil {
		if name, ok := strings.CutPrefix(ref, "origin/"); ok && name != "" {
			return name
		}
	}
	if _, err := gitOut(repoPath, "rev-parse", "--verify", "refs/heads/main"); err == nil {
		return "main"
	}
	return "master"
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func acquireSyncLock(indexDir string) (unlock func(), err error) {
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(indexDir, syncLockFile)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// Lock file exists — check if the owning process is still alive.
		if os.IsExist(err) {
			if removeStaleLock(lockPath) {
				// Retry after removing stale lock.
				f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					return nil, fmt.Errorf("acquire sync lock after stale removal: %w", err)
				}
			} else {
				return nil, fmt.Errorf("sync lock held by another process (see %s)", lockPath)
			}
		} else {
			return nil, err
		}
	}
	// Write our PID so others can detect staleness.
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

func removeStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		// No PID written (old format) — assume stale, remove.
		_ = os.Remove(lockPath)
		return true
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		_ = os.Remove(lockPath)
		return true
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(lockPath)
		return true
	}
	// Signal 0 checks if the process exists without actually signaling it.
	if proc.Signal(syscall.Signal(0)) != nil {
		_ = os.Remove(lockPath)
		return true
	}
	return false
}

// reposToIndex selects which repos need (re-)indexing after a pull phase.
// Includes repos updated by pull, newly discovered repos absent from state,
// and queued repos from ad-hoc pulls (deduped).
func reposToIndex(
	results []pullResult,
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

func syncSummary(counts [8]int) string {
	return fmt.Sprintf(
		"sync: %d ok, %d updated, %d dirty, %d fail, %d detach, %d skip, %d noremote, %d notgit",
		counts[0],
		counts[1],
		counts[2],
		counts[3],
		counts[4],
		counts[5],
		counts[6],
		counts[7],
	)
}

func printAttention(w io.Writer, results []pullResult) {
	var attention []pullResult
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
