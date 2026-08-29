package commands

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/guard"
	"github.com/mad01/thismoon/tools/suspenders/internal/history"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

// commitMessagePath is the pseudo-path findings in commit messages carry.
const commitMessagePath = "(commit message)"

// tagMessagePath is the pseudo-path findings in annotated tag messages carry.
const tagMessagePath = "(tag message)"

var (
	historyBranch         string
	historyFailOnFindings bool
	historyReplace        []string
	historyReplaceFile    string
	historyReplaceMap     []string
	historyRedactFiles    []string
	historyYes            bool
	historyDryRun         bool
)

func init() {
	historyScanCmd.Flags().
		StringVar(&historyBranch, "branch", "", "ref to walk (default: origin/HEAD, then main, master, HEAD)")
	historyScanCmd.Flags().
		BoolVar(&historyFailOnFindings, "fail-on-findings", false, "exit with code 1 if findings are detected")

	historyCleanCmd.Flags().
		StringVar(&historyBranch, "branch", "", "ref to scan when collecting replacements automatically")
	historyCleanCmd.Flags().
		StringArrayVar(&historyReplace, "replace", nil, "string to remove from history (repeatable)")
	historyCleanCmd.Flags().
		StringVar(&historyReplaceFile, "replace-file", "", "file with one string to remove per line")
	historyCleanCmd.Flags().
		StringArrayVar(&historyReplaceMap, "replace-map", nil, "old=new replacement mapping (repeatable)")
	historyCleanCmd.Flags().
		StringArrayVar(&historyRedactFiles, "redact-file", nil, "path glob whose file content is replaced wholesale with "+history.RedactedPlaceholder+" (repeatable)")
	historyCleanCmd.Flags().
		BoolVar(&historyYes, "yes", false, "skip the interactive confirmation")
	historyCleanCmd.Flags().
		BoolVar(&historyDryRun, "dry-run", false, "report what would change without rewriting anything")

	historyCmd.AddCommand(historyScanCmd)
	historyCmd.AddCommand(historyCleanCmd)
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Deep checks and cleanup across full git history",
}

var historyScanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan every commit on the public branch for secrets and blocked names",
	Long: "Walks every commit reachable from the public branch (origin/HEAD by default) and runs " +
		"the built-in checks over each commit's added lines and commit message. Findings are " +
		"attributed to the commit that introduced them, even when later commits removed them.",
	Args: cobra.MaximumNArgs(1),
	RunE: runHistoryScan,
}

var historyCleanCmd = &cobra.Command{
	Use:   "clean [path]",
	Short: "Rewrite git history to remove flagged strings",
	Long: "Rewrites all local branches and tags, replacing flagged strings in file contents and " +
		"commit messages with " + history.RedactedPlaceholder + ". " +
		"Files matching --redact-file globs have their entire content replaced instead — " +
		"for files that are secrets wholesale, like key or token files. Without --replace or " +
		"--redact-file flags, the replacement set is collected by running the history scan first; " +
		"file-name findings (a committed id_rsa, .env, ...) become whole-file redactions. A backup " +
		"bundle is always written to .git/ before rewriting. Afterwards every commit hash changes: " +
		"remotes need a force-push and collaborators need fresh clones.",
	Args: cobra.MaximumNArgs(1),
	RunE: runHistoryClean,
}

// historyFindings aggregates scan results grouped per introducing commit.
type historyFindings struct {
	commits []history.Commit             // commits with findings, oldest first
	secrets map[string][]scanner.Finding // commit hash -> findings
	blocked map[string][]string          // commit hash -> blocked-name matches
}

func (h *historyFindings) empty() bool { return len(h.commits) == 0 }

func (h *historyFindings) forCommit(c history.Commit) {
	if _, ok := h.secrets[c.Hash]; !ok && h.blocked[c.Hash] == nil {
		h.commits = append(h.commits, c)
	}
}

func (h *historyFindings) addSecrets(c history.Commit, ff []scanner.Finding) {
	if len(ff) == 0 {
		return
	}
	h.forCommit(c)
	for i := range ff {
		ff[i].Commit = c.Hash
	}
	h.secrets[c.Hash] = append(h.secrets[c.Hash], ff...)
}

// addBlocked records blocked-name matches, deduplicated by exact string:
// the history rewrite replaces case-sensitively, so every casing that
// appears must be collected as its own replacement.
func (h *historyFindings) addBlocked(c history.Commit, matches []string) {
	if len(matches) == 0 {
		return
	}
	h.forCommit(c)
	seen := make(map[string]bool, len(h.blocked[c.Hash]))
	for _, m := range h.blocked[c.Hash] {
		seen[m] = true
	}
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			h.blocked[c.Hash] = append(h.blocked[c.Hash], m)
		}
	}
}

// scanHistory walks ref and runs the built-in checks over every commit's
// added lines, new file names, and commit message.
func scanHistory(root string, cfg *config.Config, ref string) (*historyFindings, error) {
	s := buildScanner(root, cfg)

	var matcher *guard.Matcher
	if cfg != nil && cfg.Guard.Enabled && !guardExempt(root, cfg) {
		var err error
		matcher, err = guard.New(cfg.Guard).NewMatcher()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: guard check skipped: %v\n", err)
		}
	}

	results := &historyFindings{
		secrets: make(map[string][]scanner.Finding),
		blocked: make(map[string][]string),
	}

	scanLines := func(c history.Commit, path string, firstLine int, lines []string) error {
		for i, line := range lines {
			ff, err := s.ScanLine(path, firstLine+i, line)
			if err != nil {
				return err
			}
			results.addSecrets(c, ff)
			if matcher != nil {
				results.addBlocked(c, matcher.Find(line))
			}
		}
		return nil
	}

	v := history.Visitor{
		OnMessage: func(c history.Commit, message string) error {
			path := commitMessagePath
			if c.IsTag {
				path = tagMessagePath
			}
			return scanLines(c, path, 1, strings.Split(message, "\n"))
		},
		OnAddedLine: func(c history.Commit, path string, lineNum int, line string) error {
			return scanLines(c, path, lineNum, []string{line})
		},
		OnNewFile: func(c history.Commit, path string) error {
			ff, err := s.CheckFileName(path)
			if err != nil {
				return err
			}
			results.addSecrets(c, ff)
			return nil
		},
	}
	if err := history.Walk(root, ref, v); err != nil {
		return nil, err
	}
	return results, nil
}

func runHistoryScan(cmd *cobra.Command, args []string) error {
	root, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ref, err := history.ResolveRef(root, historyBranch)
	if err != nil {
		return err
	}

	results, err := scanHistory(root, cfg, ref)
	if err != nil {
		return err
	}

	if results.empty() {
		color.Green("No findings in the history of %s.", ref)
		return nil
	}

	printHistoryFindings(results, ref)

	if historyFailOnFindings {
		return scanner.ErrFindingsFound
	}
	return nil
}

func printHistoryFindings(results *historyFindings, ref string) {
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)
	bold := color.New(color.Bold)

	total := 0
	for _, c := range results.commits {
		total += len(results.secrets[c.Hash]) + len(results.blocked[c.Hash])
	}
	fmt.Printf("\nFound %d finding(s) in %d commit(s) on %s:\n\n", total, len(results.commits), ref)

	for _, c := range results.commits {
		_, _ = bold.Printf(
			"commit %s %q — %s <%s>\n",
			c.Hash[:12],
			c.Subject,
			c.AuthorName,
			c.AuthorEmail,
		)
		for _, f := range results.secrets[c.Hash] {
			_, _ = red.Printf("  [%s] %s (%s)\n", f.Rule.Severity, f.Rule.Description, f.Rule.ID)
			if f.Line == 0 {
				_, _ = cyan.Printf("    %s\n", f.File)
			} else {
				_, _ = cyan.Printf("    %s:%d:%d\n", f.File, f.Line, f.Column)
			}
			_, _ = yellow.Printf("    %s\n", f.Context)
		}
		for _, m := range results.blocked[c.Hash] {
			_, _ = red.Printf("  [guard] blocked name reference\n")
			_, _ = yellow.Printf("    %s\n", m)
		}
		fmt.Println()
	}
}

func runHistoryClean(cmd *cobra.Command, args []string) error {
	root, err := resolveRepoPath(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	rw, err := buildRewriter(root, cfg)
	if err != nil {
		return err
	}
	if len(rw.Replacements) == 0 && len(rw.ReplaceTable) == 0 && len(rw.RedactPaths) == 0 {
		color.Green("Nothing to rewrite: no replacements or redactions collected.")
		return nil
	}

	if historyDryRun {
		result, err := history.DryRun(root, rw)
		if err != nil {
			return err
		}
		printCleanStats(result.Stats)
		color.Yellow("Dry run: nothing was rewritten.")
		return nil
	}

	if err := history.CheckPreconditions(root); err != nil {
		return err
	}
	refs, err := history.LocalRefs(root)
	if err != nil {
		return err
	}
	if !historyYes && !confirmClean(rw, refs) {
		color.Yellow("Aborted; nothing was rewritten.")
		return nil
	}

	result, err := history.Clean(root, rw)
	if result.BackupPath != "" {
		fmt.Printf("Backup bundle: %s\n", result.BackupPath)
	}
	if err != nil {
		return err
	}

	printCleanStats(result.Stats)
	color.Green("Rewrote %d ref(s).", len(result.Refs))
	if result.BackupPath != "" {
		color.Yellow(
			"warning: the backup bundle at %s is an unredacted copy of the removed strings;\n"+
				"         delete or move it once you have verified the rewrite.",
			result.BackupPath,
		)
	}
	fmt.Println("\nEvery commit hash has changed. Follow-up:")
	fmt.Println("  - force-push rewritten branches: git push --force-with-lease origin <branch>")
	fmt.Println(
		"  - collaborators must re-clone (or hard-reset) — old clones still hold the removed strings",
	)
	fmt.Println(
		"  - drop old objects locally: git reflog expire --expire=now --all && git gc --prune=now",
	)
	fmt.Printf("  - to undo instead: git clone %s\n", result.BackupPath)
	fmt.Println("  - then delete the backup bundle above so it stops holding the removed strings")
	return nil
}

// buildRewriter assembles the replacement set from flags, or from a history
// scan when no --replace flags were given, plus the replace table (from
// --replace-map flags and config history.replace_table) and the redact list
// (from --redact-file flags and config history.redact_files).
func buildRewriter(root string, cfg *config.Config) (*history.Rewriter, error) {
	rw := &history.Rewriter{
		ReplaceTable:   make(map[string]string),
		ProtectedPaths: history.DefaultProtectedPaths,
	}

	seenRedact := make(map[string]bool)
	addRedact := func(p string) {
		if p != "" && !seenRedact[p] {
			seenRedact[p] = true
			rw.RedactPaths = append(rw.RedactPaths, p)
		}
	}
	if cfg != nil {
		for _, p := range cfg.History.RedactFiles {
			addRedact(p)
		}
	}
	for _, p := range historyRedactFiles {
		addRedact(p)
	}

	if cfg != nil {
		maps.Copy(rw.ReplaceTable, cfg.History.ReplaceTable)
	}
	for _, m := range historyReplaceMap {
		old, repl, ok := strings.Cut(m, "=")
		if !ok || old == "" {
			return nil, fmt.Errorf(
				"invalid --replace-map %q: expected old=new",
				m,
			)
		}
		rw.ReplaceTable[old] = repl
	}

	seen := make(map[string]bool)
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			rw.Replacements = append(rw.Replacements, s)
		}
	}

	for _, s := range historyReplace {
		add(s)
	}
	if historyReplaceFile != "" {
		content, err := os.ReadFile(historyReplaceFile)
		if err != nil {
			return nil, fmt.Errorf("read replace file: %w", err)
		}
		for line := range strings.SplitSeq(string(content), "\n") {
			add(strings.TrimRight(line, "\r"))
		}
	}
	hasExplicit := len(rw.Replacements) > 0 || len(rw.ReplaceTable) > 0 || len(rw.RedactPaths) > 0
	if hasExplicit {
		return rw, nil
	}

	// No explicit replacements: collect them by scanning history.
	ref, err := history.ResolveRef(root, historyBranch)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Collecting replacements from a history scan of %s...\n", ref)
	results, err := scanHistory(root, cfg, ref)
	if err != nil {
		return nil, err
	}
	for _, c := range results.commits {
		for _, f := range results.secrets[c.Hash] {
			if f.Line == 0 {
				// File-name findings (e.g. a committed id_rsa) have no string
				// to replace; redact the whole file content instead.
				addRedact(f.File)
				continue
			}
			add(f.RawMatch)
		}
		for _, m := range results.blocked[c.Hash] {
			add(m)
		}
	}
	return rw, nil
}

func confirmClean(rw *history.Rewriter, refs []string) bool {
	fmt.Println("\nAbout to rewrite git history. This changes every commit hash from the")
	fmt.Println("first affected commit onward and cannot be pushed without force.")
	fmt.Printf("\nRefs to rewrite (%d):\n", len(refs))
	for _, r := range refs {
		fmt.Printf("  %s\n", r)
	}
	if len(rw.ReplaceTable) > 0 {
		tableKeys := make([]string, 0, len(rw.ReplaceTable))
		for k := range rw.ReplaceTable {
			tableKeys = append(tableKeys, k)
		}
		sort.Strings(tableKeys)
		fmt.Printf("\nMapped replacements (%d):\n", len(rw.ReplaceTable))
		for _, k := range tableKeys {
			fmt.Printf("  %s -> %s\n", scanner.Redact(k), rw.ReplaceTable[k])
		}
	}
	if len(rw.Replacements) > 0 {
		fmt.Printf(
			"\nStrings to replace with %s (%d):\n",
			history.RedactedPlaceholder,
			len(rw.Replacements),
		)
		for _, s := range rw.Replacements {
			fmt.Printf("  %s\n", scanner.Redact(s))
		}
	}
	if len(rw.RedactPaths) > 0 {
		fmt.Printf(
			"\nFiles whose entire content becomes %s (%d):\n",
			history.RedactedPlaceholder,
			len(rw.RedactPaths),
		)
		for _, p := range rw.RedactPaths {
			fmt.Printf("  %s\n", p)
		}
	}
	fmt.Print("\nProceed? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func printCleanStats(stats history.Stats) {
	fmt.Printf("Replacements: %d\n", stats.Total())
	keys := make([]string, 0, len(stats.Replacements))
	for k := range stats.Replacements {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-40s %d occurrence(s)\n", scanner.Redact(k), stats.Replacements[k])
	}
	if stats.BlobsRedacted > 0 {
		fmt.Printf("File blobs redacted wholesale: %d\n", stats.BlobsRedacted)
	}
	if stats.SignaturesDropped > 0 {
		fmt.Printf("Stale commit signatures dropped: %d\n", stats.SignaturesDropped)
	}
	if stats.ProtectedSkips > 0 {
		fmt.Printf(
			"Protected file blobs skipped: %d (go.sum, lockfiles, etc.)\n",
			stats.ProtectedSkips,
		)
	}
	if stats.BinaryHits > 0 {
		color.Yellow(
			"warning: %d binary blob(s) contain a flagged string and were left untouched",
			stats.BinaryHits,
		)
	}
}
