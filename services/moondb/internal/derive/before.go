package derive

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// derived is the chosen before-content, where it came from, and the hunks
// applied to it.
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

// chooseBefore tries the before-sources in order and returns the first the
// hunks fit. Without hunks nothing can be checked, so only the harness's
// own original is taken.
func (d *Deriver) chooseBefore(
	ctx context.Context,
	id string,
	p *spool.EditPayload,
	in inputs,
) (derived, error) {
	candidates := []candidate{
		{store.BeforeAbsent, func() (content, error) { return absent(p, in), nil }},
		{store.BeforeOriginal, func() (content, error) { return in.original, nil }},
		{store.BeforeChain, func() (content, error) { return d.chain(ctx, id, p) }},
		{store.BeforeHead, func() (content, error) { return in.head, nil }},
		{store.BeforeObserved, func() (content, error) { return reversed(in), nil }},
	}
	for _, c := range candidates {
		if len(in.hunks) == 0 && !shipped(c.source) {
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

// fit applies the hunks to a candidate before-content. The candidate fits
// when every hunk line matches it and, for a Write, the result is the
// content the tool carried, line endings aside.
func fit(before content, in inputs) (content, bool) {
	if len(in.hunks) == 0 {
		return content{}, true
	}
	applied, err := patch.Apply(before.data, in.hunks)
	if err != nil {
		return content{}, false
	}
	if in.written.ok && !sameLines(applied, in.written.data) {
		return content{}, false
	}
	return known(applied), true
}

// absent is the before of a create the harness reported without an
// original: nothing, which the hunks apply to as the empty file.
func absent(p *spool.EditPayload, in inputs) content {
	if !in.original.ok && p.Kind == store.EditCreate {
		return known(nil)
	}
	return content{}
}

// chain is the after-content of the latest Edit of the same path. The
// lookup finds the edit being derived when a spool envelope is replayed
// after its Edit was appended; that is not a source.
func (d *Deriver) chain(ctx context.Context, id string, p *spool.EditPayload) (content, error) {
	if d.LatestAfter == nil {
		return content{}, nil
	}
	prev, err := d.LatestAfter(ctx, p.Location.Worktree, p.Path)
	if errors.Is(err, store.ErrNotFound) {
		return content{}, nil
	}
	if err != nil {
		return content{}, fmt.Errorf("derive: latest edit of %s: %w", p.Path, err)
	}
	if prev.ID == id {
		return content{}, nil
	}
	return d.get(ctx, prev.AfterChunk)
}

// reversed is the disk read with the hunks undone, or unknown when the
// read does not carry the hunks' result.
func reversed(in inputs) content {
	if !in.observed.ok {
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
