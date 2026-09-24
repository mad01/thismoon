package web

import (
	"bytes"
	"fmt"
	"html"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/mad01/thismoon/services/speak/internal/chunk"
)

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// chunkAttr lists, on each read block element, the keys of the parts that
// read it; <wk-read-aloud> highlights by it.
const chunkAttr = "data-ra-chunk"

// partsAttr lists, on each section element, the keys of the section's parts
// in reading order, repeats included; <wk-read-aloud> plays by it. Element
// order cannot stand in for it: a list item whose paragraphs surround a
// nested list is read before and after that list.
const partsAttr = "data-ra-parts"

// Part is one request's worth of a section: the text synthesized in one go
// and the key its audio is cached under.
type Part struct {
	Key  string
	Text string
}

// Plan is a rendered markdown document and the parts that read it aloud.
type Plan struct {
	// HTML is the document split into <section class="doc-section"
	// data-section="N" data-ra-parts="..."> blocks, each read block tagged
	// with chunkAttr.
	HTML string
	// Sections holds each section's parts in reading order; Sections[0]
	// is data-section="1".
	Sections [][]Part
}

// block is one unit of reading: its text and the element that shows it. A
// list item's own paragraphs all show in the li.
type block struct {
	node ast.Node
	text string
}

// PlanSections renders markdown into HTML split into
// <section class="doc-section" data-section="N"> blocks, and plans the parts
// each section is read in. A new section starts at every level-1 or level-2
// heading; content before the first such heading forms its own section.
// Sections are the per-button play units of <wk-read-aloud>. key names a
// part's audio from its text.
//
// Headings, paragraphs and list items are read, including those inside
// blockquotes; code blocks, tables, raw HTML, thematic breaks and images are
// shown but not read and carry no chunkAttr.
func PlanSections(source []byte, key func(text string) string) (Plan, error) {
	doc := md.Parser().Parse(text.NewReader(source))
	groups := splitSections(doc)
	plan := Plan{Sections: make([][]Part, len(groups))}
	for i, group := range groups {
		plan.Sections[i] = planSection(group, source, key)
	}
	rendered, err := renderSections(groups, plan.Sections, source)
	if err != nil {
		return Plan{}, err
	}
	plan.HTML = rendered
	return plan, nil
}

// splitSections groups the document's top-level nodes into sections.
func splitSections(doc ast.Node) [][]ast.Node {
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
	return groups
}

// planSection plans a section's readable blocks (see planBlocks) and tags
// each block's node with the keys of the parts covering it. A node showing
// several blocks (a list item whose paragraphs surround a nested list) lists
// the keys of all of them, each once.
func planSection(group []ast.Node, source []byte, key func(string) string) []Part {
	var blocks []block
	for _, n := range group {
		blocks = collectBlocks(blocks, n, source)
	}
	texts := make([]string, len(blocks))
	for j, b := range blocks {
		texts[j] = b.text
	}
	plan := planBlocks(texts, key)
	tags := make(map[ast.Node][]string)
	for j, b := range blocks {
		for _, k := range plan.blockKeys[j] {
			if keys := tags[b.node]; !slices.Contains(keys, k) {
				tags[b.node] = append(keys, k)
			}
		}
	}
	for n, keys := range tags {
		n.SetAttributeString(chunkAttr, strings.Join(keys, " "))
	}
	return plan.parts
}

// sectionPlan is one section's parts in reading order and, per block, the
// keys of the parts that read it.
type sectionPlan struct {
	parts []Part
	// blockKeys[j] covers block j, in play order; empty, never nil, for a
	// block with nothing speakable, so it encodes as [].
	blockKeys [][]string
}

// planBlocks groups a section's block texts into parts sized by
// chunk.Prepared and names each part's audio with key. Both forms of POST
// /read plan through it, so a block posted as text gets the key the same
// block rendered from markdown does. A block is one piece unless it is longer
// than a part may be; then it splits at sentences, so a part boundary falls
// inside a block only when it must.
func planBlocks(texts []string, key func(string) string) sectionPlan {
	var pieces []string
	var owners []int // owners[i] is the block pieces[i] came from
	for j, text := range texts {
		for _, p := range blockPieces(text) {
			pieces = append(pieces, p)
			owners = append(owners, j)
		}
	}
	plan := sectionPlan{blockKeys: make([][]string, len(texts))}
	for j := range plan.blockKeys {
		plan.blockKeys[j] = []string{}
	}
	for _, members := range chunk.Group(pieces, chunk.Prepared) {
		own := make([]string, len(members))
		for i, idx := range members {
			own[i] = pieces[idx]
		}
		part := Part{Text: chunk.Join(own)}
		part.Key = key(part.Text)
		plan.parts = append(plan.parts, part)
		for _, idx := range members {
			j := owners[idx]
			if !slices.Contains(plan.blockKeys[j], part.Key) {
				plan.blockKeys[j] = append(plan.blockKeys[j], part.Key)
			}
		}
	}
	return plan
}

// collectBlocks appends the readable blocks under n in reading order.
func collectBlocks(out []block, n ast.Node, source []byte) []block {
	switch n.(type) {
	case *ast.Heading, *ast.Paragraph:
		return append(out, block{node: n, text: inlineText(n, source)})
	case *ast.ListItem:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c.(type) {
			case *ast.TextBlock, *ast.Paragraph:
				out = append(out, block{node: n, text: inlineText(c, source)})
			default: // a nested list, a quote: blocks of their own
				out = collectBlocks(out, c, source)
			}
		}
		return out
	case *ast.List, *ast.Blockquote:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			out = collectBlocks(out, c, source)
		}
		return out
	default: // code, tables, raw HTML, thematic breaks: shown, not read
		return out
	}
}

// blockPieces is a block's text made speakable, as one piece when it fits
// in a part and split into sentences when it does not.
func blockPieces(text string) []string {
	s := chunk.Speakable(text)
	switch {
	case s == "":
		return nil
	case utf8.RuneCountInString(s) <= chunk.MaxChars:
		return []string{s}
	default:
		return chunk.Pieces(s)
	}
}

// inlineText is the text a reader hears for n's inline content: text with
// escapes and entities resolved, inline code as written, link labels, and a
// space for each line break. Images and raw HTML are skipped.
func inlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Image, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if seg, ok := t.(*ast.Text); ok {
					b.Write(seg.Value(source))
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.AutoLink:
			b.Write(c.Label(source))
		case *ast.String:
			b.Write(c.Value)
		case *ast.Text:
			writeText(&b, c, source)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// writeText writes a text node as the HTML renderer shows it.
func writeText(b *strings.Builder, t *ast.Text, source []byte) {
	value := t.Value(source)
	if !t.IsRaw() {
		value = util.ResolveEntityNames(
			util.ResolveNumericReferences(util.UnescapePunctuations(value)),
		)
	}
	b.Write(value)
	if t.SoftLineBreak() || t.HardLineBreak() {
		b.WriteByte(' ')
	}
}

// keysOf is the keys of parts in order; empty, never nil, so it encodes as [].
func keysOf(parts []Part) []string {
	keys := make([]string, len(parts))
	for i, p := range parts {
		keys[i] = p.Key
	}
	return keys
}

// partKeys is the keys of parts in order, space-separated.
func partKeys(parts []Part) string {
	return strings.Join(keysOf(parts), " ")
}

// renderSections renders each group as a numbered doc-section listing its
// parts; a section with nothing to read lists none.
func renderSections(groups [][]ast.Node, sections [][]Part, source []byte) (string, error) {
	var out bytes.Buffer
	for i, group := range groups {
		fmt.Fprintf(&out, `<section class="doc-section" data-section="%d"`, i+1)
		if keys := partKeys(sections[i]); keys != "" {
			fmt.Fprintf(&out, ` %s="%s"`, partsAttr, html.EscapeString(keys))
		}
		out.WriteByte('>')
		for _, n := range group {
			if err := md.Renderer().Render(&out, source, n); err != nil {
				return "", fmt.Errorf("render markdown node: %w", err)
			}
		}
		out.WriteString("</section>\n")
	}
	return out.String(), nil
}
