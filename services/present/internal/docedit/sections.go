// Package docedit edits part of a page's Doc as one store edit: a section
// patch finds a section by its table-of-contents anchor, or by index,
// applies one operation, and recompiles the whole Doc through the renderer,
// so a bad section is refused like a bad Doc. Every edit here is a
// store.EditFunc, so the read, the change, and the write happen under the
// store's lock and a concurrent writer is never overwritten. The HTTP routes
// and the MCP tools both parse their input into these types.
package docedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/mdimport"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// ErrInvalid marks an edit refused for its own content: an unknown op or
// rendition, a missing or surplus section, a progress report without a
// label or with a pct outside 0 to 100, or a Doc the renderer refuses.
var ErrInvalid = errors.New("present: invalid edit")

// ErrAnchorNotFound is a patch naming a section the Doc does not have.
var ErrAnchorNotFound = errors.New("present: no section has that anchor")

// ErrNoDoc is a patch on a rendition with no Doc source: a brief stored as
// legacy HTML, or a page without a deck.
var ErrNoDoc = errors.New("present: the rendition has no Doc source to patch")

// VersionConflictError is a patch written against a version the page has
// moved past. Current is the version to read again from.
type VersionConflictError struct {
	Base    int
	Current int
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf(
		"present: page is at version %d, not base version %d; read it again and retry",
		e.Current, e.Base,
	)
}

// AnchorConflictError is a patch refused because several sections share an
// anchor: the anchor it names is ambiguous, or, when Creating is set, the
// section it writes would give a second section that anchor. Indices are
// the sections holding the anchor, zero-based, for addressing one as
// index:N.
type AnchorConflictError struct {
	Anchor   string
	Indices  []int
	Creating bool
}

func (e *AnchorConflictError) Error() string {
	if e.Creating {
		return fmt.Sprintf(
			"present: anchor %q is already used by section(s) %v; rename the heading",
			e.Anchor, e.Indices,
		)
	}
	return fmt.Sprintf(
		"present: anchor %q names sections %v; address one as %s<index>",
		e.Anchor, e.Indices, IndexPrefix,
	)
}

// IndexPrefix starts the index form of an anchor: index:0 is the first
// section. It addresses a section whose anchor is shared or empty, and no
// heading-derived anchor can contain the colon.
const IndexPrefix = "index:"

// Rendition names which of a page's Docs an edit works on.
type Rendition string

// The renditions a page may carry, each its own Doc.
const (
	Content Rendition = "content"
	Deck    Rendition = "deck"
)

// Op is what a section patch does.
type Op string

// The section operations. Append adds at the end and names no anchor;
// the others act at the section the anchor names.
const (
	OpReplace      Op = "replace"
	OpInsertBefore Op = "insert_before"
	OpInsertAfter  Op = "insert_after"
	OpRemove       Op = "remove"
	OpAppend       Op = "append"
)

var ops = []Op{OpReplace, OpInsertBefore, OpInsertAfter, OpRemove, OpAppend}

// Body formats a section arrives in.
const (
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

// SectionPatch is one parsed section operation on a page's Doc.
type SectionPatch struct {
	Rendition Rendition
	Op        Op
	// Anchor names the section to act at: its heading-derived anchor (a
	// heading or #anchor is accepted and normalized) or IndexPrefix plus a
	// zero-based index. Empty for append.
	Anchor string
	// Section is the section to write; unused by remove.
	Section render.Section
	// BaseVersion, when above zero, is the page version the patch was
	// written against; the patch is refused once the page has moved on.
	BaseVersion int
}

// Request is a section patch as a route or tool receives it, before
// parsing. Section is Doc section JSON, or markdown text when BodyFormat is
// markdown.
type Request struct {
	Op          string
	Anchor      string
	Rendition   string
	BaseVersion int
	BodyFormat  string
	Section     []byte
}

// Result says where a patch landed: the index and anchor of the section it
// wrote, or of the one it removed.
type Result struct {
	Index  int
	Anchor string
}

// Parse checks a request and turns it into a patch. A markdown body goes
// through mdimport.ConvertSection, so it must open with the heading that
// names the section.
func Parse(r Request) (SectionPatch, error) {
	p := SectionPatch{Op: Op(r.Op), Anchor: strings.TrimSpace(r.Anchor), BaseVersion: r.BaseVersion}
	if !slices.Contains(ops, p.Op) {
		return SectionPatch{}, invalid("unknown op %q (want one of %s)", r.Op, joinOps())
	}
	var err error
	if p.Rendition, err = parseRendition(r.Rendition); err != nil {
		return SectionPatch{}, err
	}
	if r.BaseVersion < 0 {
		return SectionPatch{}, invalid("base_version %d is negative", r.BaseVersion)
	}
	switch {
	case p.Op == OpAppend && p.Anchor != "":
		return SectionPatch{}, invalid("append names no anchor")
	case p.Op != OpAppend && p.Anchor == "":
		return SectionPatch{}, invalid("op %s needs an anchor", p.Op)
	}
	hasBody := len(strings.TrimSpace(string(r.Section))) > 0
	switch {
	case p.Op == OpRemove && hasBody:
		return SectionPatch{}, invalid("remove takes no section")
	case p.Op == OpRemove:
		return p, nil
	case !hasBody:
		return SectionPatch{}, invalid("op %s needs a section", p.Op)
	}
	if p.Section, err = parseSection(r.Section, r.BodyFormat); err != nil {
		return SectionPatch{}, err
	}
	return p, nil
}

func parseRendition(s string) (Rendition, error) {
	switch Rendition(s) {
	case "", Content:
		return Content, nil
	case Deck:
		return Deck, nil
	}
	return "", invalid("unknown rendition %q (want content or deck)", s)
}

func parseSection(body []byte, format string) (render.Section, error) {
	switch format {
	case "", FormatJSON:
		var s render.Section
		if err := json.Unmarshal(body, &s); err != nil {
			return render.Section{}, fmt.Errorf("%w: section: %w", ErrInvalid, err)
		}
		if s.Blocks == nil {
			s.Blocks = []render.Block{}
		}
		return s, nil
	case FormatMarkdown:
		s, err := mdimport.ConvertSection(body)
		if err != nil {
			return render.Section{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		return s, nil
	}
	return render.Section{}, invalid("unknown body_format %q (want json or markdown)", format)
}

// Check compiles the patch's section alone, so a section the renderer
// refuses is turned away before anything else is done for it, such as
// storing the images it names. The edit compiles the whole Doc again.
func (p SectionPatch) Check(title string) error {
	if p.Op == OpRemove {
		return nil
	}
	if _, err := p.Rendition.compile(render.Doc{Sections: []render.Section{p.Section}}, title); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

// Edit returns the store edit that applies p to the page's current Doc and
// recompiles it under the page's title. res is filled in each time the edit
// succeeds; read it once the store's Edit returned without error.
func (p SectionPatch) Edit(res *Result) store.EditFunc {
	return func(cur store.Snapshot) (store.Snapshot, error) {
		if p.BaseVersion > 0 && cur.Page.Version != p.BaseVersion {
			return store.Snapshot{}, &VersionConflictError{
				Base:    p.BaseVersion,
				Current: cur.Page.Version,
			}
		}
		src := p.Rendition.source(&cur.Sources)
		if *src == nil {
			return store.Snapshot{}, fmt.Errorf("%w: %s", ErrNoDoc, p.Rendition)
		}
		var doc render.Doc
		if err := json.Unmarshal(*src, &doc); err != nil {
			return store.Snapshot{}, fmt.Errorf("stored %s Doc: %w", p.Rendition, err)
		}
		sections, at, err := p.apply(doc.Sections)
		if err != nil {
			return store.Snapshot{}, err
		}
		landed := sections
		if p.Op == OpRemove {
			landed = doc.Sections
		}
		doc.Sections = sections
		c, err := p.Rendition.compile(doc, cur.Page.Title)
		if err != nil {
			return store.Snapshot{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		*src = c.JSON
		p.Rendition.setHTML(&cur.Page, c.HTML)
		*res = Result{Index: at, Anchor: render.SectionAnchor(landed[at].Heading)}
		return cur, nil
	}
}

// apply runs the op over a copy of sections and returns the new list with
// the index the op acted at.
func (p SectionPatch) apply(sections []render.Section) ([]render.Section, int, error) {
	out := slices.Clone(sections)
	if p.Op == OpAppend {
		if err := anchorFree(out, p.Section, -1); err != nil {
			return nil, 0, err
		}
		return append(out, p.Section), len(out), nil
	}
	i, err := resolve(out, p.Anchor)
	if err != nil {
		return nil, 0, err
	}
	switch p.Op {
	case OpRemove:
		return slices.Delete(out, i, i+1), i, nil
	case OpReplace:
		if err := anchorFree(out, p.Section, i); err != nil {
			return nil, 0, err
		}
		out[i] = p.Section
		return out, i, nil
	case OpInsertAfter:
		i++
	}
	if err := anchorFree(out, p.Section, -1); err != nil {
		return nil, 0, err
	}
	return slices.Insert(out, i, p.Section), i, nil
}

// resolve finds the one section anchor names.
func resolve(sections []render.Section, anchor string) (int, error) {
	if n, ok := strings.CutPrefix(anchor, IndexPrefix); ok {
		i, err := strconv.Atoi(n)
		if err != nil || i < 0 || i >= len(sections) {
			return 0, fmt.Errorf("%w: %s%s (the Doc has %d sections, index:0 to index:%d)",
				ErrAnchorNotFound, IndexPrefix, n, len(sections), len(sections)-1)
		}
		return i, nil
	}
	want := render.SectionAnchor(strings.TrimPrefix(anchor, "#"))
	if want == "" {
		return 0, invalid("anchor %q has no letters or digits; address the section as %s<index>",
			anchor, IndexPrefix)
	}
	matches := holding(sections, want, -1)
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("%w: %q (anchors: %s)", ErrAnchorNotFound, want, anchorList(sections))
	case 1:
		return matches[0], nil
	}
	return 0, &AnchorConflictError{Anchor: want, Indices: matches}
}

// anchorFree refuses s when its anchor is already held by a section other
// than skip. A heading with an empty anchor never clashes: such a section
// is addressed by index alone.
func anchorFree(sections []render.Section, s render.Section, skip int) error {
	a := render.SectionAnchor(s.Heading)
	if a == "" {
		return nil
	}
	if held := holding(sections, a, skip); len(held) > 0 {
		return &AnchorConflictError{Anchor: a, Indices: held, Creating: true}
	}
	return nil
}

// holding lists the indices of the sections whose anchor is a, leaving out
// skip.
func holding(sections []render.Section, a string, skip int) []int {
	var out []int
	for i, s := range sections {
		if i != skip && render.SectionAnchor(s.Heading) == a {
			out = append(out, i)
		}
	}
	return out
}

func anchorList(sections []render.Section) string {
	names := make([]string, len(sections))
	for i, s := range sections {
		names[i] = render.SectionAnchor(s.Heading)
	}
	return strings.Join(names, ", ")
}

// source points at the rendition's Doc source within src.
func (r Rendition) source(src *store.Sources) *[]byte {
	if r == Deck {
		return &src.Deck
	}
	return &src.Doc
}

// setHTML stores the rendition's rendered HTML on the page.
func (r Rendition) setHTML(p *store.Page, html string) {
	if r == Deck {
		p.Deck = html
		return
	}
	p.Content = html
}

func (r Rendition) compile(d render.Doc, title string) (render.Compiled, error) {
	if r == Deck {
		return render.CompileDeckDoc(d, title)
	}
	return render.CompileDoc(d, title)
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func joinOps() string {
	names := make([]string, len(ops))
	for i, o := range ops {
		names[i] = string(o)
	}
	return strings.Join(names, ", ")
}
