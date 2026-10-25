package record

import (
	"context"
	"path/filepath"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// The helpers below decide how a file a tool named is keyed in the store.
// Every capture path uses them, the hook and `moondb import` alike, so the
// same file lands under the same repo, worktree and path whichever path
// recorded it.

// Resolve makes the tool's path absolute and physical. A file that is gone
// resolves through its directory, so it still lines up with the physical
// paths git reports; when the directory is gone too, the path is cleaned.
func Resolve(name, cwd string) string {
	if !filepath.IsAbs(name) {
		name = filepath.Join(cwd, name)
	}
	if p, err := filepath.EvalSymlinks(name); err == nil {
		return p
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(name)); err == nil {
		return filepath.Join(dir, filepath.Base(name))
	}
	return filepath.Clean(name)
}

// Locate places dir in git as the store records it; the zero Location when
// dir is outside any repository, is gone, or git cannot say before ctx ends.
func Locate(ctx context.Context, dir string) store.Location {
	l := gitloc.LocationAt(ctx, dir)
	return store.Location{
		Repo: l.Repo, Worktree: l.Toplevel, Canonical: l.Canonical, Branch: l.Branch, Head: l.Head,
	}
}

// Placed returns the location and the path to record for abs: relative to
// the worktree with forward slashes, or absolute with no location when abs
// is outside any worktree.
func Placed(loc store.Location, abs string) (store.Location, string) {
	if loc.Worktree == "" {
		return store.Location{}, abs
	}
	rel, err := filepath.Rel(loc.Worktree, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return store.Location{}, abs
	}
	return loc, filepath.ToSlash(rel)
}

// EditKind says what a call of tool did to the file, as far as the harness
// tells: writeType is the "type" a Write reports (create or update).
func EditKind(tool, writeType string) store.EditKind {
	switch {
	case tool == "NotebookEdit":
		return store.EditUnknown
	case tool == "Write" && writeType == "create":
		return store.EditCreate
	default:
		return store.EditUpdate
	}
}

// Excluded reports whether a glob matches the file's base name or its
// recorded path.
func Excluded(globs []string, abs, rel string) bool {
	base := filepath.Base(abs)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
		if ok, _ := filepath.Match(g, rel); ok {
			return true
		}
	}
	return false
}
