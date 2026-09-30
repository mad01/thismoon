package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
Use --drain to batch-index repos queued by post-merge hooks.

Every index write takes the sync lock shared with csl sync and the csl web
refresher. While one of those runs, the command fails fast instead of racing
it: wait for it to finish, then retry.`,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().
		BoolVar(&indexAllFlag, "all", false, "force full re-index of all repos (lexical + semantic)")
	indexCmd.Flags().
		BoolVar(&indexLexicalAllFlag, "lexical-all", false, "force full lexical re-index only")
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

	repos, err := cfg.DiscoverRepos()
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

	err = indexStale(cmd, indexDir, toIndex, cfg.AllowedHiddenDirs(), staleness.Current)
	if err != nil {
		return err
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

// indexWrite describes one write of shards and state.json: the repos to
// rebuild, optional progress output, and where each repo's fresh fingerprint
// comes from. A full run reuses the fingerprints its staleness check already
// computed; single-repo and queue-drain runs probe git after indexing.
type indexWrite struct {
	repos       []finder.Repo
	hiddenDirs  []string
	progress    func(i, total int, repo finder.Repo)
	fingerprint func(repo finder.Repo) (search.RepoState, bool)
}

// writeIndex rebuilds the shards for w.repos and records their fingerprints
// in state.json. The caller holds the sync lock (see withIndexLock), and the
// state is reloaded under it: a sync may have saved state.json after this
// command loaded it for the staleness check, and those entries must survive.
func writeIndex(indexDir string, w indexWrite) error {
	if err := search.IndexRepos(indexDir, w.repos, w.hiddenDirs, w.progress); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, repo := range w.repos {
		fp, ok := w.fingerprint(repo)
		if !ok {
			continue
		}
		fp.IndexedAt = now
		state.SetRepo(repo.Path, fp)
	}
	if err := state.Save(indexDir); err != nil {
		return fmt.Errorf("failed to save index state: %w", err)
	}
	return nil
}

// fingerprintFrom serves fingerprints out of a staleness check's Current map.
func fingerprintFrom(
	current map[string]search.RepoState,
) func(finder.Repo) (search.RepoState, bool) {
	return func(repo finder.Repo) (search.RepoState, bool) {
		fp, ok := current[repo.Path]
		return fp, ok
	}
}

// fingerprintNow probes git for repo's fingerprint after an ad-hoc index; a
// repo git cannot describe keeps its old state.json entry.
func fingerprintNow(repo finder.Repo) (search.RepoState, bool) {
	fp, err := search.Fingerprint(repo.Path)
	return fp, err == nil
}

// progressLine prints one "[i/total] name" line per indexed repo to w.
func progressLine(w io.Writer) func(i, total int, repo finder.Repo) {
	return func(i, total int, repo finder.Repo) {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}
}

// indexStale rebuilds the shards for toIndex under the sync lock, recording
// the fingerprints the staleness check already computed so a full run never
// probes git twice per repo. A held lock fails the command fast.
func indexStale(
	cmd *cobra.Command,
	indexDir string,
	toIndex []finder.Repo,
	hiddenDirs []string,
	current map[string]search.RepoState,
) error {
	w := cmd.ErrOrStderr()
	return withIndexLock(indexDir, func() error {
		fmt.Fprintf(w, "Indexing %d repo(s)...\n", len(toIndex))
		return writeIndex(indexDir, indexWrite{
			repos:       toIndex,
			hiddenDirs:  hiddenDirs,
			progress:    progressLine(w),
			fingerprint: fingerprintFrom(current),
		})
	})
}

// runIndexSingle re-indexes one repo by absolute path without the global
// scan, updating its state.json entry so the next `csl index` skips it. Like
// every other writer it holds the sync lock, failing fast while csl sync or
// the web refresher runs instead of writing beside them.
func runIndexSingle(cmd *cobra.Command, indexDir, repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path %s: %w", repoPath, err)
	}

	repo, err := finder.Inspect(abs)
	if err != nil {
		return fmt.Errorf("inspect repo %s: %w", abs, err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	err = withIndexLock(indexDir, func() error {
		fmt.Fprintf(cmd.ErrOrStderr(), "Indexing %s\n", repo.Name)
		return writeIndex(indexDir, indexWrite{
			repos:       []finder.Repo{repo},
			hiddenDirs:  cfg.AllowedHiddenDirs(),
			fingerprint: fingerprintNow,
		})
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "indexed %s\n", repo.Name)
	return nil
}

// runIndexDrain batch-indexes the repos queued by post-merge hooks. The sync
// lock is taken before the queue is claimed, so a drain that loses to a
// running sync fails fast and leaves the queue for that sync to drain.
func runIndexDrain(cmd *cobra.Command, indexDir string) error {
	queuePath, err := queue.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return withIndexLock(indexDir, func() error {
		return drainQueue(cmd, indexDir, queuePath, cfg.AllowedHiddenDirs())
	})
}

// drainQueue claims the queue and indexes every repo in it that still
// inspects as a git repo. The caller holds the sync lock.
func drainQueue(cmd *cobra.Command, indexDir, queuePath string, hiddenDirs []string) error {
	out, w := cmd.OutOrStdout(), cmd.ErrOrStderr()

	claimedPath, repoPaths, err := queue.Claim(queuePath)
	if err != nil {
		return fmt.Errorf("claim queue: %w", err)
	}
	if len(repoPaths) == 0 {
		fmt.Fprintln(out, "queue empty, nothing to index")
		return nil
	}

	fmt.Fprintf(w, "Draining %d repo(s) from queue...\n", len(repoPaths))

	repos := inspectQueued(w, repoPaths)
	if len(repos) == 0 {
		_ = queue.Release(claimedPath)
		fmt.Fprintln(out, "no valid repos in queue")
		return nil
	}

	err = writeIndex(indexDir, indexWrite{
		repos:       repos,
		hiddenDirs:  hiddenDirs,
		progress:    progressLine(w),
		fingerprint: fingerprintNow,
	})
	if err != nil {
		return fmt.Errorf("%w (claimed file: %s)", err, claimedPath)
	}

	if err := queue.Release(claimedPath); err != nil {
		fmt.Fprintf(w, "warning: remove claimed file %s: %v\n", claimedPath, err)
	}

	fmt.Fprintf(out, "indexed %d repo(s)\n", len(repos))
	return nil
}

// inspectQueued resolves queued repo paths, reporting and skipping the ones
// that no longer inspect as git repos.
func inspectQueued(w io.Writer, repoPaths []string) []finder.Repo {
	var repos []finder.Repo
	for _, p := range repoPaths {
		repo, err := finder.Inspect(p)
		if err != nil {
			fmt.Fprintf(w, "  skip %s: %v\n", p, err)
			continue
		}
		repos = append(repos, repo)
	}
	return repos
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
// index and lives under its own directory. Embedding runs via Ollama; the
// configured model must be pulled first.
func runIndexSemantic(cmd *cobra.Command, repoFilter string) error {
	semDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	repos, dropped, err := cfg.DiscoverReposReport()
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), cfg.EmptyDiscoveryHint(dropped))
		return nil
	}
	if repoFilter != "" {
		repos = filterReposByName(repos, repoFilter)
		if len(repos) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "no repo matching %q\n", repoFilter)
			return nil
		}
	}

	w := cmd.ErrOrStderr()
	ctx := context.Background()
	emb := semantic.NewOllamaEmbedder(
		cfg.Semantic.OllamaURL,
		cfg.Semantic.EmbedModel,
		cfg.Semantic.Dim,
	)
	if err := emb.CheckModel(ctx); err != nil {
		return fmt.Errorf("embedding backend not ready: %w", err)
	}
	// Bulk indexing shouldn't leave the model resident for the keep-alive
	// window once the run is over.
	defer func() { _ = emb.Unload(context.Background()) }()

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
