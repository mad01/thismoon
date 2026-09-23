package playback

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// ExtractSections renders markdown and returns the plain text of each section,
// where a new section starts at every level-1 or level-2 heading (content
// before the first such heading is its own section). It is the Go equivalent of
// Python's _extract_text_from_md: split at h1/h2, then take the text.
func ExtractSections(source []byte) []string {
	doc := md.Parser().Parse(text.NewReader(source))

	var sections []string
	var current []ast.Node
	flush := func() {
		if len(current) == 0 {
			return
		}
		var b strings.Builder
		for _, n := range current {
			collectText(&b, n, source)
		}
		if t := strings.TrimSpace(b.String()); t != "" {
			sections = append(sections, t)
		}
		current = nil
	}

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level <= 2 && len(current) > 0 {
			flush()
		}
		current = append(current, n)
	}
	flush()
	return sections
}

// collectText appends the readable text of node (and its descendants) to b,
// including fenced/indented code block contents, then a trailing space so
// adjacent blocks don't run together.
func collectText(b *strings.Builder, node ast.Node, source []byte) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.FencedCodeBlock:
			writeLines(b, t.Lines(), source)
		case *ast.CodeBlock:
			writeLines(b, t.Lines(), source)
		}
		return ast.WalkContinue, nil
	})
	b.WriteByte(' ')
}

func writeLines(b *strings.Builder, lines *text.Segments, source []byte) {
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
}
