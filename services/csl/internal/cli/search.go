package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/alpkeskin/gotoon"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

var (
	searchJSONFlag          bool
	searchToonFlag          bool
	searchRepoFlag          string
	searchLangFlag          string
	searchFileFlag          string
	searchLimitFlag         int
	searchOutputModeFlag    string
	searchContextLinesFlag  int
	searchCaseSensitiveFlag bool
	searchReindexFlag       bool
	searchServeFlag         bool
	searchStopFlag          bool
)

var searchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Search code across all indexed repositories",
	Long: `Search code across all indexed repositories using zoekt query syntax.

Query syntax:
  literal         Exact substring match (case-insensitive by default)
  foo.*bar        Regular expression
  "exact phrase"  Quoted exact match
  foo bar         AND: both terms must appear
  foo|bar         OR: either term matches
  -test           NOT: exclude matches
  sym:Name        Symbol definitions only (functions, types, methods)
  repo:name       Restrict to repos matching regex
  file:\.go$      Restrict to files matching regex
  lang:go         Restrict to a specific language
  case:yes        Force case-sensitive search

Empty results:
  A query that is empty, only quotes, or has an unbalanced quote is refused
  with the fix. When a search with several AND terms finds nothing and some
  terms match no file at all, it reruns once without them and says which it
  dropped. Otherwise it explains the empty result: how zoekt parsed the
  query, files per term, repos searched, and index age. Text and TOON
  output print these on stderr; --json carries them as relaxed_query,
  dropped_terms, and zero_result_hint.

Examples:
  csl search "func Walk"
  csl search "TODO|FIXME" --repo thismoon
  csl search "fmt\.Errorf" --lang go --output-mode content -C 3
  csl search "import.*cobra" --file "\.go$" --limit 10
  csl search --reindex "NewBuilder"

Indexing behavior:
  On first run, all configured repos are indexed (this may take a moment).
  Subsequent searches check for staleness: repos with new commits or
  dirty working trees are re-indexed in the background after results are
  returned. Use --reindex to force a synchronous re-index before searching.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().BoolVar(&searchJSONFlag, "json", false, "output as JSON")
	searchCmd.Flags().BoolVar(&searchToonFlag, "toon", false, "output as TOON for LLMs")
	searchCmd.Flags().
		StringVarP(&searchRepoFlag, "repo", "r", "", "restrict to repo names matching this case-insensitive regex (a plain substring also works)")
	searchCmd.Flags().
		StringVarP(&searchLangFlag, "lang", "l", "", "filter results to a specific language")
	searchCmd.Flags().
		StringVarP(&searchFileFlag, "file", "f", "", "filter results to files matching this regex")
	searchCmd.Flags().IntVar(&searchLimitFlag, "limit", 50, "maximum number of file results")
	searchCmd.Flags().
		StringVarP(&searchOutputModeFlag, "output-mode", "o", "files_with_matches", "output mode: files_with_matches or content")
	searchCmd.Flags().
		IntVarP(&searchContextLinesFlag, "context-lines", "C", 0, "number of context lines around each match")
	searchCmd.Flags().
		BoolVar(&searchCaseSensitiveFlag, "case-sensitive", false, "force case-sensitive matching")
	searchCmd.Flags().
		BoolVar(&searchReindexFlag, "reindex", false, "force synchronous re-index before searching")
	searchCmd.Flags().
		BoolVar(&searchServeFlag, "serve", false, "start the search daemon (foreground)")
	searchCmd.Flags().BoolVar(&searchStopFlag, "stop", false, "stop the running search daemon")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return err
	}
	socketPath := daemon.DefaultSocketPath()

	// Handle --serve: start daemon in foreground.
	if searchServeFlag {
		if daemon.IsRunning(daemon.DefaultPIDPath()) {
			fmt.Fprintln(cmd.ErrOrStderr(), "search daemon is already running")
			return nil
		}
		// Load config best-effort: a missing or unreadable config leaves
		// semantic search off, and the daemon still serves lexical search.
		cfg, _ := config.Load()
		return daemon.Serve(indexDir, socketPath, cfg.DaemonIdleTimeout(), cfg.SemanticEnabled())
	}

	// Handle --stop: kill running daemon.
	if searchStopFlag {
		if err := daemon.Shutdown(socketPath); err != nil {
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"daemon not running or already stopped: %v\n",
				err,
			)
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), "search daemon stopped")
		}
		return nil
	}

	// Normal search: require pattern.
	if len(args) == 0 {
		return fmt.Errorf(
			"pattern is required\n\nUsage: csl search <pattern>\nRun 'csl search --help' for examples",
		)
	}
	pattern := args[0]
	if m := lexical.CheckShape(pattern, cliSurface); m != nil {
		return m
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
		// Nothing to search is a result, not a failure, on a machine that has
		// not been configured yet. The hint names the file to create; a
		// configured machine that finds nothing keeps its error, and says so
		// in terms of the filter when a filter is what emptied the list.
		if !cfg.Loaded {
			fmt.Fprintln(cmd.ErrOrStderr(), cfg.EmptyDiscoveryHint(dropped))
			return nil
		}
		return errors.New(cfg.EmptyDiscoveryHint(dropped))
	}

	// Build repo name → path map for search.
	repoNames := make(map[string]string, len(repos))
	for _, r := range repos {
		repoNames[r.Name] = r.Path
	}

	ctx := context.Background()
	local := lexical.Local{IndexDir: indexDir, RepoNames: repoNames}
	req := lexical.Request{
		Options:  searchOptions(pattern),
		Repo:     searchRepoFlag,
		Repos:    repos,
		IndexDir: indexDir,
	}

	// Try daemon first (auto-starts if not running).
	if !searchReindexFlag {
		if err := daemon.EnsureDaemon(indexDir, socketPath); err == nil {
			matches, err := daemon.SearchVia(ctx, socketPath, req.Options, repoNames)
			if err == nil {
				backend := lexical.Daemon{Local: local, SocketPath: socketPath}
				return reportSearch(ctx, cmd, backend, req, matches)
			}
			// Daemon failed — fall through to in-process search.
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"daemon search failed, falling back to in-process: %v\n",
				err,
			)
		}
	}

	matches, err := searchInProcess(ctx, cmd, cfg, local, req)
	if err != nil {
		return err
	}
	return reportSearch(ctx, cmd, local, req, matches)
}

// searchOptions builds the search the flags ask for. The repo filter is
// widened to case-insensitive, the way csl_search's repo parameter matches.
func searchOptions(pattern string) search.SearchOptions {
	return search.SearchOptions{
		Pattern:       pattern,
		RepoFilter:    lexical.RepoFilter(searchRepoFlag),
		FileFilter:    searchFileFlag,
		Lang:          searchLangFlag,
		CaseSensitive: searchCaseSensitiveFlag,
		Limit:         searchLimitFlag,
		ContextLines:  searchContextLinesFlag,
		OutputMode:    searchOutputModeFlag,
	}
}

// searchInProcess runs the search without the daemon: it indexes first when
// there is no index yet or --reindex asks for it, searches the shards, and
// re-indexes stale repos in the background afterwards. With --repo set, only
// the repos the filter matches are checked and indexed, none when it matches
// no discovered repo.
func searchInProcess(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	local lexical.Local,
	req lexical.Request,
) ([]search.Match, error) {
	indexDir := local.IndexDir
	repos := req.Repos
	if req.Repo != "" {
		matched, err := reposMatching(repos, req.Repo)
		if err != nil {
			return nil, err
		}
		// A filter that matches no discovered repo is not an error: with
		// nothing to check or index, the search below comes back empty and
		// reportSearch explains the filter, as it does after a daemon search.
		repos = matched
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return nil, err
	}

	staleness, err := search.CheckStaleness(repos, state)
	if err != nil {
		return nil, err
	}

	// If no index exists at all, do a full sync index.
	noIndex := len(state.Repos) == 0
	if noIndex || searchReindexFlag {
		toIndex := repos
		if !noIndex && !searchReindexFlag {
			toIndex = staleness.Stale
		}
		if len(toIndex) > 0 {
			if err := indexForSearch(cmd, indexDir, toIndex, cfg.AllowedHiddenDirs(), staleness, state); err != nil {
				return nil, err
			}
		}
	}

	matches, err := local.Search(ctx, req.Options)
	if err != nil {
		return nil, err
	}

	// Background re-index for stale repos, under the sync lock so it never
	// races csl sync or the web refresher. When one of those holds the lock
	// it is picking these repos up itself, so just skip.
	if !noIndex && !searchReindexFlag && len(staleness.Stale) > 0 {
		go func() {
			unlock, err := syncer.Lock(indexDir)
			if err != nil {
				return
			}
			defer unlock()
			_ = search.IndexRepos(indexDir, staleness.Stale, cfg.AllowedHiddenDirs(), nil)
			// Reload state from disk to avoid clobbering concurrent writers.
			freshState, err := search.LoadState(indexDir)
			if err != nil {
				return
			}
			// Re-fingerprint after indexing to capture the actual indexed content.
			for _, repo := range staleness.Stale {
				fp, err := search.Fingerprint(repo.Path)
				if err != nil {
					continue
				}
				fp.IndexedAt = time.Now()
				freshState.SetRepo(repo.Path, fp)
			}
			_ = freshState.Save(indexDir)
		}()
	}
	return matches, nil
}

// reposMatching narrows repos to the ones the --repo filter names, by the
// case-insensitive regex finder.CompileMatcher builds, the same match the
// search applies to repo names.
func reposMatching(repos []finder.Repo, filter string) ([]finder.Repo, error) {
	re, err := finder.CompileMatcher(filter)
	if err != nil {
		return nil, fmt.Errorf("--repo: %w", err)
	}
	var matched []finder.Repo
	for _, r := range repos {
		if re.MatchString(r.Name) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

// indexForSearch builds the index for toIndex before the foreground search
// runs, holding the cross-process sync lock for the write. When csl sync or
// the web refresher already holds the lock, the build is skipped with a note
// and the search runs against whatever shards exist instead of racing the
// other writer's shard and state.json updates.
func indexForSearch(
	cmd *cobra.Command,
	indexDir string,
	toIndex []finder.Repo,
	hiddenDirs []string,
	staleness *search.StalenessResult,
	state *search.IndexState,
) error {
	w := cmd.ErrOrStderr()

	unlock, err := syncer.Lock(indexDir)
	if errors.Is(err, syncer.ErrLocked) {
		fmt.Fprintln(
			w,
			"another sync or background refresh is indexing; searching the existing index",
		)
		return nil
	}
	if err != nil {
		return err
	}
	defer unlock()

	fmt.Fprintf(w, "Indexing %d repo(s)...\n", len(toIndex))
	if err := search.IndexRepos(indexDir, toIndex, hiddenDirs, func(i, total int, repo finder.Repo) {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}
	for _, repo := range toIndex {
		if fp, ok := staleness.Current[repo.Path]; ok {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	if err := state.Save(indexDir); err != nil {
		fmt.Fprintf(w, "warning: failed to save index state: %v\n", err)
	}
	return nil
}

// cliSurface points the csl search hint notes at CLI commands and the
// malformed-query fixes at the pattern argument. The CLI has no parameter
// names to mistype into the pattern, so it skips that note.
var cliSurface = lexical.Surface{
	QueryArg:   "the <pattern> argument",
	RepoLookup: "csl repo --list",
	Reindex:    "csl index --repo <path>",
}

// reportSearch prints a search's matches. An empty first search goes through
// lexical.ExplainZero first: a relaxed rerun's matches print in place of the
// empty page, and otherwise the zero-result hint explains it.
func reportSearch(
	ctx context.Context,
	cmd *cobra.Command,
	b lexical.Backend,
	req lexical.Request,
	matches []search.Match,
) error {
	var res lexical.Result
	if len(matches) == 0 {
		res = lexical.ExplainZero(ctx, b, cliSurface, req)
		matches = res.Matches
	}
	return outputSearchResults(cmd, matches, res)
}

// searchJSON is the --json document: the matches and, after an empty first
// search, the relaxed rerun's query and dropped terms or the hint.
type searchJSON struct {
	Matches      []search.Match    `json:"matches"`
	RelaxedQuery string            `json:"relaxed_query,omitempty"`
	DroppedTerms []string          `json:"dropped_terms,omitempty"`
	ZeroHint     *lexical.ZeroHint `json:"zero_result_hint,omitempty"`
}

// outputSearchResults prints matches in the format the flags ask for. --json
// carries the relaxed rerun and the hint as fields; text and TOON keep stdout
// to the matches and print the relaxed line before them and the hint after
// them on stderr.
func outputSearchResults(cmd *cobra.Command, matches []search.Match, res lexical.Result) error {
	w := cmd.OutOrStdout()

	if searchJSONFlag {
		if matches == nil {
			matches = []search.Match{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(searchJSON{
			Matches:      matches,
			RelaxedQuery: res.RelaxedQuery,
			DroppedTerms: res.DroppedTerms,
			ZeroHint:     res.Hint,
		})
	}

	errw := cmd.ErrOrStderr()
	if res.RelaxedQuery != "" {
		fmt.Fprintln(errw, lexical.RelaxedLine(res.DroppedTerms, res.RelaxedQuery))
	}
	if err := printMatches(w, matches); err != nil {
		return err
	}
	for _, line := range res.Hint.Lines() {
		fmt.Fprintln(errw, line)
	}
	return nil
}

// printMatches prints matches as TOON or as text: one path per file, or
// ripgrep-style blocks in content mode.
func printMatches(w io.Writer, matches []search.Match) error {
	if searchToonFlag {
		items := make([]map[string]any, len(matches))
		for i, m := range matches {
			items[i] = map[string]any{
				"repo":   m.Repo,
				"file":   m.File,
				"line":   m.Line,
				"column": m.Column,
				"text":   m.Text,
			}
		}
		encoded, err := gotoon.Encode(map[string]any{"matches": items})
		if err != nil {
			return err
		}
		fmt.Fprintln(w, encoded)
		return nil
	}

	if len(matches) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}

	switch searchOutputModeFlag {
	case "content":
		// One block per cluster of nearby hits, in the ripgrep --heading
		// grammar the MCP text format also prints (search.FileBlocks.Render),
		// plus the match column (ripgrep --column) that only the CLI shows.
		for i, f := range search.Blocks(matches) {
			if i > 0 {
				fmt.Fprintln(w)
			}
			for _, line := range f.Render(search.RenderOptions{Column: true}) {
				fmt.Fprintln(w, line)
			}
		}
	default: // files_with_matches
		seen := make(map[string]bool)
		for _, m := range matches {
			key := m.Repo + "/" + m.File
			if !seen[key] {
				seen[key] = true
				fmt.Fprintf(w, "%s/%s\n", m.Repo, m.File)
			}
		}
	}

	return nil
}
