// Package derive turns what the capture hook shipped into the records
// serve appends. For one file a tool changed the hook ships evidence: the
// tool input, the harness's hunks, the pre-edit file when the harness had
// it, the file at HEAD, and a disk read taken right after the tool ran.
// The Deriver takes the before-content from the first source the tool call
// fits, derives the after-content from the tool input where it can, and
// records a disagreement with the disk read as an UnattributedDelta,
// because something besides the tool wrote the file. Evidence that does
// not fit degrades the record; it never fails it. For a snapshot it
// stores the file map and records an UnattributedDelta for every file
// whose content no recorded edit explains.
package derive

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// toolWrite is the harness tool that replaces a whole file. With no word
// from the payload, a Write whose before-content no source explains
// created the file.
const toolWrite = "Write"

// toolBash is the shell tool. A file its diff lists without hunks has the
// disk read as its after-content.
const toolBash = "Bash"

// Deriver derives Edit and Snapshot records. Chunks resolves the payload's
// chunk keys and stores derived content. The lookups are the store's
// (store.Store); a nil one skips the source it feeds.
type Deriver struct {
	Chunks store.Chunks
	// LatestAfter is the chain lookup.
	LatestAfter func(ctx context.Context, q store.ChainQuery) (store.Edit, error)
	// LatestSnapshot finds the worktree's snapshot before an edit or a
	// newer snapshot.
	LatestSnapshot func(ctx context.Context, q store.SnapshotQuery) (store.Snapshot, error)
	// Edits lists a path's edits, so a snapshot delta is not claimed for a
	// change a recorded edit without after-content made.
	Edits func(ctx context.Context, q store.EditQuery) ([]store.Edit, error)
}

// content is file bytes that may be unknown; ok tells an empty file from
// no information.
type content struct {
	data []byte
	ok   bool
}

func known(b []byte) content {
	if b == nil {
		b = []byte{}
	}
	return content{data: b, ok: true}
}

// inputs is an edit payload with its chunks loaded and its tool input read.
type inputs struct {
	// edits are the replacements an Edit or MultiEdit input asks for.
	edits    []replacement
	hunks    []patch.Hunk
	original content
	// written is the after-content the tool input carried (Write).
	written  content
	observed content
	head     content
}

// Edit derives the Edit for an edit envelope and, when the disk read after
// the tool disagrees with the derived after-content, the UnattributedDelta
// that records the difference. The before-content is the first source the
// tool call fits (see fit): nothing for a create, the harness's original,
// the after-content of the latest earlier Edit of the path unless a
// snapshot taken since speaks for it, the path in the latest snapshot, the
// file at HEAD, or the disk read with the call undone; BeforeUnknown when
// none does. An edit the person changed in the permission dialog is fitted
// by its hunks alone (see load). A file a Bash command changed without a
// diff takes the disk read as its after-content (see observedOnly). An
// excluded payload yields an Edit without content. Errors come from the
// chunk store and the store lookups only.
func (d *Deriver) Edit(
	ctx context.Context,
	env spool.Envelope,
) (store.Edit, *store.UnattributedDelta, error) {
	p := env.Edit
	if p == nil {
		return store.Edit{}, nil, fmt.Errorf(
			"derive: envelope %s (%s) has no edit payload",
			env.ID,
			env.Kind,
		)
	}
	e := identity(env, p)
	if p.Excluded {
		e.Excluded = true
		return e, nil, nil
	}
	e.PatchChunk = p.PatchChunk
	e.ObservedChunk = p.ObservedChunk
	in, err := d.load(ctx, p)
	if err != nil {
		return store.Edit{}, nil, err
	}
	switch {
	case checkable(in) || in.written.ok:
		return d.withPatch(ctx, e, p, in)
	case p.Tool == toolBash:
		return d.observedOnly(ctx, e, p, in)
	default:
		return d.withoutPatch(ctx, e, p, in)
	}
}

// identity fills what every Edit carries, excluded or not.
func identity(env spool.Envelope, p *spool.EditPayload) store.Edit {
	kind := p.Kind
	if kind == "" {
		kind = store.EditUnknown
	}
	return store.Edit{
		ID:           fmt.Sprintf("%s:%d", p.ToolUseID, p.Index),
		ToolUseID:    p.ToolUseID,
		SessionID:    env.Session.ID,
		TurnID:       env.Turn.ID,
		AgentID:      env.Agent.ID,
		AgentType:    env.Agent.Type,
		Tool:         p.Tool,
		At:           env.At,
		Location:     p.Location,
		Path:         p.Path,
		Kind:         kind,
		BeforeSource: store.BeforeUnknown,
	}
}

// withoutPatch records a tool that reported neither hunks, replacements nor
// content, such as a notebook edit: the disk read is all there is, so the
// after-content stays unknown and only the harness's original can stand as
// the before.
func (d *Deriver) withoutPatch(
	ctx context.Context,
	e store.Edit,
	p *spool.EditPayload,
	in inputs,
) (store.Edit, *store.UnattributedDelta, error) {
	e.Kind = store.EditUnknown
	if deleted(p) {
		e.Kind = store.EditDelete
	}
	if in.original.ok {
		e.BeforeSource = store.BeforeOriginal
	}
	if err := d.keep(ctx, &e, in.original, content{}); err != nil {
		return store.Edit{}, nil, err
	}
	e.EOL = p.EOL
	return e, nil, nil
}

// observedOnly records a file a Bash command changed that the harness
// named without a diff. The disk read is the after-content, and nothing
// can check a before against the call. The before is the harness's
// original when it shipped one, else the latest content known of the path:
// the chain, else the snapshot, else HEAD. The chain yields to a snapshot
// taken after it, so the newer of the two wins. A file the disk read did
// not find was deleted.
func (d *Deriver) observedOnly(
	ctx context.Context,
	e store.Edit,
	p *spool.EditPayload,
	in inputs,
) (store.Edit, *store.UnattributedDelta, error) {
	var h history
	sources := []candidate{
		{store.BeforeOriginal, func() (content, error) { return in.original, nil }},
		{store.BeforeChain, func() (content, error) { return d.chain(ctx, e, &h) }},
		{store.BeforeSnapshot, func() (content, error) { return d.snapshotted(ctx, e, &h) }},
		{store.BeforeHead, func() (content, error) { return in.head, nil }},
	}
	var before content
	for _, c := range sources {
		got, err := c.load()
		if err != nil {
			return store.Edit{}, nil, err
		}
		if got.ok {
			before, e.BeforeSource = got, c.source
			break
		}
	}
	after := in.observed
	switch {
	case deleted(p):
		e.Kind, after = store.EditDelete, content{}
	case before.ok:
		e.Kind = store.EditUpdate
	default:
		e.Kind = store.EditUnknown
	}
	if err := d.keep(ctx, &e, before, after); err != nil {
		return store.Edit{}, nil, err
	}
	e.EOL = p.EOL
	if after.ok {
		e.EOL = patch.DetectEOL(after.data)
	}
	return e, nil, nil
}

// withPatch records a tool that reported hunks, replacements or its whole
// after-content.
func (d *Deriver) withPatch(
	ctx context.Context,
	e store.Edit,
	p *spool.EditPayload,
	in inputs,
) (store.Edit, *store.UnattributedDelta, error) {
	got, err := d.chooseBefore(ctx, e, p, in)
	if err != nil {
		return store.Edit{}, nil, err
	}
	e.BeforeSource = got.source
	e.Kind = kind(p, got)
	after, fromObserved := pickAfter(e.Kind, in, got)
	if err := d.keep(ctx, &e, got.before, after); err != nil {
		return store.Edit{}, nil, err
	}
	e.EOL = p.EOL
	if !after.ok {
		return e, nil, nil
	}
	e.EOL = patch.DetectEOL(after.data)
	// An after-content taken from the disk read cannot be checked against it.
	if fromObserved {
		return e, nil, nil
	}
	matches, compared := observedMatches(after, in.observed, e.AfterBlob, p.ObservedBlob)
	if !compared {
		return e, nil, nil
	}
	e.ObservedMatches = &matches
	if matches || !in.observed.ok {
		return e, nil, nil
	}
	delta, err := d.delta(ctx, e, after, in.observed)
	if err != nil {
		return store.Edit{}, nil, err
	}
	return e, delta, nil
}

// kind decides what the tool call did: the payload's word for a create, a
// delete the payload reports and the missing disk read confirms, a create
// when nothing existed before, a create for a Write with no kind and no
// explainable before, an update otherwise.
func kind(p *spool.EditPayload, got derived) store.EditKind {
	switch {
	case p.Kind == store.EditCreate:
		return store.EditCreate
	case deleted(p):
		return store.EditDelete
	case got.source == store.BeforeAbsent:
		return store.EditCreate
	case p.Kind == "" && p.Tool == toolWrite && got.source == store.BeforeUnknown:
		return store.EditCreate
	default:
		return store.EditUpdate
	}
}

// deleted reports a delete the payload claims and the disk read backs: the
// hook found no file after the tool ran.
func deleted(p *spool.EditPayload) bool {
	return p.Kind == store.EditDelete && p.ObservedChunk == "" && p.ObservedBlob == ""
}

// pickAfter chooses the after-content: what a Write carried, else what fit
// derived from the chosen before, else, as a last resort, the disk read. A
// delete has none.
func pickAfter(k store.EditKind, in inputs, got derived) (after content, fromObserved bool) {
	switch {
	case k == store.EditDelete:
		return content{}, false
	case in.written.ok:
		return in.written, false
	case got.applied.ok:
		return got.applied, false
	case in.observed.ok:
		return in.observed, true
	default:
		return content{}, false
	}
}

// load resolves the payload's chunk keys. A patch that does not parse
// counts as absent: the Edit keeps PatchChunk, so the raw hunks survive
// for a later derivation. The replacements of an edit the person changed
// in the permission dialog are left out: the tool input is the model's
// proposal, and only the hunks describe what was written.
func (d *Deriver) load(ctx context.Context, p *spool.EditPayload) (inputs, error) {
	var in inputs
	if !p.UserModified {
		in.edits = replacements(p.Tool, p.Input)
	}
	raw, err := d.get(ctx, p.PatchChunk)
	if err != nil {
		return inputs{}, err
	}
	if raw.ok {
		if hunks, err := patch.Parse(raw.data); err == nil {
			in.hunks = hunks
		}
	}
	for _, c := range []struct {
		key string
		dst *content
	}{
		{p.OriginalChunk, &in.original},
		{p.ContentChunk, &in.written},
		{p.ObservedChunk, &in.observed},
		{p.HeadChunk, &in.head},
	} {
		if *c.dst, err = d.get(ctx, c.key); err != nil {
			return inputs{}, err
		}
	}
	return in, nil
}

func (d *Deriver) get(ctx context.Context, key string) (content, error) {
	if key == "" {
		return content{}, nil
	}
	b, err := d.Chunks.Get(ctx, key)
	if err != nil {
		return content{}, fmt.Errorf("derive: chunk %s: %w", key, err)
	}
	return known(b), nil
}

// keep stores the derived contents and points the Edit at them.
func (d *Deriver) keep(ctx context.Context, e *store.Edit, before, after content) error {
	if before.ok {
		key, err := d.Chunks.Put(ctx, before.data)
		if err != nil {
			return fmt.Errorf("derive: store before-content of %s: %w", e.Path, err)
		}
		e.BeforeChunk = key
	}
	if after.ok {
		key, err := d.Chunks.Put(ctx, after.data)
		if err != nil {
			return fmt.Errorf("derive: store after-content of %s: %w", e.Path, err)
		}
		e.AfterChunk = key
		e.AfterBlob = gitlink.BlobID(after.data)
	}
	return nil
}
