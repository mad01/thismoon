package main

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// listFiles lists the files under dir as sorted slash paths, skipping .git.
func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	slices.Sort(files)
	return files, err
}

// isGlob reports whether p has a glob metacharacter.
func isGlob(p string) bool {
	return strings.ContainsAny(p, `*?[\`)
}

// matchGlob matches a slash path against a glob where "**" is a whole
// segment matching zero or more segments and other segments follow
// path.Match.
func matchGlob(glob, name string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(name, "/"))
}

func matchSegments(glob, name []string) bool {
	for len(glob) > 0 {
		if glob[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegments(glob[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(glob[0], name[0]); err != nil || !ok {
			return false
		}
		glob, name = glob[1:], name[1:]
	}
	return len(name) == 0
}

// validateGlob reports a malformed glob segment; "" is valid (unset).
func validateGlob(glob string) error {
	for seg := range strings.SplitSeq(glob, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("glob %q: %w", glob, err)
		}
	}
	return nil
}
