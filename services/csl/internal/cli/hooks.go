package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/queue"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// hookMarkerPrefix is the common prefix for version-agnostic hook detection.
const hookMarkerPrefix = "# csl-managed-hook:"

// hookMarker identifies the current hook version. Bumping this lets
// `csl hooks install` recognize and overwrite older managed hooks.
const hookMarker = hookMarkerPrefix + " post-merge v2"

// hookScript renders the post-merge hook, which appends the repo path to the
// queue file instead of indexing directly. A subsequent `csl index --drain`
// or `csl sync` batch-indexes all queued repos in a single process, avoiding
// concurrent state.json writes.
//
// queuePath is baked in at install time rather than derived in the script:
// the hook runs in a bare git environment where csl may not be on PATH, so it
// cannot ask csl where the queue lives.
func hookScript(queuePath string) string {
	return `#!/usr/bin/env sh
` + hookMarker + `
# Edit csl config (` + "`csl config`" + ` prints the file it reads), not this
# file — ` + "`csl hooks install`" + ` will overwrite it.
# csl sync suppresses hooks via core.hooksPath=/dev/null, so this only
# fires on direct git pull (outside csl sync).
mkdir -p "` + filepath.Dir(queuePath) + `" && \
  printf '%s\n' "$(git rev-parse --show-toplevel)" >> "` + queuePath + `"
exit 0
`
}

var (
	hooksDryRunFlag bool
	hooksForceFlag  bool
	hooksRepoFlag   string
	hooksJSONFlag   bool
)

// deprecationNotice explains that csl no longer manages post-merge hooks and
// points users to suspenders. It is printed by `csl hooks install` and shown
// in the command long help.
const deprecationNotice = `DEPRECATED: csl no longer manages post-merge hooks.

suspenders is now the single git-hook manager. Configure a ` + "`csl-reindex`" + ` entry
under suspenders' post_merge config; it writes each merged repo path to csl's
reindex queue (` + "`csl doctor`" + ` and ` + "`csl config`" + ` name the state
directory holding it). csl still OWNS draining that queue and indexing — run
` + "`csl sync`" + ` or ` + "`csl index --drain`" + ` as before.

To migrate:
  1. Run ` + "`csl hooks uninstall`" + ` to remove any csl-managed post-merge hooks.
  2. Add the ` + "`csl-reindex`" + ` entry to suspenders' post_merge config.`

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "[DEPRECATED] Manage legacy csl-managed git hooks",
	Long: deprecationNotice + `

---

Historical behaviour (kept only so existing managed hooks can be removed):

When the post_merge hook is enabled in config.yaml, ` + "`csl hooks install`" + `
writes a .git/hooks/post-merge script into every discovered repo (except those
in hooks.post_merge.exclude). The hook appends the repo path to the reindex
queue after each git pull, with the queue path baked in at install time.

` + "`csl hooks uninstall`" + ` and ` + "`csl hooks status`" + ` remain fully functional so you
can remove csl-managed hooks and hand hook management over to suspenders.`,
}

var hooksInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "[DEPRECATED] Install or update legacy post-merge hook in every non-excluded repo",
	RunE:  runHooksInstall,
}

var hooksUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove csl-managed post-merge hook from every repo",
	RunE:  runHooksUninstall,
}

var hooksStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hook installation state for every repo",
	RunE:  runHooksStatus,
}

func init() {
	hooksInstallCmd.Flags().
		BoolVar(&hooksDryRunFlag, "dry-run", false, "print planned changes without writing files")
	hooksInstallCmd.Flags().
		BoolVar(&hooksForceFlag, "force", false, "overwrite even foreign (non-csl-managed) hooks")
	hooksInstallCmd.Flags().
		StringVar(&hooksRepoFlag, "repo", "", "operate on a single repo by absolute path (debugging)")
	hooksUninstallCmd.Flags().
		StringVar(&hooksRepoFlag, "repo", "", "operate on a single repo by absolute path (debugging)")
	hooksUninstallCmd.Flags().
		BoolVar(&hooksDryRunFlag, "dry-run", false, "print planned changes without writing files")
	hooksStatusCmd.Flags().BoolVar(&hooksJSONFlag, "json", false, "output as JSON")

	hooksCmd.AddCommand(hooksInstallCmd, hooksUninstallCmd, hooksStatusCmd)
	rootCmd.AddCommand(hooksCmd)
}

// hookState describes the state of the post-merge hook in a single repo.
type hookState struct {
	Repo     string `json:"repo"`
	Path     string `json:"path"`
	Status   string `json:"status"` // installed | missing | foreign | excluded | size_excluded
	Excluded bool   `json:"excluded"`
	Reason   string `json:"reason,omitempty"`
}

// loadReposForHooks loads config + walks repos, honoring --repo if set.
// Returns the configured PostMergeHook so callers can apply exclusions.
func loadReposForHooks() ([]finder.Repo, *config.PostMergeHook, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	if hooksRepoFlag != "" {
		abs, err := filepath.Abs(hooksRepoFlag)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve --repo path: %w", err)
		}
		repo, err := finder.Inspect(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect %s: %w", abs, err)
		}
		return []finder.Repo{repo}, &cfg.Hooks.PostMerge, nil
	}

	repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		return nil, nil, err
	}
	return repos, &cfg.Hooks.PostMerge, nil
}

func runHooksInstall(cmd *cobra.Command, args []string) error {
	// Print the deprecation warning regardless of config state so anyone still
	// wiring `csl hooks install` into automation sees it.
	fmt.Fprintln(cmd.ErrOrStderr(), deprecationNotice)
	fmt.Fprintln(cmd.ErrOrStderr())

	repos, postMerge, err := loadReposForHooks()
	if err != nil {
		return err
	}

	if !postMerge.Enabled && hooksRepoFlag == "" {
		return fmt.Errorf(
			"hooks.post_merge.enabled is false in config — nothing to install (csl post-merge hooks are deprecated; use suspenders)",
		)
	}

	queuePath, err := queue.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve reindex queue: %w", err)
	}
	script := hookScript(queuePath)

	w := cmd.OutOrStdout()
	var changed, skipped, foreign, errored int

	for _, repo := range repos {
		if postMerge.IsExcluded(repo.Path, repo.Name) {
			fmt.Fprintf(w, "  skip %s — excluded by config\n", repo.Name)
			skipped++
			continue
		}

		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		existing, readErr := os.ReadFile(hookPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			fmt.Fprintf(w, "  ERR  %s — read %s: %v\n", repo.Name, hookPath, readErr)
			errored++
			continue
		}

		isForeign := readErr == nil && !strings.Contains(string(existing), hookMarkerPrefix)
		if isForeign && !hooksForceFlag {
			fmt.Fprintf(
				w,
				"  skip %s — foreign hook present (use --force to overwrite)\n",
				repo.Name,
			)
			foreign++
			continue
		}

		if string(existing) == script {
			continue
		}

		if hooksDryRunFlag {
			fmt.Fprintf(w, "  would write %s\n", hookPath)
			changed++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
			fmt.Fprintf(w, "  ERR  %s — mkdir: %v\n", repo.Name, err)
			errored++
			continue
		}
		if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
			fmt.Fprintf(w, "  ERR  %s — write: %v\n", repo.Name, err)
			errored++
			continue
		}
		fmt.Fprintf(w, "  wrote %s\n", repo.Name)
		changed++
	}

	fmt.Fprintf(
		w,
		"\n%d changed, %d excluded, %d foreign, %d errors\n",
		changed,
		skipped,
		foreign,
		errored,
	)
	if errored > 0 {
		return fmt.Errorf("%d repo(s) failed", errored)
	}
	return nil
}

func runHooksUninstall(cmd *cobra.Command, args []string) error {
	repos, _, err := loadReposForHooks()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	var removed, skipped, foreign int

	for _, repo := range repos {
		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		existing, readErr := os.ReadFile(hookPath)
		if os.IsNotExist(readErr) {
			skipped++
			continue
		}
		if readErr != nil {
			fmt.Fprintf(w, "  ERR  %s — read %s: %v\n", repo.Name, hookPath, readErr)
			continue
		}
		if !strings.Contains(string(existing), hookMarkerPrefix) {
			fmt.Fprintf(w, "  skip %s — foreign hook (not csl-managed)\n", repo.Name)
			foreign++
			continue
		}
		if hooksDryRunFlag {
			fmt.Fprintf(w, "  would remove %s\n", hookPath)
			removed++
			continue
		}
		if err := os.Remove(hookPath); err != nil {
			fmt.Fprintf(w, "  ERR  %s — remove: %v\n", repo.Name, err)
			continue
		}
		fmt.Fprintf(w, "  removed %s\n", repo.Name)
		removed++
	}

	fmt.Fprintf(w, "\n%d removed, %d had no hook, %d foreign\n", removed, skipped, foreign)
	return nil
}

func runHooksStatus(cmd *cobra.Command, args []string) error {
	repos, postMerge, err := loadReposForHooks()
	if err != nil {
		return err
	}

	states := make([]hookState, 0, len(repos))
	for _, repo := range repos {
		s := hookState{Repo: repo.Name, Path: repo.Path}
		if postMerge.IsExcluded(repo.Path, repo.Name) {
			s.Status = "excluded"
			s.Excluded = true
			s.Reason = "matched config exclude"
			states = append(states, s)
			continue
		}

		hookPath := filepath.Join(repo.Path, ".git", "hooks", "post-merge")
		data, readErr := os.ReadFile(hookPath)
		switch {
		case os.IsNotExist(readErr):
			s.Status = "missing"
		case readErr != nil:
			s.Status = "error"
			s.Reason = readErr.Error()
		case strings.Contains(string(data), hookMarkerPrefix):
			s.Status = "installed"
		default:
			s.Status = "foreign"
		}
		states = append(states, s)
	}

	w := cmd.OutOrStdout()
	if hooksJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(states)
	}

	fmt.Fprintf(w, "%-50s %-12s %s\n", "REPO", "HOOK", "NOTE")
	for _, s := range states {
		note := s.Reason
		fmt.Fprintf(w, "%-50s %-12s %s\n", s.Repo, s.Status, note)
	}
	return nil
}
