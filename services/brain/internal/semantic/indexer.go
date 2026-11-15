package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// SyncOptions configures one Sync.
type SyncOptions struct {
	// Chunk tunes the chunker.
	Chunk chunk.Options
	// Held maps each note the index holds to the content hash it was
	// embedded from. A note whose hash still matches is not re-embedded.
	Held map[string]string
	// Progress, when set, sees each re-embedded note and its chunk count.
	Progress func(path string, chunks int)
}

// SyncReport is what one Sync changed.
type SyncReport struct {
	// Notes maps every note the index now holds to its content hash.
	Notes map[string]string
	// Embedded counts the notes re-embedded, Chunks their chunks.
	Embedded, Chunks int
	// Removed counts the notes that vanished, whose chunks were purged.
	Removed int
	// Failed maps each note that could not be split or embedded to the
	// reason, on one line. Such a note holds no chunks and is not in Notes,
	// so the next sync tries it again.
	Failed map[string]string
}

// Sync brings idx in line with the notes under vaultRoot's tiers. A note
// whose content hash matches opts.Held keeps its chunks; a new or changed
// note is re-chunked and re-embedded, except a withdrawn note's tombstone,
// which keeps none; a note in opts.Held that no longer
// exists has its chunks purged, so no ghost hits remain. Notes outside the
// four tiers, hidden directories, .git, .obsidian, .trash, attachments and
// node_modules are never read.
//
// A note that cannot be split or embedded (the embedder refuses it, say)
// does not stop the sync: its chunks are purged, it lands in
// SyncReport.Failed and the rest are synced. A failure to purge chunks, or a
// done ctx, stops it with an error.
func Sync(ctx context.Context, idx *Index, vaultRoot string, opts SyncOptions) (SyncReport, error) {
	report := SyncReport{Notes: map[string]string{}, Failed: map[string]string{}}
	walkErr := filepath.Walk(vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if path != vaultRoot && (skipDir(base) || strings.HasPrefix(base, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, ok := noteRel(vaultRoot, path, info)
		if !ok {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		return report.syncNote(ctx, idx, rel, content, opts)
	})
	if walkErr != nil {
		return report, fmt.Errorf("semantic.Sync: walk %s: %w", vaultRoot, walkErr)
	}
	vanished := make([]string, 0)
	for rel := range opts.Held {
		_, held := report.Notes[rel]
		_, failed := report.Failed[rel]
		if !held && !failed {
			vanished = append(vanished, rel)
		}
	}
	sort.Strings(vanished)
	for _, rel := range vanished {
		if err := idx.DeleteByPath(ctx, rel); err != nil {
			return report, fmt.Errorf("semantic.Sync: purge %s: %w", rel, err)
		}
		report.Removed++
	}
	return report, nil
}

// syncNote brings the chunks of the note at rel in line with content and
// records the outcome: held when its hash matches opts.Held, re-embedded,
// or failed, with its chunks purged. Only a purge that fails or a done ctx
// returns an error.
func (r *SyncReport) syncNote(
	ctx context.Context,
	idx *Index,
	rel string,
	content []byte,
	opts SyncOptions,
) error {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	r.Notes[rel] = hash
	if opts.Held[rel] == hash {
		return nil
	}
	// The purge comes first so a removed section leaves no chunk.
	if err := idx.DeleteByPath(ctx, rel); err != nil {
		return fmt.Errorf("semantic.Sync: purge %s: %w", rel, err)
	}
	if withdrawn(content) {
		return nil
	}
	n, err := embedNote(ctx, idx, rel, content, opts.Chunk)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("semantic.Sync: %s: %w", rel, err)
		}
		// Some chunks may have gone in before the one that failed.
		if err := idx.DeleteByPath(ctx, rel); err != nil {
			return fmt.Errorf("semantic.Sync: purge %s: %w", rel, err)
		}
		delete(r.Notes, rel)
		r.Failed[rel] = strings.Join(strings.Fields(err.Error()), " ")
		return nil
	}
	r.Embedded++
	r.Chunks += n
	if opts.Progress != nil {
		opts.Progress(rel, n)
	}
	return nil
}

// withdrawn reports whether content is a withdrawn note's tombstone, which
// keeps no chunks.
func withdrawn(content []byte) bool {
	fm, _, err := markdown.ParseFrontmatter(content)
	return err == nil && lifecycle.Normalize(fm.Status) == lifecycle.Withdrawn
}

// noteRel returns the vault-relative path of a markdown note in a tier, or
// false for any other file.
func noteRel(vaultRoot, path string, info os.FileInfo) (string, bool) {
	if !info.Mode().IsRegular() {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".md" && ext != ".mdx" {
		return "", false
	}
	rel, err := filepath.Rel(vaultRoot, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	return rel, tierOfPath(rel) != ""
}

// embedNote splits a note whose old chunks are purged, embeds the chunks and
// returns how many it made.
func embedNote(
	ctx context.Context,
	idx *Index,
	rel string,
	content []byte,
	opts chunk.Options,
) (int, error) {
	chunks, err := chunk.Split(rel, tierOfPath(rel), content, opts)
	if err != nil {
		return 0, fmt.Errorf("split: %w", err)
	}
	if err := idx.Upsert(ctx, chunks, 0); err != nil {
		return 0, err
	}
	return len(chunks), nil
}

// skipDir matches the skipDirs set in internal/search to keep both indexers
// scanning the same files.
func skipDir(base string) bool {
	switch base {
	case ".git", ".obsidian", ".trash", "attachments", "node_modules":
		return true
	}
	return false
}

func tierOfPath(rel string) string {
	slash := strings.IndexByte(rel, '/')
	if slash < 0 {
		return ""
	}
	head := rel[:slash]
	for _, t := range vault.AllTiers {
		if head == t.Dir() {
			return head
		}
	}
	return ""
}
