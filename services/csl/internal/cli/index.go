package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/queue"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

var (
	indexAllFlag         bool
	indexLexicalAllFlag  bool
	indexStatusFlag      bool
	indexCleanFlag       bool
	indexRepairFlag      bool
	indexDrainFlag       bool
	indexJSONFlag        bool
	indexRepoFlag        string
	indexSemanticFlag    bool
	indexSemanticAllFlag bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the search index",
	Long: `Manage the search index for code search.

By default, only stale repos (new commits, dirty working tree) are re-indexed.
Use --all to force a full re-index of every repo (lexical + semantic).
Use --lexical-all to force a full lexical re-index only.
Use --semantic-all to re-embed all repos (incremental per file; a full
re-embed happens automatically when the model or chunker version changes).
Use --status to view the current index state.
Use --repair to validate and fix corrupted shard files.
Use --clean to delete the entire index directory.
Use --drain to batch-index repos queued by post-merge hooks.`,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().BoolVar(&indexAllFlag, "all", false, "force full re-index of all repos (lexical + semantic)")
	indexCmd.Flags().BoolVar(&indexLexicalAllFlag, "lexical-all", false, "force full lexical re-index only")
	indexCmd.Flags().BoolVar(&indexStatusFlag, "status", false, "show index status table")
	indexCmd.Flags().BoolVar(&indexCleanFlag, "clean", false, "delete the index directory")
	indexCmd.Flags().
		BoolVar(&indexRepairFlag, "repair", false, "validate shards and remove corrupted ones")
	indexCmd.Flags().BoolVar(&indexJSONFlag, "json", false, "output as JSON")
	indexCmd.Flags().
		StringVar(&indexRepoFlag, "repo", "", "re-index a single repo by absolute path (skips global staleness check)")
	indexCmd.Flags().
		BoolVar(&indexDrainFlag, "drain", false, "batch-index repos queued by post-merge hooks")
	indexCmd.Flags().
		BoolVar(&indexSemanticFlag, "semantic", false, "also build the semantic embedding index for discovered repos")
	indexCmd.Flags().
		BoolVar(&indexSemanticAllFlag, "semantic-all", false, "re-embed all repos, incremental per file (fetches the model on first run)")
	rootCmd.AddCommand(indexCmd)
}

type indexStatusJSON struct {
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Status      string `json:"status"`
	HEAD        string `json:"head,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Dirty       bool   `json:"dirty"`
	IndexedAt   string `json:"indexed_at,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func runIndex(cmd *cobra.Command, args []string) error {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return err
	}

	if indexCleanFlag {
		if err := os.RemoveAll(indexDir); err != nil {
			return fmt.Errorf("failed to remove index directory %s: %w", indexDir, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", indexDir)
		return nil
	}

	if indexRepairFlag {
		return runRepair(cmd, indexDir)
	}

	if indexDrainFlag {
		return runIndexDrain(cmd, indexDir)
	}

	// --semantic-all alone: semantic only, no lexical.
	if indexSemanticAllFlag && !indexAllFlag {
		return runIndexSemantic(cmd, indexRepoFlag)
	}

	// --semantic --repo X: semantic build filtered to one repo.
	if indexSemanticFlag {
		return runIndexSemantic(cmd, indexRepoFlag)
	}

	if indexRepoFlag != "" {
		return runIndexSingle(cmd, indexDir, indexRepoFlag)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		return err
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return err
	}

	staleness, err := search.CheckStaleness(repos, state)
	if err != nil {
		return err
	}

	if indexStatusFlag {
		return printIndexStatus(cmd, repos, state, staleness)
	}

	// Determine what to index.
	toIndex := staleness.Stale
	if indexAllFlag || indexLexicalAllFlag {
		toIndex = repos
	}

	if len(toIndex) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "all repos are up to date")
		return nil
	}

	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "Indexing %d repo(s)...\n", len(toIndex))

	err = search.IndexRepos(indexDir, toIndex, func(i, total int, repo finder.Repo) {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, total, repo.Name)
	})
	if err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	// Update state.
	for _, repo := range toIndex {
		if fp, ok := staleness.Current[repo.Path]; ok {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	if err := state.Save(indexDir); err != nil {
		return fmt.Errorf("failed to save index state: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "indexed %d repo(s)\n", len(toIndex))

	// --all also runs semantic.
	if indexAllFlag {
		if err := runIndexSemantic(cmd, indexRepoFlag); err != nil {
			return err
		}
	}

	return nil
}

func printIndexStatus(
	cmd *cobra.Command,
	repos []finder.Repo,
	state *search.IndexState,
	staleness *search.StalenessResult,
) error {
	w := cmd.OutOrStdout()

	staleSet := make(map[string]bool)
	for _, r := range staleness.Stale {
		staleSet[r.Path] = true
	}

	if indexJSONFlag {
		items := make([]indexStatusJSON, 0, len(repos))
		for _, r := range repos {
			status := "fresh"
			if staleSet[r.Path] {
				status = "stale"
			}
			rs, ok := state.GetRepo(r.Path)
			item := indexStatusJSON{
				Repo:   r.Name,
				Path:   r.Path,
				Status: status,
			}
			if ok {
				item.HEAD = rs.HEAD
				item.Branch = rs.Branch
				item.Dirty = rs.Dirty
				item.IndexedAt = rs.IndexedAt.Format(time.RFC3339)
				item.Fingerprint = rs.Fingerprint
			} else {
				item.Status = "missing"
			}
			items = append(items, item)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	fmt.Fprintf(
		w,
		"%-40s %-10s %-8s %-20s %s\n",
		"REPO",
		"STATUS",
		"DIRTY",
		"INDEXED AT",
		"BRANCH",
	)
	for _, r := range repos {
		status := "fresh"
		if staleSet[r.Path] {
			status = "stale"
		}
		rs, ok := state.GetRepo(r.Path)
		if !ok {
			fmt.Fprintf(w, "%-40s %-10s %-8s %-20s %s\n", r.Name, "missing", "-", "-", "-")
			continue
		}
		dirty := "no"
		if rs.Dirty {
			dirty = "yes"
		}
		indexed := "-"
		if !rs.IndexedAt.IsZero() {
			indexed = time.Since(rs.IndexedAt).Truncate(time.Second).String() + " ago"
		}
		fmt.Fprintf(
			w,
			"%-40s %-10s %-8s %-20s %s\n",
			r.Name,
			status,
			dirty,
			indexed,
			rs.Branch,
		)
	}
	return nil
}

// runIndexSingle re-indexes one repo by absolute path. Used by the post-merge
// git hook: cheap, no global scan, fingerprint state still updated so the next
// `csl index` won't redundantly re-process this repo.
func runIndexSingle(cmd *cobra.Command, indexDir, repoPath string) error {
	if _, err := os.Stat(filepath.Join(indexDir, syncLockFile)); err == nil {
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"csl sync in progress, skipping auto-index for %s\n",
			repoPath,
		)
		return nil
	}

	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path %s: %w", repoPath, err)
	}

	repo, err := finder.Inspect(abs)
	if err != nil {
		return fmt.Errorf("inspect repo %s: %w", abs, err)
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return err
	}

	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "Indexing %s\n", repo.Name)

	if err := search.IndexRepos(indexDir, []finder.Repo{repo}, nil); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	fp, fpErr := search.Fingerprint(repo.Path)
	if fpErr == nil {
		fp.IndexedAt = time.Now()
		state.SetRepo(repo.Path, fp)
		if err := state.Save(indexDir); err != nil {
			return fmt.Errorf("failed to save index state: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "indexed %s\n", repo.Name)
	return nil
}

// runIndexDrain batch-indexes repos queued by post-merge hooks.
// Claims the queue atomically so concurrent drains don't collide.
func runIndexDrain(cmd *cobra.Command, indexDir string) error {
	queuePath, err := queue.DefaultPath()
	if err != nil {
		return err
	}

	claimedPath, repoPaths, err := queue.Claim(queuePath)
	if err != nil {
		return fmt.Errorf("claim queue: %w", err)
	}
	if len(repoPaths) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "queue empty, nothing to index")
		return nil
	}

	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "Draining %d repo(s) from queue...\n", len(repoPaths))

	state, err := search.LoadState(indexDir)
	if err != nil {
		return err
	}

	var repos []finder.Repo
	for _, p := range repoPaths {
		repo, err := finder.Inspect(p)
		if err != nil {
			fmt.Fprintf(w, "  skip %s — %v\n", p, err)
			continue
		}
		repos = append(repos, repo)
	}

	if len(repos) == 0 {
		_ = queue.Release(claimedPath)
		fmt.Fprintln(cmd.OutOrStdout(), "no valid repos in queue")
		return nil
	}

	if err := search.IndexRepos(indexDir, repos, func(i, total int, repo finder.Repo) {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}); err != nil {
		return fmt.Errorf("indexing failed (claimed file: %s): %w", claimedPath, err)
	}

	for _, repo := range repos {
		fp, fpErr := search.Fingerprint(repo.Path)
		if fpErr == nil {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	if err := state.Save(indexDir); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := queue.Release(claimedPath); err != nil {
		fmt.Fprintf(w, "warning: remove claimed file %s: %v\n", claimedPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "indexed %d repo(s)\n", len(repos))
	return nil
}

// filterReposByName keeps repos whose name contains the given substring
// (case-insensitive). Used by `csl index --semantic --repo <name>`.
func filterReposByName(repos []finder.Repo, name string) []finder.Repo {
	q := strings.ToLower(name)
	var out []finder.Repo
	for _, r := range repos {
		if strings.Contains(strings.ToLower(r.Name), q) {
			out = append(out, r)
		}
	}
	return out
}

// runIndexSemantic builds the semantic embedding index for the discovered
// repos (optionally filtered by repoFilter). It is additive to the lexical
// index and lives under its own directory. On first run it downloads the model.
func runIndexSemantic(cmd *cobra.Command, repoFilter string) error {
	semDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return err
	}
	modelDir, err := semantic.DefaultModelDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	allRepos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		return err
	}
	var repos []finder.Repo
	for _, r := range allRepos {
		if cfg.Hooks.PostMerge.IsExcluded(r.Path, r.Name) {
			continue
		}
		repos = append(repos, r)
	}
	if repoFilter != "" {
		repos = filterReposByName(repos, repoFilter)
	}
	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no repos found in configured directories")
		return nil
	}

	w := cmd.ErrOrStderr()
	if _, statErr := os.Stat(modelDir); os.IsNotExist(statErr) {
		fmt.Fprintln(w, "downloading embedding model (first run, this may take a moment)...")
	}
	ctx := context.Background()
	if err := semantic.EnsureModel(ctx, modelDir); err != nil {
		return fmt.Errorf("ensure embedding model: %w", err)
	}

	emb, err := semantic.NewHugotEmbedder(ctx, modelDir)
	if err != nil {
		return fmt.Errorf("load embedding model: %w", err)
	}
	defer func() { _ = emb.Close() }()

	tty := false
	if f, ok := w.(*os.File); ok {
		tty = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}

	fmt.Fprintf(w, "Semantically indexing %d repo(s)...\n", len(repos))
	repoWidth := len(fmt.Sprint(len(repos)))
	var totalFiles, totalChunks int
	for i, repo := range repos {
		var opts []semantic.IndexOption
		if tty {
			opts = append(opts, semantic.WithProgress(func(p semantic.IndexProgress) {
				fileWidth := len(fmt.Sprint(p.FilesTotal))
				fmt.Fprintf(w, "\r  [%*d/%d] %s [%*d/%d] %s\033[K",
					repoWidth, i+1, len(repos), repo.Name,
					fileWidth, p.FilesDone, p.FilesTotal, p.Current)
			}))
		}
		stats, err := semantic.IndexRepoSemantic(ctx, semDir, repo, emb, opts...)
		if tty {
			fmt.Fprintf(w, "\r\033[K")
		}
		if err != nil {
			return fmt.Errorf("semantic index %s: %w", repo.Name, err)
		}
		totalFiles += stats.FilesEmbedded
		totalChunks += stats.ChunksEmbedded
		fmt.Fprintf(
			w, "  [%*d/%d] %s — %d embedded, %d chunks\n",
			repoWidth, i+1, len(repos), repo.Name, stats.FilesEmbedded, stats.ChunksEmbedded,
		)
	}

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"semantic index: %d repo(s), %d file(s) embedded, %d chunk(s)\n",
		len(repos), totalFiles, totalChunks,
	)

	// A running daemon loaded the semantic index at startup and will not see
	// the rebuild. Shut it down best-effort so the next query spawns a fresh
	// daemon that loads the updated index.
	_ = daemon.Shutdown(daemon.DefaultSocketPath())
	return nil
}

func runRepair(cmd *cobra.Command, indexDir string) error {
	w := cmd.OutOrStdout()

	shards, corrupted, err := search.ValidateShards(indexDir)
	if err != nil {
		return fmt.Errorf("validate shards: %w", err)
	}

	if indexJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(shards)
	}

	fmt.Fprintf(w, "Validating %d shard(s)...\n", len(shards))

	healthy := 0
	for _, s := range shards {
		if s.OK {
			healthy++
		} else {
			fmt.Fprintf(w, "  CORRUPT: %s — %s\n", filepath.Base(s.Path), s.Error)
		}
	}
	fmt.Fprintf(w, "  %d healthy, %d corrupted\n", healthy, len(corrupted))

	if len(corrupted) == 0 {
		fmt.Fprintln(w, "\nAll shards are healthy. Nothing to repair.")
		return nil
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		fmt.Fprintf(w, "State file corrupt, resetting: %v\n", err)
		state = search.EmptyState()
	}

	removed, err := search.RepairIndex(indexDir, state)
	if err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}

	if err := state.Save(indexDir); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to save state: %v\n", err)
	}

	fmt.Fprintf(w, "\nRemoved %d corrupted shard(s):\n", len(removed))
	for _, p := range removed {
		fmt.Fprintf(w, "  %s\n", filepath.Base(p))
	}
	fmt.Fprintln(w, "\nRun 'csl index' to re-index affected repos.")
	return nil
}
