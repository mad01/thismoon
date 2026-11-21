package guard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// This file holds the file-discard search for work git status cannot
// show: a worktree or a repository inside a folder a delete reaches into,
// a nested one git lists as untracked or one inside an ignored folder git
// does not look into, and each repository's own state.

// hiddenDir is a folder git status lists as one entry and does not look
// into, relative to the top: a nested repository (untracked) or an
// ignored folder.
type hiddenDir struct {
	rel    string
	nested bool
}

// code is the status code git lists the folder under.
func (h hiddenDir) code() string {
	if h.nested {
		return "??"
	}
	return "!!"
}

// hiddenWork looks under the folders the command reaches into for work git
// status does not list. A worktree of the repository (git worktree list)
// denies, which status leaves out when an ignored folder holds it. Then
// each nested repository and each ignored folder a drop loses is searched
// with that drop (scanHidden).
func (j *discardJudge) hiddenWork(ctx context.Context, r *repoDrops, hidden []hiddenDir) *fileLoss {
	var dirs []treeDrop
	for _, d := range r.drops {
		if d.dir {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	if loss := j.worktreeIn(ctx, r, dirs); loss != nil {
		return loss
	}
	for _, h := range hidden {
		for _, d := range dirs {
			root := innerOf(d.rel, h.rel)
			if root == "" || !d.filter.loses(r.top, d.rel, h.code(), h.rel+"/") {
				continue
			}
			if loss := j.scanHidden(ctx, r, d, root, h); loss != nil {
				return loss
			}
		}
	}
	return nil
}

// worktreeIn is the loss for a work tree of the repository at or under a
// folder a drop reaches into.
func (j *discardJudge) worktreeIn(ctx context.Context, r *repoDrops, dirs []treeDrop) *fileLoss {
	trees, err := j.g.worktrees(ctx, r.top)
	if err != nil {
		return &fileLoss{call: dirs[0].call, why: notListed(err).Error()}
	}
	for _, wt := range trees {
		if wt = physical(wt); strings.EqualFold(wt, r.top) {
			continue
		}
		for _, d := range dirs {
			if pathUnder(wt, filepath.Join(r.top, d.rel)) && exists(wt) {
				return &fileLoss{call: d.call, why: "this deletes `" + wt + "`, a work tree of " +
					r.top, instead: worktreeRemoveHint(wt)}
			}
		}
	}
	return nil
}

// innerOf returns the deeper of two paths when one holds the other, "" when
// neither does.
func innerOf(a, b string) string {
	switch {
	case holds(a, b):
		return b
	case holds(b, a):
		return a
	}
	return ""
}

// walkKey is one folder searched for one drop's filter; each is searched
// once per call.
type walkKey struct {
	root   string
	filter *pathFilter
}

// scanHidden searches the folder at root (relative to the top), at or
// below the hidden folder h, for the drop d: each repository it holds is
// judged (foundRepo, and keepsEach for those the drop takes whole), and
// when the command cuts what the feeder prints, each path the feeder
// prints there must reach it whole. Inside a bare repository, a drop of
// part of it takes git's own data.
func (j *discardJudge) scanHidden(ctx context.Context, r *repoDrops, d treeDrop, root string, h hiddenDir) *fileLoss {
	key := walkKey{root: filepath.Join(r.top, root), filter: d.filter}
	if j.walked[key] {
		return nil
	}
	j.walked[key] = true
	where := "`" + key.root + "`"
	if !h.nested {
		where = "the ignored `" + filepath.Join(r.top, h.rel) + "`"
		if bare := bareAbove(key.root, filepath.Join(r.top, h.rel)); bare != "" {
			return &fileLoss{call: d.call, why: "this deletes `" + key.root + "`, part of the bare " +
				"repository `" + bare + "`, which is git's own data"}
		}
	}
	var loss *fileLoss
	var whole []string
	mode := walkMode{skipGit: true, packages: !d.filter.splits()}
	err := j.walkDirs(key.root, mode, func(dir string, entries []fs.DirEntry) walkNext {
		if kind := repoKindOf(entries); kind != notRepo {
			var next walkNext
			if loss, next = j.foundRepo(r, d, dir, kind, &whole); next != walkOn {
				return next
			}
		}
		if loss = d.walkCuts(r.top, dir, entries); loss != nil {
			return walkStop
		}
		return walkOn
	})
	switch {
	case errors.Is(err, errWalkTooBig):
		return &fileLoss{call: d.call, why: fmt.Sprintf("belt could not check every folder in %s "+
			"for a git repository: it stops at %d folders and the time budget", where, j.g.scanDirs)}
	case err != nil:
		return &fileLoss{call: d.call, why: fmt.Sprintf("belt could not check %s for a git "+
			"repository (%v)", where, err)}
	case loss != nil:
		return loss
	}
	return j.keepsEach(ctx, d, whole, where)
}

// keepsLimit bounds the keeps checks one search runs at once; each is a
// few git calls.
const keepsLimit = 4

// keepsEach runs keeps on each repository the drop d takes whole, at most
// keepsLimit at once, and returns the loss for the first one in walk order
// that a remote does not hold.
func (j *discardJudge) keepsEach(ctx context.Context, d treeDrop, repos []string, where string) *fileLoss {
	errs := make([]error, len(repos))
	var g errgroup.Group
	g.SetLimit(keepsLimit)
	for i, p := range repos {
		g.Go(func() error {
			errs[i] = j.g.keeps(ctx, p)
			return nil
		})
	}
	_ = g.Wait()
	for i, err := range errs {
		if err == nil {
			continue
		}
		what := "a nested repository"
		if !strings.HasPrefix(where, "`") {
			what = "a git repository inside " + where
		}
		instead := worktreeRemoveHint(repos[i])
		if live := (liveWorktreeErr{}); errors.As(err, &live) {
			instead = live.instead()
		}
		return &fileLoss{call: d.call, why: "this deletes `" + repos[i] + "`, " + what + ", and " +
			err.Error(), instead: instead}
	}
	return nil
}

// walkCuts is the loss for an entry of dir whose printed path the command
// cuts into other paths (treeDrop.cutsAt), nil when the command cuts
// nothing.
func (d treeDrop) walkCuts(top, dir string, entries []fs.DirEntry) *fileLoss {
	if !d.filter.splits() {
		return nil
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(dir, top), "/")
	for _, e := range entries {
		p := e.Name()
		if rel != "" {
			p = rel + "/" + p
		}
		if loss := d.cutsAt(p, string(entryType(e)), false); loss != nil {
			return loss
		}
	}
	return nil
}

// foundRepo judges the repository at p that the search under the drop d
// found. A drop that takes it whole (takesWhole) loses it unless keeps
// finds all of it on a remote, so it joins whole for keepsEach, and the
// search goes on inside a work tree, where one may sit in its ignored
// folders. A narrowed drop takes only the paths its filter hands: inside a
// bare repository those are git's own data, and a work tree is queued as
// one of its own (nestedTop), so the search stays out of it.
func (j *discardJudge) foundRepo(r *repoDrops, d treeDrop, p string, kind repoKind, whole *[]string) (*fileLoss, walkNext) {
	rel := strings.TrimPrefix(strings.TrimPrefix(p, r.top), "/")
	if d.takesWhole(rel) {
		*whole = append(*whole, p)
		if kind == bareRepo {
			return nil, walkPrune
		}
		return nil, walkOn
	}
	if kind == bareRepo {
		if loss := j.gitDataLoss(d.call, p, d.filter); loss != nil {
			return loss, walkStop
		}
		return nil, walkPrune
	}
	prefix := filepath.Join(d.prefix, below(d.rel, rel))
	if loss := cutLoss(d.call, d.filter, prefix); loss != nil {
		return loss, walkStop
	}
	return j.nestedTop(d.call, d.filter, p, prefix), walkPrune
}

// takesWhole reports whether the drop deletes the folder at rel (relative
// to the top) with all it holds, .git included: no filter; a filter with
// no tests whose feeder hands paths in .git (grep -r) or names a nested
// repository's folder as one path (git ls-files -o) to rm -r; or rm -r of
// a match on the way down to it.
func (d treeDrop) takesWhole(rel string) bool {
	f := d.filter
	switch {
	case f == nil:
		return true
	case f.nestedWhole && f.tree:
		return true
	case !f.hasTests():
		return !f.skipsGitData()
	case !f.tree:
		return false
	}
	for part := range strings.SplitSeq(below(d.rel, rel), "/") {
		if part != "" && f.matchEntry(part, 'd') {
			return true
		}
	}
	return false
}

// nestedTop queues the work tree at top, a repository inside the drop that
// a filter narrows, with that filter, so git status in it judges what the
// filter hands. Its .git is judged first, as for any top (inWorkTree).
// prefix is the path from the drop's root to top, which the paths the
// feeder prints there start with.
func (j *discardJudge) nestedTop(call *lossCall, filter *pathFilter, top, prefix string) *fileLoss {
	switch {
	case filter.hasTests():
		if loss := j.gitDataLoss(call, filepath.Join(top, ".git"), filter); loss != nil {
			return loss
		}
	case !filter.skipsGitData():
		return &fileLoss{call: call, why: "the paths it deletes may lie in `" + top +
			"/.git`, which is git's own data"}
	}
	j.queue(top, treeDrop{rel: ".", call: call, dir: true, filter: filter, prefix: prefix})
	return nil
}

// repoKind is what a folder is to git: no repository, one with a work
// tree (a .git entry, a folder or a file) or a bare one.
type repoKind int

const (
	notRepo repoKind = iota
	workRepo
	bareRepo
)

// repoKindOf reports what a folder is to git by its entries: a work tree when
// it holds .git, bare when it holds HEAD, objects and refs.
func repoKindOf(entries []fs.DirEntry) repoKind {
	head, objects, refs := false, false, false
	for _, e := range entries {
		switch name := e.Name(); {
		case strings.EqualFold(name, ".git"):
			return workRepo
		case name == "HEAD":
			head = !e.IsDir()
		case name == "objects":
			objects = e.IsDir()
		case name == "refs":
			refs = e.IsDir()
		}
	}
	if head && objects && refs {
		return bareRepo
	}
	return notRepo
}

// bareAbove returns the bare repository a folder above p lies in, up to
// and with stop, "" for none.
func bareAbove(p, stop string) string {
	for d := filepath.Dir(p); d == stop || strings.HasPrefix(d, stop+"/"); d = filepath.Dir(d) {
		if isDir(filepath.Join(d, "objects")) && isDir(filepath.Join(d, "refs")) &&
			exists(filepath.Join(d, "HEAD")) && !isDir(filepath.Join(d, "HEAD")) {
			return d
		}
		if d == stop {
			break
		}
	}
	return ""
}

// repoKeepsAll returns nil when deleting the repository at repo, which a
// drop takes whole, loses nothing a trusted remote holds. belt does not
// prove such a repository safe in detail; it passes only on a short list,
// and every other one denies:
//   - no linked worktree of it still has its folder (liveWorktree);
//   - its work tree's git status lists no change, untracked file or
//     submodule change (ignored files count as build output), no file a
//     flag hides from git status differs from the index, and no rebase or
//     merge stopped with an autostash (treeClean);
//   - every commit git keeps there is on a remote branch of a remote git
//     reaches across the network (commitsKept, trustOf).
//
// SwiftPM's cache of a package passes whole (swiftCache), and a SwiftPM
// checkout counts its cache's refs as a remote's. git runs on that
// repository alone, never on one above it. Each submodule's repository it
// keeps goes with it, so each is judged too (modulesKept).
func repoKeepsAll(ctx context.Context, repo string) error {
	gitDir, bare := filepath.Join(repo, ".git"), false
	if !exists(gitDir) {
		gitDir, bare = repo, true
	}
	at := []string{"--git-dir=" + gitDir}
	if !bare {
		at = append(at, "--work-tree="+repo)
	}
	trees, err := worktreeAdmins(gitDir)
	if err != nil {
		return err
	}
	if err := liveWorktree(repo, trees); err != nil {
		return err
	}
	c, err := readRemotes(ctx, repo, at)
	if err != nil {
		return err
	}
	if bare && swiftCache(repo, c) {
		return nil
	}
	if !bare {
		if err := treeClean(ctx, repo, at); err != nil {
			return err
		}
	}
	return historyKept(ctx, repo, gitDir, at, trees, trustOf(ctx, repo, bare, c))
}

// historyKept returns nil when every commit the repository git reads with
// at keeps is on a ref trust holds (commitsKept, with the refs of its
// linked worktrees trees) and each submodule's repository its git folder
// gitDir keeps loses nothing (modulesKept).
func historyKept(ctx context.Context, repo, gitDir string, at []string, trees []linkedTree, trust remoteTrust) error {
	linked, err := linkedRefs(ctx, repo, trees)
	if err != nil {
		return err
	}
	if err := commitsKept(ctx, repo, at, linked, trust); err != nil {
		return err
	}
	return modulesKept(ctx, repo, gitDir, trees)
}

// modulesKept returns nil when each submodule's repository git keeps in
// the git folder gitDir loses nothing (moduleKeepsAll). git keeps one in
// modules/<name>, or for a linked worktree in that worktree's admin folder,
// and one inside a submodule in that submodule's own. The walk for
// repositories does not look into a git folder, and a submodule that
// git submodule deinit or git rm took out of its work tree is found
// nowhere else.
func modulesKept(ctx context.Context, repo, gitDir string, trees []linkedTree) error {
	roots := []string{filepath.Join(gitDir, "modules")}
	for _, t := range trees {
		roots = append(roots, filepath.Join(t.admin, "modules"))
	}
	mods, err := moduleDirs(ctx, roots)
	if err != nil {
		return fmt.Errorf("belt could not read the submodules it keeps (%v)", err)
	}
	for _, m := range mods {
		if err := moduleKeepsAll(ctx, repo, m); err != nil {
			return fmt.Errorf("it keeps a submodule's repository at `%s`, which goes with it: %w", m, err)
		}
	}
	return nil
}

// moduleDirs lists the git folders under roots, each a modules folder: a
// folder that holds HEAD, objects and refs is one, and the search goes on
// into every other folder, since a submodule's name may hold slashes. A
// missing root holds none, and no symlink is followed.
func moduleDirs(ctx context.Context, roots []string) ([]string, error) {
	var mods []string
	for dirs := slices.Clone(roots); len(dirs) > 0; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dir := dirs[0]
		dirs = dirs[1:]
		entries, err := os.ReadDir(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
			continue
		case err != nil:
			return nil, err
		case repoKindOf(entries) == bareRepo:
			mods = append(mods, dir)
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(dir, e.Name()))
			}
		}
	}
	return mods, nil
}

// moduleKeepsAll returns nil when deleting the submodule's repository at
// gitDir loses nothing, by the rules for a repository with no work tree:
// no live linked worktree, an index that holds what HEAD holds (indexKept),
// no autostash, and every commit on a trusted remote branch. A work tree
// it still has lies in the superproject's, where the walk judges it. git
// reads gitDir itself as the work tree, so a core.worktree whose folder is
// gone, as after git rm, does not stop it; nothing run here reads a work
// tree.
func moduleKeepsAll(ctx context.Context, repo, gitDir string) error {
	at := []string{"--git-dir=" + gitDir, "--work-tree=" + gitDir}
	trees, err := worktreeAdmins(gitDir)
	if err != nil {
		return err
	}
	if err := liveWorktree(gitDir, trees); err != nil {
		return err
	}
	c, err := readRemotes(ctx, repo, at)
	if err != nil {
		return err
	}
	if err := indexKept(ctx, repo, gitDir, at); err != nil {
		return err
	}
	if err := autostashLost(ctx, repo, at, "it"); err != nil {
		return err
	}
	return historyKept(ctx, repo, gitDir, at, trees, trustOf(ctx, gitDir, true, c))
}

// indexKept returns nil when the repository at gitDir, which git reads with
// at, has no index or one that holds what HEAD holds: a submodule that git
// submodule deinit -f emptied keeps what was staged there, and no commit
// holds it.
func indexKept(ctx context.Context, repo, gitDir string, at []string) error {
	if !exists(filepath.Join(gitDir, "index")) {
		return nil
	}
	_, code, err := runProofGitCode(ctx, repo, nil, slices.Concat(at, []string{"diff-index", "--cached",
		"--quiet", "HEAD", "--"})...)
	switch {
	case err != nil:
		return fmt.Errorf("belt could not read its index (%v)", err)
	case code == 1:
		return errors.New("its index holds changes no commit has")
	case code != 0:
		return errors.New("belt could not read its index")
	}
	return nil
}

// treeClean returns nil when git status lists nothing in the work tree git
// reads with at, no file a flag hides from git status differs from the
// index (flaggedEdits), and it stopped no rebase or merge with an
// autostash.
func treeClean(ctx context.Context, repo string, at []string) error {
	out, err := runProofGit(ctx, repo, slices.Concat(at, []string{"-c", "core.fsmonitor=false", "status",
		"--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=none"})...)
	switch {
	case err != nil:
		return fmt.Errorf("belt could not read its git status (%v)", err)
	case len(out) > 0:
		return errors.New("its work tree holds changes no commit has")
	}
	edits, err := flaggedEdits(ctx, repo, at, nil)
	switch {
	case err != nil:
		return fmt.Errorf("belt could not read its index (%v)", err)
	case len(edits) > 0:
		return fmt.Errorf("its work tree holds changes no commit has: `%s` is marked assume-unchanged "+
			"or skip-worktree, so git status does not show them", edits[0])
	}
	return autostashLost(ctx, repo, at, "it")
}

// autostashLost returns an error when the worktree git reads with at
// stopped a rebase or a merge it started with --autostash: git keeps the
// changes it stashed for it in a commit no ref holds, in rebase-merge or
// rebase-apply's autostash file or in MERGE_AUTOSTASH, which git reads for
// either ref backend. who names the worktree in the error.
func autostashLost(ctx context.Context, repo string, at []string, who string) error {
	out, code, err := runProofGitCode(ctx, repo, nil, slices.Concat(at, []string{"rev-parse",
		"--git-path", "rebase-merge/autostash", "--git-path", "rebase-apply/autostash",
		"-q", "--verify", "MERGE_AUTOSTASH"})...)
	paths := strings.Fields(string(out))
	switch {
	case err != nil:
		return fmt.Errorf("belt could not read whether %s stopped a rebase or merge (%v)", who, err)
	case code > 1 || len(paths) < 2:
		return fmt.Errorf("belt could not read whether %s stopped a rebase or merge", who)
	case code == 0:
		return fmt.Errorf("%s stopped a merge with changes git stashed for it (MERGE_AUTOSTASH), "+
			"which no commit on a remote holds", who)
	}
	for _, p := range paths[:2] {
		if !filepath.IsAbs(p) {
			p = filepath.Join(repo, p)
		}
		if exists(p) {
			return fmt.Errorf("%s stopped a rebase with changes git stashed for it (%s), which no "+
				"commit on a remote holds", who, p)
		}
	}
	return nil
}

// linkedTree is a linked worktree of a repository: the id of its admin
// folder, that folder, and the worktree's own folder, "" when git's record
// of it cannot be read.
type linkedTree struct {
	id, admin, dir string
}

// worktreeAdmins lists the linked worktrees of the repository whose git
// folder is gitDir. Each admin folder's gitdir file names the worktree's
// .git file, relative to the admin folder when worktree.useRelativePaths
// wrote it. A worktree whose folder is gone is listed too. A .git file has
// no worktrees folder of its own.
func worktreeAdmins(gitDir string) ([]linkedTree, error) {
	admin := filepath.Join(gitDir, "worktrees")
	entries, err := os.ReadDir(admin)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("belt could not read its worktrees (%v)", err)
	}
	var trees []linkedTree
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t := linkedTree{id: e.Name(), admin: filepath.Join(admin, e.Name())}
		if b, err := os.ReadFile(filepath.Join(t.admin, "gitdir")); err == nil {
			p := strings.TrimRight(string(b), "\r\n")
			if !filepath.IsAbs(p) {
				p = filepath.Join(t.admin, p)
			}
			t.dir = filepath.Dir(p)
		}
		trees = append(trees, t)
	}
	return trees, nil
}

// liveWorktreeErr is the loss of a linked worktree whose folder is still
// there: git keeps its index, HEAD and refs in the repository a drop
// takes, so the worktree breaks and loses what it staged, and belt reads
// no git status there.
type liveWorktreeErr struct{ repo, dir string }

func (e liveWorktreeErr) Error() string {
	return "its linked worktree `" + e.dir + "` still exists, and git keeps that worktree's index and HEAD there"
}

// instead offers git's own command, which refuses a worktree with changes.
func (e liveWorktreeErr) instead() string {
	return "Remove that worktree first with `git -C " + e.repo + " worktree remove " + e.dir +
		"`, which refuses a worktree with uncommitted changes."
}

// liveWorktree returns the loss for the first linked worktree in trees of
// the repository at repo whose folder is still there, or whose folder belt
// cannot read.
func liveWorktree(repo string, trees []linkedTree) error {
	for _, t := range trees {
		switch {
		case t.dir == "":
			return fmt.Errorf("belt could not read where its linked worktree %s lies", t.id)
		case exists(t.dir):
			return liveWorktreeErr{repo: repo, dir: t.dir}
		}
	}
	return nil
}

// commitsKept returns nil when every commit git keeps in the repository is
// on a ref trust holds: a remote branch of a remote across the network
// (trust.globs), or for a SwiftPM checkout a ref of its cache. git keeps
// what each ref, each worktree's HEAD and each reflog in each worktree
// holds, for the files and the reftable backend alike, and git itself
// lists them: --all reads every ref and every worktree's HEAD, but only
// the refs the main worktree keeps for itself, so each linked worktree's
// own refs are read apart (linked, from linkedRefs), and a walk of the logs
// of all of those lists their commits (ownLogs). One rev-list then looks
// for a commit no trusted ref holds. It names HEAD too, so a HEAD with no
// commit, as in an empty repository, fails it. Only on a hit are the cache's
// refs read, as ^ lines with --ignore-missing. Lists of commits go to git
// on stdin, so no count of them is too long for a command line.
func commitsKept(ctx context.Context, repo string, at []string, linked worktreeRefs, trust remoteTrust) error {
	logs, err := ownLogs(ctx, repo, at, trust.globs, linked.names)
	if err != nil {
		return err
	}
	revs := []string{"--all", "--not"}
	for _, g := range trust.globs {
		revs = append(revs, "--glob="+g)
	}
	in := slices.Concat(linked.tips, logs)
	commit, err := firstCommit(ctx, repo, at, slices.Concat([]string{"HEAD"}, revs), in)
	if err != nil || commit == "" || trust.cache == "" {
		return lostCommit(commit, err)
	}
	tips, err := cacheTips(ctx, repo, trust.cache)
	if err != nil {
		return err
	}
	for _, tip := range tips {
		in = append(in, "^"+tip)
	}
	commit, err = firstCommit(ctx, repo, at, slices.Concat([]string{"--ignore-missing"}, revs), in)
	return lostCommit(commit, err)
}

// worktreeRefs are the refs the linked worktrees of a repository keep for
// themselves besides HEAD: tips holds the object each names, and names
// each one as the main worktree names it, worktrees/<id>/<ref>.
type worktreeRefs struct {
	tips, names []string
}

// perWorktreeRefs are the folders of refs git keeps for each worktree
// apart, as git-worktree(1) lists them; every other ref is shared.
var perWorktreeRefs = []string{"refs/bisect/", "refs/worktree/", "refs/rewritten/"}

// linkedRefsLimit bounds the worktrees linkedRefs reads at once; each is
// one short git call.
const linkedRefsLimit = 8

// linkedRefs lists the refs each linked worktree in trees keeps for itself,
// as for-each-ref reads them with the worktree's admin folder as the git
// dir, at most linkedRefsLimit at once, and denies one that stopped a
// rebase or merge with an autostash. That works for either ref backend and
// for a worktree whose folder is gone, the only kind left once
// liveWorktree passed.
func linkedRefs(ctx context.Context, repo string, trees []linkedTree) (worktreeRefs, error) {
	each := make([]worktreeRefs, len(trees))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(linkedRefsLimit)
	for i, t := range trees {
		g.Go(func() (err error) {
			if each[i], err = ownRefs(gctx, repo, t); err != nil {
				return err
			}
			return autostashLost(gctx, repo, []string{"--git-dir=" + t.admin}, "its worktree "+t.id)
		})
	}
	if err := g.Wait(); err != nil {
		return worktreeRefs{}, err
	}
	var refs worktreeRefs
	for _, r := range each {
		refs.tips = append(refs.tips, r.tips...)
		refs.names = append(refs.names, r.names...)
	}
	return refs, nil
}

// ownRefs lists the refs the linked worktree t keeps for itself
// (perWorktreeRefs).
func ownRefs(ctx context.Context, repo string, t linkedTree) (worktreeRefs, error) {
	out, err := runProofGit(ctx, repo, slices.Concat([]string{"--git-dir=" + t.admin,
		"for-each-ref", "--format=%(objectname) %(refname)"}, perWorktreeRefs)...)
	if err != nil {
		return worktreeRefs{}, fmt.Errorf("belt could not read the refs of its worktree %s (%v)", t.id, err)
	}
	var refs worktreeRefs
	for line := range strings.Lines(string(out)) {
		tip, name, ok := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		if !ok {
			return worktreeRefs{}, fmt.Errorf("belt could not read the refs of its worktree %s", t.id)
		}
		refs.tips = append(refs.tips, tip)
		refs.names = append(refs.names, "worktrees/"+t.id+"/"+name)
	}
	return refs, nil
}

// ownLogs lists the commits the reflogs git keeps name (rev-list
// --walk-reflogs): those of each ref and each worktree's HEAD (--all), and
// of each linked worktree's own refs (names, which keep none unless
// core.logAllRefUpdates is always). It leaves out the logs of the refs a
// trusted remote fills (trusted, globs as --glob reads them): a fetch from
// that remote or a push to it writes each entry, so the remote held each
// commit they name when git wrote it. After a fetch of a force-pushed
// branch, the log keeps a commit the remote no longer has. A ref written
// there by hand, or fetched from the repository itself, breaks that; the
// file-discard docs list it as a gap. The logs of every other remote's
// branches count as the repository's own.
func ownLogs(ctx context.Context, repo string, at, trusted, names []string) ([]string, error) {
	args := slices.Concat(at, []string{"rev-list", "--walk-reflogs", "--stdin"})
	for _, g := range trusted {
		args = append(args, "--exclude="+g)
	}
	out, err := runProofGitInput(ctx, repo, names, append(args, "--all")...)
	if err != nil {
		return nil, fmt.Errorf("belt could not read its reflogs (%v)", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range strings.Fields(string(out)) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// lostCommit is the error for a commit commitsKept finds on no trusted
// ref, err itself when belt could not tell, nil for no commit.
func lostCommit(commit string, err error) error {
	if err != nil || commit == "" {
		return err
	}
	return fmt.Errorf("its commit %.12s is on no remote branch of a remote across the network "+
		"(a remote in a local folder does not count)", commit)
}

// firstCommit runs rev-list -n 1 over revs in the repository at repo, with
// each of in as a line on its stdin (a commit, or ^ and one to leave out),
// and returns the commit it prints, "" for none. git reads a line on stdin
// as a commit to list whatever --not stands before it.
func firstCommit(ctx context.Context, repo string, at, revs, in []string) (string, error) {
	out, err := runProofGitInput(ctx, repo, in, slices.Concat(at, []string{"rev-list", "-n", "1", "--stdin"}, revs)...)
	if err != nil {
		return "", fmt.Errorf("belt could not list its commits, it may have none (%v)", err)
	}
	return strings.TrimSpace(string(out)), nil
}
