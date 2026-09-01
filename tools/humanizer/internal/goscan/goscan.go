// Package goscan extracts human-facing prose from Go source: doc comments,
// cobra command fields, MCP tool descriptions, error messages, and the text
// in jsonschema struct tags. Every extracted block carries the file and line
// it came from, so a prose finding traces back to the declaration that
// produced it.
//
// Extraction is pure parsing (go/parser, go/ast) with no detection in it.
// Callers run the detectors themselves: the CLI over Render's document, an
// MCP tool over the blocks.
package goscan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Kind classifies the site a block of prose was extracted from.
type Kind string

const (
	// KindDoc is a documentation comment on a package or declaration.
	KindDoc Kind = "doc"
	// KindCobra is a cobra command field: Use, Short, Long, or Example.
	KindCobra Kind = "cobra"
	// KindMCP is a Description field, the shape MCP tool definitions use.
	KindMCP Kind = "mcp"
	// KindError is the message argument of fmt.Errorf, errors.New, or
	// http.Error.
	KindError Kind = "error"
	// KindSchema is the prose in a jsonschema struct tag.
	KindSchema Kind = "schema"
)

// Kinds returns every extraction kind.
func Kinds() []Kind {
	return []Kind{KindDoc, KindCobra, KindMCP, KindError, KindSchema}
}

// ParseKind maps a kind name to its Kind, reporting whether it is known.
func ParseKind(name string) (Kind, bool) {
	k := Kind(name)
	return k, slices.Contains(Kinds(), k)
}

// Block is one span of prose extracted from a Go source file.
type Block struct {
	// File is the path the block was read from, as given to Scan.
	File string `json:"file"`
	// Line is the 1-based line the prose starts on.
	Line int `json:"line"`
	// Kind is the extraction site.
	Kind Kind `json:"kind"`
	// Label names the declaration, field, or call behind the prose,
	// e.g. "func runScan", "Short", "fmt.Errorf".
	Label string `json:"label,omitempty"`
	// Text is the prose itself, with comment markers and quoting removed.
	Text string `json:"text"`
}

// File groups the blocks extracted from one Go source file.
type File struct {
	Path   string  `json:"path"`
	Blocks []Block `json:"blocks"`
}

// Options tunes a scan.
type Options struct {
	// Kinds restricts extraction to these kinds. Empty means every kind.
	Kinds []Kind
	// Recursive controls whether a directory path descends into
	// subdirectories. Nil or true walks recursively, the existing
	// default; false scans only the direct children of the given
	// directory. Ignored when path is a single file.
	Recursive *bool
}

func (o Options) wants(k Kind) bool {
	return len(o.Kinds) == 0 || slices.Contains(o.Kinds, k)
}

func (o Options) recursive() bool {
	return o.Recursive == nil || *o.Recursive
}

// Scan extracts prose from path, either a single Go file or a directory
// walked recursively (Options.Recursive set to false limits the walk to the
// directory's direct children). Directories named vendor, node_modules, or
// testdata and dot-directories are skipped, as are generated files (a
// .pb.go name or a "Code generated ... DO NOT EDIT." header). Test files
// are included: they carry prose too. Files with no prose are left out of
// the result.
func Scan(path string, opts Options) ([]File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("goscan: stat %s: %w", path, err)
	}
	if !info.IsDir() {
		blocks, err := scanFile(path, opts)
		if err != nil {
			return nil, err
		}
		if len(blocks) == 0 {
			return nil, nil
		}
		return []File{{Path: path, Blocks: blocks}}, nil
	}
	var out []File
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip it, don't abort the walk
		}
		if d.IsDir() {
			if p != path && skipDir(d.Name()) {
				return fs.SkipDir
			}
			if p != path && !opts.recursive() {
				return fs.SkipDir
			}
			return nil
		}
		if !isGoSource(d.Name()) {
			return nil
		}
		blocks, err := scanFile(p, opts)
		if err != nil {
			return err
		}
		if len(blocks) > 0 {
			out = append(out, File{Path: p, Blocks: blocks})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanFile reads one Go file and extracts its prose. A generated file
// yields nothing.
func scanFile(path string, opts Options) ([]Block, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("goscan: read %s: %w", path, err)
	}
	if isGenerated(string(src)) {
		return nil, nil
	}
	return ScanSource(path, string(src), opts)
}

// skipDir names directories a source walk never descends into. Dot-
// directories go too: .git holds no source, and .claude holds worktrees
// that would scan the whole repo a second time.
func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "testdata":
		return true
	}
	return false
}

// isGoSource reports whether name is a Go file worth parsing. Protobuf
// output is prose-free boilerplate, so it never is.
func isGoSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".pb.go")
}

// isGenerated reports whether src carries the standard generated-code
// header, which by convention appears before the package clause.
func isGenerated(src string) bool {
	for line := range strings.Lines(src) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if strings.HasPrefix(line, "// Code generated ") &&
			strings.HasSuffix(line, " DO NOT EDIT.") {
			return true
		}
	}
	return false
}
