package importer

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/record"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Placement counts how the run keyed its edits' files, by outcome.
type Placement struct {
	// Live files sit in a directory that still exists, placed as the hook
	// places them.
	Live int `json:"live"`
	// Ancestor files sit in a directory that is gone, inside a work tree
	// one of its ancestors still belongs to.
	Ancestor int `json:"ancestor"`
	// Layout files sit in a linked worktree that is gone, found by the
	// layout of their path (see placer.linkedWorktree).
	Layout int `json:"layout"`
	// Cwd files sit under a session cwd that is gone, which stands in as
	// their worktree.
	Cwd int `json:"cwd"`
	// WithoutRepo counts the Layout and Cwd files no known location named
	// a repository for.
	WithoutRepo int `json:"without_repo"`
	// NoRepo files are recorded by absolute path with no location.
	NoRepo int `json:"norepo"`
}

// outcome is how place keyed one file.
type outcome int

const (
	placedLive outcome = iota
	placedByAncestor
	placedByLayout
	placedByCwd
	placedNowhere
)

func (c *Placement) count(how outcome, loc store.Location) {
	switch how {
	case placedLive:
		c.Live++
	case placedByAncestor:
		c.Ancestor++
	case placedByLayout:
		c.Layout++
	case placedByCwd:
		c.Cwd++
	default:
		c.NoRepo++
	}
	if (how == placedByLayout || how == placedByCwd) && loc.Repo == "" {
		c.WithoutRepo++
	}
}

// The two layouts that hold linked worktrees: entries of a <worktreesDir>
// directory, and <checkout>/<agentDir>/<agentWorktrees>/<slug>, where the
// harness keeps the worktrees of its agents inside the checkout.
const (
	worktreesDir   = ".worktrees"
	agentDir       = ".claude"
	agentWorktrees = "worktrees"
)

// placer keys files in the store for one run. It caches git locations by
// directory, since a run asks about the same few worktrees thousands of
// times, and holds the locations the store already knows, which name the
// repository of a worktree that is gone.
type placer struct {
	ctx   context.Context
	known []store.Location
	cache map[string]store.Location
}

// place returns the location and path to record for the file abs, which a
// call made from cwd on branch, and counts the outcome. A file whose
// directory still exists is placed as the hook places it; see placeGone
// for one whose directory is gone, which is most of an import.
func (p *placer) place(abs, cwd, branch string, counts *Placement) (store.Location, string) {
	cwdRoot := ""
	if cwd != "" {
		cwdRoot = record.Resolve(cwd, "")
	}
	if dir := filepath.Dir(abs); isDir(dir) {
		loc, rel := record.Placed(p.onBranch(p.locate(dir), cwdRoot, branch), abs)
		how := placedLive
		if loc.Worktree == "" {
			how = placedNowhere
		}
		counts.count(how, loc)
		return loc, rel
	}
	loc, rel, how := p.placeGone(abs, cwdRoot, branch)
	counts.count(how, loc)
	return loc, rel
}

// placeGone places a file whose directory is gone. It falls back, in order,
// to:
//
//  1. the work tree its nearest existing ancestor belongs to, unless the
//     file sat in a linked worktree nested inside that work tree;
//  2. the linked worktree its path names by layout;
//  3. its session cwd, when that is gone too and holds the file;
//  4. no location, the absolute path.
//
// A fallback worktree gets the repository known by its name, and the
// transcript's branch, which belongs to the cwd, only when the cwd lay
// inside it.
func (p *placer) placeGone(abs, cwdRoot, branch string) (store.Location, string, outcome) {
	dir := filepath.Dir(abs)
	anc := p.around(dir)
	// The session's own checkouts name repositories the store may not know.
	also := []store.Location{anc}
	if cwdRoot != "" {
		also = append(also, p.around(cwdRoot))
	}
	wt, inLayout := p.linkedWorktree(dir, also)
	if anc.Worktree != "" && (!inLayout || within(wt.Worktree, anc.Worktree)) {
		loc, rel := record.Placed(p.onBranch(anc, cwdRoot, branch), abs)
		return loc, rel, placedByAncestor
	}
	if inLayout {
		loc, rel := inGone(wt, abs, cwdRoot, branch)
		return loc, rel, placedByLayout
	}
	if cwdRoot != "" && !isDir(cwdRoot) && within(cwdRoot, abs) {
		wt := store.Location{Worktree: cwdRoot}
		wt.Repo, wt.Canonical = p.repoNamed(filepath.Base(cwdRoot), also...)
		loc, rel := inGone(wt, abs, cwdRoot, branch)
		return loc, rel, placedByCwd
	}
	return store.Location{}, abs, placedNowhere
}

// onBranch puts the branch the transcript recorded for the session's cwd
// on loc when the cwd sits in the same worktree. HEAD now says nothing
// about HEAD then, so it is left out.
func (p *placer) onBranch(loc store.Location, cwdRoot, branch string) store.Location {
	loc.Head = ""
	if branch == "" || branch == detachedHead || loc.Worktree == "" || cwdRoot == "" {
		return loc
	}
	if p.around(cwdRoot).Worktree == loc.Worktree {
		loc.Branch = branch
	}
	return loc
}

// inGone places abs in the worktree wt, which is gone and holds abs. The
// transcript's branch comes along when the cwd lay inside wt.
func inGone(wt store.Location, abs, cwdRoot, branch string) (store.Location, string) {
	if branch != detachedHead && cwdRoot != "" && within(wt.Worktree, cwdRoot) {
		wt.Branch = branch
	}
	rel, _ := filepath.Rel(wt.Worktree, abs) // callers pass a wt that holds abs
	return wt, filepath.ToSlash(rel)
}

// linkedWorktree finds the nearest linked worktree dir sits in by its
// path's layout, as a location with the repository known for it. An agent
// worktree at <checkout>/.claude/worktrees/<slug> belongs to the checkout's
// repository. An entry of a .worktrees directory is read by worktreesEntry.
// Repositories are matched against the store's locations and also.
func (p *placer) linkedWorktree(dir string, also []store.Location) (store.Location, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		parent := filepath.Dir(d)
		if parent == d {
			return store.Location{}, false
		}
		grand := filepath.Dir(parent)
		switch {
		case filepath.Base(parent) == agentWorktrees && filepath.Base(grand) == agentDir:
			wt := store.Location{Worktree: d}
			wt.Repo, wt.Canonical = p.repoNamed(filepath.Base(filepath.Dir(grand)), also...)
			return wt, true
		case filepath.Base(parent) == worktreesDir:
			return p.worktreesEntry(d, dir, also)
		}
	}
}

// worktreesEntry reads the .worktrees entry dir sits in. An entry named
// for a known repository holds its worktrees by slug (<name>/<slug>); an
// entry named for a known repository and a dash is a worktree itself
// (<name>-<slug>); an entry known by neither holds worktrees by slug of a
// repository nobody knows.
func (p *placer) worktreesEntry(entry, dir string, also []store.Location) (store.Location, bool) {
	name, slug := filepath.Base(entry), slugDir(entry, dir)
	if repo, canonical := p.repoNamed(name, also...); repo != "" {
		return store.Location{Repo: repo, Canonical: canonical, Worktree: slug}, slug != ""
	}
	if repo, canonical := p.repoPrefixed(name, also...); repo != "" {
		return store.Location{Repo: repo, Canonical: canonical, Worktree: entry}, true
	}
	return store.Location{Worktree: slug}, slug != ""
}

// slugDir is the directory directly under entry that dir is or sits in;
// "" when dir is entry itself.
func slugDir(entry, dir string) string {
	rel, err := filepath.Rel(entry, dir)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return ""
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return filepath.Join(entry, first)
}

// repoPrefixed is repoNamed for the longest part of name before a dash
// that names a repository.
func (p *placer) repoPrefixed(name string, also ...store.Location) (repo, canonical string) {
	for i := strings.LastIndexByte(name, '-'); i > 0; i = strings.LastIndexByte(name[:i], '-') {
		if repo, canonical = p.repoNamed(name[:i], also...); repo != "" {
			return repo, canonical
		}
	}
	return "", ""
}

// repoNamed is the one repository known by name: a location, among the
// store's and also, whose worktree root or repository has name as its last
// path segment. Canonical comes along when those locations agree on it.
// Both are empty when no repository or more than one matches.
func (p *placer) repoNamed(name string, also ...store.Location) (repo, canonical string) {
	canonicals := map[string]bool{}
	for _, l := range slices.Concat(also, p.known) {
		if l.Repo == "" || (filepath.Base(l.Worktree) != name && path.Base(l.Repo) != name) {
			continue
		}
		if repo != "" && l.Repo != repo {
			return "", ""
		}
		repo = l.Repo
		canonicals[l.Canonical] = true
	}
	if len(canonicals) == 1 {
		for c := range canonicals {
			canonical = c
		}
	}
	return repo, canonical
}

// around is the location of dir's nearest existing directory, dir itself
// included; the zero Location when none is in a work tree.
func (p *placer) around(dir string) store.Location {
	for !isDir(dir) {
		parent := filepath.Dir(dir)
		if parent == dir {
			return store.Location{}
		}
		dir = parent
	}
	return p.locate(dir)
}

func (p *placer) locate(dir string) store.Location {
	if loc, ok := p.cache[dir]; ok {
		return loc
	}
	ctx, cancel := context.WithTimeout(p.ctx, moondb.GitBudget)
	defer cancel()
	loc := record.Locate(ctx, dir)
	if p.cache == nil {
		p.cache = map[string]store.Location{}
	}
	p.cache[dir] = loc
	return loc
}

// within reports whether p is root or lies under it.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && filepath.IsLocal(rel)
}

func isDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// placeDir is the location of a directory, as place keys a file in it,
// symlinks resolved as the hook resolves a file's. Its outcome counts
// nowhere: Placement counts the edits' files.
func (p *placer) placeDir(dir, branch string) store.Location {
	var unused Placement
	loc, _ := p.place(filepath.Join(record.Resolve(dir, ""), "x"), dir, branch, &unused)
	return loc
}
