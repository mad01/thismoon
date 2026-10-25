package record

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// A snapshot records a worktree's state at a turn boundary: its dirty
// paths, tracked and untracked, plus the paths the session edited there,
// each with its content as a chunk, and for a path HEAD tracks the HEAD
// blob and its content. A path it cannot read is listed with the reason,
// so a path a whole snapshot lacks was clean. The turn-edge hooks are
// async, so a snapshot has a budget of its own (moondb.SnapshotBudget). A
// worktree whose state hash matches the session's last snapshot of it is
// skipped, and a file whose size and time match what the last snapshot
// read is not read again when the store already holds its content.

// snapshotID keys a snapshot by turn, phase, worktree and what it holds:
// the worktree goes in as a short hash, as in headMovedID, and so does a
// digest of the state hash and the file map. A second Stop of one prompt,
// after a Stop hook let the turn go on, records a state of its own, and so
// does a retry of a snapshot cut short. A replayed envelope keeps its id.
func snapshotID(promptID string, phase store.SnapshotPhase, p *spool.Snapshot) string {
	wt := sha256.Sum256([]byte(p.Location.Worktree))
	h := sha256.New()
	h.Write([]byte(p.StateHash))
	// Strings, numbers and booleans encode without error.
	_ = json.NewEncoder(h).Encode(p.Files)
	return promptID + ":snap:" + string(phase) + ":" + hex.EncodeToString(wt[:6]) + ":" +
		hex.EncodeToString(h.Sum(nil)[:6])
}

// takeSnapshots writes a snapshot of every worktree in scope whose state
// changed since the session last took one there, within SnapshotBudget
// from now. A worktree the budget does not reach waits for the next turn
// boundary.
func (rec *recorder) takeSnapshots(phase store.SnapshotPhase) error {
	ctx, cancel := context.WithTimeout(rec.base, rec.opts.SnapshotBudget)
	defer cancel()
	sr := rec.within(ctx)
	states := rec.state.snaps()
	changed := false
	defer func() {
		if changed {
			rec.state.saveSnaps(states)
		}
	}()
	for _, wt := range sr.snapshotWorktrees() {
		if sr.spent() {
			break
		}
		snap, ok := sr.snapshot(wt, states[wt])
		if !ok {
			continue
		}
		snap.payload.Phase = phase
		env := sr.envelope(spool.KindSnapshot, snapshotID(rec.p.promptID, phase, snap.payload))
		env.At = snap.at
		env.Snapshot = snap.payload
		if err := sr.spool.Write(env, snap.chunks); err != nil {
			return err
		}
		states[wt], changed = snap.next, true
	}
	return nil
}

// snapshotWorktrees lists the worktrees a turn boundary covers: the cwd's,
// then every worktree the session edited in, each once.
func (rec *recorder) snapshotWorktrees() []string {
	var out []string
	if loc := rec.locate(rec.p.cwd); loc.Worktree != "" {
		out = append(out, loc.Worktree)
	}
	for _, wt := range rec.state.touched() {
		if !slices.Contains(out, wt) {
			out = append(out, wt)
		}
	}
	return out
}

// taken is one worktree's snapshot, ready to spool, and the state the
// session keeps for the next one. at is when its reads were done.
type taken struct {
	at      time.Time
	payload *spool.Snapshot
	chunks  chunkSet
	next    snapState
}

// snapshot captures one worktree. It reports false when there is nothing
// to write: git cannot place the worktree or list its status in time, the
// state is the one last recorded, or the session never took a snapshot
// here and nothing is in scope. The snapshot is stamped once its reads are
// done, so an edit stamped before that may be in what it read, and one
// stamped after is not.
func (rec *recorder) snapshot(wt string, last snapState) (taken, bool) {
	started, begun := rec.opts.Now(), time.Now()
	loc := rec.locate(wt)
	if loc.Worktree == "" {
		return taken{}, false
	}
	raw, ok := rec.status(loc.Worktree)
	if !ok {
		return taken{}, false
	}
	scope, tooMany := rec.scope(loc.Worktree, parseStatus(raw))
	stamps := stampAll(loc.Worktree, scope)
	hash := stateHash(loc.Head, raw, scope, stamps)
	if hash == last.Hash || (len(scope) == 0 && last.Taken == 0) {
		return taken{}, false
	}
	b := rec.read(loc.Worktree, scope, stamps, last)
	rec.addHeads(&b, loc, last)
	b.skipped.TooMany = tooMany
	next := snapState{Hash: hash, Taken: begun.UnixNano(), Files: b.stamps, Heads: b.heads}
	if b.skipped.Timeout > 0 {
		next.Hash = "" // cut short: the next boundary takes it again
	}
	slices.SortFunc(
		b.files,
		func(x, y store.SnapshotFile) int { return cmp.Compare(x.Path, y.Path) },
	)
	return taken{
		at: rec.opts.Now(),
		payload: &spool.Snapshot{
			Location: loc, Started: started, Scope: store.SnapshotScope, Files: b.files,
			Truncated: b.skipped.Timeout > 0 || tooMany > 0, Skipped: b.skipped,
			StateHash: hash,
		},
		chunks: b.chunks,
		next:   next,
	}, true
}

// status runs git's machine-readable status in wt, untracked files listed
// one by one, without taking the index lock.
func (rec *recorder) status(wt string) ([]byte, bool) {
	ctx, cancel := rec.gitCtx()
	defer cancel()
	return gitBytes(ctx, wt, moondb.MaxStatusBytes, "--no-optional-locks", "status",
		"--porcelain=v2", "-z", "-uall", "--ignore-submodules=all")
}

// scoped is one path a snapshot covers, with git's status for it ("" for
// a touched path git reports clean) and whether an exclude glob matched it.
type scoped struct {
	path     string
	status   string
	excluded bool
}

// scope lists the paths a snapshot of wt covers: the session's edited
// paths first, then git's dirty paths, each once and sorted within its
// group, the excluded ones marked. It cuts the list at the file cap and
// reports how many paths fell past it.
func (rec *recorder) scope(wt string, entries []statusEntry) ([]scoped, int) {
	status := make(map[string]string, len(entries))
	dirty := make([]string, 0, len(entries))
	for _, e := range entries {
		status[e.path] = e.xy
		dirty = append(dirty, e.path)
	}
	touched := rec.state.editedIn(wt)
	slices.Sort(touched)
	slices.Sort(dirty)
	var out []scoped
	seen := map[string]bool{}
	for _, p := range slices.Concat(touched, dirty) {
		if seen[p] {
			continue
		}
		seen[p] = true
		excluded := Excluded(rec.opts.Excludes, filepath.Join(wt, filepath.FromSlash(p)), p)
		out = append(out, scoped{path: p, status: status[p], excluded: excluded})
	}
	if len(out) > moondb.MaxSnapshotFiles {
		return out[:moondb.MaxSnapshotFiles], len(out) - moondb.MaxSnapshotFiles
	}
	return out, 0
}

// stamp is what one stat said about a path in scope.
type stamp struct {
	missing bool
	// failed is set when the stat failed for another reason than the path
	// being gone.
	failed bool
	mode   fs.FileMode
	size   int64
	mtime  int64
}

// stampAll stats every path in scope without following symlinks. An
// excluded path is not stated: nothing about its content is captured.
func stampAll(wt string, scope []scoped) []stamp {
	out := make([]stamp, len(scope))
	for i, sc := range scope {
		if sc.excluded {
			continue
		}
		info, err := os.Lstat(filepath.Join(wt, filepath.FromSlash(sc.path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			out[i] = stamp{missing: true}
		case err != nil:
			out[i] = stamp{failed: true}
		default:
			out[i] = stamp{
				mode:  info.Mode(),
				size:  info.Size(),
				mtime: info.ModTime().UnixNano(),
			}
		}
	}
	return out
}

// stateHash is what decides whether a worktree changed since the last
// snapshot: HEAD, git's status output, and each path's existence, size and
// modification time. The status alone would miss a file modified again
// while it stays modified.
func stateHash(head string, status []byte, scope []scoped, stamps []stamp) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00", head, len(status))
	h.Write(status)
	for i, sc := range scope {
		st := stamps[i]
		fmt.Fprintf(h, "%s\x00%t\x00%d\x00%d\x00", sc.path, st.missing, st.size, st.mtime)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// snapBuild is a snapshot's file map as the reads fill it, and what the
// session keeps for the next snapshot: the stamps of the files it read and
// the chunk of each HEAD blob it shipped.
type snapBuild struct {
	files   []store.SnapshotFile
	chunks  chunkSet
	stamps  map[string]fileStamp
	heads   map[string]string
	skipped store.SnapshotSkipped
}

// read fills the file map in scope order until the budget runs out; the
// paths it does not reach count as skipped for the timeout. A missing path
// is listed as deleted, and one whose content it cannot hold is listed
// without it, with the reason.
func (rec *recorder) read(wt string, scope []scoped, stamps []stamp, last snapState) snapBuild {
	b := snapBuild{chunks: chunkSet{}, stamps: map[string]fileStamp{}}
	for i, sc := range scope {
		if rec.spent() {
			b.skipped.Timeout = len(scope) - i
			return b
		}
		st := stamps[i]
		f := store.SnapshotFile{Path: sc.path, Status: sc.status, Size: st.size}
		switch {
		case sc.excluded:
			f.Reason = store.ReasonExcluded
			b.skipped.Excluded++
		case st.missing:
			f.Deleted = true
		default:
			abs := filepath.Join(wt, filepath.FromSlash(sc.path))
			f.Reason = rec.content(&b, &f, abs, st, last)
		}
		b.files = append(b.files, f)
	}
	return b
}

// content fills f's content and returns "", or returns why it cannot.
func (rec *recorder) content(
	b *snapBuild,
	f *store.SnapshotFile,
	abs string,
	st stamp,
	last snapState,
) store.SnapshotReason {
	switch {
	case st.failed:
		return store.ReasonUnreadable
	case st.mode&fs.ModeSymlink != 0:
		return store.ReasonSymlink
	case !st.mode.IsRegular():
		return store.ReasonNotRegular
	case st.size > moondb.MaxChunkBytes:
		b.skipped.TooLarge++
		return store.ReasonTooLarge
	case !rec.readFile(b, f, abs, st, last):
		return store.ReasonUnreadable // or changed under the read; the next boundary sees it
	default:
		return ""
	}
}

// readFile fills f's content and reports whether it could. A file whose
// size and time match the stamp the last snapshot left, and that was not
// modified once that snapshot had begun, keeps the chunk the stamp names
// without a read when the store already holds that chunk.
func (rec *recorder) readFile(
	b *snapBuild,
	f *store.SnapshotFile,
	abs string,
	st stamp,
	last snapState,
) bool {
	if old, ok := last.Files[f.Path]; ok && old.Size == st.size && old.MTime == st.mtime &&
		st.mtime < last.Taken && rec.spool.Stored(old.Chunk) {
		f.Chunk, f.Blob = old.Chunk, old.Blob
		b.stamps[f.Path] = old
		return true
	}
	obs := observe(rec.ctx, abs)
	if !obs.read {
		return false
	}
	f.Chunk, f.Blob, f.Size = b.chunks.add(obs.data), obs.blob, obs.size
	b.stamps[f.Path] = fileStamp{Size: st.size, MTime: st.mtime, Blob: obs.blob, Chunk: f.Chunk}
	return true
}

// addHeads sets HeadBlob on every entry that holds content or a deletion
// and whose path HEAD tracks, and HeadChunk where the budget allows: the
// entry's own chunk when it matches HEAD, the chunk the last snapshot
// shipped for the blob when the store holds it, else the blob read from
// git. A path a whole earlier snapshot lacked held HEAD's content then,
// and serve compares the entry with it.
func (rec *recorder) addHeads(b *snapBuild, loc store.Location, last snapState) {
	b.heads = map[string]string{}
	if loc.Head == "" || rec.spent() {
		return
	}
	var paths []string
	for _, f := range b.files {
		if f.Chunk != "" || f.Deleted {
			paths = append(paths, f.Path)
		}
	}
	tree := rec.lsTree(loc.Worktree, loc.Head, paths)
	var want []string
	for i := range b.files {
		f := &b.files[i]
		e, ok := tree[f.Path]
		if !ok || (f.Chunk == "" && !f.Deleted) {
			continue
		}
		f.HeadBlob = e.blob
		switch {
		case f.Blob == e.blob:
			f.HeadChunk = f.Chunk
		case rec.spool.Stored(last.Heads[e.blob]):
			f.HeadChunk = last.Heads[e.blob]
		case e.size <= moondb.MaxChunkBytes && !slices.Contains(want, e.blob):
			want = append(want, e.blob)
		}
	}
	read := catBlobs(rec.ctx, loc.Worktree, want)
	for i := range b.files {
		f := &b.files[i]
		if data, ok := read[f.HeadBlob]; ok && f.HeadChunk == "" {
			f.HeadChunk = b.chunks.add(data)
		}
		if f.HeadChunk != "" {
			b.heads[f.HeadBlob] = f.HeadChunk
		}
	}
}
