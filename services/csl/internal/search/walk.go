package search

import (
	"io/fs"
	"maps"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt/index"

	"github.com/mad01/thismoon/services/csl/internal/cslignore"
)

// WalkFile is one regular file the repo walk visits.
type WalkFile struct {
	// Rel is the path relative to the repo root.
	Rel string
	// Abs is the absolute path.
	Abs string
	// Size is the file size in bytes.
	Size int64
}

// skipDirs are the directory names the walk never enters, on top of every
// hidden directory outside the allowlist.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	"build":        true,
	"dist":         true,
	"target":       true,
}

// allowedHiddenDirs are the hidden directory names the walk enters by
// default: CI workflows and agent configuration a developer searches for,
// never a cache, a virtualenv, or build output. A machine adds names with
// the index.allow_hidden_dirs config key; nothing removes one, and .git
// never joins. A file under an allowed hidden directory counts only when
// git tracks it, so local-only litter (a gitignored
// .claude/settings.local.json) stays out.
var allowedHiddenDirs = map[string]bool{
	".github": true,
	".claude": true,
}

// WalkRepo visits every file of a working tree the lexical index holds, in
// lexical path order. It is the one place the skip rules live: hidden
// directories other than the allowed ones (.git among them), the skipDirs
// trees, a linked worktree's .git pointer file, untracked files under an
// allowed hidden directory, anything that is not a regular file, files over
// sizeMax bytes, and paths the repo's .cslignore matches. hiddenDirs names
// further hidden directories to enter (the config's index.allow_hidden_dirs).
// Entries that cannot be read are skipped rather than reported. An error
// from visit stops the walk and is returned unchanged.
func WalkRepo(root string, sizeMax int64, hiddenDirs []string, visit func(WalkFile) error) error {
	ignore := cslignore.Load(root)
	allow := hiddenAllowlist(hiddenDirs)
	tracked := &trackedFiles{root: root}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // best-effort, skip unreadable entries
		}
		if d.IsDir() {
			return dirAction(root, path, allow)
		}
		// A linked worktree's .git pointer is a regular file ("gitdir: …"),
		// so the hidden-directory rule misses it.
		if filepath.Base(path) == ".git" || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > sizeMax {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || ignore.Match(rel) {
			return nil
		}
		if underHiddenDir(rel) && !tracked.has(rel) {
			return nil
		}
		return visit(WalkFile{Rel: rel, Abs: path, Size: info.Size()})
	})
}

// hiddenAllowlist merges the built-in allowed hidden directories with the
// configured extras. .git is never entered whatever the config says.
func hiddenAllowlist(extra []string) map[string]bool {
	allow := make(map[string]bool, len(allowedHiddenDirs)+len(extra))
	maps.Copy(allow, allowedHiddenDirs)
	for _, name := range extra {
		allow[name] = true
	}
	delete(allow, ".git")
	return allow
}

// dirAction skips generated directories and hidden ones outside allow; the
// root is entered whatever its name.
func dirAction(root, path string, allow map[string]bool) error {
	if path == root {
		return nil
	}
	base := filepath.Base(path)
	if skipDirs[base] || (strings.HasPrefix(base, ".") && !allow[base]) {
		return filepath.SkipDir
	}
	return nil
}

// underHiddenDir reports whether rel sits below a hidden directory. The
// walk enters only allowed hidden directories, so a hit means one of them.
func underHiddenDir(rel string) bool {
	dir := filepath.Dir(rel)
	if dir == "." {
		return false
	}
	for seg := range strings.SplitSeq(filepath.ToSlash(dir), "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// trackedFiles is the set of paths git tracks under a root, listed on first
// use so a repo with no allowed hidden directory never pays for it.
type trackedFiles struct {
	root   string
	loaded bool
	set    map[string]bool
}

// has reports whether git tracks rel. Outside a git work tree, or with no
// git on PATH, nothing is tracked.
func (t *trackedFiles) has(rel string) bool {
	if !t.loaded {
		t.loaded = true
		t.set = listTracked(t.root)
	}
	return t.set[rel]
}

// listTracked runs git ls-files under root and returns the tracked paths in
// the platform's path form. A failure leaves the set empty: a directory that
// is not a git work tree tracks nothing.
func listTracked(root string) map[string]bool {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			set[filepath.FromSlash(p)] = true
		}
	}
	return set
}

// IndexSizeMax is the largest file, in bytes, the lexical index takes:
// zoekt's own default, so a walk made outside the indexer sees the same
// files the index holds.
func IndexSizeMax() int64 {
	var opts index.Options
	opts.SetDefaults()
	return int64(opts.SizeMax)
}
