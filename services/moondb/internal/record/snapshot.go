package record

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
// each with its content as a chunk. The turn-edge hooks are async, so a
// snapshot has a budget of its own (moondb.SnapshotBudget). A worktree
// whose state hash matches the session's last snapshot of it is skipped,
// and a file whose size and time match what the last snapshot read is not
// read again when the store already holds its content.

// snapshotID keys a snapshot by turn, phase and worktree; the worktree goes
// in as a short hash, as in headMovedID.
func snapshotID(promptID string, phase store.SnapshotPhase, worktree string) string {
	sum := sha256.Sum256([]byte(worktree))
	return promptID + ":snap:" + string(phase) + ":" + hex.EncodeToString(sum[:6])
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
		env := sr.envelope(spool.KindSnapshot, snapshotID(rec.p.promptID, phase, wt))
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
// session keeps for the next one.
type taken struct {
	at      time.Time
	payload *spool.Snapshot
	chunks  chunkSet
	next    snapState
}

// snapshot captures one worktree. It reports false when there is nothing
// to write: git cannot place the worktree or list its status in time, the
// state is the one last recorded, or the session never took a snapshot
// here and nothing is in scope.
func (rec *recorder) snapshot(wt string, last snapState) (taken, bool) {
	at, started := rec.opts.Now(), time.Now()
	loc := rec.locate(wt)
	if loc.Worktree == "" {
		return taken{}, false
	}
	raw, ok := rec.status(loc.Worktree)
	if !ok {
		return taken{}, false
	}
	scope, skipped := rec.scope(loc.Worktree, parseStatus(raw))
	stamps := stampAll(loc.Worktree, scope)
	hash := stateHash(loc.Head, raw, scope, stamps)
	if hash == last.Hash || (len(scope) == 0 && last.Taken == 0) {
		return taken{}, false
	}
	b := rec.read(loc.Worktree, scope, stamps, last)
	b.skipped.Excluded, b.skipped.TooMany = skipped.Excluded, skipped.TooMany
	truncated := b.skipped.Timeout > 0 || b.skipped.TooMany > 0
	next := snapState{Hash: hash, Taken: started.UnixNano(), Files: b.stamps}
	if b.skipped.Timeout > 0 {
		next.Hash = "" // cut short: the next boundary takes it again
	}
	slices.SortFunc(
		b.files,
		func(x, y store.SnapshotFile) int { return cmp.Compare(x.Path, y.Path) },
	)
	return taken{
		at: at,
		payload: &spool.Snapshot{
			Location: loc, Scope: store.SnapshotScope, Files: b.files,
			Truncated: truncated, Skipped: b.skipped, StateHash: hash,
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
// a touched path git reports clean).
type scoped struct {
	path   string
	status string
}

// scope lists the paths a snapshot of wt covers: the session's edited
// paths first, then git's dirty paths, each once and sorted within its
// group, less the excluded ones and those past the file cap.
func (rec *recorder) scope(wt string, entries []statusEntry) ([]scoped, store.SnapshotSkipped) {
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
	var skipped store.SnapshotSkipped
	seen := map[string]bool{}
	for _, p := range slices.Concat(touched, dirty) {
		if seen[p] {
			continue
		}
		seen[p] = true
		if Excluded(rec.opts.Excludes, filepath.Join(wt, filepath.FromSlash(p)), p) {
			skipped.Excluded++
			continue
		}
		out = append(out, scoped{path: p, status: status[p]})
	}
	if len(out) > moondb.MaxSnapshotFiles {
		skipped.TooMany = len(out) - moondb.MaxSnapshotFiles
		out = out[:moondb.MaxSnapshotFiles]
	}
	return out, skipped
}

// stamp is what one stat said about a path in scope.
type stamp struct {
	missing bool
	regular bool
	size    int64
	mtime   int64
}

// stampAll stats every path in scope without following symlinks. A path
// that cannot be stated for another reason than being gone counts as not
// a regular file.
func stampAll(wt string, scope []scoped) []stamp {
	out := make([]stamp, len(scope))
	for i, sc := range scope {
		info, err := os.Lstat(filepath.Join(wt, filepath.FromSlash(sc.path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			out[i] = stamp{missing: true}
		case err == nil:
			out[i] = stamp{
				regular: info.Mode().IsRegular(),
				size:    info.Size(),
				mtime:   info.ModTime().UnixNano(),
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

// snapBuild is a snapshot's file map as the reads fill it, and the stamps
// the session keeps for the next snapshot.
type snapBuild struct {
	files   []store.SnapshotFile
	chunks  chunkSet
	stamps  map[string]fileStamp
	skipped store.SnapshotSkipped
}

// read fills the file map in scope order until the budget runs out; the
// paths it does not reach count as skipped for the timeout. A file over
// the chunk cap is listed by its size alone, a missing one as deleted, and
// anything but a regular file is left out.
func (rec *recorder) read(wt string, scope []scoped, stamps []stamp, last snapState) snapBuild {
	b := snapBuild{chunks: chunkSet{}, stamps: map[string]fileStamp{}}
	for i, sc := range scope {
		st := stamps[i]
		f := store.SnapshotFile{Path: sc.path, Status: sc.status, Size: st.size}
		switch {
		case rec.spent():
			b.skipped.Timeout = len(scope) - i
			return b
		case st.missing:
			f.Deleted = true
		case !st.regular:
			continue
		case st.size > moondb.MaxChunkBytes:
			b.skipped.TooLarge++
		case !rec.readFile(&b, &f, filepath.Join(wt, filepath.FromSlash(sc.path)), st, last):
			continue // changed under the read or past the cap now; the next boundary sees it
		}
		b.files = append(b.files, f)
	}
	return b
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
