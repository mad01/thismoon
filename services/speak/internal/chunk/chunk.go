// Package chunk splits text into the parts speak synthesizes one request
// each. A remote model answers slowly (seconds per request, whole clip at
// once) and reads better with a few sentences of context, so parts run to a
// few hundred characters; the first part of live playback stays one sentence
// so the first sound comes after one short request. The ramps here are
// mirrored by webkit/src/sentences.ts for the browser player; change both
// together.
package chunk

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxChars bounds a part: about 40 seconds of speech. Gemini 3.1 Flash TTS
// took about 22 seconds for 700 characters, so a part this size answers well
// inside the remote client timeout.
const MaxChars = 600

// Ramp bounds the size of each part in turn: part i holds at most Ramp[i]
// characters and the last bound repeats. A bound of 0 means exactly one
// piece. A single piece larger than its bound still makes a part of its own.
type Ramp []int

var (
	// Live is for speech that plays while it is synthesized: one sentence
	// first, so playback starts after a single short request, then parts
	// that grow while earlier ones play.
	Live = Ramp{0, 250, MaxChars}
	// Prepared is for a section synthesized ahead of playback: a short
	// first part so a section played before it is ready still starts soon.
	Prepared = Ramp{250, MaxChars}
)

// bound is the size limit of part i.
func (r Ramp) bound(i int) int {
	switch {
	case len(r) == 0:
		return MaxChars
	case i >= len(r):
		return r[len(r)-1]
	default:
		return r[i]
	}
}

// Parts splits text into the parts to synthesize, in order, sized by r.
func Parts(text string, r Ramp) []string {
	pieces := Pieces(text)
	var parts []string
	for _, group := range Group(pieces, r) {
		members := make([]string, len(group))
		for i, idx := range group {
			members[i] = pieces[idx]
		}
		parts = append(parts, Join(members))
	}
	return parts
}

// Pieces is text made speakable and split into sentences, with any sentence
// longer than MaxChars split again at word boundaries.
func Pieces(text string) []string {
	var pieces []string
	for _, s := range SplitSentences(Speakable(text)) {
		pieces = append(pieces, splitLong(s, MaxChars)...)
	}
	return pieces
}

// Group packs pieces, in order, into parts sized by r, returning each
// part's piece indices. A part always takes at least one piece.
func Group(pieces []string, r Ramp) [][]int {
	var parts [][]int
	var cur []int
	size := 0
	for i, p := range pieces {
		n := utf8.RuneCountInString(p)
		if len(cur) > 0 {
			limit := r.bound(len(parts))
			if limit == 0 || size+1+n > limit {
				parts = append(parts, cur)
				cur, size = nil, 0
			} else {
				size++ // the joining space
			}
		}
		cur = append(cur, i)
		size += n
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts
}

// Join makes one part's text from its pieces. A piece that does not end in
// terminal punctuation gets a period, so a heading or list item is read as
// its own phrase instead of running into the next sentence.
func Join(pieces []string) string {
	out := make([]string, 0, len(pieces))
	for _, p := range pieces {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, terminate(p))
		}
	}
	return strings.Join(out, " ")
}

// terminate appends a period to s unless it already ends in terminal
// punctuation, looking past closing brackets and quotes.
func terminate(s string) string {
	core := strings.TrimRight(s, `)]}'’»`)
	if core == "" {
		return s
	}
	last, _ := utf8.DecodeLastRuneInString(core)
	if strings.ContainsRune(".!?:;…", last) {
		return s
	}
	return s + "."
}

// Speakable normalizes text for a TTS engine: quote marks and backticks are
// dropped (engines render them as awkward pauses or spell them out) and
// whitespace is collapsed. The browser player's speakable() applies the same
// rule.
func Speakable(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("`\"“”„«»", r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// SplitSentences splits text into trimmed, non-empty sentences, breaking after
// a '.', '!' or '?' that is immediately followed by whitespace. It is the
// hand-rolled equivalent of Python's re.split(r'(?<=[.!?])\s+', text) — Go's
// regexp has no lookbehind, so the delimiter is kept with the sentence and the
// trailing whitespace is consumed.
func SplitSentences(s string) []string {
	runes := []rune(s)
	var out []string
	start := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		j := i + 1
		if j >= len(runes) || !unicode.IsSpace(runes[j]) {
			continue
		}
		if seg := strings.TrimSpace(string(runes[start : i+1])); seg != "" {
			out = append(out, seg)
		}
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		start = j
		i = j - 1
	}
	if start < len(runes) {
		if seg := strings.TrimSpace(string(runes[start:])); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// splitLong splits s at word boundaries into pieces of at most limit
// characters. A single word longer than limit (a URL, say) stays whole.
func splitLong(s string, limit int) []string {
	if utf8.RuneCountInString(s) <= limit {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	size := 0
	for w := range strings.FieldsSeq(s) {
		n := utf8.RuneCountInString(w)
		if size > 0 && size+1+n > limit {
			out = append(out, cur.String())
			cur.Reset()
			size = 0
		}
		if size > 0 {
			cur.WriteByte(' ')
			size++
		}
		cur.WriteString(w)
		size += n
	}
	if size > 0 {
		out = append(out, cur.String())
	}
	return out
}
