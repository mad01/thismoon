package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// toolUsed records a PostToolUse payload. The switch is the seam per tool:
// the file tools record one edit per file they name, and Bash one per file
// its diff lists.
func (rec *recorder) toolUsed() error {
	switch rec.p.tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return rec.recordEdits(rec.fileTargets())
	case toolBash:
		return rec.recordEdits(rec.bashTargets())
	default:
		return nil
	}
}

// target is one file a tool call changed, with what the payload says
// about it.
type target struct {
	name string
	kind store.EditKind
	// payload is set when the call's tool_input and tool_response describe
	// this file, as they do for the file tools; Bash's describe the command.
	payload bool
	// hunks is this file's patch when the payload lists one per file.
	hunks json.RawMessage
	// pathOnly is set when the payload names the file and nothing else: a
	// file gone from disk was then deleted.
	pathOnly bool
}

// recordEdits writes one edit envelope per target, then notes the file in
// the session state once its envelope is in the spool.
func (rec *recorder) recordEdits(targets []target) error {
	for i, t := range targets {
		env, chunks := rec.captureEdit(i, t)
		if err := rec.spool.Write(env, chunks); err != nil {
			return err
		}
		rec.state.noteEdit(env.Edit.Location, env.Edit.Path)
	}
	return nil
}

// fileTargets lists the file a file tool names. Every file tool touches one
// file today; the index in an edit's id leaves room for one that touches
// several.
func (rec *recorder) fileTargets() []target {
	in := rec.p.inputFields
	for _, key := range []string{"file_path", "notebook_path"} {
		if name := in.str(key); name != "" {
			kind := EditKind(rec.p.tool, rec.p.response.str("type"))
			return []target{{name: name, kind: kind, payload: true}}
		}
	}
	return nil
}

// captureEdit gathers everything about one file within the budget.
func (rec *recorder) captureEdit(index int, t target) (spool.Envelope, chunkSet) {
	abs := Resolve(t.name, rec.p.cwd)
	loc, rel := Placed(rec.locate(filepath.Dir(abs)), abs)
	ep := &spool.EditPayload{
		Tool:         rec.p.tool,
		ToolUseID:    rec.p.toolUseID,
		Index:        index,
		AbsPath:      abs,
		Path:         rel,
		Location:     loc,
		Kind:         t.kind,
		UserModified: t.payload && rec.p.response.flag("userModified"),
	}
	if t.pathOnly && gone(abs) {
		ep.Kind = store.EditDelete
	}
	chunks := chunkSet{}
	if Excluded(rec.opts.Excludes, abs, rel) {
		ep.Excluded = true
	} else {
		rec.gather(ep, chunks, t)
	}
	env := rec.envelope(spool.KindEdit, fmt.Sprintf("%s:%d", rec.p.toolUseID, index))
	env.Edit = ep
	return env, chunks
}

// gather fills the content fields: first what the payload carries, which
// costs nothing, then the disk read and the HEAD lookup while the budget
// lasts.
func (rec *recorder) gather(ep *spool.EditPayload, chunks chunkSet, t target) {
	if t.payload {
		rec.gatherPayload(ep, chunks)
	}
	if len(t.hunks) > 0 {
		ep.PatchChunk = chunks.add(t.hunks)
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

// gatherPayload takes what a file tool's payload says about its file: the
// tool input, the harness's hunks, the original, and a Write's content.
func (rec *recorder) gatherPayload(ep *spool.EditPayload, chunks chunkSet) {
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
}

// gone reports whether nothing exists at path.
func gone(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// gatherHead ships the blob id of the path at HEAD for every edit of a
// tracked file, and its content under the chunk cap. The spool writer
// leaves out a chunk the store holds already, so the content travels only
// while objects/ lacks it, and an edit after a commit finds the new HEAD.
func (rec *recorder) gatherHead(ep *spool.EditPayload, chunks chunkSet, obs observed) {
	ctx, cancel := rec.gitCtx()
	defer cancel()
	wt := ep.Location.Worktree
	ep.HeadBlob = gitloc.Output(ctx, wt, "rev-parse", "--verify", "--quiet",
		ep.Location.Head+":"+ep.Path)
	if ep.HeadBlob == "" {
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
