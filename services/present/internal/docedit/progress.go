package docedit

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// ProgressHeading heads the section a progress report lands in when it
// names no section and no block carries its label yet. The section is
// created at the end of the Doc the first time.
const ProgressHeading = "Progress"

// maxPercent is a full progress bar.
const maxPercent = 100

// LabelConflictError is a progress report whose label more than one
// progress block carries. Sections are the zero-based indices of the
// sections holding them, one entry per block; name one with the section
// field to report into it.
type LabelConflictError struct {
	Label    string
	Sections []int
}

func (e *LabelConflictError) Error() string {
	return fmt.Sprintf(
		"present: %d progress blocks carry label %q, in sections %v; name the section to report into",
		len(e.Sections),
		e.Label,
		e.Sections,
	)
}

// ProgressRequest is a progress report as the route or the CLI receives it.
// Percent is a pointer so a missing pct differs from zero.
type ProgressRequest struct {
	Label     string
	Percent   *float64
	Note      string
	Section   string
	Rendition string
}

// Progress is one parsed progress report: the progress block labelled
// Label shows Percent, with Note after the label. Section, when set, names
// the section to look and insert in, as a section patch's anchor does.
type Progress struct {
	Rendition Rendition
	Label     string
	Percent   float64
	Note      string
	Section   string
}

// ProgressResult says where a report landed: the section holding the
// block, and whether the block was new.
type ProgressResult struct {
	Section  int
	Anchor   string
	Inserted bool
}

// blockRef addresses one top-level block of a Doc.
type blockRef struct{ section, block int }

// ParseProgress checks a report: a label, and a pct from 0 to 100.
func ParseProgress(r ProgressRequest) (Progress, error) {
	p := Progress{
		Label: strings.TrimSpace(r.Label), Note: strings.TrimSpace(r.Note),
		Section: strings.TrimSpace(r.Section),
	}
	if p.Label == "" {
		return Progress{}, invalid("progress needs a label")
	}
	if r.Percent == nil {
		return Progress{}, invalid("progress needs pct")
	}
	p.Percent = *r.Percent
	if math.IsNaN(p.Percent) || p.Percent < 0 || p.Percent > maxPercent {
		return Progress{}, invalid("pct %v is outside 0 to %d", p.Percent, maxPercent)
	}
	var err error
	if p.Rendition, err = parseRendition(r.Rendition); err != nil {
		return Progress{}, err
	}
	return p, nil
}

// Edit returns the store edit that updates the progress block carrying
// p's label, or inserts one, and recompiles the Doc. res is filled in each
// time the edit succeeds.
func (p Progress) Edit(res *ProgressResult) store.EditFunc {
	return func(cur store.Snapshot) (store.Snapshot, error) {
		src := p.Rendition.source(&cur.Sources)
		if *src == nil {
			return store.Snapshot{}, fmt.Errorf("%w: %s", ErrNoDoc, p.Rendition)
		}
		var doc render.Doc
		if err := json.Unmarshal(*src, &doc); err != nil {
			return store.Snapshot{}, fmt.Errorf("stored %s Doc: %w", p.Rendition, err)
		}
		at, inserted, err := p.place(&doc)
		if err != nil {
			return store.Snapshot{}, err
		}
		c, err := p.Rendition.compile(doc, cur.Page.Title)
		if err != nil {
			return store.Snapshot{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		*src = c.JSON
		p.Rendition.setHTML(&cur.Page, c.HTML)
		*res = ProgressResult{
			Section: at, Anchor: render.SectionAnchor(doc.Sections[at].Heading), Inserted: inserted,
		}
		return cur, nil
	}
}

// place writes p's block into doc: over the one block carrying its label
// within the named section, or the whole Doc when none is named, else as a
// new block at the end of that section. With no section named and no
// block found, it goes to the Progress section, created when missing. It
// returns the section index and whether the block is new.
func (p Progress) place(doc *render.Doc) (int, bool, error) {
	block := render.Block{T: "progress", Percent: p.Percent, Label: p.Label, Note: p.Note}
	scope := -1
	if p.Section != "" {
		i, err := resolve(doc.Sections, p.Section)
		if err != nil {
			return 0, false, err
		}
		scope = i
	}
	hits := p.carrying(doc.Sections, scope)
	switch {
	case len(hits) == 1:
		doc.Sections[hits[0].section].Blocks[hits[0].block] = block
		return hits[0].section, false, nil
	case len(hits) > 1:
		conflict := &LabelConflictError{Label: p.Label}
		for _, h := range hits {
			conflict.Sections = append(conflict.Sections, h.section)
		}
		return 0, false, conflict
	}
	at, err := insertionSection(doc, scope)
	if err != nil {
		return 0, false, err
	}
	doc.Sections[at].Blocks = append(doc.Sections[at].Blocks, block)
	return at, true, nil
}

// carrying lists the top-level progress blocks whose label is p's, within
// section scope or every section when scope is -1. A stored label went
// through name normalization, so both sides are compared normalized.
func (p Progress) carrying(sections []render.Section, scope int) []blockRef {
	want := render.NormalizeNames(p.Label)
	var hits []blockRef
	for i, s := range sections {
		if scope >= 0 && i != scope {
			continue
		}
		for j, b := range s.Blocks {
			if b.T == "progress" && render.NormalizeNames(b.Label) == want {
				hits = append(hits, blockRef{section: i, block: j})
			}
		}
	}
	return hits
}

// insertionSection is the section a new progress block goes to: the named
// one, else the one Progress section, appended to the Doc when missing.
func insertionSection(doc *render.Doc, scope int) (int, error) {
	if scope >= 0 {
		return scope, nil
	}
	anchor := render.SectionAnchor(ProgressHeading)
	held := holding(doc.Sections, anchor, -1)
	switch len(held) {
	case 0:
		doc.Sections = append(
			doc.Sections,
			render.Section{Heading: ProgressHeading, Blocks: []render.Block{}},
		)
		return len(doc.Sections) - 1, nil
	case 1:
		return held[0], nil
	}
	return 0, &AnchorConflictError{Anchor: anchor, Indices: held}
}
