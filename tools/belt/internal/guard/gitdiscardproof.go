package guard

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// proofBudget bounds one nothing-lost proof. Every git call and file read
// it makes shares this deadline, so a hook never waits longer than this on
// it; a proof that runs out refuses the discard as before.
const proofBudget = 2 * time.Second

// maxProofPaths caps the paths one proof checks. A discard that drops more
// refuses as before rather than hash a whole tree inside a hook.
const maxProofPaths = 256

// proofPipeGrace bounds how long a proof's git call waits for its stdout
// pipe once git has exited or been killed.
const proofPipeGrace = 100 * time.Millisecond

// originHead names origin's default branch. It counts only while it points
// under originRefs: a symbolic ref any command can aim at a local branch
// would otherwise make a throwaway branch count as shared.
const (
	originHead = "refs/remotes/origin/HEAD"
	originRefs = "refs/remotes/origin/"
)

// gitlinkMode is the tree mode of a submodule entry.
const gitlinkMode = "160000"

// prover proves that a discard loses nothing: every file version it drops
// equals a committed one at the same path in HEAD or a shared commit
// (proofRepo.shared), or what the command itself writes there, and a hard
// reset strands no commit.
type prover struct {
	maxPaths int
	budget   time.Duration
}

// newProver returns the prover with the hook's bounds.
func newProver() prover {
	return prover{maxPaths: maxProofPaths, budget: proofBudget}
}

// treeEntry is one entry of a tree or the index. size is -1 when git did
// not report it (an index entry).
type treeEntry struct {
	mode  string
	kind  string
	oid   string
	size  int64
	stage string
}

// drop is one path a discard rewrites or deletes.
type drop struct {
	path string
	// written is what the command leaves at path; zero when it deletes the
	// file or writes nothing there.
	written treeEntry
	// staged is the index version the command drops; zero when it keeps the
	// index or the index has no entry.
	staged treeEntry
}

// proofRepo runs the proof's git calls under one deadline.
type proofRepo struct {
	ctx context.Context
	// dir is where the command runs; its pathspecs are relative to it.
	dir string
	// top is the work tree root; every listed path is relative to it.
	top string
	// maxPaths caps the paths the proof checks (prover.maxPaths).
	maxPaths int
	// shared are the commits besides HEAD whose files count as safe: the
	// upstream and origin's default branch (sharedCommits).
	shared []string
}

// notListed wraps the reason a proof could not list what a discard drops.
func notListed(cause any) error {
	return fmt.Errorf("belt could not list what it drops (%v)", cause)
}

// prove returns nil when the discard c loses nothing, and otherwise the
// first loss or the reason the losses could not be listed exactly.
// A global option other than -C or a config override is refused: the proof
// runs git with --literal-pathspecs and its own config, so an option like
// --icase-pathspecs would make it list other paths than the command drops.
func (p prover) prove(c gitInvocation) error {
	spec, err := readDiscardSpec(c)
	switch {
	case errors.Is(err, errNoProof):
		return err
	case err != nil:
		return notListed(err)
	case gitEnvRedirected():
		return notListed("a GIT_DIR-style variable is set")
	case c.configured || c.optioned:
		return notListed("a global git option")
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.budget)
	defer cancel()
	out, err := runProofGit(ctx, c.dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return notListed(err)
	}
	top := strings.TrimSpace(string(out))
	r := proofRepo{ctx: ctx, dir: c.dir, top: top, maxPaths: p.maxPaths}
	r.shared = r.sharedCommits()
	drops, err := r.drops(spec)
	if err != nil {
		return err
	}
	drops = withAncestors(r.top, drops)
	if err := r.capped(len(drops)); err != nil {
		return err
	}
	return r.checkDrops(drops)
}

// drops lists every path spec rewrites or deletes, with what it writes there
// and the index version it drops. A restore or reset over an index entry
// git status does not look at is refused first (hiddenEntries).
func (r proofRepo) drops(spec discardSpec) ([]drop, error) {
	switch spec.kind {
	case pathRestore:
		if err := r.hiddenEntries(r.dir, spec.pathspecs); err != nil {
			return nil, err
		}
		return r.restoreDrops(spec)
	case untrackedClean:
		return r.cleanDrops(spec)
	case hardReset, mergeReset, treeRead:
		if err := r.hiddenEntries(r.top, nil); err != nil {
			return nil, err
		}
		if err := r.recursesIntoSubmodules(); err != nil {
			return nil, err
		}
		return r.resetDrops(spec)
	case trackedRemoval:
		return r.removedTracked(spec)
	case forcedMove:
		return r.moveDrops(spec)
	case patchApply:
		return r.applyDrops(spec)
	case worktreeRemoval:
		if err := r.hiddenEntries(r.top, nil); err != nil {
			return nil, err
		}
		return r.worktreeDrops()
	}
	return nil, notListed(errNotRead)
}

// recursesIntoSubmodules refuses a hard reset that submodule.recurse carries
// into a submodule: the reset then rewrites each submodule worktree, which
// the proof does not look into. git config reads every scope git does, and a
// value git cannot read as a boolean refuses too.
func (r proofRepo) recursesIntoSubmodules() error {
	out, err := runProofGit(
		r.ctx, r.top, "config", "--type=bool", "--default=false", "--get", "submodule.recurse",
	)
	if err != nil {
		return notListed(err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return nil
	}
	index, err := r.indexEntries(r.top, nil)
	if err != nil {
		return err
	}
	for _, p := range mapKeys(index) {
		if index[p].mode == gitlinkMode {
			return notListed("submodule.recurse is on, so the reset rewrites the submodule at " + p)
		}
	}
	return nil
}

// hiddenEntries refuses a discard over an index entry marked assume-unchanged
// (a lowercase `ls-files -v` tag) or skip-worktree (S): git status skips
// both, so their worktree copies may hold edits no listing shows. Only H
// (cached) and M (unmerged, refused later as a conflict) pass. No pathspecs
// means the whole index.
func (r proofRepo) hiddenEntries(dir string, pathspecs []string) error {
	args := append([]string{"ls-files", "-v", "-z", "--full-name", "--"}, pathspecs...)
	out, err := runProofGit(r.ctx, dir, args...)
	if err != nil {
		return notListed(err)
	}
	for _, f := range nulFields(out) {
		if tag, p, _ := strings.Cut(f, " "); tag != "H" && tag != "M" {
			return notListed("`" + p + "` is marked assume-unchanged or skip-worktree")
		}
	}
	return nil
}

// restoreDrops lists the paths a checkout or restore writes. From the index
// that is every index path the pathspecs match. From a tree it is every path
// the tree has there, plus, for restore, the index paths the tree lacks,
// which it deletes.
func (r proofRepo) restoreDrops(spec discardSpec) ([]drop, error) {
	index, err := r.indexEntries(r.dir, spec.pathspecs)
	if err != nil {
		return nil, err
	}
	if spec.source == "" {
		drops := make([]drop, 0, len(index))
		for p, e := range index {
			drops = append(drops, drop{path: p, written: e})
		}
		return drops, nil
	}
	tree, err := r.treeEntries(r.dir, spec.source, true, spec.pathspecs)
	if err != nil {
		return nil, err
	}
	paths := mapKeys(tree)
	if !spec.overlay {
		paths = appendNew(paths, mapKeys(index))
	}
	drops := make([]drop, 0, len(paths))
	for _, p := range paths {
		d := drop{path: p, written: tree[p]}
		if spec.staged {
			d.staged = index[p]
		}
		drops = append(drops, d)
	}
	return drops, nil
}

// cleanDrops lists the untracked files a clean deletes, file by file. A
// clean without -d keeps untracked directories, so the list can be longer
// than what goes, never shorter. A nested repository lists as a directory,
// which the proof cannot see into.
func (r proofRepo) cleanDrops(spec discardSpec) ([]drop, error) {
	args := []string{"ls-files", "-z", "--others", "--full-name"}
	switch spec.scope {
	case cleanUntracked:
		args = append(args, "--exclude-standard")
	case cleanOnlyIgnored:
		args = append(args, "--ignored", "--exclude-standard")
	case cleanWithIgnored:
	}
	out, err := runProofGit(r.ctx, r.dir, append(append(args, "--"), spec.pathspecs...)...)
	if err != nil {
		return nil, notListed(err)
	}
	var drops []drop
	for _, p := range nulFields(out) {
		if strings.HasSuffix(p, "/") {
			return nil, notListed("a nested repository at " + p)
		}
		drops = append(drops, drop{path: p})
	}
	return drops, nil
}

// resetDrops lists what `reset --hard <target>` rewrites: every tracked
// path git status reports changed, plus, when the target is another commit,
// each file it adds over an untracked or ignored file and each untracked
// file standing where it needs a directory. It refuses a reset that would
// strand commits neither a shared commit nor the target holds. A read-tree
// rewrites the same paths and leaves the branch alone. A merge reset drops
// what a hard one does less what it keeps (mergeKeeps), and nothing when git
// refuses it.
func (r proofRepo) resetDrops(spec discardSpec) ([]drop, error) {
	head, err := r.commit("HEAD")
	if err != nil {
		return nil, err
	}
	target, err := r.commit(spec.source)
	if err != nil {
		return nil, err
	}
	paths, unstaged, err := r.changedTracked()
	if err != nil {
		return nil, err
	}
	if target != head {
		if spec.kind == hardReset {
			if err := r.keepsCommits(head, target); err != nil {
				return nil, err
			}
		}
		added, err := r.addedFiles(head, target)
		if err != nil {
			return nil, err
		}
		paths = appendNew(paths, inTheWay(r.top, added))
	}
	drops, err := r.rewritten(target, paths)
	if err != nil || spec.kind != mergeReset {
		return drops, err
	}
	drops, refused := mergeKeeps(drops, unstaged)
	switch {
	case refused:
		return nil, nil
	case target != head:
		if err := r.keepsCommits(head, target); err != nil {
			return nil, err
		}
	}
	return drops, nil
}

// mergeKeeps takes from the drops of a hard reset what `reset --merge`
// keeps (git 2.55): a path whose worktree copy differs from the index
// (unstaged) keeps it, index and all, since its index entry already equals
// the target. When such a path's index entry differs from the target, git
// refuses the whole reset and changes nothing, and refused says so. A
// conflicted entry is reset like any other, so it stays a drop.
func mergeKeeps(drops []drop, unstaged map[string]bool) (kept []drop, refused bool) {
	for _, d := range drops {
		switch {
		case !unstaged[d.path] || conflicted(d.staged):
			kept = append(kept, d)
		case d.staged.oid != d.written.oid || d.staged.mode != d.written.mode:
			return nil, true
		}
	}
	return kept, false
}

// rewritten pairs each path with what the target commit writes there and the
// index version the reset drops. The cap applies before the paths reach git.
func (r proofRepo) rewritten(target string, paths []string) ([]drop, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if err := r.capped(len(paths)); err != nil {
		return nil, err
	}
	index, err := r.indexEntries(r.top, paths)
	if err != nil {
		return nil, err
	}
	tree, err := r.treeEntries(r.top, target, false, paths)
	if err != nil {
		return nil, err
	}
	drops := make([]drop, 0, len(paths))
	for _, p := range paths {
		drops = append(drops, drop{path: p, written: tree[p], staged: index[p]})
	}
	return drops, nil
}

// capped refuses a proof over more than maxPaths paths.
func (r proofRepo) capped(n int) error {
	if n > r.maxPaths {
		return notListed(fmt.Sprintf("more than %d paths", r.maxPaths))
	}
	return nil
}

// keepsCommits refuses a branch move that strands a commit: every commit
// HEAD reaches must also be reachable from the target or a shared commit.
func (r proofRepo) keepsCommits(head, target string) error {
	args := append([]string{"rev-list", "--count", head, "--not", target}, r.shared...)
	out, err := runProofGit(r.ctx, r.top, args...)
	if err != nil {
		return notListed(err)
	}
	if n := strings.TrimSpace(string(out)); n != "0" {
		return fmt.Errorf("it moves the branch off %s commit(s) no other ref holds", n)
	}
	return nil
}

// checkDrops checks every drop against what the command writes there and
// the commits that count, in path order, and returns the first loss.
func (r proofRepo) checkDrops(drops []drop) error {
	if len(drops) == 0 {
		return nil
	}
	slices.SortFunc(drops, func(a, b drop) int { return strings.Compare(a.path, b.path) })
	paths := make([]string, 0, len(drops))
	for _, d := range drops {
		paths = append(paths, d.path)
	}
	committed, err := r.committed(paths)
	if err != nil {
		return err
	}
	for _, d := range drops {
		cands := slices.Clone(committed[d.path])
		if d.written.oid != "" {
			cands = append(cands, d.written)
		}
		if err := r.checkDrop(d, cands); err != nil {
			return err
		}
	}
	return nil
}

// checkDrop checks one path: the index version it drops and the worktree
// file must each equal one of cands. A submodule is not looked into.
func (r proofRepo) checkDrop(d drop, cands []treeEntry) error {
	if conflicted(d.staged) || conflicted(d.written) {
		return notListed(d.path + " is conflicted")
	}
	if d.written.mode == gitlinkMode || d.staged.mode == gitlinkMode {
		return notListed("a submodule at " + d.path)
	}
	if d.staged.oid != "" &&
		!slices.ContainsFunc(cands, func(e treeEntry) bool { return e.oid == d.staged.oid }) {
		return fmt.Errorf("the staged copy of `%s` is in no commit", d.path)
	}
	same, err := worktreeMatches(r.ctx, filepath.Join(r.top, d.path), cands)
	if err != nil {
		return notListed(fmt.Errorf("%s: %w", d.path, err))
	}
	if !same {
		return fmt.Errorf("`%s` holds content no commit has", d.path)
	}
	return nil
}

// conflicted reports whether an index entry is an unmerged one.
func conflicted(e treeEntry) bool {
	return e.stage != "" && e.stage != "0"
}

// committed returns, for each path, the entries HEAD and the shared commits
// have there. An unborn HEAD counts for nothing.
func (r proofRepo) committed(paths []string) (map[string][]treeEntry, error) {
	var seen []string
	out := map[string][]treeEntry{}
	for _, rev := range append([]string{"HEAD"}, r.shared...) {
		oid, err := r.commit(rev)
		if err != nil || slices.Contains(seen, oid) {
			continue
		}
		seen = append(seen, oid)
		tree, err := r.treeEntries(r.top, oid, false, paths)
		if err != nil {
			return nil, err
		}
		for p, e := range tree {
			out[p] = append(out[p], e)
		}
	}
	return out, nil
}

// sharedCommits resolves the commits besides HEAD whose files count as
// safe: the upstream, and origin's default branch while origin/HEAD points
// under originRefs. One that does not resolve (no upstream, no origin
// default) counts for nothing.
func (r proofRepo) sharedCommits() []string {
	var oids []string
	if oid, err := r.commit("@{u}"); err == nil {
		oids = append(oids, oid)
	}
	out, err := runProofGit(r.ctx, r.dir, "symbolic-ref", "-q", originHead)
	target := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(target, originRefs) {
		return oids
	}
	if oid, err := r.commit(target); err == nil {
		oids = append(oids, oid)
	}
	return oids
}

// commit resolves rev to a commit id.
func (r proofRepo) commit(rev string) (string, error) {
	out, err := runProofGit(
		r.ctx, r.dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}",
	)
	if err != nil {
		return "", notListed("no commit " + rev)
	}
	return strings.TrimSpace(string(out)), nil
}

// changedTracked lists the tracked paths git status reports changed in the
// index or the worktree, untracked files left out, and marks unstaged those
// whose worktree copy differs from the index. A submodule's `ignore` setting
// is overridden, so a dirty submodule is listed (and refused later).
func (r proofRepo) changedTracked() (paths []string, unstaged map[string]bool, err error) {
	out, err := runProofGit(
		r.ctx, r.top, "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=no",
		"--ignore-submodules=none",
	)
	if err != nil {
		return nil, nil, notListed(err)
	}
	unstaged = map[string]bool{}
	for _, f := range nulFields(out) {
		if len(f) > 3 {
			paths = append(paths, f[3:])
			unstaged[f[3:]] = f[1] != ' '
		}
	}
	return paths, unstaged, nil
}

// addedFiles lists the files commit to has that commit from lacks.
func (r proofRepo) addedFiles(from, to string) ([]string, error) {
	out, err := runProofGit(
		r.ctx, r.top, "diff", "--name-status", "-z", "--no-renames", from, to, "--",
	)
	if err != nil {
		return nil, notListed(err)
	}
	fields := nulFields(out)
	var added []string
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "A" {
			added = append(added, fields[i+1])
		}
	}
	return added, nil
}

// inTheWay returns the worktree paths a checkout of the added files
// overwrites although no index entry covers them: an untracked or ignored
// file at an added path, and a file where an added path needs a directory.
func inTheWay(top string, added []string) []string {
	var hit []string
	for _, p := range added {
		if _, err := os.Lstat(filepath.Join(top, p)); err == nil {
			hit = append(hit, p)
		}
		hit = append(hit, nonDirAncestors(top, p)...)
	}
	return hit
}

// withAncestors adds a drop for every parent of a dropped path that holds a
// file or symlink in the worktree: writing the path replaces that parent
// with a directory, and git does it for a restore and a reset alike. Such a
// parent passes only when a commit that counts has its content there.
func withAncestors(top string, drops []drop) []drop {
	have := make([]string, 0, len(drops))
	for _, d := range drops {
		have = append(have, d.path)
	}
	for _, d := range slices.Clone(drops) {
		for _, a := range nonDirAncestors(top, d.path) {
			if !slices.Contains(have, a) {
				have = append(have, a)
				drops = append(drops, drop{path: a})
			}
		}
	}
	return drops
}

// nonDirAncestors returns the parents of p that hold something other than
// a directory in the worktree at top.
func nonDirAncestors(top, p string) []string {
	var hit []string
	for d := path.Dir(p); d != "."; d = path.Dir(d) {
		if fi, err := os.Lstat(filepath.Join(top, d)); err == nil && !fi.IsDir() {
			hit = append(hit, d)
		}
	}
	return hit
}

// indexEntries lists the index entries the pathspecs match, run in dir.
func (r proofRepo) indexEntries(dir string, pathspecs []string) (map[string]treeEntry, error) {
	args := append([]string{"ls-files", "-s", "-z", "--full-name", "--"}, pathspecs...)
	out, err := runProofGit(r.ctx, dir, args...)
	if err != nil {
		return nil, notListed(err)
	}
	entries := map[string]treeEntry{}
	for _, f := range nulFields(out) {
		meta, p, ok := strings.Cut(f, "\t")
		m := strings.Fields(meta)
		if !ok || len(m) != 3 {
			return nil, notListed("ls-files output")
		}
		e := treeEntry{mode: m[0], oid: m[1], stage: m[2], size: -1}
		if old, dup := entries[p]; dup && old.stage != e.stage {
			e.stage = "conflict"
		}
		entries[p] = e
	}
	return entries, nil
}

// treeEntries lists the entries of tree at the paths, run in dir. With
// recursive the paths are pathspecs to walk; without, each names one entry.
// A tree that is not a blob list (a submodule) is refused for a walk.
func (r proofRepo) treeEntries(
	dir, tree string,
	recursive bool,
	paths []string,
) (map[string]treeEntry, error) {
	args := []string{"ls-tree", "-z", "-l", "--full-name"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(append(args, "--end-of-options", tree, "--"), paths...)
	out, err := runProofGit(r.ctx, dir, args...)
	if err != nil {
		return nil, notListed(err)
	}
	entries := map[string]treeEntry{}
	for _, f := range nulFields(out) {
		e, p, ok := parseTreeLine(f)
		if !ok {
			return nil, notListed("ls-tree output")
		}
		if recursive && e.kind != "blob" {
			return nil, notListed("a submodule at " + p)
		}
		entries[p] = e
	}
	return entries, nil
}

// parseTreeLine reads one `ls-tree -l` line: mode, type, id, size, a tab,
// the path.
func parseTreeLine(line string) (treeEntry, string, bool) {
	meta, p, ok := strings.Cut(line, "\t")
	m := strings.Fields(meta)
	if !ok || len(m) != 4 {
		return treeEntry{}, "", false
	}
	size, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		size = -1 // a tree or submodule prints "-"
	}
	return treeEntry{mode: m[0], kind: m[1], oid: m[2], size: size}, p, true
}

// worktreeMatches reports whether the worktree file at file holds what one
// of cands holds, compared as git object ids. A missing file drops nothing;
// one under a parent that is no directory (ENOTDIR) drops nothing at its own
// path either, since withAncestors makes that parent a drop of its own. A
// directory or special file is an error: the proof cannot vouch for it.
func worktreeMatches(ctx context.Context, file string, cands []treeEntry) (bool, error) {
	fi, err := os.Lstat(file)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch {
	case fi.Mode().IsRegular():
		return regularMatches(ctx, file, fi.Size(), modeCands(cands, "100"))
	case fi.Mode()&fs.ModeSymlink != 0:
		link, err := os.Readlink(file)
		if err != nil {
			return false, err
		}
		cands = modeCands(cands, "120000")
		return blobMatches(ctx, strings.NewReader(link), int64(len(link)), cands)
	}
	return false, errors.New("not a file")
}

// modeCands keeps the candidates whose mode starts with prefix: "100" for
// a regular file, "120000" for a symlink.
func modeCands(cands []treeEntry, prefix string) []treeEntry {
	var keep []treeEntry
	for _, e := range cands {
		if e.kind != "tree" && strings.HasPrefix(e.mode, prefix) {
			keep = append(keep, e)
		}
	}
	return keep
}

// regularMatches compares a regular file against cands, reading it only
// when a candidate could have its size.
func regularMatches(ctx context.Context, file string, size int64, cands []treeEntry) (bool, error) {
	if !slices.ContainsFunc(cands, func(e treeEntry) bool { return e.size < 0 || e.size == size }) {
		return false, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()
	return blobMatches(ctx, f, size, cands)
}

// blobMatches hashes content as a git blob of the given size and reports
// whether one of cands has that id. The hash follows the id length, SHA-1
// or SHA-256, as the repository's object format does.
func blobMatches(
	ctx context.Context,
	content io.Reader,
	size int64,
	cands []treeEntry,
) (bool, error) {
	if len(cands) == 0 {
		return false, nil
	}
	var h hash.Hash
	switch len(cands[0].oid) {
	case 2 * sha1.Size:
		h = sha1.New()
	case 2 * sha256.Size:
		h = sha256.New()
	default:
		return false, errors.New("unknown object format")
	}
	fmt.Fprintf(h, "blob %d\x00", size)
	n, err := io.Copy(h, ctxReader{ctx: ctx, r: content})
	if err != nil {
		return false, err
	}
	if n != size {
		return false, nil // the file changed under the read
	}
	id := fmt.Sprintf("%x", h.Sum(nil))
	return slices.ContainsFunc(cands, func(e treeEntry) bool { return e.oid == id }), nil
}

// ctxReader stops a read once ctx is done, so hashing a large file stays
// inside the proof's budget.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// runProofGit runs one git command for a proof in dir and returns its raw
// stdout. Pathspecs are literal, since the proof refuses every glob before
// it runs git, and git takes no optional lock, so a status here never
// rewrites the index under the session.
func runProofGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, _, err := execGit(ctx, gitExec{dir: dir, pre: []string{"--literal-pathspecs"}}, args...)
	return out, err
}

// gitExec is how a proof runs git: in dir, with the global options pre in
// front of the subcommand, env added to the session's environment, and
// stdin as its input.
type gitExec struct {
	dir   string
	pre   []string
	env   []string
	stdin []byte
}

// execGit runs one read-only git command and returns its stdout and stderr.
// git takes no optional lock, and the run stops at ctx, with proofPipeGrace
// for its pipes. A run that exits non-zero fails with a *gitRunError.
func execGit(ctx context.Context, e gitExec, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, "git", slices.Concat(e.pre, args)...)
	cmd.Dir = e.dir
	cmd.Env = slices.Concat(os.Environ(), []string{"GIT_OPTIONAL_LOCKS=0"}, e.env)
	if e.stdin != nil {
		cmd.Stdin = bytes.NewReader(e.stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.WaitDelay = proofPipeGrace
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("git %s timed out", args[0])
		}
		return nil, nil, &gitRunError{sub: args[0], status: cmd.ProcessState.ExitCode()}
	}
	return out.Bytes(), errOut.Bytes(), nil
}

// gitRunError is a git run that failed: status is its exit status, or -1
// when it did not exit on its own.
type gitRunError struct {
	sub    string
	status int
}

func (e *gitRunError) Error() string { return "git " + e.sub + " failed" }

// exitedWith reports whether err is a git run that exited with status.
func exitedWith(err error, status int) bool {
	var f *gitRunError
	return errors.As(err, &f) && f.status == status
}

// nulFields splits NUL-terminated git output into its fields.
func nulFields(out []byte) []string {
	var fields []string
	for f := range bytes.SplitSeq(out, []byte{0}) {
		if len(f) > 0 {
			fields = append(fields, string(f))
		}
	}
	return fields
}

// mapKeys returns the keys of m in sorted order.
func mapKeys(m map[string]treeEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// appendNew appends the items of more that list does not hold yet.
func appendNew(list, more []string) []string {
	for _, s := range more {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}
