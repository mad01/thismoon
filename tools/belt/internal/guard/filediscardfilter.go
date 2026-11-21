package guard

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

// This file holds pathFilter, which narrows a tree drop to the paths a find
// or a feeder hands the command.

// pathFilter narrows a tree drop to the paths a find or a feeder hands the
// command. names and types are find's -name, -iname and -type tests, all
// of which must hold. tree says the command drops each match whole (rm -r),
// so a match on a folder above a path counts too. tracked limits the paths
// to files git tracks (git ls-files, git grep -l), skipsGit marks a feeder
// that never hands a path in .git (rg, grep --exclude-dir=.git), and
// skipsIgnored one that never hands a path git ignores, so it never goes
// into an ignored folder (rg, git ls-files -o --exclude-standard). A filter
// with none of these hands every path, .git included.
//
// split holds the bytes at which the command cuts what the feeder prints
// into paths (xargs without -0, an unquoted $( )), "" when it takes each
// path whole: a printed path that holds one hands the command other paths.
// quoted marks a git feeder without -z, which prints a path with odd bytes
// in C quotes, so the command gets another path. nestedWhole marks a feeder
// that prints a nested repository's folder as one path (git ls-files -o),
// which rm -r takes whole, .git and all. all marks a filter that stands for
// none, which only split and quoted are set in.
type pathFilter struct {
	all          bool
	names        []findName
	types        string
	tree         bool
	tracked      bool
	skipsGit     bool
	skipsIgnored bool
	nestedWhole  bool
	split        string
	quoted       bool
}

// findName is one -name or -iname pattern, as path.Match reads it.
type findName struct {
	pattern string
	fold    bool
}

func (n findName) match(name string) bool {
	p := n.pattern
	if n.fold {
		p, name = strings.ToLower(p), strings.ToLower(name)
	}
	ok, _ := path.Match(p, name)
	return ok
}

// hasTests reports whether the filter holds find tests. A nil filter has
// none.
func (f *pathFilter) hasTests() bool {
	return f != nil && (len(f.names) > 0 || f.types != "")
}

// onlyTracked reports whether the filter hands only files git tracks.
func (f *pathFilter) onlyTracked() bool { return f != nil && f.tracked }

// skipsGitData reports whether the filter never hands a path in .git.
func (f *pathFilter) skipsGitData() bool { return f != nil && (f.tracked || f.skipsGit) }

// splits reports whether the command cuts what the feeder prints into
// paths at some byte (pathFilter.split).
func (f *pathFilter) splits() bool { return f != nil && f.split != "" }

// cuts reports whether the command would cut the printed text p into other
// paths.
func (f *pathFilter) cuts(p string) bool { return f.splits() && strings.ContainsAny(p, f.split) }

// printed returns the deepest part of rest, a path below the drop's root,
// that the feeder prints: rest itself, or the deepest folder on the way
// that find's tests match; "" when it prints nothing on that path. A filter
// with no tests prints every path. last holds the -type letters the last
// part may have.
func (f *pathFilter) printed(rest, last string) string {
	if !f.hasTests() {
		return rest
	}
	parts := strings.Split(rest, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		types := "d"
		if i == len(parts)-1 {
			types = last
		}
		for k := 0; k < len(types); k++ {
			if f.matchEntry(parts[i], types[k]) {
				return strings.Join(parts[:i+1], "/")
			}
		}
	}
	return ""
}

// matchEntry reports whether find's tests match one entry, by its name and
// its -type letter.
func (f *pathFilter) matchEntry(name string, typ byte) bool {
	if f.types != "" && strings.IndexByte(f.types, typ) < 0 {
		return false
	}
	for _, n := range f.names {
		if !n.match(name) {
			return false
		}
	}
	return true
}

// whole is the filter for a drop a match takes whole: no tests, and only
// how the command reads what the feeder prints. nil when it takes each
// path whole, as the filter it stands for.
func (f *pathFilter) whole() *pathFilter {
	if !f.splits() {
		return nil
	}
	return &pathFilter{all: true, tree: f.tree, split: f.split, quoted: f.quoted}
}

// takes reports whether a filter that drops matches whole matches the root
// of its drop, which is then lost whole.
func (f *pathFilter) takes(name string, typ byte) bool {
	return f.hasTests() && f.tree && f.matchEntry(name, typ)
}

// loses reports whether the drop at root (relative to top, "." for the
// top) loses the git status entry p with code. A tracked-only filter loses
// no untracked or ignored entry, and one that skips ignored paths no
// ignored entry. Any other filter loses a folder entry,
// which find and grep go into, a submodule, and every path its tests match;
// a path gone from disk is no loss.
func (f *pathFilter) loses(top, root, code, p string) bool {
	switch {
	case f == nil:
		return true
	case f.tracked:
		return code != "??" && code != "!!"
	case f.skipsIgnored && code == "!!":
		return false
	case strings.HasSuffix(p, "/") || !f.hasTests():
		return true
	}
	fi, err := os.Lstat(filepath.Join(top, p))
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		return false
	case err != nil || fi.IsDir():
		return true
	case !f.tree:
		return f.matchEntry(path.Base(p), fileType(fi))
	}
	rest := p
	if root != "." {
		rest = strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
	}
	parts := strings.Split(rest, "/")
	for i, part := range parts {
		typ := byte('d')
		if i == len(parts)-1 {
			typ = fileType(fi)
		}
		if part != "" && f.matchEntry(part, typ) {
			return true
		}
	}
	return false
}

func fileType(fi fs.FileInfo) byte { return typeLetter(fi.Mode()) }

func entryType(e fs.DirEntry) byte { return typeLetter(e.Type()) }

// typeLetter is find's -type letter for a file mode.
func typeLetter(m fs.FileMode) byte {
	switch {
	case m.IsDir():
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeSocket != 0:
		return 's'
	case m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	}
	return 'f'
}
