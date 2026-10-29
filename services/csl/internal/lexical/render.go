package lexical

import (
	"fmt"
	"strings"
)

// Lines renders the hint as text, one fact per line, for the reader of a
// "no matches" result: how zoekt parsed the query, files per term, how many
// repos the filters covered, index age, and the targeted notes. A nil hint
// renders as no lines.
func (h *ZeroHint) Lines() []string {
	if h == nil {
		return nil
	}
	var lines []string
	if h.ParsedQuery != "" {
		lines = append(lines, "parsed query: "+h.ParsedQuery)
	}
	if len(h.TermCounts) > 0 {
		lines = append(lines, "files per term: "+termCountsLine(h.TermCounts))
	}
	lines = append(lines, fmt.Sprintf(
		"repos searched: %d (%d indexed, %d discovered)",
		h.ReposSearched, h.ReposIndexed, h.ReposDiscovered,
	))
	if h.NewestIndexedAt != "" {
		lines = append(lines, fmt.Sprintf(
			"index age: newest %s, oldest %s", h.NewestIndexedAt, h.OldestIndexedAt,
		))
	}
	for _, n := range h.Notes {
		lines = append(lines, "- "+n)
	}
	return lines
}

// termCountsLine renders term counts as `retry=37, backoff=12, jitter=0`.
func termCountsLine(counts []TermCount) string {
	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = fmt.Sprintf("%s=%d", c.Term, c.Files)
	}
	return strings.Join(parts, ", ")
}

// RelaxedLine leads a relaxed result, so the reader knows which terms were
// dropped and which query the results answer.
func RelaxedLine(dropped []string, relaxed string) string {
	each := ""
	if len(dropped) > 1 {
		each = " each"
	}
	return fmt.Sprintf(
		"relaxed: dropped %s (0 files%s); showing results for `%s`",
		quoteList(dropped), each, relaxed,
	)
}
