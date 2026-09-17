// Package outline ranks a repo's definitions by how many other files
// reference them, so an agent asking how a service is structured gets the
// important types and functions first, in one call instead of a run of
// searches and reads. It reads the working tree through the same walk the
// lexical index uses and never touches the index: definitions come from the
// tree-sitter extractor, the rank from one identifier pass over every walked
// file. A repo map in the Aider sense, with a cross-file reference count
// where Aider runs PageRank.
package outline

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/symbols"
)

const (
	// DefaultLimit is the number of symbols returned when the caller sets
	// no limit.
	DefaultLimit = 100
	// MaxLimit is the hard cap on the number of symbols returned.
	MaxLimit = 500
	// DefaultMaxFiles is the number of files a walk stops at when the caller
	// sets no MaxFiles, so a huge repo answers late rather than never.
	DefaultMaxFiles = 20000
)

// Options narrows and caps an outline.
type Options struct {
	// Path is a directory (or file) inside the repo; only files under it
	// contribute definitions. Empty means the whole repo. Reference counting
	// covers the whole repo either way, so a package's public API ranks by
	// repo-wide use.
	Path string
	// Kinds keeps only definitions of these kinds, in zoekt's kind
	// vocabulary; empty means DefaultKinds.
	Kinds []string
	// Limit caps the ranked list: 0 means DefaultLimit, and values over
	// MaxLimit are clamped to it.
	Limit int
	// IncludeTests keeps test files in the walk. By default they are left
	// out of both the definitions and the reference count, so the rank
	// reflects production use.
	IncludeTests bool
	// MaxFiles stops the walk after this many files, in path order, and
	// marks the result truncated; 0 means DefaultMaxFiles.
	MaxFiles int
	// HiddenDirs names the hidden directories the walk enters on top of the
	// built-in ones: the config's index.allow_hidden_dirs, so the outline
	// sees the files the index holds.
	HiddenDirs []string
}

// Entry is one ranked definition.
type Entry struct {
	Name   string `json:"name"             jsonschema:"the defined name"`
	Kind   string `json:"kind"             jsonschema:"zoekt kind: function, method, struct, interface, type, ..."`
	Parent string `json:"parent,omitempty" jsonschema:"enclosing declaration (receiver type, class, message); absent at top level"`
	File   string `json:"file"             jsonschema:"defining file, relative to the repo root"`
	Line   int    `json:"line"             jsonschema:"1-based line of the definition"`
	Refs   int    `json:"refs"             jsonschema:"number of other walked files holding the name as a whole identifier"`
}

// Result is the outline of one repo scope and the csl_outline tool's output
// object.
type Result struct {
	Repo           string  `json:"repo"                      jsonschema:"the resolved repo name (canonical org/repo form)"`
	Path           string  `json:"path"                      jsonschema:"the scope relative to the repo root ('.' for the whole repo)"`
	FilesScanned   int     `json:"files_scanned"             jsonschema:"files tokenized for references (the whole repo, minus skipped and test files)"`
	SymbolsTotal   int     `json:"symbols_total"             jsonschema:"definitions found in scope before the limit"`
	SymbolsSkipped int     `json:"symbols_skipped,omitempty" jsonschema:"definitions in scope left out by kinds (fields, enumerators, and headings by default)"`
	Truncated      bool    `json:"truncated"                 jsonschema:"true when symbols_total exceeds the limit, or when the walk stopped at max_files"`
	FilesCapped    bool    `json:"files_capped,omitempty"    jsonschema:"true when the walk stopped at max_files, so definitions and counts cover the first files_scanned files in path order only"`
	Symbols        []Entry `json:"symbols"                   jsonschema:"definitions ranked by refs desc, then kind, then name"`
}

// Build outlines repo under opts. A file whose symbols cannot be extracted
// is logged and still counted for references; only a bad scope or a walk
// failure fails the call.
func Build(repo finder.Repo, opts Options) (Result, error) {
	scope, err := cleanScope(repo.Path, opts.Path)
	if err != nil {
		return Result{}, err
	}
	keep, err := kindFilter(opts.Kinds)
	if err != nil {
		return Result{}, err
	}
	files, capped, err := listFiles(repo.Path, opts.IncludeTests, opts.MaxFiles, opts.HiddenDirs)
	if err != nil {
		return Result{}, fmt.Errorf("outline: walk %s: %w", repo.Path, err)
	}

	// Files go directory by directory so package-private references can be
	// counted inside the package they belong to (see refCounter.refs).
	var defs []Entry
	skipped := 0
	refs := newRefCounter()
	for _, group := range byDir(files) {
		refs.beginDir()
		dirStart := len(defs)
		for _, f := range group {
			content, err := os.ReadFile(f.Abs)
			if err != nil || bytes.IndexByte(content, 0) >= 0 {
				continue // unreadable or binary: nothing to define or reference
			}
			refs.addFile(content)
			for _, e := range extract(f.Rel, content) {
				refs.addDefiner(e.Name, e.Kind)
				switch {
				case !scope.contains(f.Rel):
				case keep[e.Kind]:
					defs = append(defs, e)
				default:
					skipped++
				}
			}
		}
		for i := dirStart; i < len(defs); i++ {
			if isPackagePrivate(defs[i]) {
				defs[i].Refs = refs.refs(defs[i])
			}
		}
	}
	for i := range defs {
		if !isPackagePrivate(defs[i]) {
			defs[i].Refs = refs.refs(defs[i])
		}
	}
	rank(defs)

	limit := clampLimit(opts.Limit)
	res := Result{
		Repo:           repo.Name,
		Path:           scope.String(),
		FilesScanned:   refs.files,
		SymbolsTotal:   len(defs),
		SymbolsSkipped: skipped,
		Truncated:      len(defs) > limit || capped,
		FilesCapped:    capped,
		Symbols:        defs[:min(len(defs), limit)],
	}
	if res.Symbols == nil {
		res.Symbols = []Entry{}
	}
	return res, nil
}

// errFilesCapped stops the walk once maxFiles files are listed.
var errFilesCapped = errors.New("outline: max_files reached")

// listFiles walks the repo with the indexer's rules and returns the files
// to scan, test files left out unless asked for. capped reports that the
// walk stopped at maxFiles (DefaultMaxFiles when 0) with files unvisited.
func listFiles(
	root string,
	includeTests bool,
	maxFiles int,
	hiddenDirs []string,
) ([]search.WalkFile, bool, error) {
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	var files []search.WalkFile
	err := search.WalkRepo(root, search.IndexSizeMax(), hiddenDirs, func(f search.WalkFile) error {
		if !includeTests && IsTestFile(f.Rel) {
			return nil
		}
		if len(files) == maxFiles {
			return errFilesCapped
		}
		files = append(files, f)
		return nil
	})
	if errors.Is(err, errFilesCapped) {
		return files, true, nil
	}
	return files, false, err
}

// byDir yields the files grouped by directory, directories and files in
// path order. The walk interleaves a directory's files with its
// subdirectories, so the groups are formed here.
func byDir(files []search.WalkFile) iter.Seq2[string, []search.WalkFile] {
	slices.SortStableFunc(files, func(a, b search.WalkFile) int {
		return strings.Compare(filepath.Dir(a.Rel), filepath.Dir(b.Rel))
	})
	return func(yield func(string, []search.WalkFile) bool) {
		for start := 0; start < len(files); {
			dir := filepath.Dir(files[start].Rel)
			end := start
			for end < len(files) && filepath.Dir(files[end].Rel) == dir {
				end++
			}
			if !yield(dir, files[start:end]) {
				return
			}
			start = end
		}
	}
}

// extract returns the definitions in one file, with names and line numbers
// resolved. An extraction failure is logged and costs the file its
// definitions, never the call.
func extract(rel string, content []byte) []Entry {
	syms, err := symbols.Extract(rel, content)
	if err != nil {
		log.Printf("csl: outline %s: %v (definitions skipped)", rel, err)
		return nil
	}
	out := make([]Entry, 0, len(syms))
	lines := lineCounter{src: content, line: 1}
	for _, s := range syms {
		out = append(out, Entry{
			Name:   string(content[s.Start:s.End]),
			Kind:   s.Kind,
			Parent: s.Parent,
			File:   rel,
			Line:   lines.at(int(s.Start)),
		})
	}
	return out
}

// lineCounter turns byte offsets into 1-based line numbers, walking forward
// only: the extractor returns symbols sorted by offset, so each lookup
// counts newlines from the previous one.
type lineCounter struct {
	src  []byte
	pos  int
	line int
}

func (c *lineCounter) at(offset int) int {
	if offset > c.pos {
		c.line += bytes.Count(c.src[c.pos:offset], []byte{'\n'})
		c.pos = offset
	}
	return c.line
}

// rank orders definitions by references (most first), then kind weight, then
// name. The sort is stable, so ties keep walk order: file path, then offset.
func rank(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		if c := cmp.Compare(b.Refs, a.Refs); c != 0 {
			return c
		}
		if c := cmp.Compare(kindWeight(a.Kind), kindWeight(b.Kind)); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// kindWeights orders the kinds within one reference count: containers and
// types first, then callables, then members and headings. The keys are also
// the vocabulary Options.Kinds is checked against, and the weight is the
// group definers share mentions within.
var kindWeights = map[string]int{
	"interface": 0, "struct": 0, "class": 0, "type": 0, "typealias": 0, "enum": 0,
	"namespace": 0,
	"function":  1, "method": 1, "methodSpec": 1,
	"const": 2, "var": 2, "field": 2, "enumerator": 2, "section": 2,
}

// kindGroups is the number of weights, the last one for kinds outside the
// vocabulary.
const kindGroups = 4

// kindWeight returns a kind's sort weight; a kind outside the vocabulary
// sorts last.
func kindWeight(kind string) int {
	if w, ok := kindWeights[kind]; ok {
		return w
	}
	return kindGroups - 1
}

// defaultSkipped are the kinds left out unless Options.Kinds names them:
// members of a type and document headings, which say little about how a
// repo is structured and whose common names (name, file, Overview) would
// otherwise crowd out the types and functions.
var defaultSkipped = map[string]bool{"field": true, "enumerator": true, "section": true}

// DefaultKinds lists the kinds an outline shows when the caller names none:
// every kind but field, enumerator, and section.
func DefaultKinds() []string {
	var kinds []string
	for _, k := range Kinds() {
		if !defaultSkipped[k] {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// Kinds lists the kind vocabulary, grouped by weight and alphabetical within
// a group, for help text and error messages.
func Kinds() []string {
	kinds := slices.Collect(func(yield func(string) bool) {
		for k := range kindWeights {
			if !yield(k) {
				return
			}
		}
	})
	slices.SortFunc(kinds, func(a, b string) int {
		if c := cmp.Compare(kindWeights[a], kindWeights[b]); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return kinds
}

// kindSet is the kinds a build keeps.
type kindSet map[string]bool

// kindFilter validates the requested kinds against the vocabulary; none
// requested means DefaultKinds.
func kindFilter(kinds []string) (kindSet, error) {
	if len(kinds) == 0 {
		kinds = DefaultKinds()
	}
	set := make(kindSet, len(kinds))
	for _, k := range kinds {
		k = strings.TrimSpace(k)
		if _, ok := kindWeights[k]; !ok {
			return nil, fmt.Errorf(
				"outline: unknown kind %q; kinds are %s", k, strings.Join(Kinds(), ", "),
			)
		}
		set[k] = true
	}
	return set, nil
}

// scope is the cleaned Options.Path: empty for the whole repo, else a
// repo-relative path whose subtree contributes definitions.
type scope string

// contains reports whether rel is the scope itself or lies under it.
func (s scope) contains(rel string) bool {
	if s == "" || rel == string(s) {
		return true
	}
	return strings.HasPrefix(rel, string(s)+string(filepath.Separator))
}

// String renders the scope the way csl_ls renders its path: "." for the
// root.
func (s scope) String() string {
	if s == "" {
		return "."
	}
	return string(s)
}

// cleanScope normalizes a caller's path and checks it exists in the repo; a
// path that escapes the root is refused.
func cleanScope(root, path string) (scope, error) {
	p := filepath.Clean(strings.TrimSpace(path))
	if p == "." || p == "" {
		return "", nil
	}
	if filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("outline: path %q must be relative to the repo root", path)
	}
	if _, err := os.Stat(filepath.Join(root, p)); err != nil {
		return "", fmt.Errorf("outline: path %q: %w", p, err)
	}
	return scope(p), nil
}

// clampLimit applies the default and the hard cap.
func clampLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	return min(n, MaxLimit)
}
