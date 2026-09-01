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

	// Long-text em-dash density picks up where ShortTextEmDash stops.
	// Human prose runs 2-5 em-dashes per 10k words; LLM output runs
	// 17-45 (Gemini-family excepted). The ceiling sits between the two,
	// and a two-hit minimum keeps a single stylistic dash from flagging.
	minWordsForEmDashDensity = shortTextWordCeil
	emDashDensityCeil        = 0.15 // per 100 words = 15 per 10k
	minEmDashCount           = 2

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

	// Both em-dash checks work on fenced-code-stripped text: the Vale span
	// rules skip fenced code blocks by default, so a code sample (git log
	// output, a YAML comment) inside a README fence must not count as
	// prose. The word count and em-dash count both come from that stripped
	// text, so the reported density is the count of real em-dashes over
	// the words that actually contain them, not Profile.EmDashDensity,
	// which also matches "--" and would count CLI flags like --branch as
	// em-dash substitutes.
	prose := stripFencedCode(text)
	proseWords := len(tokenizeWords(prose))
	emDashCount := strings.Count(prose, "—")
	emDashDensity := per100(emDashCount, proseWords)

	// Any em-dash in short text. The Vale EmDashOveruse rule flags each
	// em-dash span too; this whole-sample check keeps the signal when only
	// the statistical path runs.
	if proseWords >= shortTextWordFloor && proseWords < shortTextWordCeil && emDashDensity > 0 {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.ShortTextEmDash",
			Name:      "Em-dash in short text",
			Category:  "style",
			Severity:  "warning",
			Metric:    "em_dash_density_per_100_words",
			Value:     round2(emDashDensity),
			Threshold: 0,
			Message: fmt.Sprintf(
				"Em-dash in a %d-word passage — in short text even one em-dash is a strong AI tell; prefer a comma or period.",
				proseWords,
			),
		})
	}

	// Em-dash density in longer text.
	if proseWords >= minWordsForEmDashDensity &&
		emDashCount >= minEmDashCount && emDashDensity > emDashDensityCeil {
		out = append(out, StatFinding{
			RuleID:    "Humanizer.EmDashDensity",
			Name:      "Em-dash density",
			Category:  "style",
			Severity:  "warning",
			Metric:    "em_dash_density_per_100_words",
			Value:     round2(emDashDensity),
			Threshold: emDashDensityCeil,
			Message: fmt.Sprintf(
				"%d em-dashes across %d words (%.2f per 100 words, ceiling %.2f) — human prose runs an order of magnitude lower; swap most for commas or periods.",
				emDashCount, proseWords, emDashDensity, emDashDensityCeil,
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

// stripFencedCode removes fenced code blocks from text: any line whose
// trimmed content opens with three or more backticks or tildes starts a
// fence, dropped along with every line up to and including the matching
// close (same character, same or greater run length). Text outside fences
// passes through unchanged, so callers that never see a fence marker get
// back the original text. Matches the Vale span rules' default markdown
// scope, which skips fenced code the same way.
func stripFencedCode(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	var fenceChar byte
	var fenceLen int
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if fenceChar == 0 {
			if ch, n, ok := fenceMarker(trimmed); ok {
				fenceChar, fenceLen = ch, n
				continue
			}
			out = append(out, line)
			continue
		}
		if ch, n, ok := fenceMarker(trimmed); ok && ch == fenceChar && n >= fenceLen {
			fenceChar, fenceLen = 0, 0
		}
	}
	return strings.Join(out, "\n")
}

// fenceMarker reports whether a trimmed line is a fence delimiter: three or
// more of the same backtick or tilde character.
func fenceMarker(trimmed string) (ch byte, n int, ok bool) {
	if trimmed == "" {
		return 0, 0, false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	return c, n, true
}
