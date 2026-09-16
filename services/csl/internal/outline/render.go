package outline

import (
	"fmt"
	"strings"
)

// Render prints an outline the way csl_search prints matches: a `repo/path`
// header per file, files in rank order of their best symbol, and under each
// one line per symbol, `LINE kind name (parent)  refs=N`, in rank order. A
// trailer carries the counts and, when the list was capped, how to see
// more. The CLI and the MCP text format print this same text, with no
// trailing newline.
func Render(r Result) string {
	if len(r.Symbols) == 0 {
		msg := fmt.Sprintf("no symbols in %s/%s; %s scanned",
			r.Repo, r.Path, plural(r.FilesScanned, "file", "files"))
		if r.SymbolsSkipped > 0 {
			msg += fmt.Sprintf(
				"; %s of other kinds skipped (set kinds; fields, enumerators, and headings are out by default)",
				plural(r.SymbolsSkipped, "definition", "definitions"),
			)
		}
		return msg
	}
	groups := groupByFile(r.Symbols)
	var lines []string
	for _, g := range groups {
		lines = append(lines, r.Repo+"/"+g.file)
		for _, e := range g.entries {
			lines = append(lines, entryLine(e))
		}
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("%s in %s; %s scanned",
		plural(len(r.Symbols), "symbol", "symbols"),
		plural(len(groups), "file", "files"),
		plural(r.FilesScanned, "file", "files")))
	if r.SymbolsTotal > len(r.Symbols) {
		lines = append(lines, fmt.Sprintf(
			"truncated: showing %d of %d symbols; raise limit or narrow with path/kinds",
			len(r.Symbols), r.SymbolsTotal,
		))
	}
	if r.FilesCapped {
		lines = append(lines, fmt.Sprintf(
			"truncated: the walk stopped at %d files (max_files); definitions and counts cover those files only",
			r.FilesScanned,
		))
	}
	return strings.Join(lines, "\n")
}

// entryLine is `LINE kind name (parent)  refs=N`, the parent only when set.
func entryLine(e Entry) string {
	name := e.Name
	if e.Parent != "" {
		name += " (" + e.Parent + ")"
	}
	return fmt.Sprintf("%d %s %s  refs=%d", e.Line, e.Kind, name, e.Refs)
}

// fileGroup is one file's symbols in rank order.
type fileGroup struct {
	file    string
	entries []Entry
}

// groupByFile buckets ranked entries by file, files ordered by their first
// (best) entry.
func groupByFile(entries []Entry) []fileGroup {
	var groups []fileGroup
	index := make(map[string]int)
	for _, e := range entries {
		i, ok := index[e.File]
		if !ok {
			i = len(groups)
			index[e.File] = i
			groups = append(groups, fileGroup{file: e.File})
		}
		groups[i].entries = append(groups[i].entries, e)
	}
	return groups
}

// plural formats n with the singular or plural noun.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
