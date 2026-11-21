package guard

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// This file holds the file-discard reads of the disk that need more than
// one stat: a glob expanded the way bash and zsh expand it, a path spelled
// the way its folder lists it, and the folder walk the search for git's
// own data and for hidden repositories runs on.

// maxGlobDirs caps the folders one call's globs read. Past it, a glob that
// cannot leave its folder is judged as that folder, and any other denies.
const maxGlobDirs = 1024

// maxScanDirs caps the folders one call reads looking for a repository
// inside an ignored folder or a nested one, or for a printed path the
// command cuts. Past it the delete is denied.
const maxScanDirs = 20000

// errWalkTooBig stops a walk past its cap or the call's deadline.
var errWalkTooBig = errors.New("too many folders to read")

// globPart is one part of a glob path between two slashes: a literal name,
// zsh's and bash's recursive `**`, or a pattern for path.Match. dots says
// the pattern may match a name that starts with a dot: bash and zsh match
// one only to a literal leading dot, and a bracket expression is let
// through to path.Match, which can only match more.
type globPart struct {
	name      string
	literal   bool
	recursive bool
	dots      bool
}

// match reports whether the pattern part matches one folder entry.
func (p globPart) match(name string) bool {
	if name[0] == '.' && !p.dots {
		return false
	}
	ok, _ := path.Match(p.name, name)
	return ok
}

// errGlob is why belt cannot expand a glob the way the shell does.
type errGlob string

func (e errGlob) Error() string { return string(e) }

// splitGlob splits an absolute path with its active glob bytes into the
// literal folder before the first globbing part and the parts from there
// on. A part zsh reads in a way path.Match does not is an error: `***`
// follows symlinks, a POSIX class such as `[[:alpha:]]` is a set Go does
// not know, and a pattern that matches `.` or `..` can climb out of its
// folder (bash before 5.2 matches `.*` to `..`).
// The prefix keeps its `..` parts for the kernel to resolve (physical), and
// a `..` after a globbing part is an error: the kernel resolves it from
// where a symlink match leads, and a joined path would not.
func splitGlob(lit string, active []bool) (prefix string, parts []globPart, err error) {
	var names []string
	for start := 0; start < len(lit); {
		end := strings.IndexByte(lit[start:], '/')
		if end < 0 {
			end = len(lit) - start
		}
		text, act := lit[start:start+end], active[start:start+end]
		start += end + 1
		if text == "" {
			continue
		}
		part, err := readGlobPart(text, act)
		switch {
		case err != nil:
			return "", nil, err
		case part.literal && parts == nil:
			names = append(names, text)
			continue
		case text == "..":
			return "", nil, errGlob("belt cannot place the `..` after a glob")
		}
		parts = append(parts, part)
	}
	return "/" + strings.Join(names, "/"), parts, nil
}

// readGlobPart reads one part of a glob path.
func readGlobPart(text string, act []bool) (globPart, error) {
	globbing, stars := false, 0
	for i, a := range act {
		globbing = globbing || a
		if a && text[i] == '*' {
			stars++
		}
	}
	switch {
	case !globbing:
		return globPart{name: text, literal: true}, nil
	case stars == len(text) && stars == 2:
		return globPart{name: text, recursive: true}, nil
	case stars == len(text) && stars > 2:
		return globPart{}, errGlob("zsh's `" + text + "` follows symlinks belt does not")
	case posixClass(text, act):
		return globPart{}, errGlob("belt does not read the bracket class in `" + text + "`")
	}
	pattern := globEscape(text, act)
	if _, err := path.Match(pattern, ""); err != nil {
		return globPart{}, errGlob("belt cannot read the glob `" + text + "`")
	}
	part := globPart{name: pattern, dots: text[0] == '.' && !act[0] || text[0] == '[' && act[0]}
	if part.dots && (part.match(".") || part.match("..")) {
		return globPart{}, errGlob("`" + text + "` can match `..`, the folder above")
	}
	return part, nil
}

// posixClass reports whether a `[:`, `[=` or `[.` follows an active `[`:
// a POSIX class, an equivalence class or a collating symbol inside a
// bracket expression, which path.Match reads as plain characters.
func posixClass(text string, act []bool) bool {
	for i := range text {
		if act[i] && text[i] == '[' {
			rest := text[i+1:]
			return strings.Contains(rest, "[:") || strings.Contains(rest, "[=") ||
				strings.Contains(rest, "[.")
		}
	}
	return false
}

// leavesPrefix reports whether a match of parts can lie outside the prefix
// folder: the command follows the last part (a trailing slash, shred), or a
// part before the last is a pattern or a name, which the shell follows
// through a symlink. A `**` follows none.
func leavesPrefix(parts []globPart, follows bool) bool {
	if follows {
		return true
	}
	for _, p := range parts[:len(parts)-1] {
		if !p.recursive {
			return true
		}
	}
	return false
}

// globWalk expands one glob. dirs counts the folders it reads, and the
// walk stops past maxDirs folders, maxMatches matches or the deadline.
type globWalk struct {
	dirs, maxDirs, maxMatches int
	deadline                  time.Time
	matches                   []string
}

// expand adds the paths under dir that parts match.
func (g *globWalk) expand(dir string, parts []globPart) error {
	if len(parts) == 0 {
		return g.add(dir)
	}
	p, rest := parts[0], parts[1:]
	switch {
	case p.literal && len(rest) == 0:
		next := filepath.Join(dir, p.name)
		if _, err := os.Lstat(next); err != nil {
			return nil
		}
		return g.add(next)
	case p.literal:
		return g.expand(filepath.Join(dir, p.name), rest)
	case p.recursive:
		return g.recurse(dir, rest)
	}
	entries, err := g.read(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !p.match(e.Name()) {
			continue
		}
		if err := g.expand(filepath.Join(dir, e.Name()), rest); err != nil {
			return err
		}
	}
	return nil
}

// recurse expands a `**` part: rest under dir and under every folder below
// it, hidden folders and symlinks aside, as zsh's `**/` and bash's globstar
// do. A `**` last matches every entry below dir, as globstar does; zsh
// reads it as `*`, which matches less.
func (g *globWalk) recurse(dir string, rest []globPart) error {
	entries, err := g.read(dir)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		if err := g.expand(dir, rest); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if e.Name()[0] == '.' {
			continue
		}
		next := filepath.Join(dir, e.Name())
		if len(rest) == 0 {
			if err := g.add(next); err != nil {
				return err
			}
		}
		if e.IsDir() {
			if err := g.recurse(next, rest); err != nil {
				return err
			}
		}
	}
	return nil
}

// read lists dir, sorted by name as the shell sorts matches. A folder the
// shell cannot list matches nothing.
func (g *globWalk) read(dir string) ([]fs.DirEntry, error) {
	if g.dirs++; g.dirs > g.maxDirs || time.Now().After(g.deadline) {
		return nil, errWalkTooBig
	}
	entries, _ := os.ReadDir(dir)
	return entries, nil
}

func (g *globWalk) add(p string) error {
	if g.matches = append(g.matches, p); len(g.matches) > g.maxMatches {
		return errWalkTooBig
	}
	return nil
}

// spelled returns p, an absolute clean path that exists, with each part
// spelled the way its folder lists it. A case-insensitive volume, APFS by
// default, finds b.txt as B.txt, while git matches a pathspec by its bytes.
// A part keeps its spelling when the folder lists it so, else takes the one
// entry equal to it but for case. ok is false when no entry matches, as for
// a name in another Unicode form, or a folder cannot be listed. Folders are
// remembered for the rest of the call.
func (j *discardJudge) spelled(p string) (string, bool) {
	if p == "/" {
		return p, true
	}
	if c, seen := j.spellings[p]; seen {
		return c, c != ""
	}
	dir, name := filepath.Split(p)
	c := ""
	if d, ok := j.spelled(filepath.Clean(dir)); ok {
		if n, ok := entryName(d, name); ok {
			c = filepath.Join(d, n)
		}
	}
	j.spellings[p] = c
	return c, c != ""
}

// entryName returns the entry of dir spelled name, else the one entry equal
// to it but for case.
func entryName(dir, name string) (string, bool) {
	f, err := os.Open(dir)
	if err != nil {
		return "", false
	}
	defer f.Close()
	folded := []string{}
	for {
		names, err := f.Readdirnames(256)
		for _, n := range names {
			switch {
			case n == name:
				return n, true
			case strings.EqualFold(n, name):
				folded = append(folded, n)
			}
		}
		switch {
		case errors.Is(err, io.EOF):
			return firstOnly(folded)
		case err != nil:
			return "", false
		}
	}
}

func firstOnly(names []string) (string, bool) {
	if len(names) != 1 {
		return "", false
	}
	return names[0], true
}

// findMatch walks the path at root, git's own data, for an entry find's
// tests match (pathFilter.matchEntry): root itself or any entry below it.
// It returns the first match, "" for none.
func (j *discardJudge) findMatch(root string, f *pathFilter) (string, error) {
	fi, err := os.Lstat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", err
	case f.matchEntry(filepath.Base(root), fileType(fi)):
		return root, nil
	case !fi.IsDir():
		return "", nil
	}
	hit := ""
	err = j.walkDirs(root, walkMode{}, func(dir string, entries []fs.DirEntry) walkNext {
		for _, e := range entries {
			if f.matchEntry(e.Name(), entryType(e)) {
				hit = filepath.Join(dir, e.Name())
				return walkStop
			}
		}
		return walkOn
	})
	return hit, err
}

// walkNext is what a walk does once a visit has read a folder.
type walkNext int

const (
	walkOn    walkNext = iota // read its subfolders too
	walkPrune                 // leave its subfolders out
	walkStop                  // end the walk
)

// walkMode says which folders a walk leaves out: .git folders (skipGit),
// and the inside of each package a node_modules folder holds (packages).
// With packages, a walk reads a node_modules folder, each package in it
// and each package in an @scope folder in it, so a .git or a bare layout
// there is found, but not the folders inside a package. A package manager
// unpacks packages and never leaves a .git in one, and a real node_modules
// holds tens of thousands of folders, more than one call can read; a
// repository someone cloned deeper inside a package goes unseen.
type walkMode struct {
	skipGit, packages bool
}

// walkDir is one folder a walk is to read: level counts the steps below a
// node_modules folder, 0 outside one.
type walkDir struct {
	path  string
	level int
}

// walkDirs reads every folder at and below root, breadth first, and hands
// each one's entries, sorted by name so every file system names the same
// first loss, to visit, which says what the walk does next
// (walkNext). It reads hidden folders, follows no symlink, leaves out what
// mode says and stops past scanDirs folders in the whole call or the
// deadline.
func (j *discardJudge) walkDirs(root string, mode walkMode, visit func(dir string, entries []fs.DirEntry) walkNext) error {
	queue := []walkDir{{path: root}}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if j.scanned++; j.scanned > j.g.scanDirs || time.Now().After(j.deadline) {
			return errWalkTooBig
		}
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			return err
		}
		switch visit(dir.path, entries) {
		case walkStop:
			return nil
		case walkPrune:
			continue
		}
		level, ok := mode.childLevel(dir)
		if !ok {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && (!mode.skipGit || !strings.EqualFold(e.Name(), ".git")) {
				queue = append(queue, walkDir{path: filepath.Join(dir.path, e.Name()), level: level})
			}
		}
	}
	return nil
}

// childLevel is the level of dir's subfolders (walkDir), and false when the
// walk leaves them out: the inside of a package, at level 1 a package and
// at level 2 a package in an @scope folder.
func (m walkMode) childLevel(dir walkDir) (int, bool) {
	switch {
	case !m.packages:
		return 0, true
	case filepath.Base(dir.path) == "node_modules":
		return 1, true
	case dir.level == 1 && strings.HasPrefix(filepath.Base(dir.path), "@"):
		return 2, true
	case dir.level > 0:
		return 0, false
	}
	return 0, true
}
