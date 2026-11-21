package guard

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// removedTracked lists what `rm -f` deletes: every index path the pathspecs
// match, the worktree file and the staged copy both.
func (r proofRepo) removedTracked(spec discardSpec) ([]drop, error) {
	index, err := r.indexEntries(r.dir, spec.pathspecs)
	if err != nil {
		return nil, err
	}
	drops := make([]drop, 0, len(index))
	for _, p := range mapKeys(index) {
		drops = append(drops, drop{path: p, staged: index[p]})
	}
	return drops, nil
}

// moveDrops lists what a forced mv overwrites: whatever stands at each
// destination, and its index entry. A destination that is a directory takes
// each source under its base name, as git places them, and a source that is
// a directory brings every index path under it. With several sources and no
// such directory git refuses the move, so nothing is listed.
func (r proofRepo) moveDrops(spec discardSpec) ([]drop, error) {
	out, err := runProofGit(r.ctx, r.dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, notListed(err)
	}
	prefix := strings.TrimSpace(string(out))
	dest, err := topPath(prefix, spec.dest)
	if err != nil {
		return nil, err
	}
	into := dest == "" || r.isDir(dest)
	if !into && len(spec.pathspecs) > 1 {
		return nil, nil
	}
	var dsts []string
	for _, s := range spec.pathspecs {
		src, err := topPath(prefix, s)
		if err != nil {
			return nil, err
		}
		to := dest
		if into {
			to = path.Join(dest, path.Base(src))
		}
		moved, err := r.movedPaths(src, to)
		if err != nil {
			return nil, err
		}
		dsts = appendNew(dsts, moved)
	}
	return r.stagedDrops(dsts)
}

// movedPaths returns where moving src to dst puts each file: dst itself for
// a file, and dst plus the rest of each index path under src for a
// directory.
func (r proofRepo) movedPaths(src, dst string) ([]string, error) {
	if src == "" {
		return nil, notListed("a move of the whole tree")
	}
	if !r.isDir(src) {
		return []string{dst}, nil
	}
	index, err := r.indexEntries(r.top, []string{src})
	if err != nil {
		return nil, err
	}
	var moved []string
	for _, p := range mapKeys(index) {
		if rest, ok := strings.CutPrefix(p, src+"/"); ok {
			moved = append(moved, path.Join(dst, rest))
		}
	}
	return moved, nil
}

// isDir reports whether the worktree holds a directory, not a symlink to
// one, at p.
func (r proofRepo) isDir(p string) bool {
	fi, err := os.Lstat(filepath.Join(r.top, p))
	return err == nil && fi.IsDir()
}

// topPath turns a path written in the directory at prefix into one from the
// work tree root, "" for the root itself. One outside the tree, or written
// from the file system root, is refused.
func topPath(prefix, p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", notListed("the absolute path " + p)
	}
	q := path.Clean(prefix + p)
	switch {
	case q == ".":
		return "", nil
	case q == ".." || strings.HasPrefix(q, "../"):
		return "", notListed("the path " + p + " outside the tree")
	}
	return q, nil
}

// applyDrops lists the files an apply rewrites, as `git apply --numstat`
// names them under the same path options, with their index entries when it
// writes the index too. A patch that renames or copies a file names only
// the new path there, which `git apply --summary` shows, so it is refused.
func (r proofRepo) applyDrops(spec discardSpec) ([]drop, error) {
	report := func(how ...string) ([]byte, error) {
		args := slices.Concat([]string{"apply"}, how, spec.patchOpts, []string{"--"}, spec.patches)
		out, err := runProofGit(r.ctx, r.dir, args...)
		if err != nil {
			return nil, notListed(err)
		}
		return out, nil
	}
	summary, err := report("--summary")
	if err != nil {
		return nil, err
	}
	for line := range strings.SplitSeq(string(summary), "\n") {
		if strings.HasPrefix(line, " rename ") || strings.HasPrefix(line, " copy ") {
			return nil, notListed("a patch that renames or copies a file")
		}
	}
	out, err := report("--numstat", "-z")
	if err != nil {
		return nil, err
	}
	fields := nulFields(out)
	if err := r.capped(len(fields)); err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range fields {
		stat := strings.SplitN(f, "\t", 3)
		if len(stat) != 3 {
			return nil, notListed("apply --numstat output")
		}
		paths = appendNew(paths, stat[2:])
	}
	if spec.staged {
		return r.stagedDrops(paths)
	}
	drops := make([]drop, 0, len(paths))
	for _, p := range paths {
		drops = append(drops, drop{path: p})
	}
	return drops, nil
}

// worktreeDrops lists what a forced worktree remove deletes in the tree the
// proof runs in: every tracked path git status reports changed, with its
// staged copy, and every untracked file the ignore rules keep. Ignored files
// go too, as they do in a remove without --force, so they are not listed.
func (r proofRepo) worktreeDrops() ([]drop, error) {
	tracked, _, err := r.changedTracked()
	if err != nil {
		return nil, err
	}
	untracked, err := r.cleanDrops(discardSpec{kind: untrackedClean})
	if err != nil {
		return nil, err
	}
	staged, err := r.stagedDrops(tracked)
	if err != nil {
		return nil, err
	}
	return append(staged, untracked...), nil
}

// stagedDrops pairs each path with its index entry, for a discard that
// deletes or overwrites the file and the entry both. The cap applies before
// the paths reach git.
func (r proofRepo) stagedDrops(paths []string) ([]drop, error) {
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
	drops := make([]drop, 0, len(paths))
	for _, p := range paths {
		drops = append(drops, drop{path: p, staged: index[p]})
	}
	return drops, nil
}
