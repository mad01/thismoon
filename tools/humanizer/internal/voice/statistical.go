package voice

import (
	"fmt"
	"regexp"
	"strings"
)

// StatFinding is a single statistical AI-writing signal. Unlike a Vale
// Finding it has no line/column — the signal is a property of the whole
// sample, not a span. Fields mirror the detect output shape so MCP
// clients can fold statistical and Vale findings into one list.
type StatFinding struct {
	RuleID    string  `json:"rule_id"`
	Name      string  `json:"rule_name"`
	Category  string  `json:"category"`
	Severity  string  `json:"severity"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Message   string  `json:"message"`
}

// Statistical thresholds. Tuned to fire on robotic uniformity without
// punishing ordinary human prose. Each check also gates on a minimum
// sample size so short snippets don't produce noisy verdicts.
const (
	minSentencesForStdDev = 10
	stdDevFloor           = 4.0

	minWordsForContraction = 50
	contractionFloor       = 0.5

	minWordsForSemicolon = 500

	minWordsForTTR = 200
	ttrFloor       = 0.4

	shortTextWordFloor = 15
	shortTextWordCeil  = 100

	minWordsForHeadings = 100
	headingsPerWords    = 100 // 1 heading per 100 words is the ceiling

	anaphoraRun = 3
)

var reHeading = regexp.MustCompile(`(?m)^#{1,6}\s+\S`)

// DetectStatistical runs every threshold check over a single text sample
// and returns the signals that tripped. Pure: no I/O, deterministic for a
// given input. Returns an empty slice (never nil-with-panic) on empty text.
func DetectStatistical(text string) []StatFinding {
	var out []StatFinding
	if strings.TrimSpace(text) == "" {
		return out
	}
	p := Compute(text)

	// Robotic sentence-length uniformity.
	if p.SentenceCount >= minSentencesForStdDev && p.SentenceLengthStdDev < stdDevFloor {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.SentenceUniformity",
			Name:      "Sentence-length uniformity",
			Category:  "style",
			Severity:  "warning",
			Metric:    "sentence_length_stddev",
			Value:     round2(p.SentenceLengthStdDev),
			Threshold: stdDevFloor,
			Message: fmt.Sprintf(
				"Sentence length is robotically uniform (stddev %.1f over %d sentences, floor %.1f) — vary sentence length.",
				p.SentenceLengthStdDev,
				p.SentenceCount,
				stdDevFloor,
			),
		})
	}

	// Under-use of contractions in conversational-length text.
	if p.WordCount >= minWordsForContraction && p.ContractionRate < contractionFloor {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.LowContractionRate",
			Name:      "Low contraction rate",
			Category:  "language",
			Severity:  "suggestion",
			Metric:    "contraction_rate_per_100_words",
			Value:     round2(p.ContractionRate),
			Threshold: contractionFloor,
			Message: fmt.Sprintf(
				"Few or no contractions (%.2f per 100 words, floor %.2f) — informal text usually contracts ('it's', 'don't').",
				p.ContractionRate,
				contractionFloor,
			),
		})
	}

	// Long text with zero semicolons (uniform, list-like punctuation).
	if p.WordCount > minWordsForSemicolon && p.SemicolonDensity == 0 {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.SemicolonAbsence",
			Name:      "Semicolon absence",
			Category:  "style",
			Severity:  "suggestion",
			Metric:    "semicolon_density_per_100_words",
			Value:     0,
			Threshold: 0,
			Message: fmt.Sprintf(
				"No semicolons across %d words — uniform punctuation can read machine-flattened.",
				p.WordCount,
			),
		})
	}

	// Low lexical diversity.
	if p.WordCount >= minWordsForTTR && p.TypeTokenRatio < ttrFloor {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.LowLexicalDiversity",
			Name:      "Low lexical diversity",
			Category:  "language",
			Severity:  "warning",
			Metric:    "type_token_ratio",
			Value:     round2(p.TypeTokenRatio),
			Threshold: ttrFloor,
			Message: fmt.Sprintf(
				"Low type-token ratio (%.2f over %d words, floor %.2f) — repetitive vocabulary.",
				p.TypeTokenRatio, p.WordCount, ttrFloor,
			),
		})
	}

	// Any em-dash in short text. The Vale EmDashOveruse rule only counts
	// per-block density, so a single em-dash in a short Slack-length draft
	// slips through; this catches it.
	if p.WordCount >= shortTextWordFloor && p.WordCount < shortTextWordCeil && p.EmDashDensity > 0 {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.ShortTextEmDash",
			Name:      "Em-dash in short text",
			Category:  "style",
			Severity:  "warning",
			Metric:    "em_dash_density_per_100_words",
			Value:     round2(p.EmDashDensity),
			Threshold: 0,
			Message: fmt.Sprintf(
				"Em-dash in a %d-word passage — in short text even one em-dash is a strong AI tell; prefer a comma or period.",
				p.WordCount,
			),
		})
	}

	// Heading-heavy structure.
	if headings := len(reHeading.FindAllString(text, -1)); p.WordCount >= minWordsForHeadings &&
		headings*headingsPerWords > p.WordCount {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.HeadingDensity",
			Name:      "Heading density",
			Category:  "style",
			Severity:  "suggestion",
			Metric:    "headings_per_100_words",
			Value:     round2(float64(headings) * 100 / float64(p.WordCount)),
			Threshold: 1,
			Message: fmt.Sprintf(
				"%d headings across %d words — over one heading per 100 words reads as LLM outline scaffolding.",
				headings,
				p.WordCount,
			),
		})
	}

	// Anaphora: 3+ consecutive sentences opening with the same word.
	if word, run := longestAnaphoraRun(text); run >= anaphoraRun {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.AnaphoraAbuse",
			Name:      "Anaphora abuse",
			Category:  "language",
			Severity:  "warning",
			Metric:    "consecutive_sentence_repeat",
			Value:     float64(run),
			Threshold: anaphoraRun,
			Message: fmt.Sprintf(
				"%d consecutive sentences start with %q — vary sentence openings.",
				run, word,
			),
		})
	}

	return out
}

// longestAnaphoraRun returns the longest run of consecutive sentences that
// begin with the same opening word, and that word. Single-letter openers
// (e.g. "A", "I") are ignored to avoid noise.
func longestAnaphoraRun(text string) (string, int) {
	sentences := splitSentences(text)
	bestWord, best := "", 0
	curWord, cur := "", 0
	for _, s := range sentences {
		first := firstWord(s)
		if first == "" || len(first) < 2 {
			curWord, cur = "", 0
			continue
		}
		if first == curWord {
			cur++
		} else {
			curWord, cur = first, 1
		}
		if cur > best {
			bestWord, best = curWord, cur
		}
	}
	return bestWord, best
}

func firstWord(s string) string {
	ws := tokenizeWords(s)
	if len(ws) == 0 {
		return ""
	}
	return ws[0]
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
