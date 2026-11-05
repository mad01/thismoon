package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// IndexVault walks vaultRoot, chunks every markdown note, and upserts
// chunks into idx. Notes outside the four known tiers are skipped (the
// caller's invariant: notes live under inbox/, notes/, outputs/, or
// MOCs/). Hidden directories, .git, .obsidian, .trash, attachments, and
// node_modules are never visited.
//
// Concurrency: chromem-go's AddDocuments handles parallel embedding
// internally; this walker stays single-threaded because we want to keep
// memory bounded and the per-note overhead is dominated by the embedding
// HTTP call regardless.
//
// progress (if non-nil) is invoked with each note's vault-relative path
// after its chunks are upserted.
func IndexVault(
	ctx context.Context,
	idx *Index,
	vaultRoot string,
	chunkOpts chunk.Options,
	progress func(path string, chunks int),
) error {
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
		if !info.Mode().IsRegular() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".mdx" {
			return nil
		}
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		tier := tierOfPath(rel)
		if tier == "" {
			// Not under a recognized tier — leave to the user (e.g. _attachments
			// or top-level README).
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		chunks, err := chunk.Split(rel, tier, content, chunkOpts)
		if err != nil {
			return fmt.Errorf("semantic.IndexVault: split %s: %w", rel, err)
		}
		// Always purge first so deleted chunks (e.g. when a section is
		// removed) don't linger.
		if err := idx.PurgePath(ctx, rel); err != nil {
			return fmt.Errorf("semantic.IndexVault: purge %s: %w", rel, err)
		}
		if len(chunks) == 0 {
			if progress != nil {
				progress(rel, 0)
			}
			return nil
		}
		if err := idx.Upsert(ctx, chunks, 0); err != nil {
			return fmt.Errorf("semantic.IndexVault: upsert %s: %w", rel, err)
		}
		if progress != nil {
			progress(rel, len(chunks))
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("semantic.IndexVault: walk %s: %w", vaultRoot, walkErr)
	}
	return nil
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
