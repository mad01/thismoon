package spool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// pollInterval is how often Watch lists the spool when the filesystem
// watcher cannot be created.
const pollInterval = time.Second

// sweepGrace is how old an unreferenced chunk or temp file must be before
// Sweep deletes it. A writer puts chunks in place before it renames their
// envelope, so a young chunk with no envelope yet is in flight, not garbage.
const sweepGrace = time.Minute

// Reader drains a spool directory. Serve is its only user: it lists or
// watches envelopes, reads each with its chunks, ingests it, and removes it.
type Reader struct {
	// Dir is the spool directory, <workdir>/spool.
	Dir string
}

// List returns the paths of the pending envelopes in name order, which is
// the order they were written. A spool directory that does not exist yet
// has no envelopes.
func (r *Reader) List() ([]string, error) {
	entries, err := os.ReadDir(r.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("spool: list: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), envSuffix) {
			paths = append(paths, filepath.Join(r.Dir, e.Name()))
		}
	}
	return paths, nil
}

// Read decodes the envelope at path. An envelope from another schema
// version is an error: this build cannot say what its fields mean.
func (r *Reader) Read(path string) (Envelope, error) {
	env, err := decodeEnvelope(path)
	if err != nil {
		return Envelope{}, err
	}
	if env.Version != Version {
		return Envelope{}, fmt.Errorf("spool: %s has version %d, want %d",
			filepath.Base(path), env.Version, Version)
	}
	return env, nil
}

// Chunk returns the bytes of the spooled chunk sha after checking they
// still hash to it. A chunk that is not in the spool wraps fs.ErrNotExist;
// serve may already hold it in the store, since a writer skips chunks the
// store has.
func (r *Reader) Chunk(sha string) ([]byte, error) {
	if !validKey(sha) {
		return nil, fmt.Errorf("spool: %q is not a chunk key", sha)
	}
	data, err := os.ReadFile(filepath.Join(r.Dir, sha+chunkSuffix))
	if err != nil {
		return nil, fmt.Errorf("spool: chunk %s: %w", sha, err)
	}
	if ChunkKey(data) != sha {
		return nil, fmt.Errorf("spool: chunk %s does not match its key", sha)
	}
	return data, nil
}

// Remove deletes an ingested envelope. Removing one that is already gone
// is not an error, so a replayed ingest can call it again.
func (r *Reader) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("spool: remove: %w", err)
	}
	return nil
}

// Sweep deletes the chunk files no pending envelope references and the
// temp files a crashed writer left behind, once they are older than
// sweepGrace. It deletes nothing when an envelope cannot be decoded: the
// chunks that envelope needs are unknown.
func (r *Reader) Sweep() error {
	paths, err := r.List()
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, p := range paths {
		env, err := decodeEnvelope(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue // ingested while we listed
		}
		if err != nil {
			return fmt.Errorf("spool: sweep: %w", err)
		}
		for _, key := range env.ChunkKeys() {
			keep[key] = true
		}
	}
	cutoff := time.Now().Add(-sweepGrace)
	err = removeOld(r.Dir, cutoff, func(name string) bool {
		key, ok := strings.CutSuffix(name, chunkSuffix)
		return ok && !keep[key]
	})
	return errors.Join(err, removeOld(filepath.Join(r.Dir, tmpDir), cutoff, func(string) bool {
		return true
	}))
}

// ChunkKeys returns every chunk key the envelope's payload points at, each
// once.
func (e Envelope) ChunkKeys() []string {
	var keys []string
	add := func(ks ...string) {
		for _, k := range ks {
			if k != "" && !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	if p := e.TurnOpen; p != nil {
		add(p.PromptChunk)
	}
	if p := e.TurnClose; p != nil {
		add(p.FinalChunk, p.TranscriptChunk)
	}
	if p := e.Edit; p != nil {
		add(p.PatchChunk, p.OriginalChunk, p.ContentChunk, p.ObservedChunk, p.HeadChunk)
	}
	if p := e.Snapshot; p != nil {
		for _, f := range p.Files {
			add(f.Chunk, f.HeadChunk)
		}
	}
	if p := e.AgentClose; p != nil {
		add(p.TranscriptChunk, p.BriefChunk)
		for _, m := range p.Messages {
			add(m.Chunk)
		}
	}
	return keys
}

// Watch calls fn with the path of every envelope in the spool when it
// starts and of every envelope that arrives after, in name order, from one
// goroutine, until ctx is done. fn sees each path once per Watch; an
// envelope fn leaves in place is offered again only by the next Watch. It
// uses filesystem notifications and falls back to listing every
// pollInterval when the watcher cannot be created. It returns nil when ctx
// ends and an error only when the spool directory cannot be created.
func (r *Reader) Watch(ctx context.Context, fn func(path string)) error {
	if err := os.MkdirAll(r.Dir, dirPerm); err != nil {
		return fmt.Errorf("spool: create %s: %w", r.Dir, err)
	}
	seen := map[string]bool{}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		r.poll(ctx, seen, fn, pollInterval)
		return nil
	}
	defer func() { _ = w.Close() }()
	if err := w.Add(r.Dir); err != nil {
		r.poll(ctx, seen, fn, pollInterval)
		return nil
	}
	// Watching before the first listing: an envelope renamed in between
	// shows up in the listing, the event, or both, never neither.
	r.offer(seen, fn)
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-w.Events:
			if !ok {
				r.poll(ctx, seen, fn, pollInterval)
				return nil
			}
		case _, ok := <-w.Errors:
			// An overflow or a read error: the listing below catches up.
			if !ok {
				r.poll(ctx, seen, fn, pollInterval)
				return nil
			}
		}
		drain(w)
		r.offer(seen, fn)
	}
}

// poll lists the spool every interval until ctx is done.
func (r *Reader) poll(
	ctx context.Context,
	seen map[string]bool,
	fn func(string),
	every time.Duration,
) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		r.offer(seen, fn)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// offer lists the spool and calls fn for every envelope not offered yet.
// seen forgets paths that are gone, so it stays as small as the spool. A
// listing error is skipped: the next event or tick lists again.
func (r *Reader) offer(seen map[string]bool, fn func(string)) {
	paths, err := r.List()
	if err != nil {
		return
	}
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
		if !seen[p] {
			seen[p] = true
			fn(p)
		}
	}
	for p := range seen {
		if !present[p] {
			delete(seen, p)
		}
	}
}

// drain discards the events already queued, so a burst of writes costs
// one listing. It stops at a closed channel; the Watch loop sees the close.
func drain(w *fsnotify.Watcher) {
	for {
		select {
		case _, ok := <-w.Events:
			if !ok {
				return
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// decodeEnvelope reads and decodes an envelope without checking its
// version.
func decodeEnvelope(path string) (Envelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Envelope{}, fmt.Errorf("spool: read envelope: %w", err)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("spool: decode %s: %w", filepath.Base(path), err)
	}
	return env, nil
}

// removeOld deletes the regular files in dir that match and were last
// modified before cutoff. A missing dir has nothing to remove.
func removeOld(dir string, cutoff time.Time, match func(name string) bool) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("spool: sweep: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if !e.Type().IsRegular() || !match(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		err = os.Remove(filepath.Join(dir, e.Name()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("spool: sweep: %w", err))
		}
	}
	return errors.Join(errs...)
}
