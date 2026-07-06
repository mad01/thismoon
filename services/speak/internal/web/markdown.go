package web

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// RenderSections renders markdown into HTML split into
// <section class="doc-section"> blocks. A new section starts at every level-1
// or level-2 heading; content before the first such heading forms its own
// section. Sections are the per-button play units of <wk-read-aloud>.
func RenderSections(source []byte) (string, error) {
	doc := md.Parser().Parse(text.NewReader(source))

	var groups [][]ast.Node
	var current []ast.Node
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level <= 2 && len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
		current = append(current, n)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}

	var out bytes.Buffer
	for _, group := range groups {
		out.WriteString(`<section class="doc-section">`)
		for _, n := range group {
			if err := md.Renderer().Render(&out, source, n); err != nil {
				return "", fmt.Errorf("render markdown node: %w", err)
			}
		}
		out.WriteString("</section>\n")
	}
	return out.String(), nil
}
