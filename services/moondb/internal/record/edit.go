package record

import (
	"fmt"
	"path/filepath"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// toolUsed records a PostToolUse payload. The switch is the seam per tool:
// file-editing tools record one edit per file; Bash capture lands here as
// its own case.
func (rec *recorder) toolUsed() error {
	switch rec.p.tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return rec.recordEdits()
	default:
		return nil
	}
}

// recordEdits writes one edit envelope per file the tool call names, then
// notes the file in the session state. The state follows the envelope, so
// a spool write that fails leaves the next edit of the path still shipping
// its HEAD content.
func (rec *recorder) recordEdits() error {
	for i, name := range rec.targets() {
		env, chunks := rec.captureEdit(i, name)
		if err := rec.spool.Write(env, chunks); err != nil {
			return err
		}
		rec.state.noteEdit(env.Edit.Location, env.Edit.Path)
	}
	return nil
}

// targets lists the files the tool call names. Every file tool touches one
// file today; the index in an edit's id leaves room for one that touches
// several.
func (rec *recorder) targets() []string {
	in := rec.p.inputFields
	for _, key := range []string{"file_path", "notebook_path"} {
		if name := in.str(key); name != "" {
			return []string{name}
		}
	}
	return nil
}

// captureEdit gathers everything about one file within the budget.
func (rec *recorder) captureEdit(index int, name string) (spool.Envelope, chunkSet) {
	abs := Resolve(name, rec.p.cwd)
	loc, rel := Placed(rec.locate(filepath.Dir(abs)), abs)
	ep := &spool.EditPayload{
		Tool:         rec.p.tool,
		ToolUseID:    rec.p.toolUseID,
		Index:        index,
		AbsPath:      abs,
		Path:         rel,
		Location:     loc,
		Kind:         EditKind(rec.p.tool, rec.p.response.str("type")),
		UserModified: rec.p.response.flag("userModified"),
	}
	chunks := chunkSet{}
	if Excluded(rec.opts.Excludes, abs, rel) {
		ep.Excluded = true
	} else {
		rec.gather(ep, chunks)
	}
	env := rec.envelope(spool.KindEdit, fmt.Sprintf("%s:%d", rec.p.toolUseID, index))
	env.Edit = ep
	return env, chunks
}

// gather fills the content fields: first what the payload carries, which
// costs nothing, then the disk read and the HEAD lookup while the budget
// lasts.
func (rec *recorder) gather(ep *spool.EditPayload, chunks chunkSet) {
	ep.Input = rec.p.input
	if patch := rec.p.response.array("structuredPatch"); patch != nil {
		ep.PatchChunk = chunks.add(patch)
	}
	if original, ok := rec.p.response.text("originalFile"); ok {
		ep.OriginalChunk = chunks.add([]byte(original))
	}
	if content, ok := rec.p.inputFields.text("content"); ok && ep.Tool == "Write" {
		ep.ContentChunk = chunks.add([]byte(content))
	}
	if rec.spent() {
		return
	}
	obs := observe(rec.ctx, ep.AbsPath)
	ep.ObservedBlob, ep.ObservedSize = obs.blob, obs.size
	if obs.read {
		ep.ObservedChunk = chunks.add(obs.data)
		ep.EOL = detectEOL(obs.data)
	}
	if rec.spent() || ep.Location.Head == "" {
		return
	}
	rec.gatherHead(ep, chunks, obs)
}

// gatherHead ships the blob id of the path at HEAD for every edit of a
// tracked file, and its content on the session's first edit of the path.
// After that the previous edit's after-content is the better before.
func (rec *recorder) gatherHead(ep *spool.EditPayload, chunks chunkSet, obs observed) {
	ctx, cancel := rec.gitCtx()
	defer cancel()
	wt := ep.Location.Worktree
	ep.HeadBlob = gitloc.Output(ctx, wt, "rev-parse", "--verify", "--quiet",
		ep.Location.Head+":"+ep.Path)
	if ep.HeadBlob == "" || rec.state.edited(wt, ep.Path) {
		return
	}
	if obs.read && obs.blob == ep.HeadBlob {
		ep.HeadChunk = ep.ObservedChunk
		return
	}
	data, ok := gitBytes(ctx, wt, moondb.MaxChunkBytes, "cat-file", "blob", ep.HeadBlob)
	if ok && blobID(data) == ep.HeadBlob {
		ep.HeadChunk = chunks.add(data)
	}
}
