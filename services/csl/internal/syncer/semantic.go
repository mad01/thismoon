package syncer

import (
	"context"
	"fmt"
	"io"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// runSemanticSync loads the embedding model and re-embeds the changed repos into
// the semantic index. It is best-effort and decoupled from the lexical reindex:
// a missing model or unavailable backend is reported but never fails the sync.
func runSemanticSync(ctx context.Context, w io.Writer, repos []finder.Repo, tty bool) {
	if len(repos) == 0 {
		return
	}
	semDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		fmt.Fprintf(w, "semantic: skipped (%v)\n", err)
		return
	}
	emb := semantic.NewDefaultEmbedder()
	if err := emb.CheckModel(ctx); err != nil {
		fmt.Fprintf(w, "semantic: skipped (embedding backend not ready: %v)\n", err)
		return
	}
	// Bulk indexing shouldn't leave the model resident for the keep-alive
	// window once the run is over.
	defer func() { _ = emb.Unload(context.Background()) }()

	fmt.Fprintf(w, "\nSemantically indexing %d repo(s)...\n", len(repos))
	files, chunks, failed := semanticSyncRepos(ctx, w, semDir, emb, repos, tty)
	fmt.Fprintf(w, "semantic: %d file(s) embedded, %d chunk(s)", files, chunks)
	if failed > 0 {
		fmt.Fprintf(w, ", %d repo(s) failed", failed)
	}
	fmt.Fprintln(w)

	// The running daemon loaded the semantic index at startup; bounce it so the
	// next query spawns a fresh daemon that reloads the updated index.
	_ = daemon.Shutdown(daemon.DefaultSocketPath())
}

// semanticSyncRepos re-embeds each repo into the semantic index at semDir using
// emb. Indexing is incremental (only changed files are re-embedded) and
// best-effort: a repo that fails is logged and skipped, never aborting the rest.
// Returns the totals embedded and the count of repos that failed.
func semanticSyncRepos(
	ctx context.Context,
	w io.Writer,
	semDir string,
	emb semantic.Embedder,
	repos []finder.Repo,
	tty bool,
) (files, chunks, failed int) {
	repoWidth := len(fmt.Sprint(len(repos)))
	for i, repo := range repos {
		var progOpt semantic.IndexOption
		if tty {
			progOpt = semantic.WithProgress(func(p semantic.IndexProgress) {
				fileWidth := len(fmt.Sprint(p.FilesTotal))
				fmt.Fprintf(w, "\r  [%*d/%d] %s [%*d/%d] %s\033[K",
					repoWidth, i+1, len(repos), repo.Name,
					fileWidth, p.FilesDone, p.FilesTotal, p.Current)
			})
		}

		var opts []semantic.IndexOption
		if progOpt != nil {
			opts = append(opts, progOpt)
		}
		stats, err := semantic.IndexRepoSemantic(ctx, semDir, repo, emb, opts...)

		if tty {
			fmt.Fprintf(w, "\r\033[K")
		}
		if err != nil {
			failed++
			fmt.Fprintf(w, "  semantic: %s failed: %v\n", repo.Name, err)
			continue
		}
		files += stats.FilesEmbedded
		chunks += stats.ChunksEmbedded
		fmt.Fprintf(w, "  [%*d/%d] %s — %d embedded, %d chunks\n",
			repoWidth, i+1, len(repos), repo.Name,
			stats.FilesEmbedded, stats.ChunksEmbedded)
	}
	return files, chunks, failed
}
