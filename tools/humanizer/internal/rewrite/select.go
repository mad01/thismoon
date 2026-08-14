package rewrite

import (
	"regexp"
	"strings"
)

var tokenRE = regexp.MustCompile(`[A-Za-z0-9]+`)

func tokens(text string) []string {
	return tokenRE.FindAllString(strings.ToLower(text), -1)
}

type bigram struct{ a, b string }

func bigrams(toks []string) map[bigram]struct{} {
	out := map[bigram]struct{}{}
	for i := 0; i+1 < len(toks); i++ {
		out[bigram{toks[i], toks[i+1]}] = struct{}{}
	}
	return out
}

// LexicalDivergence is the bigram Jaccard distance between two texts: 0.0 when
// identical, 1.0 when fully different.
func LexicalDivergence(original, candidate string) float64 {
	a := tokens(original)
	b := tokens(candidate)
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}
	ba := bigrams(a)
	bb := bigrams(b)
	union := map[bigram]struct{}{}
	for g := range ba {
		union[g] = struct{}{}
	}
	for g := range bb {
		union[g] = struct{}{}
	}
	if len(union) == 0 {
		return 0.0
	}
	inter := 0
	for g := range ba {
		if _, ok := bb[g]; ok {
			inter++
		}
	}
	return 1.0 - float64(inter)/float64(len(union))
}

// SelectCandidate picks the most lexically diverged rewrite, gently penalising
// extreme length drift. Returns the winner and every candidate's score.
func SelectCandidate(original string, candidates []string) (string, []float64) {
	scores := make([]float64, len(candidates))
	bestIdx := 0
	for i, cand := range candidates {
		score := LexicalDivergence(original, cand)
		if len(original) > 0 {
			ratio := float64(len(cand)) / float64(len(original))
			if ratio > 2.0 || ratio < 0.5 {
				score -= 0.15
			}
		}
		scores[i] = score
		if score > scores[bestIdx] {
			bestIdx = i
		}
	}
	if len(candidates) == 0 {
		return "", scores
	}
	return candidates[bestIdx], scores
}
