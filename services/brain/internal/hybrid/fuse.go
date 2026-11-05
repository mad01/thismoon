// Package hybrid combines a literal (zoekt) result list and a semantic
// (vector) result list into one ranked sequence using Reciprocal Rank
// Fusion (RRF), with the small position-aware tie-break bonuses that
// QMD-style hybrid search uses on small corpora.
//
// Pure function package — no I/O, no goroutines.
package hybrid

import (
	"slices"
	"sort"
)

// DefaultK is RRF's k constant. 60 is the standard value from the
// original RRF paper and what QMD uses; raising it favors deeper-list
// agreement, lowering it favors top-3 winners.
const DefaultK = 60

// Hit is one fused result. Sources tracks which retrievers found it.
type Hit struct {
	// ID is the chunk ID (or, for literal hits, the file path acting as a
	// stable identity since zoekt doesn't chunk).
	ID string
	// Score is the RRF-fused score, post-bonus. Higher is better.
	Score float64
	// Sources lists retrievers that contributed: "zoekt" and/or "vector".
	Sources []string
	// ZoektRank is the 1-based rank in the literal list, 0 when absent.
	ZoektRank int
	// VectorRank is the 1-based rank in the semantic list, 0 when absent.
	VectorRank int
}

// Input is one ranked list to fuse, typed by source label.
type Input struct {
	Source string
	IDs    []string
}

// Fuse computes RRF over multiple ranked input lists. k is RRF's smoothing
// constant — pass DefaultK unless you have a reason. Returns hits sorted
// descending by Score.
//
// The position-aware bonus follows QMD's empirical schedule on small vaults:
// the #1-ranked doc in any source gets +0.05, #2 and #3 get +0.02. These
// bonuses break ties when many docs land in only one list.
func Fuse(inputs []Input, k int) []Hit {
	if k <= 0 {
		k = DefaultK
	}
	agg := make(map[string]*Hit)

	for _, in := range inputs {
		for i, id := range in.IDs {
			rank := i + 1
			h, ok := agg[id]
			if !ok {
				h = &Hit{ID: id}
				agg[id] = h
			}
			contrib := 1.0 / float64(k+rank)
			switch rank {
			case 1:
				contrib += 0.05
			case 2, 3:
				contrib += 0.02
			}
			h.Score += contrib
			h.Sources = appendUnique(h.Sources, in.Source)
			switch in.Source {
			case "zoekt":
				if h.ZoektRank == 0 || rank < h.ZoektRank {
					h.ZoektRank = rank
				}
			case "vector":
				if h.VectorRank == 0 || rank < h.VectorRank {
					h.VectorRank = rank
				}
			}
		}
	}

	out := make([]Hit, 0, len(agg))
	for _, h := range agg {
		out = append(out, *h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Tie-break: prefer hits found by more sources, then alphabetical
		// ID for stability.
		if len(out[i].Sources) != len(out[j].Sources) {
			return len(out[i].Sources) > len(out[j].Sources)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func appendUnique(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
