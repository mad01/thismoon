package guard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// lostCommits are the commits a command strands when it deletes worktree
// admin dirs: how many no surviving ref reaches, and the newest of them
// (tip), which no other lost commit has as an ancestor.
type lostCommits struct {
	n   int
	tip string
}

// refusal returns nil when nothing is lost, else the reason: what deletes
// the refs, the count, and the branch that saves the newest commit.
func (l lostCommits) refusal(what string) error {
	if l.n == 0 {
		return nil
	}
	return fmt.Errorf(
		"%s, the last refs that hold %d commit(s); run `git branch <name> %s` first",
		what, l.n, l.tip[:min(len(l.tip), shortOIDLen)],
	)
}

// shortOIDLen is how much of a commit id a reason shows.
const shortOIDLen = 12

// removeStranded counts what a remove of the linked worktree wt, run in
// dir, strands: the commits its HEAD and HEAD reflog alone hold
// (countStranded). git deletes the admin dir whose gitdir file names the
// worktree's path (adminPath), and every other admin dir stays. A worktree
// no admin dir names cannot be counted.
func removeStranded(ctx context.Context, dir string, wt listedWorktree) (lostCommits, error) {
	common, err := commonDir(ctx, dir)
	if err != nil {
		return lostCommits{}, err
	}
	ids, err := adminIDs(common)
	if err != nil {
		return lostCommits{}, err
	}
	var dropped, kept []string
	for _, id := range ids {
		p, ok := adminPath(filepath.Join(common, "worktrees", id))
		if ok && samePath(p, wt.path) {
			dropped = append(dropped, id)
			continue
		}
		kept = append(kept, id)
	}
	if len(dropped) == 0 {
		return lostCommits{}, notListed("no worktree admin dir names " + wt.path)
	}
	return countStranded(ctx, common, dropped, kept)
}

// pruneStranded counts what `git worktree prune` with the expire words, run
// in dir, strands: the admin dirs a dry run of the same prune names
// (prunedIDs), and the commits their HEADs and HEAD reflogs alone hold
// (countStranded). Every other admin dir stays.
func pruneStranded(ctx context.Context, dir string, expire []string) (lostCommits, error) {
	common, err := commonDir(ctx, dir)
	if err != nil {
		return lostCommits{}, err
	}
	pruned, err := prunedIDs(ctx, common, expire)
	if err != nil || len(pruned) == 0 {
		return lostCommits{}, err
	}
	ids, err := adminIDs(common)
	if err != nil {
		return lostCommits{}, err
	}
	for _, id := range pruned {
		if !slices.Contains(ids, id) {
			return lostCommits{}, notListed("a prune of the admin dir " + strconv.Quote(id))
		}
	}
	kept := slices.DeleteFunc(ids, func(id string) bool { return slices.Contains(pruned, id) })
	return countStranded(ctx, common, pruned, kept)
}

// countStranded counts the commits the HEADs and HEAD reflogs of the
// dropped admin dirs reach that nothing left reaches, read on the common
// dir, where the main worktree's view of the refs applies (git 2.55). What
// is left is every ref with its reflog, the main HEAD with its reflog, and
// the kept admin dirs' HEADs and HEAD reflogs. --all reads every worktree's
// HEAD but the dropped ones (--exclude). --exclude does not reach --reflog,
// which reads every worktree's HEAD reflog, so --single-worktree keeps it to
// the shared reflogs and the main HEAD's, and a reflog walk lists the kept
// ones. The count starts from the dropped HEADs that resolve (bornHeads),
// and with none of them nothing is lost.
func countStranded(ctx context.Context, common string, dropped, kept []string) (lostCommits, error) {
	heads := adminHeads(dropped)
	born, err := bornHeads(ctx, common, heads)
	if err != nil || len(born) == 0 {
		return lostCommits{}, err
	}
	ours, err := reflogCommits(ctx, common, born)
	if err != nil {
		return lostCommits{}, err
	}
	theirs, err := reflogCommits(ctx, common, adminHeads(kept))
	if err != nil {
		return lostCommits{}, err
	}
	in := slices.Concat(born, ours, []string{"--not"}, theirs)
	args := []string{"rev-list", "--topo-order"}
	for _, h := range heads {
		args = append(args, "--exclude="+h)
	}
	args = append(args, "--not", "--all", "--single-worktree", "--reflog", "--not", "--stdin")
	out, _, err := commonGit(ctx, common, []byte(strings.Join(in, "\n")+"\n"), args...)
	if err != nil {
		return lostCommits{}, notListed(err)
	}
	lost := strings.Fields(string(out))
	if len(lost) == 0 {
		return lostCommits{}, nil
	}
	return lostCommits{n: len(lost), tip: lost[0]}, nil
}

// reflogCommits lists the commits the reflogs of the admin dir HEADs heads
// (adminHeads) name. A HEAD that does not resolve, or that keeps no reflog,
// names none.
func reflogCommits(ctx context.Context, common string, heads []string) ([]string, error) {
	if len(heads) == 0 {
		return nil, nil
	}
	args := slices.Concat([]string{"rev-list", "--walk-reflogs", "--ignore-missing"}, heads)
	out, _, err := commonGit(ctx, common, nil, args...)
	if err != nil {
		return nil, notListed(err)
	}
	return strings.Fields(string(out)), nil
}

// bornHeads returns the admin dir HEADs of heads that resolve. A HEAD that
// does not resolve is on an unborn branch, as `git worktree add --orphan`
// leaves it, and holds no commit, so it is left out while it keeps no
// reflog. A reflog git cannot walk from such a HEAD may still hold commits,
// so it fails the count.
func bornHeads(ctx context.Context, common string, heads []string) ([]string, error) {
	var born []string
	for _, h := range heads {
		_, _, err := commonGit(ctx, common, nil, "rev-parse", "-q", "--verify", h)
		if err == nil {
			born = append(born, h)
			continue
		}
		if !exitedWith(err, 1) {
			return nil, notListed(err)
		}
		_, _, err = commonGit(ctx, common, nil, "reflog", "exists", h)
		switch {
		case err == nil:
			return nil, notListed(h + " does not resolve and keeps a reflog")
		case !exitedWith(err, 1):
			return nil, notListed(err)
		}
	}
	return born, nil
}

// adminHeads names the HEAD of each admin dir as git reads it from any
// worktree: worktrees/<id>/HEAD.
func adminHeads(ids []string) []string {
	heads := make([]string, 0, len(ids))
	for _, id := range ids {
		heads = append(heads, "worktrees/"+id+"/HEAD")
	}
	return heads
}

// prunedIDs returns the admin dirs `git worktree prune` with the expire
// words would delete, as a dry run of it on the common dir names them on
// stderr in the C locale: `Removing worktrees/<id>: <reason>`. A line of
// any other shape fails the read.
func prunedIDs(ctx context.Context, common string, expire []string) ([]string, error) {
	args := slices.Concat([]string{"worktree", "prune", "-n", "-v"}, expire)
	_, stderr, err := commonGit(ctx, common, nil, args...)
	if err != nil {
		return nil, notListed(err)
	}
	var ids []string
	for line := range strings.Lines(string(stderr)) {
		rest, ok := strings.CutPrefix(line, "Removing worktrees/")
		id, _, found := strings.Cut(rest, ": ")
		if !ok || !found || id == "" {
			return nil, notListed("git worktree prune -n printed " + strconv.Quote(line))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// commonDir returns the absolute common dir of the repository at dir.
func commonDir(ctx context.Context, dir string) (string, error) {
	out, err := runProofGit(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", notListed(err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// adminIDs returns the names of the linked worktrees' admin dirs, the
// directories in <common>/worktrees. A repository without linked worktrees
// has none.
func adminIDs(common string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(common, "worktrees"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, notListed(err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// adminPath returns the path git lists the linked worktree whose admin dir
// is admin at: its gitdir file, trimmed, without the /.git it ends in, and
// taken from the admin dir when relative (get_linked_worktree). An admin
// dir with no such file names none, and git does not list it.
func adminPath(admin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	p := strings.TrimSuffix(strings.TrimRightFunc(string(b), unicode.IsSpace), "/.git")
	if err != nil || p == "" {
		return "", false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(admin, p)
	}
	return p, true
}

// commonGit runs one read-only git command on the common dir itself
// (execGit), with input on its stdin, fsmonitor off and the C locale, so
// stderr holds git's own words. It returns stdout and stderr.
func commonGit(ctx context.Context, common string, input []byte, args ...string) (stdout, stderr []byte, err error) {
	return execGit(ctx, gitExec{
		dir:   common,
		pre:   []string{"--git-dir=" + common, "-c", "core.fsmonitor=false"},
		env:   []string{"LC_ALL=C"},
		stdin: input,
	}, args...)
}
