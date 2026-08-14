// Package scrub is Layer A of the watermark text cleaner: deterministic
// detection and removal of invisible/format Unicode and space homoglyphs that
// are used as edit-based watermark carriers or arrive from broken pastes.
//
// Inspect reports what it found without changing anything; Clean applies the
// fix and returns the cleaned text plus stats. Both share one classifier so a
// report and its fix never disagree.
//
// The default posture is non-intrusive: strip zero-width/format controls and
// normalize exotic spaces to U+0020, neither of which changes visible meaning.
// The visibly-altering transforms — mapping confusable letters to ASCII, NFKC
// normalization, and stripping load-bearing emoji/script glue — are opt-in via
// Options (the "risky" levels).
//
// Ported from watermarks-remover's text_unicode.py (MIT,
// guillaumemeyer/watermarks-remover @28eca2d); see docs/MIGRATED-FROM.md.
package scrub

import (
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/unicode/runenames"
)

// Options tunes both Inspect and Clean. The zero value is not the default —
// use DefaultOptions, which turns on the non-intrusive space normalization.
type Options struct {
	// NormalizeSpaces rewrites exotic space homoglyphs (no-break space, em
	// space, ...) to a plain ASCII space. Non-intrusive; on by default.
	NormalizeSpaces bool
	// NFKC applies Unicode NFKC normalization after the scrub. Risky: it can
	// change visible characters (ligatures, fullwidth forms, ...).
	NFKC bool
	// AggressiveHomoglyphs maps Cyrillic/fullwidth Latin lookalikes to ASCII.
	// Risky: it rewrites visible letters. In Inspect it flags them instead.
	AggressiveHomoglyphs bool
	// StripEmojiGlue removes load-bearing invisibles too — emoji ZWJ/variation
	// selectors, script joiners, flag tag chars, orthographic Arabic/Syriac Cf.
	// Paranoid: stripping these visibly alters correct text.
	StripEmojiGlue bool
}

// DefaultOptions is the non-intrusive posture: normalize spaces, touch nothing
// visible.
func DefaultOptions() Options {
	return Options{NormalizeSpaces: true}
}

// Hit is one class of suspicious codepoint found by Inspect, aggregated across
// all of its occurrences in the text.
type Hit struct {
	Codepoint     rune   `json:"-"`
	CodepointHex  string `json:"codepoint"`
	Char          string `json:"-"`
	Label         string `json:"label"`
	Count         int    `json:"count"`
	Kind          string `json:"kind"`
	Confidence    string `json:"confidence"`
	SampleOffsets []int  `json:"sample_offsets"`
}

// Report is the result of Inspect: what was found, not what was changed.
type Report struct {
	Length          int      `json:"length"`
	SuspiciousTotal int      `json:"suspicious_total"`
	Hits            []Hit    `json:"hits"`
	Notes           []string `json:"notes"`
}

// Stats is the result of Clean: how much changed.
type Stats struct {
	InputLength   int            `json:"input_length"`
	OutputLength  int            `json:"output_length"`
	Removed       map[string]int `json:"removed"`
	Replaced      map[string]int `json:"replaced"`
	RemovedCount  int            `json:"removed_count"`
	ReplacedCount int            `json:"replaced_count"`
}

// action is what the classifier decided to do with one input rune.
type action int

const (
	actKeep action = iota
	actStrip
	actReplace
)

// spaceHomoglyphs are code points that look like or substitute for U+0020.
var spaceHomoglyphs = map[rune]rune{
	0x00A0: ' ', // no-break space
	0x1680: ' ', // Ogham space mark
	0x2000: ' ', // en quad
	0x2001: ' ', // em quad
	0x2002: ' ', // en space
	0x2003: ' ', // em space
	0x2004: ' ', // three-per-em space
	0x2005: ' ', // four-per-em space
	0x2006: ' ', // six-per-em space
	0x2007: ' ', // figure space
	0x2008: ' ', // punctuation space
	0x2009: ' ', // thin space
	0x200A: ' ', // hair space
	0x202F: ' ', // narrow no-break space
	0x205F: ' ', // medium mathematical space
	0x3000: ' ', // ideographic space
}

// latinConfusables maps Cyrillic and fullwidth Latin lookalikes to ASCII.
// Only consulted in aggressive mode.
var latinConfusables = map[rune]rune{
	0x0410: 'A', 0x0412: 'B', 0x0415: 'E', 0x041A: 'K', 0x041C: 'M',
	0x041D: 'H', 0x041E: 'O', 0x0420: 'P', 0x0421: 'C', 0x0422: 'T',
	0x0425: 'X', 0x0430: 'a', 0x0435: 'e', 0x043E: 'o', 0x0440: 'p',
	0x0441: 'c', 0x0443: 'y', 0x0445: 'x', 0x0456: 'i',
	0xFF21: 'A', 0xFF22: 'B', 0xFF23: 'C', 0xFF24: 'D', 0xFF25: 'E',
	0xFF26: 'F', 0xFF27: 'G', 0xFF28: 'H', 0xFF29: 'I', 0xFF2A: 'J',
	0xFF2B: 'K', 0xFF2C: 'L', 0xFF2D: 'M', 0xFF2E: 'N', 0xFF2F: 'O',
	0xFF30: 'P', 0xFF31: 'Q', 0xFF32: 'R', 0xFF33: 'S', 0xFF34: 'T',
	0xFF35: 'U', 0xFF36: 'V', 0xFF37: 'W', 0xFF38: 'X', 0xFF39: 'Y',
	0xFF3A: 'Z', 0xFF41: 'a', 0xFF42: 'b', 0xFF43: 'c', 0xFF44: 'd',
	0xFF45: 'e', 0xFF46: 'f', 0xFF47: 'g', 0xFF48: 'h', 0xFF49: 'i',
	0xFF4A: 'j', 0xFF4B: 'k', 0xFF4C: 'l', 0xFF4D: 'm', 0xFF4E: 'n',
	0xFF4F: 'o', 0xFF50: 'p', 0xFF51: 'q', 0xFF52: 'r', 0xFF53: 's',
	0xFF54: 't', 0xFF55: 'u', 0xFF56: 'v', 0xFF57: 'w', 0xFF58: 'x',
	0xFF59: 'y', 0xFF5A: 'z',
}

// stripCodepoints are format/invisible controls commonly used for
// steganography or arriving from broken pastes.
var stripCodepoints = map[rune]struct{}{}

func init() {
	for _, cp := range []rune{
		0x00AD, 0x034F, 0x061C, 0x115F, 0x1160, 0x17B4, 0x17B5,
		0x180B, 0x180C, 0x180D, 0x180E,
		0x200B, 0x200C, 0x200D, 0x200E, 0x200F,
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E,
		0x2060, 0x2061, 0x2062, 0x2063, 0x2064,
		0x2066, 0x2067, 0x2068, 0x2069,
		0x206A, 0x206B, 0x206C, 0x206D, 0x206E, 0x206F,
		0xFEFF,
		0xFE00, 0xFE01, 0xFE02, 0xFE03, 0xFE04, 0xFE05, 0xFE06, 0xFE07,
		0xFE08, 0xFE09, 0xFE0A, 0xFE0B, 0xFE0C, 0xFE0D, 0xFE0E, 0xFE0F,
		0xFFF9, 0xFFFA, 0xFFFB,
	} {
		stripCodepoints[cp] = struct{}{}
	}
}

// bidiCodepoints is a subset of the strip set used for a finer inspect label.
var bidiCodepoints = map[rune]struct{}{
	0x061C: {}, 0x200E: {}, 0x200F: {},
	0x202A: {}, 0x202B: {}, 0x202C: {}, 0x202D: {}, 0x202E: {},
	0x2066: {}, 0x2067: {}, 0x2068: {}, 0x2069: {},
}

// zwFamily is the zero-width family, common edit-based carriers.
var zwFamily = map[rune]struct{}{
	0x200B: {}, 0x200C: {}, 0x200D: {}, 0x2060: {}, 0xFEFF: {}, 0x180E: {},
}

// emojiGlue is presentation glue: zero-width joiner and text/emoji variation
// selectors. Invisible when free-floating, load-bearing after an emoji base.
var emojiGlue = map[rune]struct{}{0x200D: {}, 0xFE0E: {}, 0xFE0F: {}}

// scriptJoiners are orthographic inside complex scripts (Persian, Devanagari).
var scriptJoiners = map[rune]struct{}{0x200C: {}, 0x200D: {}}

// orthographicCf are Cf code points that are normal Arabic/Syriac orthography,
// not carriers.
var orthographicCf = map[rune]struct{}{
	0x0600: {}, 0x0601: {}, 0x0602: {}, 0x0603: {}, 0x0604: {}, 0x0605: {},
	0x06DD: {}, 0x070F: {}, 0x08E2: {}, 0x110BD: {}, 0x110CD: {},
}

func inMap(m map[rune]struct{}, r rune) bool { _, ok := m[r]; return ok }

// isVSSupplement covers VS17–VS256 in the Supplementary Special-purpose plane.
func isVSSupplement(r rune) bool { return r >= 0xE0100 && r <= 0xE01EF }

// isTagChar covers the tag characters used by some stego schemes and flag emoji.
func isTagChar(r rune) bool { return r >= 0xE0001 && r <= 0xE007F }

// isTagRange is the flag-emoji tag range (subset of isTagChar).
func isTagRange(r rune) bool { return r >= 0xE0020 && r < 0xE0080 }

func isStripCP(r rune) bool {
	if inMap(stripCodepoints, r) {
		return true
	}
	if isVSSupplement(r) {
		return true
	}
	return isTagChar(r)
}

func stripKind(r rune) string {
	switch {
	case r >= 0xE0001 && r <= 0xE007F:
		return "tag_chars"
	case isVSSupplement(r) || (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0x180B && r <= 0x180D):
		return "variation_selector"
	case inMap(bidiCodepoints, r):
		return "bidi"
	case inMap(zwFamily, r):
		return "zwj_family"
	default:
		return "strip"
	}
}

func isEmojiGlue(r rune) bool { return inMap(emojiGlue, r) }

// isEmojiBase reports whether r can start or continue an emoji sequence.
func isEmojiBase(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols / dingbats / arrows
		return true
	case r >= 0x2B00 && r <= 0x2BFF: // misc symbols and arrows
		return true
	}
	switch r {
	case 0x00A9, 0x00AE, 0x2122, 0x3030, 0x303D, 0x3297, 0x3299:
		return true
	case 0x0023, 0x002A: // keycap bases # *
		return true
	}
	return r >= 0x0030 && r <= 0x0039 // keycap digit bases
}

// isJoiningLetter is a non-ASCII letter/mark — the neighbour that makes a
// joiner orthographic.
func isJoiningLetter(r rune) bool {
	return r > 0x7F && unicode.In(r, unicode.L, unicode.M)
}

// isGlue reports a load-bearing invisible: emoji glue, script joiner, or flag
// tag char. Glue does not advance the "previous kept base".
func isGlue(r rune) bool {
	return isEmojiGlue(r) || inMap(scriptJoiners, r) || isTagRange(r)
}

// decide classifies one input rune for both Inspect and Clean. kind is empty
// when the rune is not suspicious.
func decide(r rune, prevKept rune, hasPrev bool, opts Options) (action, rune, string) {
	if isEmojiGlue(r) && !opts.StripEmojiGlue {
		if hasPrev && isEmojiBase(prevKept) {
			return actKeep, r, ""
		}
	}
	if !opts.StripEmojiGlue {
		if inMap(scriptJoiners, r) && hasPrev && isJoiningLetter(prevKept) {
			return actKeep, r, ""
		}
		if isTagRange(r) && hasPrev && isEmojiBase(prevKept) {
			return actKeep, r, ""
		}
		if inMap(orthographicCf, r) {
			return actKeep, r, ""
		}
	}
	if isStripCP(r) {
		return actStrip, 0, stripKind(r)
	}
	if opts.NormalizeSpaces {
		if to, ok := spaceHomoglyphs[r]; ok {
			return actReplace, to, "space"
		}
	}
	if opts.AggressiveHomoglyphs {
		if to, ok := latinConfusables[r]; ok {
			return actReplace, to, "confusable"
		}
	}
	if _, isSpace := spaceHomoglyphs[r]; unicode.Is(unicode.Cf, r) && !isSpace {
		return actStrip, 0, "other_cf"
	}
	return actKeep, r, ""
}

// hitConfidence: Layer A hits are edit-based carriers; space homoglyphs are
// weaker context.
func hitConfidence(kind string) string {
	if kind == "space" {
		return "informational"
	}
	return "probable"
}

func charLabel(r rune) string {
	name := runenames.Name(r)
	if name == "" {
		name = "UNKNOWN"
	}
	return fmt.Sprintf("U+%04X %s (%s)", r, name, generalCategory(r))
}

// generalCategory returns the two-letter Unicode general category (e.g. "Cf",
// "Lo", "Zs"). Every rune belongs to exactly one, so the result is stable
// despite the map iteration.
func generalCategory(r rune) string {
	for name, tbl := range unicode.Categories {
		if len(name) == 2 && unicode.Is(tbl, r) {
			return name
		}
	}
	return "??"
}

// bucketKey aggregates hits by codepoint and inspect kind.
type bucketKey struct {
	cp   rune
	kind string
}

// Inspect classifies the text and reports suspicious codepoints without
// changing it. Space normalization is always considered here (reported as
// informational); confusables are only flagged when AggressiveHomoglyphs is
// set. StripEmojiGlue widens the net to load-bearing invisibles.
func Inspect(text string, opts Options) Report {
	// Inspect always considers spaces so it can report them.
	opts.NormalizeSpaces = true

	buckets := map[bucketKey][]int{}
	var order []bucketKey
	var prevKept rune
	hasPrev := false

	idx := 0
	for _, r := range text {
		act, out, kind := decide(r, prevKept, hasPrev, opts)
		if kind == "" {
			if !isGlue(r) {
				prevKept, hasPrev = out, true
			}
			idx++
			continue
		}
		key := bucketKey{cp: r, kind: kind}
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], idx)
		if act == actReplace {
			prevKept, hasPrev = out, true
		}
		idx++
	}

	sort.SliceStable(order, func(i, j int) bool {
		li, lj := len(buckets[order[i]]), len(buckets[order[j]])
		if li != lj {
			return li > lj
		}
		return order[i].cp < order[j].cp
	})

	hits := make([]Hit, 0, len(order))
	total := 0
	for _, key := range order {
		offsets := buckets[key]
		samples := offsets
		if len(samples) > 10 {
			samples = samples[:10]
		}
		hits = append(hits, Hit{
			Codepoint:     key.cp,
			CodepointHex:  fmt.Sprintf("U+%04X", key.cp),
			Char:          string(key.cp),
			Label:         charLabel(key.cp),
			Count:         len(offsets),
			Kind:          key.kind,
			Confidence:    hitConfidence(key.kind),
			SampleOffsets: samples,
		})
		total += len(offsets)
	}

	notes := []string{
		"Layer A only: invisible/format Unicode and space homoglyphs (edit-based carriers).",
		"Statistical (token-sampling) watermarks are not detectable here; use the rewrite pass.",
		"Inspect kinds: strip, bidi, tag_chars, variation_selector, zwj_family, space, confusable, other_cf.",
		"Load-bearing invisibles are preserved by default: emoji glue (ZWJ/VS after an emoji base), script joiners (ZWNJ/ZWJ inside complex scripts), flag tag chars, and orthographic Arabic/Syriac Cf marks. Use StripEmojiGlue for paranoid mode.",
	}
	if len(hits) == 0 {
		notes = append(notes,
			"No deterministic Layer A (invisible Unicode/format) carriers detected; "+
				"statistical and pixel-domain marks are out of scope here.")
	}
	return Report{
		Length:          utf8.RuneCountInString(text),
		SuspiciousTotal: total,
		Hits:            hits,
		Notes:           notes,
	}
}

// Clean applies the scrub and returns the cleaned text plus stats.
func Clean(text string, opts Options) (string, Stats) {
	removed := map[string]int{}
	replaced := map[string]int{}
	var b []rune
	var prevKept rune
	hasPrev := false

	for _, r := range text {
		act, out, _ := decide(r, prevKept, hasPrev, opts)
		switch act {
		case actKeep:
			b = append(b, out)
			if !isGlue(r) {
				prevKept, hasPrev = out, true
			}
		case actReplace:
			b = append(b, out)
			replaced[charLabel(r)]++
			prevKept, hasPrev = out, true
		case actStrip:
			removed[charLabel(r)]++
		}
	}

	result := string(b)
	if opts.NFKC {
		before := result
		result = norm.NFKC.String(result)
		if result != before {
			d := utf8.RuneCountInString(before) - utf8.RuneCountInString(result)
			if d < 0 {
				d = -d
			}
			if d == 0 {
				d = 1
			}
			replaced["NFKC_normalize"] += d
		}
	}

	replacedCount := 0
	for k, v := range replaced {
		if k != "NFKC_normalize" {
			replacedCount += v
		}
	}
	removedCount := 0
	for _, v := range removed {
		removedCount += v
	}

	return result, Stats{
		InputLength:   utf8.RuneCountInString(text),
		OutputLength:  utf8.RuneCountInString(result),
		Removed:       removed,
		Replaced:      replaced,
		RemovedCount:  removedCount,
		ReplacedCount: replacedCount,
	}
}
