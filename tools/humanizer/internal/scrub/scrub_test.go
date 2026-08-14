package scrub

import "testing"

func kinds(r Report) map[string]bool {
	out := map[string]bool{}
	for _, h := range r.Hits {
		out[h.Kind] = true
	}
	return out
}

func TestStripsZeroWidthAndSoftHyphen(t *testing.T) {
	cleaned, stats := Clean("Hello\u200bWorld\u00ad!", DefaultOptions())
	if cleaned != "HelloWorld!" {
		t.Fatalf("got %q", cleaned)
	}
	if stats.RemovedCount < 2 {
		t.Fatalf("removed_count = %d, want >= 2", stats.RemovedCount)
	}
}

func TestNormalizesExoticSpaces(t *testing.T) {
	cleaned, stats := Clean("a b　c", DefaultOptions()) // em space, ideographic space
	if cleaned != "a b c" {
		t.Fatalf("got %q", cleaned)
	}
	if stats.ReplacedCount < 2 {
		t.Fatalf("replaced_count = %d, want >= 2", stats.ReplacedCount)
	}
}

func TestNoNormalizeSpacesLeavesThem(t *testing.T) {
	raw := "a b"
	cleaned, _ := Clean(raw, Options{NormalizeSpaces: false})
	if cleaned != raw {
		t.Fatalf("expected exotic space kept, got %q", cleaned)
	}
}

func TestInspectFindsZWSP(t *testing.T) {
	r := Inspect("x\u200by", DefaultOptions())
	if r.SuspiciousTotal < 1 {
		t.Fatalf("suspicious_total = %d", r.SuspiciousTotal)
	}
	k := kinds(r)
	if !k["zwj_family"] && !k["strip"] {
		t.Fatalf("kinds = %v", k)
	}
}

func TestInspectTagChars(t *testing.T) {
	raw := "hi" + string(rune(0xE0041)) + "there"
	r := Inspect(raw, DefaultOptions())
	if !kinds(r)["tag_chars"] {
		t.Fatalf("expected tag_chars, kinds = %v", kinds(r))
	}
	cleaned, stats := Clean(raw, DefaultOptions())
	for _, c := range cleaned {
		if c == 0xE0041 {
			t.Fatal("tag char survived clean")
		}
	}
	if stats.RemovedCount < 1 {
		t.Fatalf("removed_count = %d", stats.RemovedCount)
	}
}

func TestInspectBidi(t *testing.T) {
	raw := "ab\u202eef" // RLO
	if !kinds(Inspect(raw, DefaultOptions()))["bidi"] {
		t.Fatal("expected bidi kind")
	}
	cleaned, _ := Clean(raw, DefaultOptions())
	for _, c := range cleaned {
		if c == 0x202e {
			t.Fatal("RLO survived clean")
		}
	}
}

func TestCleanPreservesNormalText(t *testing.T) {
	raw := "Normal ASCII and café — fine."
	cleaned, stats := Clean(raw, DefaultOptions())
	if cleaned != raw {
		t.Fatalf("got %q", cleaned)
	}
	if stats.RemovedCount != 0 {
		t.Fatalf("removed_count = %d", stats.RemovedCount)
	}
}

func TestAggressiveConfusable(t *testing.T) {
	raw := "pаy" // p + cyrillic a + y
	cleaned, _ := Clean(raw, Options{NormalizeSpaces: true, AggressiveHomoglyphs: true})
	if cleaned != "pay" {
		t.Fatalf("got %q", cleaned)
	}
	// Without the flag it stays put.
	if plain, _ := Clean(raw, DefaultOptions()); plain != raw {
		t.Fatalf("non-aggressive changed text: %q", plain)
	}
}

func TestInspectConfusableGatedOnAggressive(t *testing.T) {
	raw := "pаy"
	if Inspect(raw, DefaultOptions()).SuspiciousTotal != 0 {
		t.Fatal("confusable flagged without aggressive")
	}
	if !kinds(Inspect(raw, Options{AggressiveHomoglyphs: true}))["confusable"] {
		t.Fatal("confusable not flagged with aggressive")
	}
}

func TestCleanPreservesEmojiVS16(t *testing.T) {
	raw := "Balance returns. ⚖\ufe0f" // ⚖\ufe0f
	cleaned, stats := Clean(raw, DefaultOptions())
	if cleaned != raw || stats.RemovedCount != 0 {
		t.Fatalf("altered emoji: %q removed=%d", cleaned, stats.RemovedCount)
	}
}

func TestCleanPreservesZWJFamily(t *testing.T) {
	raw := "Family time: \U0001F468\u200d\U0001F469\u200d\U0001F467" // 👨\u200d👩\u200d👧
	cleaned, stats := Clean(raw, DefaultOptions())
	if cleaned != raw || stats.RemovedCount != 0 {
		t.Fatalf("altered zwj family: %q removed=%d", cleaned, stats.RemovedCount)
	}
}

func TestCleanPreservesZWJChain(t *testing.T) {
	raw := "❤\ufe0f\u200d\U0001F525" // ❤\ufe0f\u200d🔥
	cleaned, stats := Clean(raw, DefaultOptions())
	if cleaned != raw || stats.RemovedCount != 0 {
		t.Fatalf("altered zwj chain: %q removed=%d", cleaned, stats.RemovedCount)
	}
}

func TestCleanStripsFloatingEmojiGlue(t *testing.T) {
	cleaned, stats := Clean("a\u200db\ufe0f", DefaultOptions())
	if cleaned != "ab" {
		t.Fatalf("got %q", cleaned)
	}
	if stats.RemovedCount != 2 {
		t.Fatalf("removed_count = %d, want 2", stats.RemovedCount)
	}
}

func TestInspectEmojiGlueNotSuspiciousByDefault(t *testing.T) {
	raw := "Balance returns. ⚖\ufe0f Family time: \U0001F468\u200d\U0001F469\u200d\U0001F467"
	if got := Inspect(raw, DefaultOptions()).SuspiciousTotal; got != 0 {
		t.Fatalf("suspicious_total = %d, want 0", got)
	}
}

func TestInspectFloatingEmojiGlueIsSuspicious(t *testing.T) {
	if Inspect("a\u200d", DefaultOptions()).SuspiciousTotal < 1 {
		t.Fatal("floating ZWJ not flagged")
	}
}

func TestCleanStripEmojiGlueFlag(t *testing.T) {
	cleaned, stats := Clean("⚖\ufe0f", Options{NormalizeSpaces: true, StripEmojiGlue: true})
	if cleaned != "⚖" {
		t.Fatalf("got %q", cleaned)
	}
	if stats.RemovedCount != 1 {
		t.Fatalf("removed_count = %d, want 1", stats.RemovedCount)
	}
}

func TestInspectStripEmojiGlueFlag(t *testing.T) {
	if Inspect("⚖\ufe0f", Options{StripEmojiGlue: true}).SuspiciousTotal < 1 {
		t.Fatal("glue not flagged in paranoid mode")
	}
}

func TestCleanPreservesScriptJoiners(t *testing.T) {
	// Persian mi-ravam (ZWNJ) and a Devanagari conjunct (ZWJ) — orthographic.
	for _, raw := range []string{
		"می\u200cروم",
		"क्\u200dष",
	} {
		if cleaned, _ := Clean(raw, DefaultOptions()); cleaned != raw {
			t.Fatalf("altered script joiner text: %q -> %q", raw, cleaned)
		}
	}
}

func TestCleanPreservesFlagTagSequence(t *testing.T) {
	// Scotland flag: emoji base U+1F3F4 + tag chars ending in U+E007F.
	raw := "\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F"
	if cleaned, _ := Clean(raw, DefaultOptions()); cleaned != raw {
		t.Fatalf("altered flag: %q", cleaned)
	}
}

func TestCleanPreservesOrthographicArabicCf(t *testing.T) {
	raw := "x\u0600y\u06ddz" // ARABIC NUMBER SIGN, END OF AYAH
	if cleaned, _ := Clean(raw, DefaultOptions()); cleaned != raw {
		t.Fatalf("altered orthographic Cf: %q", cleaned)
	}
}

func TestCleanStillStripsJoinersBetweenLatin(t *testing.T) {
	for _, raw := range []string{"a\u200db", "a\u200cb", "ab\u200c"} {
		cleaned, _ := Clean(raw, DefaultOptions())
		for _, c := range cleaned {
			if c == 0x200c || c == 0x200d {
				t.Fatalf("joiner between latin survived: %q", raw)
			}
		}
	}
}

func TestStripEmojiGlueFlagRestoresBlanketStrip(t *testing.T) {
	cleaned, _ := Clean("می\u200cر", Options{StripEmojiGlue: true})
	for _, c := range cleaned {
		if c == 0x200c {
			t.Fatal("ZWNJ survived paranoid strip")
		}
	}
	if got, _ := Clean("x\u0600y", Options{StripEmojiGlue: true}); got != "xy" {
		t.Fatalf("got %q, want xy", got)
	}
}

func TestNFKCNormalizes(t *testing.T) {
	// Fullwidth "Ａ" (U+FF21) NFKC-folds to ASCII "A".
	cleaned, stats := Clean("Ａ", Options{NormalizeSpaces: true, NFKC: true})
	if cleaned != "A" {
		t.Fatalf("got %q", cleaned)
	}
	if stats.Replaced["NFKC_normalize"] == 0 {
		t.Fatal("NFKC replacement not recorded")
	}
}
