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

// Repo is the CONTEXT.md glossaries found under one repo root.
type Repo struct {
	// root is the absolute repo root with symlinks resolved, so Target can
	// compare it with a resolved file path.
	root     string
	contexts []Context
}

// Load walks root and parses every CONTEXT.md under it. Errors wrap the
// underlying fs error, so a sandbox denial still matches
// errors.Is(err, fs.ErrPermission).
func Load(root string) (*Repo, error) {
	paths, err := findContextFiles(root)
	if err != nil {
		return nil, err
	}
	resolved, err := resolvePath(root)
	if err != nil {
		return nil, fmt.Errorf("glossary: repo %s: %w", root, err)
	}
	repo := &Repo{root: resolved}
	for _, p := range paths {
		ctx, err := parseFile(p)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, fmt.Errorf("glossary: %s: %w", p, err)
		}
		ctx.Path = filepath.ToSlash(rel)
		repo.contexts = append(repo.contexts, ctx)
	}
	return repo, nil
}

// Target returns p as the slash path relative to the repo root that Rule
// scopes by. p is absolute, or relative to the repo root. Target returns ""
// (no location, so every glossary applies) when p is empty or lies outside
// the repo.
func (r *Repo) Target(p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.root, p)
	}
	rel, err := filepath.Rel(r.root, resolveExisting(filepath.Clean(p)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Rule renders the substitution rule for target (see InScope and Target),
// or nil when no glossary entry applies there, so the caller can run
// without a glossary rule.
func (r *Repo) Rule(target string) []byte {
	return RuleFor(r.contexts, target)
}

// Warnings describes every skipped _Avoid_ piece as "path:line: ...", in
// walk order, for the caller to print.
func (r *Repo) Warnings() []string {
	var out []string
	for _, c := range r.contexts {
		for _, s := range c.Skips {
			w := fmt.Sprintf("%s:%d: skipped %q: %s", c.Path, s.Line, s.Synonym, s.Reason)
			out = append(out, w)
		}
	}
	return out
}

// resolveExisting resolves the symlinks in the longest existing prefix of
// the absolute path p, so a file that does not exist yet still compares
// with the resolved repo root. It returns p unchanged when nothing resolves.
func resolveExisting(p string) string {
	rest := ""
	for dir := p; ; {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// resolvePath returns p as an absolute path with symlinks resolved.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
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

func parseFile(path string) (Context, error) {
	f, err := os.Open(path)
	if err != nil {
		return Context{}, fmt.Errorf("glossary: open %s: %w", path, err)
	}
	defer f.Close()
	ctx, err := Parse(f)
	if err != nil {
		return Context{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return ctx, nil
}
