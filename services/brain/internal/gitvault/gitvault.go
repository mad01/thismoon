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
	"io"
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

// notARepo starts what git prints, in the C locale, when no repository
// holds the directory it runs in. With exit status 128 it is the one git
// failure that means "not a repo"; any other failure is git not working.
const notARepo = "fatal: not a git repository"

// Repo is one vault checkout. The zero value is not usable; call New.
type Repo struct {
	// Root is the vault root, the directory git runs in.
	Root string

	mu     sync.Mutex
	probed bool // git answered whether Root is a top level
	top    bool
}

// New returns the Repo of the vault at root. It runs no git until a method
// needs it, so opening a vault costs nothing.
func New(root string) *Repo {
	return &Repo{Root: root}
}

// IsRepo reports whether the vault root is the top level of a git work
// tree. A git that fails, or is missing, reads as false here; Commit
// reports that failure instead.
func (r *Repo) IsRepo() bool {
	top, _ := r.isRepo()
	return top
}

// isRepo is IsRepo with git's failure kept. git's answer is cached; a
// failure is not, so the next call asks again.
func (r *Repo) isRepo() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.probed {
		return r.top, nil
	}
	top, err := r.probe()
	if err != nil {
		return false, err
	}
	r.probed, r.top = true, top
	return top, nil
}

// probe asks git for the top level of the work tree holding the vault
// root. git's own "not a git repository" is a clean no; any other failure,
// such as a global config git may not read, is an error. git runs in the C
// locale, so its answer reads the same under every language setting.
func (r *Repo) probe() (bool, error) {
	var out bytes.Buffer
	err := r.run(context.Background(), localTimeout,
		gitIO{stdout: &out, env: []string{"LC_ALL=C"}}, "rev-parse", "--show-toplevel")
	if exitCode(err) == 128 && strings.Contains(err.Error(), notARepo) {
		return false, nil
	}
	// No git on PATH: a plain-directory vault on a machine without git must
	// still take writes, and there is nothing to commit with either way.
	if errors.Is(err, exec.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(out.String()))
	if err != nil {
		return false, err
	}
	root, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return false, err
	}
	return top == root, nil
}

// Commit commits exactly paths, vault-relative, with message. It stages
// only them (git add -- paths) and commits only them (git commit -- paths),
// so changes a person left in other files, staged or not, stay as they
// were. A path that is neither on disk nor tracked (a note created and
// moved again before any commit) is left out. Commit does nothing when git
// says the vault is not a repo or none of the paths changed, and never
// pushes. Any other git failure is an error. When the commit fails after
// the paths were staged, Commit puts their index entries back as they were
// and leaves the files on disk; if that fails too, the error says the paths
// are still staged.
func (r *Repo) Commit(message string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if isRepo, err := r.isRepo(); err != nil || !isRepo {
		return err
	}
	ctx := context.Background()
	keep, err := r.known(ctx, paths)
	if err != nil || len(keep) == 0 {
		return err
	}
	before, err := r.indexEntries(ctx, keep)
	if err != nil {
		return err
	}
	// A git add that fails writes no index, so there is nothing to unstage.
	if _, err := r.output(ctx, localTimeout, append([]string{"add", "--"}, keep...)...); err != nil {
		return err
	}
	if err := r.commitStaged(ctx, message, keep); err != nil {
		return r.unstage(ctx, keep, before, err)
	}
	return nil
}

// commitStaged commits paths, which git add has just staged, unless the
// index holds no change to them.
func (r *Repo) commitStaged(ctx context.Context, message string, paths []string) error {
	changed, err := r.stagedChanges(ctx, paths)
	if err != nil || !changed {
		return err
	}
	args := append([]string{"commit", "--quiet", "-m", message, "--"}, paths...)
	_, err = r.output(ctx, localTimeout, args...)
	return err
}

// unstage puts the index entries of paths back to before, their listing
// from ahead of git add, once committing them failed with err. It returns
// err, and says the paths are still staged when the restore fails too.
func (r *Repo) unstage(ctx context.Context, paths []string, before []byte, err error) error {
	if restoreErr := r.restoreIndex(ctx, paths, before); restoreErr != nil {
		return fmt.Errorf("%w; %s still staged (unstage failed: %w)",
			err, strings.Join(paths, ", "), restoreErr)
	}
	return err
}

// restoreIndex replaces every index entry of paths with the entries in
// before. In one git update-index, a mode 0 line drops each path the index
// now holds, every stage of it, and before's lines put back what was
// there. Only the index changes; the files on disk stay as they are.
func (r *Repo) restoreIndex(ctx context.Context, paths []string, before []byte) error {
	after, err := r.indexEntries(ctx, paths)
	if err != nil {
		return err
	}
	var in bytes.Buffer
	for rec := range strings.SplitSeq(string(after), "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return fmt.Errorf("git ls-files --stage printed %q", rec)
		}
		// The zero object id has the length of the repo's object ids.
		fmt.Fprintf(&in, "0 %s\t%s\x00", strings.Repeat("0", len(fields[1])), path)
	}
	in.Write(before)
	return r.run(ctx, localTimeout, gitIO{stdin: &in}, "update-index", "-z", "--index-info")
}

// indexEntries returns git ls-files --stage -z for paths: one
// "<mode> <object> <stage>\t<path>" record per index entry, each ending in
// a NUL byte, the form git update-index --index-info reads back.
func (r *Repo) indexEntries(ctx context.Context, paths []string) ([]byte, error) {
	var out bytes.Buffer
	args := append([]string{"ls-files", "--stage", "-z", "--"}, paths...)
	if err := r.run(ctx, localTimeout, gitIO{stdout: &out}, args...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
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
// repo without a commit yet has everything to commit: rev-parse --verify
// --quiet exits 1, and only 1, when HEAD names no commit.
func (r *Repo) stagedChanges(ctx context.Context, paths []string) (bool, error) {
	err := r.run(ctx, localTimeout, gitIO{}, "rev-parse", "--verify", "--quiet", "HEAD")
	if exitCode(err) == 1 {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	args := append([]string{"diff", "--cached", "--quiet", "--"}, paths...)
	err = r.run(ctx, localTimeout, gitIO{}, args...)
	if exitCode(err) == 1 {
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
	err := r.run(context.Background(), localTimeout, gitIO{stdout: &out},
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
	if err := r.run(ctx, timeout, gitIO{stdout: &stdout}, args...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitIO is what one git command reads and writes besides its arguments.
// The zero value gives git no input and drops its output.
type gitIO struct {
	stdin  io.Reader
	stdout *bytes.Buffer
	env    []string // set over brain's own environment
}

// run runs git in the vault root with gio's input, output and environment.
// A failure carries git's stderr.
func (r *Repo) run(ctx context.Context, timeout time.Duration, gio gitIO, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Root}, args...)...)
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), gio.env...)
	cmd.Stdin = gio.stdin
	if gio.stdout != nil {
		cmd.Stdout = gio.stdout
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

// exitCode returns the exit status of a git that ran and failed, or -1 for
// no error or one where git never finished (not started, timed out).
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
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
