package goscan

import "strings"

// Doc is a file's blocks joined into one plain-text document, plus the map
// from document line back to the Go source line behind it. Detection runs
// once per file over Text instead of once per block, and SourceLine turns
// each finding's line back into a source location.
type Doc struct {
	// Text is the joined prose, one blank line between blocks.
	Text string
	// src holds the source line of every document line, indexed by the
	// 1-based document line. Index 0 is unused padding.
	src []int
}

// Render joins blocks into a document. Blocks arrive in source order and
// keep their internal line offsets, so a finding on the third line of a doc
// comment maps to the third line of that comment in the source.
func Render(blocks []Block) Doc {
	var b strings.Builder
	src := []int{0}
	for i, blk := range blocks {
		if i > 0 {
			b.WriteByte('\n')
			src = append(src, blk.Line)
		}
		for offset, line := range strings.Split(blk.Text, "\n") {
			b.WriteString(line)
			b.WriteByte('\n')
			src = append(src, blk.Line+offset)
		}
	}
	return Doc{Text: b.String(), src: src}
}

// SourceLine returns the Go source line behind a 1-based document line, or
// 0 when the line falls outside the document.
func (d Doc) SourceLine(line int) int {
	if line < 1 || line >= len(d.src) {
		return 0
	}
	return d.src[line]
}
