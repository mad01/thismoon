// Package gitvault runs git in a vault's checkout: the commit brain makes
// after each change it writes, the pull and push of brain sync, and the
// temporary worktrees a pull request is prepared in. A vault is a repo only
// when its root is the top level of a git work tree, so a vault directory
// inside some other repo (a home directory kept in git, say) never gets
// brain's commits.
//
// Git runs with GIT_TERMINAL_PROMPT=0, so a remote that wants credentials
// fails instead of waiting on a terminal no MCP client has.
package gitvault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// networkTimeout bounds a git command that talks to a remote. A remote on a
// dead connection would otherwise hang the command for as long as TCP waits.
const networkTimeout = 2 * time.Minute

// localTimeout bounds a git command that only touches the checkout.
const localTimeout = 30 * time.Second

// ErrNoUpstream means the checkout's branch tracks no remote branch, so
// brain does not know what to pull from or push to.
var ErrNoUpstream = errors.New("gitvault: no upstream branch")

// Repo is one vault checkout. The zero value is not usable; call New.
type Repo struct {
	// Root is the vault root, the directory git runs in.
	Root string

	once sync.Once
	top  bool
}

// New returns the Repo of the vault at root. It runs no git until a method
// needs it, so opening a vault costs nothing.
func New(root string) *Repo {
	return &Repo{Root: root}
}

// IsRepo reports whether the vault root is the top level of a git work
// tree. A missing git binary reads as false. The answer is cached.
func (r *Repo) IsRepo() bool {
	r.once.Do(func() {
		out, err := r.output(context.Background(), localTimeout, "rev-parse", "--show-toplevel")
		if err != nil {
			return
		}
		top, err1 := filepath.EvalSymlinks(out)
		root, err2 := filepath.EvalSymlinks(r.Root)
		r.top = err1 == nil && err2 == nil && top == root
	})
	return r.top
}

// Commit commits exactly paths, vault-relative, with message. It stages
// only them (git add -- paths) and commits only them (git commit -- paths),
// so changes a person left in other files, staged or not, stay as they
// were. A path that is neither on disk nor tracked (a note created and
// moved again before any commit) is left out. Commit does nothing when
// the vault is not a repo or none of the paths changed, and never pushes.
func (r *Repo) Commit(message string, paths []string) error {
	if !r.IsRepo() || len(paths) == 0 {
		return nil
	}
	ctx := context.Background()
	keep, err := r.known(ctx, paths)
	if err != nil || len(keep) == 0 {
		return err
	}
	if _, err := r.output(ctx, localTimeout, append([]string{"add", "--"}, keep...)...); err != nil {
		return err
	}
	changed, err := r.stagedChanges(ctx, keep)
	if err != nil || !changed {
		return err
	}
	args := append([]string{"commit", "--quiet", "-m", message, "--"}, keep...)
	_, err = r.output(ctx, localTimeout, args...)
	return err
}

// known returns the paths that are on disk or tracked by git.
func (r *Repo) known(ctx context.Context, paths []string) ([]string, error) {
	keep := make([]string, 0, len(paths))
	for _, p := range dedupe(paths) {
		if _, err := os.Lstat(filepath.Join(r.Root, filepath.FromSlash(p))); err == nil {
			keep = append(keep, p)
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		tracked, err := r.output(ctx, localTimeout, "ls-files", "--", p)
		if err != nil {
			return nil, err
		}
		if tracked != "" {
			keep = append(keep, p)
		}
	}
	return keep, nil
}

// stagedChanges reports whether the index differs from HEAD for paths. A
// repo without a commit yet has everything to commit.
func (r *Repo) stagedChanges(ctx context.Context, paths []string) (bool, error) {
	if _, err := r.output(ctx, localTimeout, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return true, nil //nolint:nilerr // no HEAD yet: the first commit
	}
	args := append([]string{"diff", "--cached", "--quiet", "--"}, paths...)
	err := r.run(ctx, localTimeout, nil, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

// Remotes returns the names of the checkout's remotes; none means the
// vault is local-only.
func (r *Repo) Remotes() ([]string, error) {
	out, err := r.output(context.Background(), localTimeout, "remote")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Fields(out), nil
}

// RemoteURL returns the fetch URL of the named remote.
func (r *Repo) RemoteURL(remote string) (string, error) {
	return r.output(context.Background(), localTimeout, "remote", "get-url", remote)
}

// Upstream returns the remote and remote branch the checked-out branch
// tracks, or ErrNoUpstream. A detached HEAD has no upstream either.
func (r *Repo) Upstream() (remote, branch string, err error) {
	ctx := context.Background()
	local, err := r.output(ctx, localTimeout, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("%w: HEAD is detached", ErrNoUpstream)
	}
	remote, _ = r.output(ctx, localTimeout, "config", "--get", "branch."+local+".remote")
	merge, _ := r.output(ctx, localTimeout, "config", "--get", "branch."+local+".merge")
	if remote == "" || remote == "." || merge == "" {
		return "", "", fmt.Errorf("%w: branch %s tracks no remote branch", ErrNoUpstream, local)
	}
	return remote, strings.TrimPrefix(merge, "refs/heads/"), nil
}

// Dirty returns the tracked files with uncommitted changes, staged or not.
// Untracked files do not count: a pull never touches them.
func (r *Repo) Dirty() ([]string, error) {
	var out bytes.Buffer
	err := r.run(context.Background(), localTimeout, &out,
		"status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	var files []string
	for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
		// A porcelain line is two status letters, a space, then the path.
		if len(line) > 3 {
			files = append(files, line[3:])
		}
	}
	return files, nil
}

// Fetch fetches the named remote.
func (r *Repo) Fetch(remote string) error {
	_, err := r.output(context.Background(), networkTimeout, "fetch", "--quiet", remote)
	return err
}

// Counts returns how many commits HEAD has that ref lacks (ahead) and ref
// has that HEAD lacks (behind).
func (r *Repo) Counts(ref string) (ahead, behind int, err error) {
	out, err := r.output(context.Background(), localTimeout,
		"rev-list", "--left-right", "--count", "HEAD..."+ref)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("git rev-list --count printed %q", out)
	}
	if ahead, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, err
	}
	behind, err = strconv.Atoi(fields[1])
	return ahead, behind, err
}

// PullFFOnly runs git pull --ff-only, which refuses anything but a
// fast-forward.
func (r *Repo) PullFFOnly() error {
	_, err := r.output(context.Background(), networkTimeout, "pull", "--ff-only", "--quiet")
	return err
}

// Push pushes HEAD to branch on remote.
func (r *Repo) Push(remote, branch string) error {
	_, err := r.output(context.Background(), networkTimeout,
		"push", "--quiet", remote, "HEAD:refs/heads/"+branch)
	return err
}

// RemoteBranchExists reports whether remote has branch.
func (r *Repo) RemoteBranchExists(remote, branch string) (bool, error) {
	out, err := r.output(context.Background(), networkTimeout,
		"ls-remote", "--heads", remote, "refs/heads/"+branch)
	return out != "", err
}

// SymbolicRef returns where a symbolic ref points, such as the branch
// refs/remotes/origin/HEAD names, or "" when it is not set.
func (r *Repo) SymbolicRef(ref string) string {
	out, err := r.output(context.Background(), localTimeout, "symbolic-ref", "--quiet", ref)
	if err != nil {
		return ""
	}
	return out
}

// AddWorktree checks base out, detached, into the empty directory dir and
// returns the worktree's Repo. Nothing in the vault's own checkout changes.
func (r *Repo) AddWorktree(dir, base string) (*Repo, error) {
	_, err := r.output(context.Background(), localTimeout,
		"worktree", "add", "--quiet", "--detach", dir, base)
	if err != nil {
		return nil, err
	}
	return New(dir), nil
}

// RemoveWorktree removes the worktree at dir, and the directory with it.
func (r *Repo) RemoveWorktree(dir string) error {
	ctx := context.Background()
	_, err := r.output(ctx, localTimeout, "worktree", "remove", "--force", dir)
	if rmErr := os.RemoveAll(dir); rmErr != nil {
		err = errors.Join(err, rmErr)
	}
	_, pruneErr := r.output(ctx, localTimeout, "worktree", "prune")
	return errors.Join(err, pruneErr)
}

// output runs git in the vault root and returns its trimmed stdout.
func (r *Repo) output(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	var stdout bytes.Buffer
	if err := r.run(ctx, timeout, &stdout, args...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// run runs git in the vault root, writing stdout to out when it is set.
// A failure carries git's stderr.
func (r *Repo) run(
	ctx context.Context,
	timeout time.Duration,
	out *bytes.Buffer,
	args ...string,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out != nil {
		cmd.Stdout = out
	}
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("git %s: timed out after %s", args[0], timeout)
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return fmt.Errorf("git %s: %w: %s", args[0], err, msg)
	}
	return fmt.Errorf("git %s: %w", args[0], err)
}

// dedupe drops repeated paths, keeping the first of each.
func dedupe(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
