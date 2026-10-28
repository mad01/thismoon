package scorecard

import (
	"cmp"
	"slices"
)

// hintStats counts the hints a session was shown, by id. nil hints, the
// events service not answering, give nil.
func hintStats(hints []HintEvent) *HintStats {
	if hints == nil {
		return nil
	}
	return &HintStats{Shown: len(hints), ByHint: byHint(hints)}
}

// byHint counts the hints by id, most shown first, ties by id.
func byHint(hints []HintEvent) []HintCount {
	counts := map[string]int{}
	for _, h := range hints {
		counts[h.Hint]++
	}
	out := make([]HintCount, 0, len(counts))
	for hint, n := range counts {
		out = append(out, HintCount{Hint: hint, Count: n})
	}
	slices.SortFunc(out, func(a, b HintCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Hint, b.Hint))
	})
	return out
}
