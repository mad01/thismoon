package syncer

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Retry tuning for transient `git pull` failures. Running many pulls in
// parallel fires enough simultaneous SSH handshakes that one occasionally
// times out (kex_exchange_identification); a couple of backed-off retries ride
// that out so concurrency can stay high without polluting "Needs attention".
const (
	pullMaxAttempts = 3
	pullBackoffBase = 1 * time.Second
)

// pullTimeout bounds one `git pull` attempt. A remote on a dead socket (the
// Mac slept mid-pull, the VPN dropped) otherwise hangs the pull for hours, and
// with it the cycle that holds the sync lock. A var so tests can shrink it.
var pullTimeout = 5 * time.Minute

// pullWaitDelay caps how long a cancelled pull may keep its output pipes open
// after the kill, so a grandchild that escaped the process group cannot block
// the pull from returning.
const pullWaitDelay = 3 * time.Second

// msgPullCancelled is the result message for a repo whose pull the caller's
// context stopped.
const msgPullCancelled = "pull cancelled"

// Errors a pull attempt reports when it was stopped instead of failing on its
// own. Neither is retried: a hung remote stays hung, and a cancelled sync is
// shutting down.
var (
	errPullTimeout   = errors.New("syncer: pull timed out")
	errPullCancelled = errors.New("syncer: pull cancelled")
)

func pullAll(
	ctx context.Context,
	repos []finder.Repo,
	concurrency int,
	dryRun bool,
	onDone func(),
) []PullResult {
	sem := semaphore.NewWeighted(int64(concurrency))
	results := make([]PullResult, len(repos))

	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r finder.Repo) {
			defer wg.Done()
			results[idx] = pullWithSlot(ctx, sem, r, dryRun)
			if onDone != nil {
				onDone()
			}
		}(i, repo)
	}
	wg.Wait()

	return results
}

// pullWithSlot waits for a worker slot, then pulls r. A ctx cancelled while
// waiting fails the repo without pulling; Acquire holds no slot when it errors,
// so there is nothing to release.
func pullWithSlot(
	ctx context.Context,
	sem *semaphore.Weighted,
	r finder.Repo,
	dryRun bool,
) PullResult {
	if err := sem.Acquire(ctx, 1); err != nil {
		return PullResult{Repo: r, Status: "fail", Message: msgPullCancelled}
	}
	defer sem.Release(1)
	return pullRepo(ctx, r, dryRun)
}

func pullRepo(ctx context.Context, repo finder.Repo, dryRun bool) PullResult {
	r := PullResult{Repo: repo}

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

	r.Status, r.Message = pullWithRetry(ctx, repo.Path)
	return r
}

// pullWithRetry pulls the repo at dir, retrying transient SSH/network failures
// with backoff, and returns its pull status and message.
func pullWithRetry(ctx context.Context, dir string) (status, message string) {
	var out string
	var err error
	for attempt := 1; attempt <= pullMaxAttempts; attempt++ {
		out, err = pullOnce(ctx, dir)
		// Success, a failure that retrying won't fix (auth, diverged, missing
		// ref), or a pull that was stopped: give up. Only flaky network/SSH
		// errors are worth a retry.
		if err == nil || errors.Is(err, errPullTimeout) || errors.Is(err, errPullCancelled) ||
			!isTransientPullError(out) {
			break
		}
		if attempt == pullMaxAttempts || !waitBackoff(ctx, pullBackoff(attempt)) {
			break
		}
	}
	// The pull may have changed git state; drop any cached fingerprint so the
	// next staleness check re-reads it instead of waiting out the TTL.
	search.InvalidateFingerprint(dir)
	switch {
	case errors.Is(err, errPullTimeout):
		return "fail", fmt.Sprintf("pull timed out after %s", pullTimeout)
	case errors.Is(err, errPullCancelled):
		return "fail", msgPullCancelled
	case err != nil:
		return "fail", classifyPullError(out)
	case strings.Contains(out, "Already up to date"):
		return "ok", ""
	}
	return "updated", ""
}

// pullOnce runs one pull attempt bounded by pullTimeout. It reports
// errPullTimeout when the deadline stopped the pull and errPullCancelled when
// ctx did, so the caller can tell a hung remote from one that refused. The
// low-speed settings make an https remote on a dead socket abort by itself;
// ssh remotes rely on the deadline, since overriding the ssh command would
// clobber users' agent setups.
func pullOnce(ctx context.Context, dir string) (string, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	out, err := gitOutContext(
		attemptCtx,
		dir,
		"-c", "core.hooksPath=/dev/null",
		"-c", "http.lowSpeedLimit=1",
		"-c", "http.lowSpeedTime=30",
		"pull", "--ff-only", "--no-rebase",
	)
	switch {
	case err == nil:
		return out, nil
	case ctx.Err() != nil:
		return out, fmt.Errorf("%w: %w", errPullCancelled, ctx.Err())
	case attemptCtx.Err() != nil:
		return out, fmt.Errorf("%w after %s: %w", errPullTimeout, pullTimeout, err)
	}
	return out, err
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

// gitOutContext is gitOut bound to ctx. git leads its own process group and
// cancellation kills the whole group: a hung pull's ssh or git-remote-https
// grandchild holds the output pipe and would outlive a kill of git alone.
func gitOutContext(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process.Pid) }
	cmd.WaitDelay = pullWaitDelay
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// killProcessGroup sends SIGKILL to every process in the group that pid leads.
// A group that already exited reports os.ErrProcessDone, so exec returns the
// command's own exit status instead of a cancellation error.
func killProcessGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
