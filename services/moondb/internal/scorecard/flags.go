package scorecard

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
)

// flags names the costliest patterns a scorecard shows, each once its
// count reaches its threshold in facts.go. They come in a fixed order, the
// ones that fill the context most directly first: large reads, then
// searches, failed calls, compactions and errors, then unused hints.
func flags(sc Scorecard) []string {
	out := []string{}
	add := func(n, threshold int, one, many, rest string) {
		if n > 0 && n >= threshold {
			out = append(
				out,
				strings.TrimSpace(fmt.Sprintf("%d %s %s", n, noun(n, one, many), rest)),
			)
		}
	}
	r, s := sc.Reads, sc.Search
	add(r.Large, moondb.FlagLargeReads, "whole-file read", "whole-file reads",
		fmt.Sprintf("of %d lines or more", moondb.LargeReadLines))
	add(s.Zero, moondb.FlagZeroSearches, "csl search", "csl searches", "with zero results")
	add(s.Truncated, moondb.FlagTruncatedSearches, "csl search", "csl searches",
		"cut at the limit")
	add(s.Errors, moondb.FlagFailedSearches, "csl search", "csl searches", "failed")
	add(sc.Totals.ToolErrors, moondb.FlagToolErrors, "tool call", "tool calls", "failed")
	if l := sc.Agents.Ledger; l != nil {
		add(l.PreCompact, moondb.FlagCompactions, "context compaction", "context compactions", "")
		add(l.StopFailure, moondb.FlagStopFailures, "turn", "turns", "ended on an API error")
		add(l.PermissionDenied, moondb.FlagPermissionDenials, "tool call", "tool calls",
			"denied by a permission check")
	}
	if h := sc.Hints; h != nil && unusedPins(h.Kof) {
		k := h.Kof
		out = append(out, fmt.Sprintf(
			"kof assertions shown were pinned to %d %s, %d read or edited later",
			k.PinnedPaths, noun(k.PinnedPaths, "file", "files"), k.UsedPaths))
	}
	return out
}

// unusedPins reports whether enough pinned paths are known and too few of
// them were touched after their hint.
func unusedPins(k KofStats) bool {
	return k.PinnedPaths >= moondb.FlagUnusedPins && k.HitRate != nil &&
		*k.HitRate < moondb.FlagPinHitRate
}

// noun is one for a count of 1, many otherwise.
func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
