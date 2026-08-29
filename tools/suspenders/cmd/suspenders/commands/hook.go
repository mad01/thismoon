package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/gobwas/glob"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/guard"
	"github.com/mad01/thismoon/tools/suspenders/internal/hook"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

var hookAll bool

func init() {
	hookInstallCmd.Flags().
		BoolVar(&hookAll, "all", false, "install hooks in all discovered repositories")
	hookUpdateCmd.Flags().
		BoolVar(&hookAll, "all", false, "update hooks in all discovered repositories")
	hookUninstallCmd.Flags().
		BoolVar(&hookAll, "all", false, "uninstall hooks from all discovered repositories")
	hookStatusCmd.Flags().
		BoolVar(&hookAll, "all", false, "show status for all discovered repositories")

	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookUpdateCmd)
	hookCmd.AddCommand(hookStatusCmd)
	hookCmd.AddCommand(hookRunCmd)
	rootCmd.AddCommand(hookCmd)
}

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git hooks",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install [path]",
	Short: "Install git hooks",
	Long:  "Install suspenders-managed git hooks to a repository. Use --all to install to all discovered repositories.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHookInstall,
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall [path]",
	Short: "Uninstall git hooks",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHookUninstall,
}

var hookUpdateCmd = &cobra.Command{
	Use:   "update [path]",
	Short: "Update outdated git hooks",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHookUpdate,
}

var hookStatusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "Show git hook status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHookStatus,
}

var hookRunCmd = &cobra.Command{
	Use:   "run <event>",
	Short: "Run hooks for a git event (called by git hook scripts)",
	Long:  "Dispatches to configured external hooks and built-in checks for the given event. Normally called from .git/hooks/<event>, not directly.",
	Args:  cobra.ExactArgs(1),
	RunE:  runHookRun,
}

// configuredEvents returns which events should be installed based on config.
// pre-commit is always included (scan is enabled by default).
// post-merge is included if any post_merge hooks are configured.
func configuredEvents(cfg *config.Config) []hook.Event {
	events := []hook.Event{hook.PreCommit}
	if cfg != nil && len(cfg.Hooks.PostMerge) > 0 {
		events = append(events, hook.PostMerge)
	}
	return events
}

func runHookInstall(cmd *cobra.Command, args []string) error {
	mgr := hook.New()
	// config.Load returns a default config (nil error) when no config file
	// exists, and (nil, error) only when the file exists but can't be read or
	// parsed. Propagate that error rather than swallowing it: a broken config
	// must fail the command, not silently disable the guard and external hooks.
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	events := configuredEvents(cfg)

	if hookAll {
		repos, err := discoverRepos()
		if err != nil {
			return err
		}
		return forEachRepo(repos, "Installing", func(r repofind.Repo) error {
			return mgr.Install(r.Path, events)
		})
	}

	repoPath, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	if err := mgr.Install(repoPath, events); err != nil {
		return err
	}
	color.Green("Hooks installed in %s (%s)", repoPath, eventNames(events))
	return nil
}

func runHookUninstall(cmd *cobra.Command, args []string) error {
	mgr := hook.New()
	allEvents := []hook.Event{hook.PreCommit, hook.PostMerge}

	if hookAll {
		repos, err := discoverRepos()
		if err != nil {
			return err
		}
		return forEachRepo(repos, "Uninstalling", func(r repofind.Repo) error {
			return mgr.Uninstall(r.Path, allEvents)
		})
	}

	repoPath, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	if err := mgr.Uninstall(repoPath, allEvents); err != nil {
		return err
	}
	color.Green("Hooks uninstalled from %s", repoPath)
	return nil
}

func runHookUpdate(cmd *cobra.Command, args []string) error {
	mgr := hook.New()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	events := configuredEvents(cfg)

	if hookAll {
		repos, err := discoverRepos()
		if err != nil {
			return err
		}
		updated := 0
		err = forEachRepo(repos, "Checking", func(r repofind.Repo) error {
			for _, event := range events {
				if !mgr.NeedsUpdate(r.Path, event) {
					continue
				}
				updated++
				if err := mgr.Update(r.Path, event); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if updated == 0 {
			color.Green("All hooks are up to date.")
		}
		return nil
	}

	repoPath, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	updated := false
	for _, event := range events {
		if !mgr.NeedsUpdate(repoPath, event) {
			continue
		}
		if err := mgr.Update(repoPath, event); err != nil {
			return err
		}
		updated = true
		color.Green("Updated %s hook in %s", event, repoPath)
	}
	if !updated {
		color.Green("All hooks are up to date.")
	}
	return nil
}

func runHookStatus(cmd *cobra.Command, args []string) error {
	mgr := hook.New()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	events := configuredEvents(cfg)

	if hookAll {
		repos, err := discoverRepos()
		if err != nil {
			return err
		}
		green := color.New(color.FgGreen)
		yellow := color.New(color.FgYellow)
		red := color.New(color.FgRed)
		cyan := color.New(color.FgCyan)

		for _, r := range repos {
			var parts []string
			for _, event := range events {
				status := mgr.Status(r.Path, event)
				parts = append(parts, fmt.Sprintf("%s:%s", event, status))
			}
			summary := strings.Join(parts, " ")

			c := green
			if strings.Contains(summary, "error") {
				c = red
			} else if strings.Contains(summary, "outdated") {
				c = yellow
			} else if strings.Contains(summary, "not-installed") {
				c = red
			} else if strings.Contains(summary, "foreign") {
				c = cyan
			}
			_, _ = c.Printf("  %-50s %s\n", r.Name, summary)
		}
		return nil
	}

	repoPath, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	for _, event := range events {
		status := mgr.Status(repoPath, event)
		fmt.Printf("  %s: %s\n", event, status)
	}
	return nil
}

// runHookRun is the dispatcher called from git hook scripts.
func runHookRun(cmd *cobra.Command, args []string) error {
	eventName := args[0]

	root, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// A missing config yields defaults (nil error); a present-but-broken config
	// yields an error. Fail the hook run on the latter rather than silently
	// running with the guard and external hooks disabled.
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	switch hook.Event(eventName) {
	case hook.PreCommit:
		return runPreCommit(root, cfg)
	case hook.PostMerge:
		return runPostMerge(root, cfg)
	default:
		return fmt.Errorf("unknown event: %s", eventName)
	}
}

func runPreCommit(root string, cfg *config.Config) error {
	repoName := repoNameFromPath(root)

	// Aggregate any too-large/unreadable files skipped by the guard and scan
	// into one note, printed on every return path (including a blocked commit).
	skips := &skipCollector{}
	defer skips.report()

	// 1. External pre-commit hooks.
	if cfg != nil {
		for _, h := range cfg.Hooks.PreCommit {
			if !h.IsEnabled() {
				continue
			}
			if !matchesRepo(repoName, h.Repos) {
				continue
			}
			if len(h.FilePatterns) > 0 {
				staged, err := guard.StagedFiles(root)
				if err == nil && !anyFileMatches(staged, h.FilePatterns) {
					continue
				}
			}
			if err := runShellCommand(h.Command, root); err != nil {
				return fmt.Errorf("hook %s: %w", h.Name, err)
			}
		}
	}

	// 2. Built-in guard — skip for exempt repos (inside workspace_dirs or
	// matching a top-level exclude pattern).
	if cfg != nil && cfg.Guard.Enabled && !guardExempt(root, cfg) {
		g := guard.New(guardConfigFor(root, cfg))
		g.OnSkip = skips.guardSkip
		findings, err := g.Check(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: guard check failed: %v\n", err)
		} else if len(findings) > 0 {
			printGuardFindings(findings)
			notify.EmitEventSync("suspenders", "warn",
				fmt.Sprintf("commit blocked: %d internal reference(s)", len(findings)),
				"", map[string]string{"repo": repoName, "check": "guard"})
			return scanner.ErrFindingsFound
		}
	}

	// 3. Built-in scan.
	if cfg == nil || cfg.Scan.ScanEnabled() {
		return runBuiltinScan(root, cfg, skips)
	}

	return nil
}

// guardExempt reports whether the guard should not run in the repo at root:
// repos inside guard.workspace_dirs are internal by definition, and repos
// whose org/repo name matches a top-level exclude pattern are opted out
// explicitly. Neither affects name collection — an exempt repo's own name
// still contributes blocked names for other repos.
func guardExempt(root string, cfg *config.Config) bool {
	if isInsideWorkspaceDirs(root, cfg.Guard.WorkspaceDirs) {
		return true
	}
	return len(cfg.Exclude) > 0 && matchesRepo(repoNameFromPath(root), cfg.Exclude)
}

func isInsideWorkspaceDirs(repoPath string, dirs []string) bool {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return false
	}
	for _, d := range dirs {
		expanded := config.ExpandPath(d)
		expanded, err := filepath.Abs(expanded)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, expanded+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func runPostMerge(root string, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	repoName := repoNameFromPath(root)

	for _, h := range cfg.Hooks.PostMerge {
		if !h.IsEnabled() {
			continue
		}
		if !matchesRepo(repoName, h.Repos) {
			continue
		}
		if err := runShellCommand(h.Command, root); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-merge hook %s: %v\n", h.Name, err)
		}
	}
	return nil
}

func runBuiltinScan(root string, cfg *config.Config, skips *skipCollector) error {
	s := buildScanner(root, cfg)
	if skips != nil {
		s.OnSkip = skips.scannerSkip
	}

	findings, err := s.ScanStaged(root)
	if err != nil {
		return err
	}

	if len(findings) == 0 {
		return nil
	}

	printFindings(findings, root)
	notify.EmitEventSync("suspenders", "warn",
		fmt.Sprintf("commit blocked: %d secret finding(s)", len(findings)),
		"", map[string]string{"repo": repoNameFromPath(root), "check": "scan"})
	return scanner.ErrFindingsFound
}

func printGuardFindings(findings []guard.Finding) {
	red := color.New(color.FgRed, color.Bold)
	_, _ = red.Fprintf(os.Stderr, "Blocked:")
	fmt.Fprintf(os.Stderr, " staged changes reference internal repo names:\n")
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "  %s\n", f.Match)
	}
	fmt.Fprintf(
		os.Stderr,
		"\nThis is a public repo. Remove internal references before committing.\n",
	)
	fmt.Fprintf(os.Stderr, "To bypass: git commit --no-verify\n")
}

func runShellCommand(command, dir string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func repoNameFromPath(repoPath string) string {
	remote, err := readGitRemote(repoPath)
	if err != nil {
		parent := filepath.Base(filepath.Dir(repoPath))
		return parent + "/" + filepath.Base(repoPath)
	}
	name := repofind.ParseRemote(remote)
	if name == "" {
		parent := filepath.Base(filepath.Dir(repoPath))
		return parent + "/" + filepath.Base(repoPath)
	}
	return name
}

func readGitRemote(repoPath string) (string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func matchesRepo(repoName string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			continue
		}
		if g.Match(repoName) {
			return true
		}
	}
	return false
}

func anyFileMatches(files []string, patterns []string) bool {
	for _, f := range files {
		for _, p := range patterns {
			matched, err := filepath.Match(p, filepath.Base(f))
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

func eventNames(events []hook.Event) string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = string(e)
	}
	return strings.Join(names, ", ")
}

func resolveRepoPath(args []string) (string, error) {
	p := "."
	if len(args) > 0 {
		p = args[0]
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !repofind.IsRepo(abs) {
		return "", fmt.Errorf("%s is not a git repository", abs)
	}
	return abs, nil
}

func discoverRepos() ([]repofind.Repo, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	// An empty dirs list is only reachable when the config file spells it
	// out, and walking nothing used to end in "no repositories found",
	// which reads as "your directories are empty" rather than "there are no
	// directories".
	if len(cfg.Dirs) == 0 {
		path, pathErr := configFilePath()
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, fmt.Errorf("config sets no dirs — nothing to discover; list the directories to walk under dirs in %s", path)
	}

	expanded := make([]string, len(cfg.Dirs))
	for i, d := range cfg.Dirs {
		expanded[i] = config.ExpandPath(d)
	}

	repos, err := repofind.Find(expanded, cfg.Exclude)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found in configured directories")
	}
	return repos, nil
}

func forEachRepo(repos []repofind.Repo, action string, fn func(repofind.Repo) error) error {
	var errs int
	for _, r := range repos {
		if err := fn(r); err != nil {
			color.Red("  %s %s: %v", action, r.Name, err)
			errs++
		} else {
			color.Green("  %s %s", action, r.Name)
		}
	}
	if errs > 0 {
		return fmt.Errorf("%d error(s) occurred", errs)
	}
	return nil
}
