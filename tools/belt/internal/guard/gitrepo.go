package guard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// This file finds the repo `git -C <dir>` would read, without running git:
// asking git would run code the repo's config names. It follows git's own
// discovery (setup.c) and gives no answer for any layout where git might
// read a git directory or a config the allow cannot vouch for.

// gitRepo is the repo git discovers from a directory: the top of its work
// tree and every git directory git reads for it.
type gitRepo struct {
	top     string
	gitDirs []string
}

// within reports whether the work tree and every git directory sit inside
// a root.
func (r gitRepo) within(roots []string) bool {
	return insideAny(r.top, roots) && !slices.ContainsFunc(r.gitDirs, func(d string) bool {
		return !insideAny(d, roots)
	})
}

// discoverRepo walks up from dir, a resolved absolute path, the way git
// does. At each directory git first looks for a .git entry and then asks
// whether the directory is a git directory itself. The first .git entry
// marks the top of the work tree, and only two kinds count there: a .git
// directory git accepts, and a .git file of a linked worktree (see
// linkedWorktree). It reports false when:
//   - the dir, or any directory above it up to the top, looks like a git
//     directory, since git would use it, config and all, and a clone can
//     carry one as tracked files;
//   - the .git entry is a symlink, another kind of file, or a directory git
//     would reject and walk past toward a repo further up;
//   - the repo's config moves or bares the work tree, sets core.hooksPath,
//     or includes another file (configRisky);
//   - there is no .git entry at all.
func discoverRepo(dir string) (gitRepo, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if looksLikeGitDir(d) {
			return gitRepo{}, false
		}
		fi, err := os.Lstat(filepath.Join(d, ".git"))
		if err == nil {
			return repoAt(d, fi)
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(d) == d {
			return gitRepo{}, false
		}
	}
}

// gitDirMarkers are the entries beside a HEAD that make a directory look
// like a git directory. git itself needs objects and refs, or a commondir
// naming where they are; config is listed too, so a HEAD and config pair is
// refused without weighing git's finer checks.
var gitDirMarkers = []string{"objects", "refs", "commondir", "config"}

// looksLikeGitDir reports whether d holds a HEAD entry and any marker.
func looksLikeGitDir(d string) bool {
	return present(filepath.Join(d, "HEAD")) &&
		slices.ContainsFunc(gitDirMarkers, func(m string) bool {
			return present(filepath.Join(d, m))
		})
}

// present reports whether an entry may exist at path: any Lstat error but
// "does not exist" counts as present, the answer that refuses.
func present(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// repoAt reads the .git entry at top, which Lstat described as fi.
func repoAt(top string, fi fs.FileInfo) (gitRepo, bool) {
	dotGit := filepath.Join(top, ".git")
	switch {
	case fi.IsDir():
		ok := mainGitDir(dotGit) && configsClean(dotGit)
		return gitRepo{top: top, gitDirs: []string{dotGit}}, ok
	case fi.Mode().IsRegular():
		return linkedWorktree(top, dotGit)
	}
	return gitRepo{}, false
}

// mainGitDir reports whether g is a git directory git accepts on its own:
// a HEAD git can read, objects and refs as real directories git may enter,
// and no commondir sending git to objects and refs elsewhere. It is at
// least as strict as git's is_git_directory, so git never walks past a
// .git directory this accepts.
func mainGitDir(g string) bool {
	return validHead(filepath.Join(g, "HEAD")) &&
		enterableDir(filepath.Join(g, "objects")) &&
		enterableDir(filepath.Join(g, "refs")) &&
		!present(filepath.Join(g, "commondir"))
}

// accessExecute is access(2)'s X_OK: git checks objects and refs with it.
const accessExecute = 0x1

// enterableDir reports whether p is a directory, not a symlink to one,
// that this process may enter.
func enterableDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir() && syscall.Access(p, accessExecute) == nil
}

// validHead reports whether path is a HEAD git accepts (validate_headref):
// a regular file, not a symlink, naming a ref under refs/ or holding an
// object id.
func validHead(path string) bool {
	s, ok := smallFile(path)
	if !ok {
		return false
	}
	if ref, isRef := strings.CutPrefix(s, "ref:"); isRef {
		return strings.HasPrefix(strings.TrimLeft(ref, " \t\n\v\f\r"), "refs/")
	}
	return len(s) >= 40 &&
		!strings.ContainsFunc(s[:40], func(r rune) bool { return !isHexDigit(r) })
}

func isHexDigit(r rune) bool {
	return '0' <= r && r <= '9' || 'a' <= r && r <= 'f' || 'A' <= r && r <= 'F'
}

// maxSmallFile bounds the git bookkeeping files belt reads: HEAD, a .git
// file, commondir and gitdir each hold one short line.
const maxSmallFile = 4096

// smallFile returns the content of a regular file, not a symlink, of at
// most maxSmallFile bytes.
func smallFile(path string) (string, bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxSmallFile {
		return "", false
	}
	b, err := os.ReadFile(path)
	return string(b), err == nil
}

// pathLine returns the one path a bookkeeping file holds, the way git reads
// it: the content with trailing line endings trimmed, after prefix. A
// second line or an empty path fails it.
func pathLine(path, prefix string) (string, bool) {
	s, ok := smallFile(path)
	if !ok {
		return "", false
	}
	s, ok = strings.CutPrefix(strings.TrimRight(s, "\r\n"), prefix)
	return s, ok && s != "" && !strings.ContainsAny(s, "\r\n")
}

// resolveFrom resolves p, relative to base when it is not absolute, with
// every symlink followed. base is never cleaned into p first, as in
// resolveGitCDir.
func resolveFrom(base, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		p = base + string(filepath.Separator) + p
	}
	real, err := filepath.EvalSymlinks(p)
	return real, err == nil
}

// linkedWorktree reads the .git file at top. It answers only for the file
// `git worktree add` writes: gitdir naming <main>/.git/worktrees/<name>, a
// directory whose HEAD git accepts, whose commondir leads back to
// <main>/.git and whose gitdir leads back to this .git file, with
// <main>/.git a git directory git accepts on its own. A clone cannot carry
// that layout, since git refuses to check out a path with a .git
// component. A submodule's .git file, or one naming any other directory,
// gets no answer.
func linkedWorktree(top, dotGit string) (gitRepo, bool) {
	target, ok := pathLine(dotGit, "gitdir: ")
	if !ok {
		return gitRepo{}, false
	}
	gd, ok := resolveFrom(top, target)
	if !ok {
		return gitRepo{}, false
	}
	common := filepath.Dir(filepath.Dir(gd))
	if filepath.Base(filepath.Dir(gd)) != "worktrees" || filepath.Base(common) != ".git" {
		return gitRepo{}, false
	}
	if !pointsAt(gd, "commondir", common) || !pointsAt(gd, "gitdir", dotGit) {
		return gitRepo{}, false
	}
	ok = validHead(filepath.Join(gd, "HEAD")) && mainGitDir(common) &&
		configsClean(common) && configsClean(gd)
	return gitRepo{top: top, gitDirs: []string{gd, common}}, ok
}

// pointsAt reports whether the bookkeeping file name in gd holds a path
// that resolves, relative to gd, to want.
func pointsAt(gd, name, want string) bool {
	p, ok := pathLine(filepath.Join(gd, name), "")
	if !ok {
		return false
	}
	got, ok := resolveFrom(gd, p)
	return ok && got == want
}

// configsClean reports whether neither config file git reads from the git
// directory g is risky: config, and config.worktree, which git reads when
// extensions.worktreeConfig is set.
func configsClean(g string) bool {
	return !configRisky(filepath.Join(g, "config")) &&
		!configRisky(filepath.Join(g, "config.worktree"))
}

// maxGitConfig bounds the config files belt scans. A real repo's config
// is a few kilobytes; a bigger one gets no answer.
const maxGitConfig = 1 << 20

// configRisky reports whether a git config file moves or bares the work
// tree (core.worktree, core.bare set to anything but false), points hooks
// elsewhere (core.hooksPath), or pulls in another file (any include or
// includeIf section). A missing file is not risky. A file the scan cannot
// read with certainty is: unreadable, too big, a line continuation (which
// can make git read a later line as part of a value), or a malformed
// section header or key. The scan errs toward risky wherever it and git's
// parser could disagree.
func configRisky(path string) bool {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxGitConfig {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, `\`) {
			return true
		}
		for strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return true
			}
			name, ok := configSection(line[1:end])
			if !ok {
				return true
			}
			section, line = name, strings.TrimSpace(line[end+1:])
		}
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		key, value, ok := configEntry(line)
		if !ok || riskyConfigEntry(section, key, value) {
			return true
		}
	}
	return false
}

// configSection names the section a header opens: its lowercased name, with
// a trailing "." when it has a subsection (quoted, or the old dotted form).
// ok is false for a header the scan cannot read.
func configSection(header string) (string, bool) {
	header = strings.TrimSpace(header)
	i := strings.IndexAny(header, " \t\"")
	if i < 0 {
		i = len(header)
	}
	name, sub := header[:i], strings.TrimSpace(header[i:])
	if name == "" || name[0] == '.' ||
		strings.ContainsFunc(name, func(r rune) bool { return r != '.' && !isConfigKeyChar(r) }) {
		return "", false
	}
	base, _, dotted := strings.Cut(name, ".")
	quoted := len(sub) >= 2 && sub[0] == '"' && sub[len(sub)-1] == '"'
	switch {
	case sub == "" && !dotted:
		return strings.ToLower(base), true
	case sub == "" || quoted:
		return strings.ToLower(base) + ".", true
	}
	return "", false
}

// isConfigKeyChar reports whether r may appear in a config key or
// section name: a letter, a digit or -, as git's iskeychar. A section
// name may also hold dots.
func isConfigKeyChar(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '-'
}

// configEntry splits a key line into its lowercased key and its value with
// quotes removed and any trailing comment cut. A key with no = has the
// empty value. ok is false for a line that is not a key line.
func configEntry(line string) (key, value string, ok bool) {
	n := strings.IndexFunc(line, func(r rune) bool { return !isConfigKeyChar(r) })
	if n < 0 {
		n = len(line)
	}
	key, rest := strings.ToLower(line[:n]), strings.TrimSpace(line[n:])
	if key == "" {
		return "", "", false
	}
	if rest == "" {
		return key, "", true
	}
	value, isValue := strings.CutPrefix(rest, "=")
	if !isValue {
		return "", "", false
	}
	value = strings.ReplaceAll(value, `"`, "")
	if i := strings.IndexAny(value, "#;"); i >= 0 {
		value = value[:i]
	}
	return key, strings.ToLower(strings.TrimSpace(value)), true
}

// riskyConfigEntry reports whether one key in one section is a key
// configRisky refuses.
func riskyConfigEntry(section, key, value string) bool {
	switch section {
	case "include", "include.", "includeif", "includeif.":
		return true
	case "core":
		switch key {
		case "worktree", "hookspath":
			return true
		case "bare":
			return !slices.Contains([]string{"false", "no", "off", "0"}, value)
		}
	}
	return false
}

// gitEnvRedirects are the variables that make git read a repo, index or
// object store other than the one discovery finds. belt sees only its own
// environment, which the session's shell inherits; a shell rc file can
// still set them out of belt's sight (docs/adr/0032).
var gitEnvRedirects = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
}

// gitEnvRedirected reports whether the hook's environment sets any of
// gitEnvRedirects.
func gitEnvRedirected() bool {
	return slices.ContainsFunc(gitEnvRedirects, func(k string) bool { return os.Getenv(k) != "" })
}
