// Package chunk splits a brain markdown note into retrieval-friendly chunks.
//
// Chunking is markdown-aware: headings carve the note into sections, fenced
// code blocks are treated as atomic paragraphs, and chunks within a section
// share a configurable word-overlap so context near chunk boundaries is
// preserved for the retriever. No chunk is longer than maxChunkBytes, the
// most the embed model is sure to take: a longer paragraph or fence is cut
// at a line break, else a space, else a character boundary.
//
// Frontmatter is parsed once per note and propagated to every emitted chunk
// so downstream filters (tier / tag / evidence) work without re-reading
// disk per hit.
//
// "Tokens" in the configuration are approximated by whitespace-separated
// words — close enough for retrieval purposes, and avoids a tokenizer
// dependency. The default 800-token target maps to ~600 words.
package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Chunk is one indexable slice of a note. The caller embeds Body and stores
// the rest as metadata in the vector store so filters and rendering work
// without re-reading the original file.
type Chunk struct {
	// ID is deterministic per (path, seq). Stable across re-chunking when
	// the chunk count is unchanged.
	ID string
	// Path is the vault-relative note path (e.g. "notes/topic/foo.md").
	Path string
	// NoteID is the note's frontmatter id, empty when the note has none.
	NoteID string
	// Title is the note's frontmatter.title (or the filename stem if missing).
	Title string
	// HeadingPath joins the headings above this chunk with " › ", e.g.
	// "Concept Alpha › Background › Refinements". Empty for the leading
	// pre-heading prelude.
	HeadingPath string
	// Body is the chunk's text (markdown source, with frontmatter stripped).
	// Includes overlap from the previous chunk when applicable.
	Body string
	// Tier is the vault tier (inbox/notes/outputs/MOCs).
	Tier string
	// Tags are the note's frontmatter tags.
	Tags []string
	// Type is the note's frontmatter type.
	Type string
	// Status is the note's effective status (vault.StatusOf): the stored
	// value, normalized, else the default of the note's tier.
	Status string
	// Evidence is the note's evidence tier (A/B/C/D/mixed) — frontmatter
	// field first, "evidence/<tier>" tag second, empty otherwise.
	Evidence string
	// CharStart and CharEnd bound this chunk in the original body bytes.
	// Overlap content does not extend these bounds.
	CharStart int
	CharEnd   int
	// Hash is the first 16 hex chars of sha256(Body) — used for cheap
	// equality checks during incremental indexing.
	Hash string
	// Seq is the zero-based chunk index within the note.
	Seq int
}

// Options tune the chunker. Zero values fall back to defaults (800/100).
type Options struct {
	// TargetWords is the soft upper bound on words per chunk. Used as the
	// pack-until size when accumulating paragraphs. maxChunkBytes caps a
	// chunk too, and prose reaches it first at about 330 words.
	TargetWords int
	// OverlapWords is the number of trailing words from the previous chunk
	// to prepend to the next chunk within the same section, at most
	// maxOverlapBytes of them.
	OverlapWords int
}

const (
	defaultTargetWords  = 600
	defaultOverlapWords = 75
)

// maxChunkBytes caps a chunk's body, overlap included, so the embed model
// always takes it. nomic-embed-text, the default model, has a 2048-token
// context in Ollama, and Ollama refuses a longer prompt outright ("the input
// length exceeds the context length"). Its WordPiece tokenizer, like
// byte-level BPE, makes at most one token of each byte, so 2000 bytes fit
// whatever the text is, with room for the two tokens the model adds.
const maxChunkBytes = 2000

// maxOverlapBytes caps the overlap carried into the next chunk, so at least
// three quarters of a chunk is its own text.
const maxOverlapBytes = maxChunkBytes / 4

// paraSep joins the paragraphs of a chunk.
const paraSep = "\n\n"

// maxPartBytes is the size of the parts a paragraph over maxChunkBytes is cut
// into: small enough that each part still takes a full overlap.
const maxPartBytes = maxChunkBytes - maxOverlapBytes - len(paraSep)

func (o Options) resolved() Options {
	if o.TargetWords <= 0 {
		o.TargetWords = defaultTargetWords
	}
	if o.OverlapWords < 0 {
		o.OverlapWords = 0
	}
	if o.OverlapWords == 0 {
		o.OverlapWords = defaultOverlapWords
	}
	return o
}

// FromTokens converts a token-shaped Options (chunk_size / chunk_overlap in
// tokens, as the user-facing config calls them) into the chunker's internal
// word-shaped Options. The conversion uses 0.75 words/token, which lines up
// with English markdown empirically.
func FromTokens(chunkSizeTokens, chunkOverlapTokens int) Options {
	const wordsPerToken = 0.75
	return Options{
		TargetWords:  int(float64(chunkSizeTokens) * wordsPerToken),
		OverlapWords: int(float64(chunkOverlapTokens) * wordsPerToken),
	}
}

// Split splits one note into chunks. content is the entire file (frontmatter
// + body). path is the vault-relative path. tier is the directory tier. The
// frontmatter is parsed inline and applied to every chunk.
//
// Returns nil (no error) when the note has no body — empty notes don't get
// indexed.
func Split(path, tier string, content []byte, opts Options) ([]Chunk, error) {
	opts = opts.resolved()

	fm, body, err := markdown.ParseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", path, err)
	}

	title := fm.Title
	if title == "" {
		// Fallback: filename stem.
		base := path
		if idx := strings.LastIndexAny(base, "/\\"); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndexByte(base, '.'); idx >= 0 {
			base = base[:idx]
		}
		title = base
	}

	sections := splitByHeadings(string(body))
	if len(sections) == 0 {
		return nil, nil
	}

	pathHash := sha256.Sum256([]byte(path))
	idPrefix := hex.EncodeToString(pathHash[:])[:8]
	evidence := markdown.EvidenceOf(fm)
	status := vault.StatusOf(path, fm)

	var chunks []Chunk
	seq := 0
	for _, sec := range sections {
		secChunks := chunkSection(sec, opts)
		for _, body := range secChunks {
			if strings.TrimSpace(body.text) == "" {
				continue
			}
			h := sha256.Sum256([]byte(body.text))
			chunks = append(chunks, Chunk{
				ID:          fmt.Sprintf("%s-%04d", idPrefix, seq),
				Path:        path,
				NoteID:      fm.ID,
				Title:       title,
				HeadingPath: sec.headingPath,
				Body:        body.text,
				Tier:        tier,
				Tags:        append([]string(nil), fm.Tags...),
				Type:        fm.Type,
				Status:      status,
				Evidence:    evidence,
				CharStart:   sec.start + body.charStart,
				CharEnd:     sec.start + body.charEnd,
				Hash:        hex.EncodeToString(h[:])[:16],
				Seq:         seq,
			})
			seq++
		}
	}
	return chunks, nil
}

// section is one heading-bounded slice of the body.
type section struct {
	headingPath string
	text        string
	start       int // byte offset of this section in body
}

// splitByHeadings walks body line-by-line, tracking heading stack and code
// fences, and yields a section per heading boundary. The leading pre-heading
// region (if any) becomes a section with empty headingPath.
func splitByHeadings(body string) []section {
	var sections []section

	stack := []string{} // current heading hierarchy (level → text)
	headingLevels := []int{}

	flushStart := 0
	currentPath := ""

	inFence := false
	var fenceMarker string

	pos := 0
	for pos < len(body) {
		nl := strings.IndexByte(body[pos:], '\n')
		var line string
		var lineEnd int
		if nl < 0 {
			line = body[pos:]
			lineEnd = len(body)
		} else {
			line = body[pos : pos+nl]
			lineEnd = pos + nl + 1
		}

		trimmed := strings.TrimRight(line, "\r")
		// Code fence detection — only opens/closes on lines that begin with
		// ``` or ~~~ (allowing leading whitespace, mirroring CommonMark).
		fence := detectFence(trimmed)
		if !inFence && fence != "" {
			inFence = true
			fenceMarker = fence
		} else if inFence && (fence == fenceMarker || fence != "" && fence == fenceMarker) {
			inFence = false
			fenceMarker = ""
		}

		// Heading detection — ignored inside code fences.
		if !inFence {
			if level, text := atxHeading(trimmed); level > 0 {
				// Flush the section preceding this heading.
				if pos > flushStart {
					sections = append(sections, section{
						headingPath: currentPath,
						text:        body[flushStart:pos],
						start:       flushStart,
					})
				}
				// Update heading stack (pop deeper-or-equal levels).
				for len(headingLevels) > 0 && headingLevels[len(headingLevels)-1] >= level {
					headingLevels = headingLevels[:len(headingLevels)-1]
					stack = stack[:len(stack)-1]
				}
				headingLevels = append(headingLevels, level)
				stack = append(stack, text)
				currentPath = strings.Join(stack, " › ")
				flushStart = lineEnd
			}
		}

		pos = lineEnd
	}

	if flushStart < len(body) {
		sections = append(sections, section{
			headingPath: currentPath,
			text:        body[flushStart:],
			start:       flushStart,
		})
	}

	// Drop sections that are pure whitespace.
	out := sections[:0]
	for _, s := range sections {
		if strings.TrimSpace(s.text) != "" {
			out = append(out, s)
		}
	}
	return out
}

// atxHeading parses an ATX heading. Returns (level, text). level is 0 when
// the line is not a heading. Setext headings (=== / ---) are not handled —
// they are vanishingly rare in this vault.
func atxHeading(line string) (int, string) {
	s := strings.TrimLeft(line, " \t")
	level := 0
	for level < len(s) && s[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	rest := s[level:]
	if rest == "" {
		return 0, ""
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return 0, ""
	}
	text := strings.TrimSpace(rest)
	// Strip optional trailing # ###.
	text = strings.TrimRight(text, " #\t")
	if text == "" {
		return 0, ""
	}
	return level, text
}

// detectFence returns the fence marker ("```" or "~~~") for a line that
// opens or closes a fenced code block; "" otherwise. Allows the optional
// info string after the marker.
func detectFence(line string) string {
	s := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(s, "```") {
		return "```"
	}
	if strings.HasPrefix(s, "~~~") {
		return "~~~"
	}
	return ""
}

// chunkPiece represents one chunk's text plus its bounds within its section.
type chunkPiece struct {
	text      string
	charStart int
	charEnd   int
}

// chunkSection packs a section's paragraphs into chunks of at most
// opts.TargetWords words and maxChunkBytes bytes, applying overlap between
// adjacent chunks. A paragraph over maxChunkBytes is cut into parts first; a
// paragraph or part at the word target or over it is a chunk of its own.
func chunkSection(s section, opts Options) []chunkPiece {
	k := packer{opts: opts}
	for _, p := range splitParagraphs(s.text) {
		for _, part := range boundParagraph(p) {
			k.add(part)
		}
	}
	k.emit()
	return k.pieces
}

// packer accumulates a section's paragraphs into chunk pieces.
type packer struct {
	opts   Options
	pieces []chunkPiece
	buf    []paragraph
	words  int // words in buf
	size   int // bytes of joinParas(buf)
}

// add appends p to the chunk being packed, first emitting that chunk when p
// does not fit beside it, and dropping the overlap when p does not fit
// beside that either.
func (k *packer) add(p paragraph) {
	if !k.fits(p) {
		k.emit()
	}
	if !k.fits(p) {
		k.reset()
	}
	if len(k.buf) > 0 {
		k.size += len(paraSep)
	}
	k.buf = append(k.buf, p)
	k.words += p.words
	k.size += len(p.text)
}

// fits reports whether p joins the chunk being packed within both bounds.
func (k *packer) fits(p paragraph) bool {
	return len(k.buf) == 0 ||
		(k.words+p.words <= k.opts.TargetWords &&
			k.size+len(paraSep)+len(p.text) <= maxChunkBytes)
}

// emit turns the packed paragraphs into a chunk piece and carries its
// trailing words into the next chunk as a synthetic overlap paragraph,
// which does not move the section bounds. A buffer holding only overlap is
// dropped, never emitted.
func (k *packer) emit() {
	own := k.buf
	if len(own) > 0 && own[0].overlap {
		own = own[1:]
	}
	if len(own) == 0 {
		k.reset()
		return
	}
	text := joinParas(k.buf)
	k.pieces = append(k.pieces, chunkPiece{
		text:      text,
		charStart: own[0].start,
		charEnd:   own[len(own)-1].end,
	})
	k.reset()
	if over := tailWords(text, k.opts.OverlapWords, maxOverlapBytes); over != "" {
		k.buf = []paragraph{{text: over, words: countWords(over), overlap: true}}
		k.words, k.size = k.buf[0].words, len(over)
	}
}

func (k *packer) reset() {
	k.buf, k.words, k.size = nil, 0, 0
}

// boundParagraph returns p whole when it fits maxChunkBytes, else cut into
// consecutive parts of at most maxPartBytes (see cutPoint). A fence is cut
// like any other paragraph, since a chunk the model refuses is not indexed
// at all.
func boundParagraph(p paragraph) []paragraph {
	if len(p.text) <= maxChunkBytes {
		return []paragraph{p}
	}
	var parts []paragraph
	for text, start := p.text, p.start; text != ""; {
		cut, next := cutPoint(text, maxPartBytes)
		if part := text[:cut]; strings.TrimSpace(part) != "" {
			parts = append(parts, paragraph{
				text:  part,
				start: start,
				end:   start + cut,
				words: countWords(part),
			})
		}
		text, start = text[next:], start+next
	}
	return parts
}

// cutPoint returns where the first part of text ends (cut) and the rest
// begins (next) for a part of at most limit bytes: the whole text when it
// fits, else at the last line break within the limit, else the last space or
// tab, dropping that byte, else the last character boundary, so a table, a
// long list and a line with no spaces are all cut. limit is well over
// utf8.UTFMax; text that is not UTF-8 is cut where the search for a
// boundary gives up.
func cutPoint(text string, limit int) (cut, next int) {
	if len(text) <= limit {
		return len(text), len(text)
	}
	window := text[:limit+1]
	if i := strings.LastIndexByte(window, '\n'); i > 0 {
		return i, i + 1
	}
	if i := strings.LastIndexAny(window, " \t"); i > 0 {
		return i, i + 1
	}
	cut = limit
	for cut > limit-utf8.UTFMax+1 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut, cut
}

// paragraph is one block of text (a regular paragraph, a list, or a fenced
// code block treated as a unit) within a section.
type paragraph struct {
	text       string
	start, end int
	words      int
	overlap    bool // synthetic carry-over from previous chunk
}

// splitParagraphs walks section text and emits paragraphs split by blank
// lines, with fenced code blocks held intact. Returned bounds are relative
// to the section start (caller adds section.start to make them body-relative).
func splitParagraphs(text string) []paragraph {
	var paras []paragraph
	var buf strings.Builder
	bufStart := 0
	pos := 0

	inFence := false
	var fenceMarker string

	flush := func(end int) {
		body := strings.TrimRight(buf.String(), "\n")
		if strings.TrimSpace(body) != "" {
			paras = append(paras, paragraph{
				text:  body,
				start: bufStart,
				end:   end,
				words: countWords(body),
			})
		}
		buf.Reset()
	}

	for pos < len(text) {
		nl := strings.IndexByte(text[pos:], '\n')
		var line string
		var lineEnd int
		if nl < 0 {
			line = text[pos:]
			lineEnd = len(text)
		} else {
			line = text[pos : pos+nl]
			lineEnd = pos + nl + 1
		}
		trimmed := strings.TrimRight(line, "\r")

		fence := detectFence(trimmed)
		if !inFence && fence != "" {
			// Boundary into a fence: flush any preceding paragraph, then
			// start accumulating the fenced block.
			if buf.Len() > 0 {
				flush(pos)
			}
			bufStart = pos
			inFence = true
			fenceMarker = fence
			buf.WriteString(line)
			buf.WriteByte('\n')
			pos = lineEnd
			continue
		}
		if inFence {
			buf.WriteString(line)
			buf.WriteByte('\n')
			if fence == fenceMarker {
				flush(lineEnd)
				inFence = false
				fenceMarker = ""
			}
			pos = lineEnd
			continue
		}

		if strings.TrimSpace(trimmed) == "" {
			if buf.Len() > 0 {
				flush(pos)
			}
			pos = lineEnd
			continue
		}

		if buf.Len() == 0 {
			bufStart = pos
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		pos = lineEnd
	}
	if buf.Len() > 0 {
		flush(len(text))
	}
	return paras
}

// joinParas concatenates paragraphs with blank-line separators, preserving
// the within-paragraph layout.
func joinParas(paras []paragraph) string {
	var b strings.Builder
	for i, p := range paras {
		if i > 0 {
			b.WriteString(paraSep)
		}
		b.WriteString(p.text)
	}
	return b.String()
}

// countWords returns the count of whitespace-separated tokens in s.
func countWords(s string) int {
	return len(strings.Fields(s))
}

// tailWords returns the trailing whitespace-separated tokens of s, at most n
// of them and at most maxBytes bytes once joined with single spaces. Returns
// "" when n <= 0, s is empty or its last token alone is over maxBytes.
func tailWords(s string, n, maxBytes int) string {
	fields := strings.Fields(s)
	start, size := len(fields), 0
	for start > 0 && len(fields)-start < n {
		grown := size + len(fields[start-1])
		if start < len(fields) {
			grown++ // the joining space
		}
		if grown > maxBytes {
			break
		}
		start, size = start-1, grown
	}
	return strings.Join(fields[start:], " ")
}
