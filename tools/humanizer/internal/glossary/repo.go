package glossary

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// contextFile is the glossary file name the walk collects.
const contextFile = "CONTEXT.md"

// skippedDirs are directories the walk never enters, besides dot-directories:
// vendored and fixture trees carry other projects' glossaries.
var skippedDirs = map[string]bool{
	"node_modules": true,
	"testdata":     true,
	"vendor":       true,
}

// FromRepo renders the substitution rule for every CONTEXT.md under root.
// It returns nil when the repo has no glossary entries, so the caller can run
// without a glossary rule. Errors wrap the underlying fs error, so a sandbox
// denial still matches errors.Is(err, fs.ErrPermission).
func FromRepo(root string) ([]byte, error) {
	paths, err := findContextFiles(root)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, p := range paths {
		got, err := parseFile(p)
		if err != nil {
			return nil, err
		}
		entries = append(entries, got...)
	}
	subs := Substitutions(entries)
	if len(subs) == 0 {
		return nil, nil
	}
	return Render(subs, MultiWordTerms(entries)), nil
}

// findContextFiles walks root in lexical order and returns every CONTEXT.md,
// skipping dot-directories and skippedDirs.
func findContextFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("glossary: repo %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("glossary: repo %s is not a directory", root)
	}
	var paths []string
	walk := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("glossary: walk %s: %w", p, err)
		}
		if d.IsDir() {
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == contextFile {
			paths = append(paths, p)
		}
		return nil
	}
	if err := filepath.WalkDir(root, walk); err != nil {
		return nil, err
	}
	return paths, nil
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || skippedDirs[name]
}

func parseFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("glossary: open %s: %w", path, err)
	}
	defer f.Close()
	entries, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return entries, nil
}
