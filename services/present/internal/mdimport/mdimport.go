// Package mdimport converts a markdown file into the title and Doc a present
// page is rendered from, so a file written by hand or by another tool becomes
// a page without an agent rewriting it as Doc JSON. The mapping is lossy
// where present has no equivalent: raw HTML, thematic breaks, and footnotes
// are dropped, nested lists flatten into their parent, a blockquote becomes
// one callout, and inline markup is rewritten into present's own inline
// syntax, which the renderer applies to prose again (so a literal asterisk
// in the source still italicises).
package mdimport

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// ErrEmpty is returned when the markdown yields no blocks at all.
var ErrEmpty = errors.New("mdimport: markdown has no content to import")

// Page is a converted markdown file: the page title and the Doc it renders
// from.
type Page struct {
	Title string
	Doc   render.Doc
}

// introHeading titles the section holding what sits between the title and
// the first level-2 heading, once the summary has been taken from it.
const introHeading = "Introduction"

// untitled is the title of a file with no level-1 heading and no usable name.
const untitled = "Untitled"

// md parses GitHub-flavored markdown. Footnotes are parsed so that both the
// references and the definitions can be dropped as units; without the
// extension they would come through as literal text.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))

// alertMarker is the GitHub alert prefix a blockquote may open with.
var alertMarker = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*`)

// Convert parses src as markdown and maps it onto a Doc. name is the file
// name, which titles the page when the document has no level-1 heading.
func Convert(name string, src []byte) (Page, error) {
	root := md.Parser().Parse(text.NewReader(src))
	c := &converter{src: src}
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		c.top(n)
	}
	return c.page(name)
}

// converter accumulates the Doc while walking the document's top level.
type converter struct {
	src      []byte
	title    string
	intro    []render.Block // blocks before the first level-2 heading
	sections []render.Section
	emphasis int // nesting depth of the emphasis being written
}

// top routes one top-level node: headings shape the outline, anything else
// becomes blocks of the current section.
func (c *converter) top(n ast.Node) {
	if h, ok := n.(*ast.Heading); ok {
		c.heading(h)
		return
	}
	c.add(c.blocks(n)...)
}

// heading takes the first level-1 heading as the title, opens a section for
// every level-2 heading (and every later level-1 one), and turns deeper
// levels into subheadings.
func (c *converter) heading(h *ast.Heading) {
	txt := c.plain(h)
	switch {
	case h.Level == 1 && c.title == "":
		c.title = txt
	case h.Level <= 2:
		if txt == "" {
			txt = "Section"
		}
		c.sections = append(c.sections, render.Section{Heading: txt})
	case txt != "":
		c.add(render.Block{T: "h3", Text: txt})
	}
}

// add appends blocks to the open section, or to the intro when no level-2
// heading has opened one yet.
func (c *converter) add(bs ...render.Block) {
	if len(c.sections) == 0 {
		c.intro = append(c.intro, bs...)
		return
	}
	s := &c.sections[len(c.sections)-1]
	s.Blocks = append(s.Blocks, bs...)
}

// page assembles the Doc. A leading paragraph before the first section is
// the page summary when anything else follows it; alone it stays the body,
// so a one-paragraph file is not reduced to a title and a blurb. Whatever
// else precedes the first section becomes an Introduction section, and a
// document with no sections gets one named after the page.
func (c *converter) page(name string) (Page, error) {
	title := c.title
	if title == "" {
		title = titleFromName(name)
	}
	var doc render.Doc
	intro := c.intro
	if len(intro) > 0 && intro[0].T == "p" && (len(intro) > 1 || len(c.sections) > 0) {
		doc.Summary = intro[0].Text
		intro = intro[1:]
	}
	switch {
	case len(c.sections) == 0:
		doc.Sections = []render.Section{{Heading: title, Blocks: intro}}
	case len(intro) > 0:
		doc.Sections = append(
			[]render.Section{{Heading: introHeading, Blocks: intro}}, c.sections...,
		)
	default:
		doc.Sections = c.sections
	}
	total := 0
	for i := range doc.Sections {
		if doc.Sections[i].Blocks == nil {
			doc.Sections[i].Blocks = []render.Block{}
		}
		total += len(doc.Sections[i].Blocks)
	}
	if total == 0 {
		return Page{}, ErrEmpty
	}
	return Page{Title: title, Doc: doc}, nil
}

// titleFromName is the file name without directory and extension.
func titleFromName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	base = strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return untitled
	}
	return base
}

// blocks maps one block node onto zero or more Doc blocks.
func (c *converter) blocks(n ast.Node) []render.Block {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		if t := c.inline(n); t != "" {
			return []render.Block{{T: "p", Text: t}}
		}
	case *ast.Heading:
		if t := c.plain(n); t != "" {
			return []render.Block{{T: "h3", Text: t}}
		}
	case *ast.List:
		return c.list(n)
	case *ast.FencedCodeBlock:
		return []render.Block{{T: "code", Lang: string(n.Language(c.src)), Text: c.lines(n)}}
	case *ast.CodeBlock:
		return []render.Block{{T: "code", Text: c.lines(n)}}
	case *extast.Table:
		return c.table(n)
	case *ast.Blockquote:
		return c.callout(n)
	}
	// Raw HTML, thematic breaks, footnote definitions: present has nothing
	// to map them to.
	return nil
}

// lines is a code block's text: its source lines as written, without the
// trailing newline the fence closes on.
func (c *converter) lines(n ast.Node) string {
	var b strings.Builder
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		b.Write(seg.Value(c.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

// listBuild flattens one markdown list, nested lists included, into list
// blocks. A child present cannot hold in a list item (a code block, a table,
// a quote) closes the list block, is emitted on its own, and a new list
// block collects the items after it, so nothing is dropped and order is
// kept; an ordered list restarts at 1 after the split.
type listBuild struct {
	ordered bool
	items   []string
	out     []render.Block
}

func (b *listBuild) flush() {
	if len(b.items) > 0 {
		b.out = append(b.out, render.Block{T: "list", Ordered: b.ordered, Items: b.items})
		b.items = nil
	}
}

func (c *converter) list(l *ast.List) []render.Block {
	b := &listBuild{ordered: l.IsOrdered()}
	c.listItems(l, b)
	b.flush()
	return b.out
}

// listItems adds l's items to b. An item's paragraphs join with a space; a
// nested list's items follow their parent item in the same flat list.
func (c *converter) listItems(l *ast.List, b *listBuild) {
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		var texts []string
		emit := func() {
			if len(texts) > 0 {
				b.items = append(b.items, strings.Join(texts, " "))
				texts = nil
			}
		}
		for ch := item.FirstChild(); ch != nil; ch = ch.NextSibling() {
			switch ch := ch.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				if t := c.inline(ch); t != "" {
					texts = append(texts, t)
				}
			case *ast.List:
				emit()
				c.listItems(ch, b)
			default:
				// Only a child that yields blocks splits the list; a dropped
				// one (raw HTML, a rule) leaves the items together.
				if bs := c.blocks(ch); len(bs) > 0 {
					emit()
					b.flush()
					b.out = append(b.out, bs...)
				}
			}
		}
		emit()
	}
}

// table maps a GFM table: header cells as plain text, since present renders
// them without inline markup, body cells through the inline mapping.
func (c *converter) table(t *extast.Table) []render.Block {
	b := render.Block{T: "table"}
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		_, header := row.(*extast.TableHeader)
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if header {
				cells = append(cells, c.plain(cell))
			} else {
				cells = append(cells, c.inline(cell))
			}
		}
		if header {
			b.Cols = cells
		} else {
			b.Rows = append(b.Rows, cells)
		}
	}
	if len(b.Cols) == 0 && len(b.Rows) == 0 {
		return nil
	}
	return []render.Block{b}
}

// callout maps a blockquote onto one callout: its blocks flattened into one
// text, info unless a GitHub alert marker names a warning.
func (c *converter) callout(q *ast.Blockquote) []render.Block {
	txt := c.flatten(q)
	sev := "info"
	if m := alertMarker.FindStringSubmatch(txt); m != nil {
		if m[1] == "WARNING" || m[1] == "CAUTION" {
			sev = "warn"
		}
		txt = strings.TrimSpace(txt[len(m[0]):])
	}
	if txt == "" {
		return nil
	}
	return []render.Block{{T: "callout", Severity: sev, Text: txt}}
}

// flatten joins the text of every block under n with spaces into one string
// present will render as prose: paragraphs and table cells through the
// inline mapping, headings and code as neutralized text (code in backticks
// when it can be), containers recursively.
func (c *converter) flatten(n ast.Node) string {
	var parts []string
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		var part string
		switch ch := ch.(type) {
		case *ast.Paragraph, *ast.TextBlock, *extast.TableCell:
			part = c.inline(ch)
		case *ast.Heading:
			part = neutralize(c.plain(ch))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			part = codeSpan(c.lines(ch))
		case *ast.HTMLBlock, *ast.ThematicBreak:
		default:
			part = c.flatten(ch)
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

// codeSpan writes code that has to live inside prose: on one line, neutral
// to present's link and chip syntax, and in backticks unless it holds one.
func codeSpan(code string) string {
	s := neutralize(strings.ReplaceAll(code, "\n", " "))
	if s == "" || strings.Contains(s, "`") {
		return s
	}
	return "`" + s + "`"
}

// inlineBuf assembles one run of inline text for present. Everything the
// converter writes as text lands in a pending run that neutralize sees as a
// whole once the run ends, so a link or chip spelled across several nodes,
// or glued together by a node that emits nothing (raw HTML, a footnote
// reference, an image with no alt text), cannot reach the page. Links the
// converter emits itself bypass that pass, since they are what it protects.
type inlineBuf struct {
	out  strings.Builder
	text strings.Builder
}

func (b *inlineBuf) WriteString(s string) { b.text.WriteString(s) }

// link ends the pending text run and writes a present link verbatim.
func (b *inlineBuf) link(s string) {
	b.endRun()
	b.out.WriteString(s)
}

func (b *inlineBuf) endRun() {
	b.out.WriteString(neutralize(b.text.String()))
	b.text.Reset()
}

// String ends the run and returns the assembled text, trimmed.
func (b *inlineBuf) String() string {
	b.endRun()
	return strings.TrimSpace(b.out.String())
}

// inline maps n's inline content onto present's inline syntax.
func (c *converter) inline(n ast.Node) string {
	var b inlineBuf
	c.writeInline(&b, n, false)
	return b.String()
}

// plain writes n's inline content as words alone, for headings and table
// header cells, which present renders without inline markup.
func (c *converter) plain(n ast.Node) string {
	var b inlineBuf
	c.writeInline(&b, n, true)
	return b.String()
}

func (c *converter) writeInline(b *inlineBuf, n ast.Node, plain bool) {
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch ch := ch.(type) {
		case *ast.Text:
			c.writeText(b, ch)
		case *ast.String:
			b.WriteString(string(ch.Value))
		case *ast.CodeSpan:
			c.writeCode(b, ch, plain)
		case *ast.Emphasis:
			c.writeEmphasis(b, ch, plain)
		case *ast.Link:
			c.writeLink(b, ch, plain)
		case *ast.AutoLink:
			c.writeAutoLink(b, ch, plain)
		case *ast.Image:
			b.WriteString(c.plain(ch)) // the alt text
		case *extast.TaskCheckBox:
			if ch.IsChecked {
				b.WriteString("[x] ")
			} else {
				b.WriteString("[ ] ")
			}
		case *ast.RawHTML, *extast.FootnoteLink:
			// present has nothing to map them to.
		default:
			// Strikethrough and anything else: keep the words, drop the mark.
			c.writeInline(b, ch, plain)
		}
	}
}

// writeText writes a text node as a reader sees it: escapes and entities
// resolved, a space for a line break.
func (c *converter) writeText(b *inlineBuf, t *ast.Text) {
	v := t.Value(c.src)
	if !t.IsRaw() {
		v = unescape(v)
	}
	b.WriteString(string(v))
	if t.SoftLineBreak() || t.HardLineBreak() {
		b.WriteString(" ")
	}
}

// unescape resolves backslash escapes and character references the way the
// markdown renderer would before showing the bytes.
func unescape(v []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
}

// neutralize breaks the two present inline forms that text could otherwise
// spell by accident: `](` (a link the renderer would honour, even one
// goldmark refused because of a control character in the URL) and `@chip(`.
// Inside a code span both would also render as a stray token. A space after
// the bracket or the chip name is the one edit that stops all of it.
func neutralize(s string) string {
	s = strings.ReplaceAll(s, "](", "] (")
	return strings.ReplaceAll(s, "@chip(", "@chip (")
}

// writeCode writes a code span in backticks. present's syntax has no escape
// for a backtick inside one, so code holding a backtick is written as words.
func (c *converter) writeCode(b *inlineBuf, n *ast.CodeSpan, plain bool) {
	var code strings.Builder
	for t := n.FirstChild(); t != nil; t = t.NextSibling() {
		if seg, ok := t.(*ast.Text); ok {
			code.Write(seg.Value(c.src))
		}
	}
	s := strings.ReplaceAll(code.String(), "\n", " ")
	if plain || strings.Contains(s, "`") {
		b.WriteString(s)
		return
	}
	b.WriteString("`" + s + "`")
}

// writeEmphasis writes *italic* or **bold**. present's renderer restores
// code spans and links after it applies bold, so a bold span holding either
// would render as a stray token: the words stay, the bold goes. Emphasis
// nested in emphasis is written without its own mark for the same reason.
func (c *converter) writeEmphasis(b *inlineBuf, e *ast.Emphasis, plain bool) {
	mark := "*"
	switch {
	case plain, c.emphasis > 0, e.Level >= 2 && holdsLinkOrCode(e):
		mark = ""
	case e.Level >= 2:
		mark = "**"
	}
	c.emphasis++
	b.WriteString(mark)
	c.writeInline(b, e, plain)
	b.WriteString(mark)
	c.emphasis--
}

// holdsLinkOrCode reports whether a code span or link sits anywhere under n.
func holdsLinkOrCode(n ast.Node) bool {
	found := false
	_ = ast.Walk(n, func(ch ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch ch.(type) {
		case *ast.CodeSpan, *ast.Link, *ast.AutoLink:
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// writeLink writes [label](href). The label is words alone, because
// present's link regex stops at the first closing bracket and applies no
// markup inside one. The destination is unescaped the way the markdown
// renderer would before it is checked, so `&amp;` reaches the page as `&`.
func (c *converter) writeLink(b *inlineBuf, l *ast.Link, plain bool) {
	label := c.plain(l)
	href := safeHref(string(unescape(l.Destination)))
	if label == "" {
		label = href
	}
	c.writeLinkSyntax(b, label, href, plain)
}

// writeAutoLink writes a bare URL or email address as a link to itself.
func (c *converter) writeAutoLink(b *inlineBuf, l *ast.AutoLink, plain bool) {
	label := string(l.Label(c.src))
	href := string(l.URL(c.src))
	if l.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(href, "mailto:") {
		href = "mailto:" + href
	}
	c.writeLinkSyntax(b, label, safeHref(href), plain)
}

// writeLinkSyntax writes present's [label](href), or the label alone when
// the link cannot be expressed: no href survived, or the label holds the
// bracket that would end it early. The label is neutralized here because a
// finished link bypasses the run-level pass.
func (c *converter) writeLinkSyntax(b *inlineBuf, label, href string, plain bool) {
	if plain || href == "" || strings.Contains(label, "]") {
		b.WriteString(label)
		return
	}
	b.link(fmt.Sprintf("[%s](%s)", neutralize(label), href))
}

// safeHref returns href fit for a present link, or "" when the renderer
// would refuse it (render.LinkHrefAllowed: http, https, mailto, relative),
// in which case only the label is worth writing. A closing parenthesis is
// percent-encoded because present's link syntax ends at the first one.
func safeHref(href string) string {
	h := strings.TrimSpace(href)
	if h == "" || !render.LinkHrefAllowed(h) {
		return ""
	}
	return strings.ReplaceAll(h, ")", "%29")
}
