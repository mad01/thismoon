package catalog

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ServiceInfoFile is the filename the scanner looks for in each source tree.
const ServiceInfoFile = "service-info.yaml"

// skipDir reports directory names that are never descended into during a scan.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".idea", "dist", "build":
		return true
	}
	return false
}

// ScanDir walks a single directory tree and returns every entity found in any
// service-info.yaml file, in path order. A parse/validation error in any file
// aborts the scan with an error naming the file. The walk stops early if ctx
// is cancelled.
func ScanDir(ctx context.Context, root string) ([]Entity, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ServiceInfoFile {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	sort.Strings(files)

	// Read the repo's git config once so every entity under this root can carry
	// a link to its remote. Absent or unreadable config just means no link.
	gitConfig := readGitConfig(root)

	var entities []Entity
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		parsed, err := ParseEntities(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for i := range parsed {
			parsed[i].SourcePath = f
			parsed[i].RepoURL = RepoURLFor(gitConfig, root, f)
		}
		entities = append(entities, parsed...)
	}
	return entities, nil
}

// readGitConfig returns the text of root/.git/config, or "" if it cannot be
// read (the source may not be a git repo). The empty string yields no remote.
func readGitConfig(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		return ""
	}
	return string(data)
}

// ScanPaths scans each path and concatenates the results. Missing paths are
// skipped (a registry may reference a repo that is not checked out); any other
// error aborts.
func ScanPaths(ctx context.Context, paths []string) ([]Entity, error) {
	var all []Entity
	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			continue
		}
		ents, err := ScanDir(ctx, p)
		if err != nil {
			return nil, err
		}
		all = append(all, ents...)
	}
	return all, nil
}
