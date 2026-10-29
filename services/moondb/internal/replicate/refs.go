package replicate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// chunkRefs returns the chunk keys lines point at, each once, in the order
// first met. A snapshot also points at the content of its files, which its
// map chunk lists, so the map is read from chunks. A map chunks lacks adds
// nothing: there is nothing more to send. A line of an unknown kind points
// at nothing this build can name.
func chunkRefs(ctx context.Context, chunks store.Chunks, lines []local.Line) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(keys ...string) {
		for _, k := range keys {
			if k != "" && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	for _, l := range lines {
		rec, err := l.Decode()
		if errors.Is(err, local.ErrUnknownKind) {
			continue
		}
		if err != nil {
			return nil, err
		}
		add(recordRefs(rec)...)
		if snap, ok := rec.(store.Snapshot); ok && snap.MapChunk != "" {
			files, err := snapshotRefs(ctx, chunks, snap.MapChunk)
			if err != nil {
				return nil, err
			}
			add(files...)
		}
	}
	return out, nil
}

// recordRefs lists the chunk fields of one record.
func recordRefs(rec any) []string {
	switch v := rec.(type) {
	case store.Turn:
		return []string{v.PromptChunk, v.FinalChunk, v.TranscriptChunk}
	case store.Agent:
		return []string{v.TranscriptChunk}
	case store.Edit:
		return []string{v.PatchChunk, v.BeforeChunk, v.AfterChunk, v.ObservedChunk}
	case store.UnattributedDelta:
		return []string{v.FromChunk, v.ToChunk, v.PatchChunk}
	case store.Snapshot:
		return []string{v.MapChunk}
	}
	return nil
}

// snapshotRefs lists the content and HEAD chunks of the files a snapshot
// map holds.
func snapshotRefs(ctx context.Context, chunks store.Chunks, mapKey string) ([]string, error) {
	raw, err := chunks.Get(ctx, mapKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("replicate: snapshot map %s: %w", mapKey, err)
	}
	var files []store.SnapshotFile
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("replicate: snapshot map %s: %w", mapKey, err)
	}
	var out []string
	for _, f := range files {
		out = append(out, f.Chunk, f.HeadChunk)
	}
	return out, nil
}
