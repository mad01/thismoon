package derive

import (
	"bytes"
	"context"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// derived is the chosen before-content, where it came from, and the
// after-content it yields when fit could derive one.
type derived struct {
	source  store.BeforeSource
	before  content
	applied content
}

// candidate is one before-source, loaded only when the ones ahead of it
// did not fit.
type candidate struct {
	source store.BeforeSource
	load   func() (content, error)
}

// chooseBefore tries the before-sources in order and returns the first
// that fits the tool call. With neither hunks nor an input to check
// against, only the sources the harness vouches for are taken.
func (d *Deriver) chooseBefore(
	ctx context.Context,
	e store.Edit,
	p *spool.EditPayload,
	in inputs,
) (derived, error) {
	var h history
	candidates := []candidate{
		{store.BeforeAbsent, func() (content, error) { return absent(p, in), nil }},
		{store.BeforeOriginal, func() (content, error) { return in.original, nil }},
		{store.BeforeChain, func() (content, error) { return d.chain(ctx, e, &h) }},
		{store.BeforeSnapshot, func() (content, error) { return d.snapshotted(ctx, e, &h) }},
		{store.BeforeHead, func() (content, error) { return in.head, nil }},
		{store.BeforeObserved, func() (content, error) { return unreplaced(in), nil }},
		{store.BeforeObserved, func() (content, error) { return reversed(in), nil }},
	}
	for _, c := range candidates {
		if !checkable(in) && !shipped(c.source) {
			break
		}
		before, err := c.load()
		if err != nil {
			return derived{}, err
		}
		if !before.ok {
			continue
		}
		applied, ok := fit(before, in)
		if !ok {
			continue
		}
		if c.source == store.BeforeAbsent {
			before = content{} // nothing existed, so there is no chunk to keep
		}
		return derived{source: c.source, before: before, applied: applied}, nil
	}
	return derived{source: store.BeforeUnknown}, nil
}

// shipped reports the sources the harness vouches for itself; the others
// are guesses that only the hunks can confirm.
func shipped(s store.BeforeSource) bool {
	return s == store.BeforeAbsent || s == store.BeforeOriginal
}

// checkable reports evidence that can confirm a guessed before-content.
func checkable(in inputs) bool {
	return len(in.hunks) > 0 || in.edits != nil
}

// fit checks a candidate before-content against the tool call and returns
// the after-content when it can derive one. The after-content comes from
// the tool input, never from the hunks, when the input has it:
//
//   - Edit and MultiEdit: see fitReplacements.
//   - Write: the hunks must apply and lead to the written content, both
//     with tabs read as two spaces; the caller takes the written content.
//   - Hunks without an input: see fitHunks.
func fit(before content, in inputs) (content, bool) {
	switch {
	case in.edits != nil:
		return fitReplacements(before.data, in)
	case len(in.hunks) == 0:
		return content{}, true
	case in.written.ok:
		got, ok := applyUntabbed(before.data, in.hunks)
		return content{}, ok && sameLines(got, untab(in.written.data))
	default:
		return fitHunks(before.data, in)
	}
}

// fitReplacements checks a candidate against an Edit or MultiEdit input.
// The candidate must take the call's hunks, when it shipped any, with tabs
// read as two spaces, and no replacement may break the occurrence rule;
// an edit without hunks rests on the rule alone. The replacements give
// the after-content when the hunks, read the same way, lead to it too.
// When the hunks lead elsewhere, or a replacement's text is missing from
// the candidate, a normalization the harness applies explains it, and the
// hunks stand in for the replacements.
func fitReplacements(before []byte, in inputs) (content, bool) {
	hunked := len(in.hunks) > 0
	var patched []byte
	if hunked {
		got, ok := applyUntabbed(before, in.hunks)
		if !ok {
			return content{}, false
		}
		patched = got
	}
	after, o := replace(before, in.edits)
	switch {
	case o == swapped && (!hunked || sameLines(untab(after), patched)):
		return known(after), true
	case hunked && (o == swapped || o == missing):
		return fitHunks(before, in)
	default:
		return content{}, false
	}
}

// fitHunks fits a candidate to the hunks alone: a strict apply yields the
// after-content; an apply with tabs read as two spaces confirms the
// candidate and no more, so the after-content comes from the disk read.
// A disk read that differs from the strict apply only where the hunks wrote
// a tab as two spaces is the after-content: the hunks cannot tell the two
// apart, and the file can.
func fitHunks(before []byte, in inputs) (content, bool) {
	applied, err := patch.Apply(before, in.hunks)
	if err != nil {
		_, ok := applyUntabbed(before, in.hunks)
		return content{}, ok
	}
	if in.observed.ok && sameLines(untab(applied), untab(in.observed.data)) {
		return in.observed, true
	}
	return known(applied), true
}

// The harness writes every tab in its hunks as two spaces, while the file,
// its original and the tool input keep the tab. Comparing with tabs read as
// two spaces on both sides puts them on the same footing; the expanded
// bytes are for comparing only and never become a stored content.
var tab, twoSpaces = []byte("\t"), []byte("  ")

func untab(b []byte) []byte { return bytes.ReplaceAll(b, tab, twoSpaces) }

// applyUntabbed applies the hunks to before with tabs read as two spaces
// on both sides and returns the result in that form.
func applyUntabbed(before []byte, hunks []patch.Hunk) ([]byte, bool) {
	expanded := make([]patch.Hunk, len(hunks))
	for i, h := range hunks {
		lines := make([]string, len(h.Lines))
		for j, l := range h.Lines {
			lines[j] = strings.ReplaceAll(l, "\t", "  ")
		}
		h.Lines = lines
		expanded[i] = h
	}
	got, err := patch.Apply(untab(before), expanded)
	return got, err == nil
}

// absent is the before of a create the harness reported without an
// original: nothing, which the hunks and replacements apply to as the empty
// file. A create is a payload that says so, or an Edit or MultiEdit whose
// first replacement replaces nothing, also when the person changed it in
// the permission dialog.
func absent(p *spool.EditPayload, in inputs) content {
	if !in.original.ok && (p.Kind == store.EditCreate || creates(p.Tool, p.Input)) {
		return known(nil)
	}
	return content{}
}

// chain is the after-content of the latest earlier Edit of the same path,
// made at or before e and not e itself, which a replayed envelope finds
// already recorded. A snapshot taken since that edit makes it stale.
func (d *Deriver) chain(ctx context.Context, e store.Edit, h *history) (content, error) {
	if err := d.recall(ctx, e, h); err != nil {
		return content{}, err
	}
	if h.edit == nil || h.chainStale() {
		return content{}, nil
	}
	return d.get(ctx, h.edit.AfterChunk)
}

// snapshotted is the path's content in the worktree's latest snapshot taken
// at or before e, or unknown when that snapshot does not hold it.
func (d *Deriver) snapshotted(ctx context.Context, e store.Edit, h *history) (content, error) {
	if err := d.recall(ctx, e, h); err != nil {
		return content{}, err
	}
	if h.file == nil || h.file.Chunk == "" {
		return content{}, nil
	}
	return d.get(ctx, h.file.Chunk)
}

// unreplaced is the disk read with the input's replacements undone (see
// unreplace), or unknown when they cannot be. Like any other candidate it
// must still fit the call.
func unreplaced(in inputs) content {
	if !in.observed.ok || in.edits == nil {
		return content{}
	}
	before, ok := unreplace(in.observed.data, in.edits)
	if !ok {
		return content{}
	}
	return known(before)
}

// reversed is the disk read with the hunks reversed strictly, or unknown
// when they do not reverse. It is tried when unreplaced is unknown or does
// not fit: undoing a swap finds the first occurrence of its new text, which
// need not be the one the call wrote.
func reversed(in inputs) content {
	if !in.observed.ok || len(in.hunks) == 0 {
		return content{}
	}
	before, err := patch.Reverse(in.observed.data, in.hunks)
	if err != nil {
		return content{}
	}
	return known(before)
}

// sameLines compares two contents with CRLF read as LF.
func sameLines(a, b []byte) bool {
	crlf, lf := []byte("\r\n"), []byte("\n")
	return bytes.Equal(bytes.ReplaceAll(a, crlf, lf), bytes.ReplaceAll(b, crlf, lf))
}
