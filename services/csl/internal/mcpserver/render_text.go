package mcpserver

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Text renderers for the tools whose output has a shape worth more than a
// key/value dump: searches, reads, listings, counts, and the doctor report.
// Every renderer is deterministic, emits no trailing whitespace, and is
// written for an LLM reader, so paths and line numbers stay greppable.
// Tools without a renderer fall back to markdown-kv in withFormat.

// renderSearchText renders csl_search output in ripgrep's --heading grammar:
// a `repo/path` header per file, `LINE:text` for matches, `LINE-text` for
// context, `--` between non-contiguous groups, and a trailer with counts.
func renderSearchText(out searchOutput) string {
	if out.Total == 0 {
		return renderZeroHint(out.ZeroHint)
	}
	body := renderFileMatches(out)
	if out.OutputMode == contentOutputMode {
		body = renderContentMatches(out)
	}
	if out.RelaxedQuery == "" {
		return body
	}
	return relaxedLine(out) + "\n\n" + body
}

// relaxedLine leads a relaxed result, so the reader knows which terms were
// dropped and which query the results answer.
func relaxedLine(out searchOutput) string {
	each := ""
	if len(out.DroppedTerms) > 1 {
		each = " each"
	}
	return fmt.Sprintf(
		"relaxed: dropped %s (0 files%s); showing results for `%s`",
		quoteList(out.DroppedTerms), each, out.RelaxedQuery,
	)
}

// renderFileMatches lists one `repo/path` per line. When truncated, the page
// holds exactly limit files, so the next offset is offset + total.
func renderFileMatches(out searchOutput) string {
	lines := make([]string, 0, len(out.Files)+3)
	for _, f := range out.Files {
		lines = append(lines, f.Repo+"/"+f.Path)
	}
	lines = append(lines, "", count(out.Total, "file", "files"))
	if out.Truncated {
		lines = append(lines, fmt.Sprintf(
			"truncated: showing %d of %d files; next offset=%d",
			out.Total, out.TotalAvailable, out.Offset+out.Total,
		))
	}
	return strings.Join(lines, "\n")
}

// renderContentMatches prints matches grouped per file in first-appearance
// order, with nearby hits merged into one block per context window
// (search.Blocks). Content mode caps lines, not files, so the next offset is
// derived from the files this page showed (see nextContentOffset).
func renderContentMatches(out searchOutput) string {
	files := search.Blocks(contentMatches(out.Lines))
	var lines []string
	for _, f := range files {
		lines = append(lines, f.Render()...)
		lines = append(lines, "")
	}
	lines = append(
		lines,
		count(out.Total, "line", "lines")+" in "+count(len(files), "file", "files"),
	)
	if out.Truncated {
		lines = append(lines, fmt.Sprintf(
			"truncated: showing %d of %d lines; next offset=%d",
			out.Total, out.TotalAvailable, nextContentOffset(out.Offset, len(files)),
		))
	}
	return strings.Join(lines, "\n")
}

// contentMatches maps the tool's line entries back to search matches, the
// shape the shared block merge takes, so the text form is built from the same
// page the JSON form carries.
func contentMatches(lines []searchMatchLine) []search.Match {
	matches := make([]search.Match, len(lines))
	for i, l := range lines {
		matches[i] = search.Match{
			Repo:   l.Repo,
			File:   l.Path,
			Line:   l.Line,
			Column: l.Column,
			Text:   l.Text,
			Before: l.Before,
			After:  l.After,
			Kind:   l.Kind,
			Parent: l.Parent,
		}
	}
	return matches
}

// nextContentOffset picks the offset that continues a line-capped content
// page without losing matches. The cap almost always lands inside the last
// file shown, so the next page starts at that file again: a few lines print
// twice, none are skipped. With a single file there is nothing earlier to
// repeat, so the page moves past it and progress is guaranteed.
func nextContentOffset(offset, filesShown int) int {
	if filesShown > 1 {
		return offset + filesShown - 1
	}
	return offset + 1
}

// renderZeroHint explains an empty search: how zoekt parsed the query, how
// many repos the filters covered, index age, and the targeted notes.
func renderZeroHint(hint *searchZeroHint) string {
	lines := []string{"no matches"}
	if hint == nil {
		return lines[0]
	}
	if hint.ParsedQuery != "" {
		lines = append(lines, "parsed query: "+hint.ParsedQuery)
	}
	if len(hint.TermCounts) > 0 {
		lines = append(lines, "files per term: "+termCountsLine(hint.TermCounts))
	}
	lines = append(lines, fmt.Sprintf(
		"repos searched: %d (%d indexed, %d discovered)",
		hint.ReposSearched, hint.ReposIndexed, hint.ReposDiscovered,
	))
	if hint.NewestIndexedAt != "" {
		lines = append(lines, fmt.Sprintf(
			"index age: newest %s, oldest %s", hint.NewestIndexedAt, hint.OldestIndexedAt,
		))
	}
	for _, n := range hint.Notes {
		lines = append(lines, "- "+n)
	}
	return strings.Join(lines, "\n")
}

// termCountsLine renders term_counts as `retry=37, backoff=12, jitter=0`.
func termCountsLine(counts []termCount) string {
	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = fmt.Sprintf("%s=%d", c.Term, c.Files)
	}
	return strings.Join(parts, ", ")
}

// renderSemanticText prints one header per hit (`repo/path:START-END
// score=0.83 kind=func`), the snippet, a blank line, and a hit count.
func renderSemanticText(out semanticSearchOutput) string {
	if !out.Available {
		return unavailableNote(out.Note)
	}
	var lines []string
	for _, h := range out.Hits {
		header := fmt.Sprintf(
			"%s/%s:%d-%d score=%.2f",
			h.Repo,
			h.Path,
			h.StartLine,
			h.EndLine,
			h.Score,
		)
		if h.Kind != "" {
			header += " kind=" + h.Kind
		}
		lines = append(lines, header)
		lines = appendSnippet(lines, h.Snippet)
		lines = append(lines, "")
	}
	lines = append(lines, count(len(out.Hits), "hit", "hits"))
	return strings.Join(lines, "\n")
}

// renderHybridText prints the fused hits with each backend's evidence. When
// the semantic side is unavailable the hits are lexical-only, so the note
// leads the output instead of replacing it.
func renderHybridText(out hybridSearchOutput) string {
	var lines []string
	if !out.SemanticAvailable {
		lines = append(lines, "lexical-only results; "+unavailableNote(out.Note))
	}
	for _, h := range out.Hits {
		lines = append(lines, hybridHeader(h))
		if h.LexText != "" {
			lines = append(lines, fmt.Sprintf("L%d:%s", h.LexLine, h.LexText))
		}
		lines = appendSnippet(lines, h.Snippet)
		lines = append(lines, "")
	}
	lines = append(lines, count(len(out.Hits), "hit", "hits"))
	return strings.Join(lines, "\n")
}

// hybridHeader is `repo/path score=0.0312 lex=#3 L71 sem=#1 L40-60 0.83`,
// omitting the side whose rank is 0 (the file was absent from that backend).
func hybridHeader(h hybridHit) string {
	parts := []string{fmt.Sprintf("%s/%s score=%.4f", h.Repo, h.Path, h.Score)}
	if h.LexRank > 0 {
		parts = append(parts, fmt.Sprintf("lex=#%d L%d", h.LexRank, h.LexLine))
	}
	if h.SemRank > 0 {
		parts = append(
			parts,
			fmt.Sprintf("sem=#%d L%d-%d %.2f", h.SemRank, h.SemStart, h.SemEnd, h.SemScore),
		)
	}
	return strings.Join(parts, " ")
}

// appendSnippet adds the snippet's lines as-is, minus trailing newlines.
func appendSnippet(dst []string, snippet string) []string {
	if snippet == "" {
		return dst
	}
	return append(dst, strings.Split(strings.TrimRight(snippet, "\n"), "\n")...)
}

// unavailableNote returns the tool's note, or a generic line when it is empty.
func unavailableNote(note string) string {
	if note == "" {
		return "semantic search unavailable"
	}
	return note
}

// renderReadText prints `repo/path lines A-B of TOTAL` then `LINE:text`.
func renderReadText(out readOutput) string {
	name := out.Repo + "/" + out.Path
	lines := make([]string, 0, len(out.Lines)+2)
	if len(out.Lines) == 0 {
		lines = append(lines, fmt.Sprintf("%s no lines in range (%d lines)", name, out.TotalLines))
	} else {
		first, last := out.Lines[0].Number, out.Lines[len(out.Lines)-1].Number
		lines = append(lines, fmt.Sprintf("%s lines %d-%d of %d", name, first, last, out.TotalLines))
	}
	for _, l := range out.Lines {
		lines = append(lines, fmt.Sprintf("%d:%s", l.Number, l.Text))
	}
	if out.Truncated {
		lines = append(lines, "truncated: read more with start_line/end_line")
	}
	return strings.Join(lines, "\n")
}

// renderLsText prints a header, directories as `path/`, and files as
// `path  BYTES`.
func renderLsText(out lsOutput) string {
	name := out.Repo
	if out.Path != "." {
		name += "/" + out.Path
	}
	lines := make([]string, 0, len(out.Entries)+2)
	lines = append(lines, fmt.Sprintf("%s (%s)", name, count(out.Total, "entry", "entries")))
	for _, e := range out.Entries {
		if e.Dir {
			lines = append(lines, e.Path+"/")
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %d", e.Path, e.Bytes))
	}
	if out.Truncated {
		lines = append(lines, fmt.Sprintf(
			"truncated: showing %d of %d entries; narrow with path/glob",
			out.Total, out.TotalAvailable,
		))
	}
	return strings.Join(lines, "\n")
}

// renderCountText prints `total: N` then one `group  count` line per group.
func renderCountText(out countOutput) string {
	lines := []string{fmt.Sprintf("total: %d", out.Total)}
	for _, g := range out.Groups {
		lines = append(lines, fmt.Sprintf("%s  %d", g.Group, g.Count))
	}
	return strings.Join(lines, "\n")
}

// renderDoctorText prints `ok` or `FAIL` (with the note when set), then one
// `[ok|FAIL|skip] name: detail` line per check.
func renderDoctorText(out doctorOutput) string {
	head := "FAIL"
	if out.OK {
		head = "ok"
	}
	if out.Note != "" {
		head += ": " + out.Note
	}
	lines := []string{head}
	for _, c := range out.Checks {
		line := fmt.Sprintf("[%s] %s", doctorStatusTag(c.Status), c.Name)
		if c.Detail != "" {
			line += ": " + c.Detail
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// doctorStatusTag maps a check status to its bracketed tag; an unknown status
// prints as itself rather than being hidden.
func doctorStatusTag(status string) string {
	switch status {
	case doctor.StatusOK:
		return "ok"
	case doctor.StatusSkipped:
		return "skip"
	case doctor.StatusFail:
		return "FAIL"
	}
	return status
}

// count formats n with the singular or plural noun.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
