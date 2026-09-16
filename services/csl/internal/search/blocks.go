package search

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// FileBlocks is one file's share of a content-mode result: its matched lines
// and the context around them, merged so that each line prints once even when
// several nearby hits asked for it. Blocks are the contiguous runs; the gap
// between two blocks is where a renderer prints `--`.
type FileBlocks struct {
	// Repo is the org/repo name.
	Repo string
	// File is the path relative to the repo root.
	File string
	// Blocks are the contiguous line runs in ascending line order.
	Blocks []Block
}

// Block is a run of consecutive lines from one file.
type Block struct {
	// Lines holds the run in ascending line order.
	Lines []BlockLine
}

// BlockLine is one printable line of a block: a matched line or a context line.
type BlockLine struct {
	// Number is the 1-based line number.
	Number int
	// Text is the line's content without its newline.
	Text string
	// Match is true for a matched line, false for a context line.
	Match bool
	// Kind and Parent carry a sym: hit's symbol info (see Match); both are
	// empty for plain hits and context lines.
	Kind   string
	Parent string
}

// Blocks merges per-line matches into blocks. Files keep the order in which
// they first appear (zoekt's rank order); within a file lines sort ascending
// and every line number appears once. Overlapping or adjacent context windows
// collapse into one run, and a line that is both a hit and another hit's
// context is a match, with the hit's own text and symbol info.
func Blocks(matches []Match) []FileBlocks {
	var files []*fileLines
	byName := make(map[string]*fileLines)
	for _, m := range matches {
		key := m.Repo + "/" + m.File
		f, ok := byName[key]
		if !ok {
			f = &fileLines{repo: m.Repo, file: m.File, lines: map[int]BlockLine{}}
			byName[key] = f
			files = append(files, f)
		}
		f.add(m)
	}
	out := make([]FileBlocks, 0, len(files))
	for _, f := range files {
		out = append(out, f.blocks())
	}
	return out
}

// Render returns the file in ripgrep's --heading grammar: the `repo/path`
// header, then one line per BlockLine (see BlockLine.String) with `--`
// between blocks. The CLI's content mode and the MCP text format both print
// this form, so it is the one place the grammar lives.
func (f FileBlocks) Render() []string {
	out := []string{f.Repo + "/" + f.File}
	for i, b := range f.Blocks {
		if i > 0 {
			out = append(out, "--")
		}
		for _, l := range b.Lines {
			out = append(out, l.String())
		}
	}
	return out
}

// String formats the line as `LINE:text` for a match and `LINE-text` for
// context. A sym: hit's line ends with `  kind=<kind>`, plus `parent=<name>`
// when the definition is nested, the same tokens the semantic renderer uses.
func (l BlockLine) String() string {
	sep := "-"
	if l.Match {
		sep = ":"
	}
	s := fmt.Sprintf("%d%s%s", l.Number, sep, l.Text)
	if l.Kind == "" {
		return s
	}
	s += "  kind=" + l.Kind
	if l.Parent != "" {
		s += " parent=" + l.Parent
	}
	return s
}

// fileLines accumulates one file's printable lines keyed by line number, so
// overlapping or adjacent context between hits is recorded once.
type fileLines struct {
	repo  string
	file  string
	lines map[int]BlockLine
}

// add records one match and its context. Before and After are "\n"-joined
// blocks, so their line numbers derive from the match line.
func (f *fileLines) add(m Match) {
	before := contextLines(m.Before)
	for i, text := range before {
		f.addContext(m.Line-len(before)+i, text)
	}
	f.lines[m.Line] = BlockLine{
		Number: m.Line,
		Text:   m.Text,
		Match:  true,
		Kind:   m.Kind,
		Parent: m.Parent,
	}
	for i, text := range contextLines(m.After) {
		f.addContext(m.Line+1+i, text)
	}
}

// addContext keeps the first text seen for a line; a match recorded later
// overwrites it in add because the match line's text is authoritative.
func (f *fileLines) addContext(n int, text string) {
	if _, ok := f.lines[n]; !ok {
		f.lines[n] = BlockLine{Number: n, Text: text}
	}
}

// blocks sorts the lines and splits them wherever consecutive line numbers
// are not adjacent in the file.
func (f *fileLines) blocks() FileBlocks {
	fb := FileBlocks{Repo: f.repo, File: f.file}
	var cur Block
	for i, n := range slices.Sorted(maps.Keys(f.lines)) {
		if i > 0 && n != cur.Lines[len(cur.Lines)-1].Number+1 {
			fb.Blocks = append(fb.Blocks, cur)
			cur = Block{}
		}
		cur.Lines = append(cur.Lines, f.lines[n])
	}
	if len(cur.Lines) > 0 {
		fb.Blocks = append(fb.Blocks, cur)
	}
	return fb
}

// contextLines splits a "\n"-joined context block into lines. A trailing
// newline closes the last line rather than adding an empty one.
func contextLines(block string) []string {
	if block == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(block, "\n"), "\n")
}
