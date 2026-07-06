// Package hybrid fuses lexical (zoekt) and semantic (vector) search results
// into a single ranking via Reciprocal Rank Fusion (RRF).
//
// RRF ranks by list position only — it never compares the two backends' raw
// scores (zoekt exposes none, and a cosine similarity is not comparable to a
// lexical rank anyway). A file's fused score is the sum of 1/(k+rank) across
// the lists it appears in, so a file present in BOTH lists outranks a file that
// tops only one. The default k of 60 is the long-standing empirical choice.
package hybrid

import (
	"sort"

	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// DefaultK is the standard RRF smoothing constant (Cormack et al., 2009).
const DefaultK = 60

// FusedHit is one file-level result after fusion. Lexical and semantic evidence
// for the same file are carried side by side; a zero LexRank or SemRank means
// the file was absent from that backend's results.
type FusedHit struct {
	Repo     string
	RepoPath string
	Path     string
	Score    float64 // RRF score: sum of 1/(k+rank) over the lists this file is in

	LexRank int    // 1-based rank in the lexical list; 0 if absent
	LexLine int    // line of the first lexical match (evidence); 0 if none
	LexText string // text of that line

	SemRank  int     // 1-based rank in the semantic list; 0 if absent
	SemStart int     // semantic chunk start line (evidence); 0 if none
	SemEnd   int     // semantic chunk end line
	SemScore float32 // raw cosine similarity, for display only
	Snippet  string  // semantic chunk snippet
}

// Fuse merges ranked lexical and semantic results into file-level hits via RRF.
// k is the RRF constant (k <= 0 falls back to DefaultK). limit caps the returned
// slice (limit <= 0 returns all). Ordering is deterministic: by fused score
// descending, then Repo, then Path.
func Fuse(lexical []search.Match, sem []semantic.Result, k, limit int) []FusedHit {
	if k <= 0 {
		k = DefaultK
	}

	hits := make(map[string]*FusedHit)
	order := make([]string, 0)

	get := func(key, repo, repoPath, path string) *FusedHit {
		h, ok := hits[key]
		if !ok {
			h = &FusedHit{Repo: repo, RepoPath: repoPath, Path: path}
			hits[key] = h
			order = append(order, key)
		}
		return h
	}

	// Lexical: first occurrence of a (repo, file) defines its rank and evidence.
	lexRank := 0
	for _, m := range lexical {
		key := m.Repo + "\x00" + m.File
		if h, ok := hits[key]; ok && h.LexRank != 0 {
			continue // already ranked this file lexically
		}
		lexRank++
		h := get(key, m.Repo, m.RepoPath, m.File)
		h.LexRank = lexRank
		h.LexLine = m.Line
		h.LexText = m.Text
		h.Score += 1.0 / float64(k+lexRank)
	}

	// Semantic: first occurrence of a (repo, path) defines its rank and evidence.
	semRank := 0
	for _, r := range sem {
		key := r.Repo + "\x00" + r.Path
		if h, ok := hits[key]; ok && h.SemRank != 0 {
			continue // already ranked this file semantically
		}
		semRank++
		h := get(key, r.Repo, r.RepoPath, r.Path)
		h.SemRank = semRank
		h.SemStart = r.StartLine
		h.SemEnd = r.EndLine
		h.SemScore = r.Score
		h.Snippet = r.Snippet
		h.Score += 1.0 / float64(k+semRank)
	}

	out := make([]FusedHit, 0, len(order))
	for _, key := range order {
		out = append(out, *hits[key])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Path < out[j].Path
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
