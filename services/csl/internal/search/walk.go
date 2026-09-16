package search

import (
	"io/fs"
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
// hidden directory.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	"build":        true,
	"dist":         true,
	"target":       true,
}

// WalkRepo visits every file of a working tree the lexical index holds, in
// lexical path order. It is the one place the skip rules live: hidden
// directories (.git among them), the skipDirs trees, a linked worktree's
// .git pointer file, anything that is not a regular file, files over
// sizeMax bytes, and paths the repo's .cslignore matches. Entries that
// cannot be read are skipped rather than reported. An error from visit
// stops the walk and is returned unchanged.
func WalkRepo(root string, sizeMax int64, visit func(WalkFile) error) error {
	ignore := cslignore.Load(root)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // best-effort, skip unreadable entries
		}
		if d.IsDir() {
			return dirAction(root, path)
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
		return visit(WalkFile{Rel: rel, Abs: path, Size: info.Size()})
	})
}

// dirAction skips hidden and generated directories; the root is entered
// whatever its name.
func dirAction(root, path string) error {
	if path == root {
		return nil
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") || skipDirs[base] {
		return filepath.SkipDir
	}
	return nil
}

// IndexSizeMax is the largest file, in bytes, the lexical index takes:
// zoekt's own default, so a walk made outside the indexer sees the same
// files the index holds.
func IndexSizeMax() int64 {
	var opts index.Options
	opts.SetDefaults()
	return int64(opts.SizeMax)
}
