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
