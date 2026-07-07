// Package voice computes quantitative voice profiles for a text sample
// and diffs two profiles. Used by humanizer_voice_profile and
// humanizer_voice_diff. Pure metric computation; no LLM calls.
package voice

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Profile is the full set of metrics for a sample.
type Profile struct {
	WordCount      int     `json:"word_count"`
	SentenceCount  int     `json:"sentence_count"`
	ParagraphCount int     `json:"paragraph_count"`
	UniqueWords    int     `json:"unique_words"`
	TypeTokenRatio float64 `json:"type_token_ratio"`

	SentenceLengthMean   float64 `json:"sentence_length_mean"`
	SentenceLengthStdDev float64 `json:"sentence_length_stddev"`
	SentenceLengthP50    int     `json:"sentence_length_p50"`
	SentenceLengthP90    int     `json:"sentence_length_p90"`

	// Per 100 words.
	EmDashDensity     float64 `json:"em_dash_density_per_100_words"`
	SemicolonDensity  float64 `json:"semicolon_density_per_100_words"`
	ColonDensity      float64 `json:"colon_density_per_100_words"`
	ParenDensity      float64 `json:"paren_density_per_100_words"`
	CommaDensity      float64 `json:"comma_density_per_100_words"`
	HyphenatedDensity float64 `json:"hyphenated_pair_density_per_100_words"`
	BoldDensity       float64 `json:"bold_density_per_100_words"`
	ContractionRate   float64 `json:"contraction_rate_per_100_words"`

	FleschReadingEase float64 `json:"flesch_reading_ease"`

	TopBigrams  []NGramCount `json:"top_bigrams"`
	TopTrigrams []NGramCount `json:"top_trigrams"`
}

// NGramCount is a single n-gram with its frequency.
type NGramCount struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// Compute walks the text once per metric family and returns a Profile.
// Safe to call on empty input (returns a zero-valued Profile).
func Compute(text string) Profile {
	p := Profile{}
	if strings.TrimSpace(text) == "" {
		return p
	}

	words := tokenizeWords(text)
	p.WordCount = len(words)

	if p.WordCount == 0 {
		return p
	}

	unique := map[string]struct{}{}
	for _, w := range words {
		unique[w] = struct{}{}
	}
	p.UniqueWords = len(unique)
	p.TypeTokenRatio = float64(p.UniqueWords) / float64(p.WordCount)

	sentences := splitSentences(text)
	p.SentenceCount = len(sentences)

	paragraphs := splitParagraphs(text)
	p.ParagraphCount = len(paragraphs)

	lengths := make([]int, 0, len(sentences))
	for _, s := range sentences {
		sw := tokenizeWords(s)
		if len(sw) > 0 {
			lengths = append(lengths, len(sw))
		}
	}
	p.SentenceLengthMean, p.SentenceLengthStdDev = meanStdDev(lengths)
	p.SentenceLengthP50 = percentile(lengths, 0.5)
	p.SentenceLengthP90 = percentile(lengths, 0.9)

	emDashes := strings.Count(text, "—") + strings.Count(text, "--")
	semicolons := strings.Count(text, ";")
	colons := strings.Count(text, ":")
	parens := strings.Count(text, "(")
	commas := strings.Count(text, ",")

	p.EmDashDensity = per100(emDashes, p.WordCount)
	p.SemicolonDensity = per100(semicolons, p.WordCount)
	p.ColonDensity = per100(colons, p.WordCount)
	p.ParenDensity = per100(parens, p.WordCount)
	p.CommaDensity = per100(commas, p.WordCount)

	hyphenatedCount := countHyphenatedCompounds(text)
	p.HyphenatedDensity = per100(hyphenatedCount, p.WordCount)

	boldCount := len(reBold.FindAllString(text, -1))
	p.BoldDensity = per100(boldCount, p.WordCount)

	contractions := countContractions(words)
	p.ContractionRate = per100(contractions, p.WordCount)

	syllables := 0
	for _, w := range words {
		syllables += estimateSyllables(w)
	}
	if p.SentenceCount > 0 && p.WordCount > 0 {
		// Flesch Reading Ease = 206.835 - 1.015*(W/S) - 84.6*(Sy/W)
		p.FleschReadingEase = 206.835 -
			1.015*float64(p.WordCount)/float64(p.SentenceCount) -
			84.6*float64(syllables)/float64(p.WordCount)
	}

	p.TopBigrams = topNGrams(words, 2, 10)
	p.TopTrigrams = topNGrams(words, 3, 10)

	return p
}

// --- helpers ---

var (
	reBold               = regexp.MustCompile(`\*\*[^*\n]+?\*\*`)
	reHyphenatedCompound = regexp.MustCompile(`\b[a-zA-Z]+-[a-zA-Z]+\b`)
	reParagraphSplit     = regexp.MustCompile(`\n\s*\n+`)
)

// tokenizeWords lowercases + extracts unicode word runs with internal apostrophes.
func tokenizeWords(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, strings.ToLower(b.String()))
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if r == '\'' || r == '’' {
			if b.Len() > 0 {
				b.WriteRune('\'')
				continue
			}
		}
		flush()
	}
	flush()
	return out
}

func splitSentences(text string) []string {
	// Pragmatic: split on . ! ? followed by whitespace. Drops empty results.
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != '.' && c != '!' && c != '?' {
			continue
		}
		end := i + 1
		if end < len(text) {
			c := text[end]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				continue
			}
		}
		s := strings.TrimSpace(text[start:end])
		if s != "" {
			out = append(out, s)
		}
		start = end
	}
	if start < len(text) {
		tail := strings.TrimSpace(text[start:])
		if tail != "" {
			out = append(out, tail)
		}
	}
	return out
}

func splitParagraphs(text string) []string {
	paragraphs := reParagraphSplit.Split(text, -1)
	out := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func meanStdDev(xs []int) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	sum := 0
	for _, x := range xs {
		sum += x
	}
	mean := float64(sum) / float64(len(xs))
	if len(xs) == 1 {
		return mean, 0
	}
	var sqSum float64
	for _, x := range xs {
		d := float64(x) - mean
		sqSum += d * d
	}
	return mean, math.Sqrt(sqSum / float64(len(xs)-1))
}

func percentile(xs []int, p float64) int {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]int, len(xs))
	copy(cp, xs)
	sort.Ints(cp)
	idx := int(math.Ceil(p*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func per100(count, words int) float64 {
	if words == 0 {
		return 0
	}
	return float64(count) * 100.0 / float64(words)
}

func countHyphenatedCompounds(text string) int {
	return len(reHyphenatedCompound.FindAllString(text, -1))
}

// contractionsSuffixes covers the common English contraction tails.
var contractionsSuffixes = []string{"'re", "'ve", "'ll", "'d", "'s", "'t", "'m"}

func countContractions(words []string) int {
	n := 0
	for _, w := range words {
		for _, suf := range contractionsSuffixes {
			if strings.HasSuffix(w, suf) && len(w) > len(suf) {
				n++
				break
			}
		}
	}
	return n
}

// estimateSyllables is a rough vowel-group counter. Good enough for Flesch.
func estimateSyllables(word string) int {
	if word == "" {
		return 0
	}
	count := 0
	prevVowel := false
	for _, r := range word {
		isV := isVowel(r)
		if isV && !prevVowel {
			count++
		}
		prevVowel = isV
	}
	// Silent trailing e
	if strings.HasSuffix(word, "e") && count > 1 {
		count--
	}
	if count == 0 {
		count = 1
	}
	return count
}

func isVowel(r rune) bool {
	switch unicode.ToLower(r) {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

// topNGrams returns the n most frequent n-grams. Stopword-heavy n-grams
// are filtered so results highlight content-word patterns.
func topNGrams(words []string, n, top int) []NGramCount {
	if len(words) < n {
		return nil
	}
	counts := map[string]int{}
	for i := 0; i+n <= len(words); i++ {
		slice := words[i : i+n]
		if allStopwords(slice) {
			continue
		}
		counts[strings.Join(slice, " ")]++
	}
	type pair struct {
		k string
		v int
	}
	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		if v < 2 {
			continue
		}
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	if top < len(pairs) {
		pairs = pairs[:top]
	}
	out := make([]NGramCount, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, NGramCount{Text: p.k, Count: p.v})
	}
	return out
}

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"to": true, "of": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "as": true, "that": true, "this": true, "it": true,
	"from": true, "has": true, "have": true, "had": true, "not": true, "no": true,
	"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
}

func allStopwords(words []string) bool {
	for _, w := range words {
		if !stopwords[w] {
			return false
		}
	}
	return true
}
