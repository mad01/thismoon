package voice

import (
	"strings"
	"testing"
)

func hasRule(fs []StatFinding, id string) bool {
	for _, f := range fs {
		if f.RuleID == id {
			return true
		}
	}
	return false
}

func findRule(fs []StatFinding, id string) (StatFinding, bool) {
	for _, f := range fs {
		if f.RuleID == id {
			return f, true
		}
	}
	return StatFinding{}, false
}

func TestStatisticalEmpty(t *testing.T) {
	if got := DetectStatistical(""); len(got) != 0 {
		t.Fatalf("expected no findings on empty text, got %d", len(got))
	}
	if got := DetectStatistical("   \n\t "); len(got) != 0 {
		t.Fatalf("expected no findings on blank text, got %d", len(got))
	}
}

func TestShortTextEmDash(t *testing.T) {
	// ~20 words, contains an em-dash → should fire ShortTextEmDash.
	text := "The deploy went out this morning and the dashboards look clean — no errors so far across any of the regions we watch."
	fs := DetectStatistical(text)
	if !hasRule(fs, "Humanizer.ShortTextEmDash") {
		t.Fatalf("expected ShortTextEmDash to fire, got %+v", fs)
	}
}

func TestShortTextEmDashSkipsLongText(t *testing.T) {
	// Long text with an em-dash should NOT trip the short-text check.
	long := strings.Repeat(
		"This is a perfectly ordinary sentence about the work we did today and why. ",
		20,
	)
	long += "Here is an aside — with an em-dash."
	fs := DetectStatistical(long)
	if hasRule(fs, "Humanizer.ShortTextEmDash") {
		t.Fatalf("ShortTextEmDash should not fire on long text")
	}
}

func TestEmDashDensity(t *testing.T) {
	// ~130 words with 4 em-dashes → density ~3 per 100 words, well over
	// the 0.15 ceiling, and past the short-text range.
	text := strings.Repeat(
		"The rollout finished on Tuesday and nothing in the dashboards moved after that point in time. ",
		8,
	)
	text += "The cache layer — the part we rewrote — held up fine, and the queue — always the weak spot — stayed flat."
	fs := DetectStatistical(text)
	if !hasRule(fs, "Humanizer.EmDashDensity") {
		t.Fatalf("expected EmDashDensity to fire, got %+v", fs)
	}
}

func TestEmDashDensitySkipsSparse(t *testing.T) {
	// ~1500 words with 2 em-dashes → density ~0.13 per 100 words, under
	// the ceiling; two dashes across a long document is human-range.
	text := strings.Repeat(
		"This is a perfectly ordinary sentence about the work we did today and why it went the way it did. ",
		80,
	)
	text += "One aside — early on. And another — near the end."
	fs := DetectStatistical(text)
	if hasRule(fs, "Humanizer.EmDashDensity") {
		t.Fatalf("EmDashDensity should not fire on sparse em-dash use")
	}
}

func TestEmDashDensitySkipsSingleDash(t *testing.T) {
	// A single em-dash never fires the density check regardless of length.
	text := strings.Repeat(
		"Here is more ordinary prose that keeps the word count over the minimum for this check to run. ",
		7,
	)
	text += "Just one aside — that is all."
	fs := DetectStatistical(text)
	if hasRule(fs, "Humanizer.EmDashDensity") {
		t.Fatalf("EmDashDensity should not fire on a single em-dash")
	}
}

func TestEmDashDensityExcludesFencedCode(t *testing.T) {
	// Prose: 20 reps of a 9-word sentence (180 words), with 3 em-dashes
	// inserted as standalone tokens so word count is unaffected.
	sentence := "The rollout finished on Tuesday and nothing broke overnight. "
	prose := strings.Repeat(sentence, 20)
	prose = strings.Replace(prose, " and ", " — and ", 3)

	// A fenced code block (git-log-style output) with its own em-dashes
	// and words that must not leak into the prose counts.
	fenced := "```\n" + strings.Repeat("alpha beta — gamma delta\n", 10) + "```\n"

	text := prose + "\n\n" + fenced
	fs := DetectStatistical(text)

	finding, ok := findRule(fs, "Humanizer.EmDashDensity")
	if !ok {
		t.Fatalf("expected EmDashDensity to fire on the prose alone, got %+v", fs)
	}

	wantWords := len(tokenizeWords(prose))
	wantEmDashes := strings.Count(prose, "—")
	wantDensity := round2(float64(wantEmDashes) * 100 / float64(wantWords))

	if finding.Value != wantDensity {
		t.Fatalf("density = %v, want %v (fenced code must not inflate the count or the word total)",
			finding.Value, wantDensity)
	}
}

func TestEmDashDensityIgnoresDoubleHyphens(t *testing.T) {
	// CLI-flag-style "--" tokens (as in a tool's README) must not count as
	// em-dashes. Two real em-dashes are mixed in so the density check's
	// minimum-count gate still trips.
	line := "Run the scan with --branch origin/main and --fail-on-findings enabled for a strict check. "
	prose := strings.Repeat(line, 15)
	prose = strings.Replace(prose, " for a ", " — for a ", 2)

	fs := DetectStatistical(prose)

	finding, ok := findRule(fs, "Humanizer.EmDashDensity")
	if !ok {
		t.Fatalf("expected EmDashDensity to fire, got %+v", fs)
	}

	wantWords := len(tokenizeWords(prose))
	wantEmDashes := strings.Count(prose, "—")
	wantDensity := round2(float64(wantEmDashes) * 100 / float64(wantWords))

	if finding.Value != wantDensity {
		t.Fatalf("density = %v, want %v (double hyphens must not count as em-dashes)",
			finding.Value, wantDensity)
	}
}

func TestSentenceUniformity(t *testing.T) {
	// 12 sentences, all the same length → stddev 0.
	s := strings.Repeat("The team built the tool and shipped it fast. ", 12)
	fs := DetectStatistical(s)
	if !hasRule(fs, "Humanizer.SentenceUniformity") {
		t.Fatalf("expected SentenceUniformity to fire, got %+v", fs)
	}
}

func TestAnaphoraAbuse(t *testing.T) {
	text := "The team worked hard. The team believed in it. The team delivered on time. Then everyone went home."
	fs := DetectStatistical(text)
	if !hasRule(fs, "Humanizer.AnaphoraAbuse") {
		t.Fatalf("expected AnaphoraAbuse to fire, got %+v", fs)
	}
}

func TestAnaphoraIgnoresSingleLetterOpeners(t *testing.T) {
	text := "I went home. I ate dinner. I slept well. The next day was fine."
	fs := DetectStatistical(text)
	if hasRule(fs, "Humanizer.AnaphoraAbuse") {
		t.Fatalf("single-letter openers should be ignored")
	}
}

func TestLowLexicalDiversity(t *testing.T) {
	// 200+ words, tiny vocabulary → low TTR.
	text := strings.Repeat("data data data system system process process flow flow run ", 25)
	fs := DetectStatistical(text)
	if !hasRule(fs, "Humanizer.LowLexicalDiversity") {
		t.Fatalf("expected LowLexicalDiversity to fire, got %+v", fs)
	}
}

func TestHeadingDensity(t *testing.T) {
	var b strings.Builder
	for range 4 {
		b.WriteString("## A heading goes here\n")
		b.WriteString(strings.Repeat("filler word ", 12))
		b.WriteString("\n\n")
	}
	fs := DetectStatistical(b.String())
	if !hasRule(fs, "Humanizer.HeadingDensity") {
		t.Fatalf("expected HeadingDensity to fire, got %+v", fs)
	}
}

func TestNaturalProseStaysQuiet(t *testing.T) {
	// Varied, contraction-rich human-ish prose under the size gates: should
	// produce few or no statistical findings (no uniformity / TTR / em-dash).
	text := "I rewrote the cache layer yesterday. It's smaller now, and the retry storm we kept hitting on Mondays is gone. " +
		"Took about three hours, mostly untangling the old eviction logic. The tricky part wasn't the code; it was proving the old behavior first."
	fs := DetectStatistical(text)
	for _, f := range fs {
		if f.RuleID == "Humanizer.ShortTextEmDash" || f.RuleID == "Humanizer.SentenceUniformity" {
			t.Fatalf("unexpected finding on natural prose: %s", f.RuleID)
		}
	}
}
