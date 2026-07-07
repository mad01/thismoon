package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/guard"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

// skipCollector accumulates files the scanner or guard skipped (too large or
// unreadable) so they surface as one aggregated note rather than one stderr
// line each. Safe for concurrent use: ScanDir walks files in parallel.
type skipCollector struct {
	mu    sync.Mutex
	files []string
}

func (c *skipCollector) scannerSkip(sk scanner.SkippedFile) { c.add(sk.Path, sk.Reason) }
func (c *skipCollector) guardSkip(sk guard.SkippedFile)     { c.add(sk.Path, sk.Reason) }

func (c *skipCollector) add(path, reason string) {
	c.mu.Lock()
	c.files = append(c.files, fmt.Sprintf("%s (%s)", path, reason))
	c.mu.Unlock()
}

// report prints one aggregated note to stderr if any files were skipped.
func (c *skipCollector) report() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.files) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "skipped %d file(s) (too large/unreadable): %s\n",
		len(c.files), strings.Join(c.files, ", "))
}

var (
	scanStaged         bool
	scanFailOnFindings bool
)

func init() {
	scanCmd.Flags().
		BoolVar(&scanStaged, "staged", false, "scan only git-staged files (for pre-commit hook use)")
	scanCmd.Flags().
		BoolVar(&scanFailOnFindings, "fail-on-findings", false, "exit with code 1 if findings are detected")
	rootCmd.AddCommand(scanCmd)
}

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan a directory or repository for secrets",
	Long:  "Scan files for checked-in tokens, passwords, API keys, private keys, and other secrets. Defaults to the current directory.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

func runScan(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// config.Load returns defaults (nil error) when no config file exists, and
	// an error only when the file exists but can't be read or parsed. Surface
	// that error instead of silently scanning without the configured guard,
	// watch rules, and allowlist.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s := buildScanner(root, cfg)

	skips := &skipCollector{}
	s.OnSkip = skips.scannerSkip

	var findings []scanner.Finding
	if scanStaged {
		findings, err = s.ScanStaged(root)
	} else {
		findings, err = s.ScanDir(root)
	}
	if err != nil {
		return err
	}

	blocked, err := checkBlockedNames(root, cfg, skips)
	if err != nil {
		return err
	}

	skips.report()

	if len(findings) == 0 && len(blocked) == 0 {
		color.Green("No findings.")
		return nil
	}

	if len(findings) > 0 {
		printFindings(findings, root)
	}
	if len(blocked) > 0 {
		printBlockedNames(blocked)
	}

	if scanFailOnFindings {
		return scanner.ErrFindingsFound
	}
	return nil
}

// checkBlockedNames runs the guard over the scan target when the guard is
// enabled and root is a public repo (outside workspace dirs). Staged scans
// use the staged-diff check so `scan --staged` matches the pre-commit hook;
// full scans check every tracked file in the working tree.
func checkBlockedNames(root string, cfg *config.Config, skips *skipCollector) ([]guard.Finding, error) {
	if cfg == nil || !cfg.Guard.Enabled || isInsideWorkspaceDirs(root, cfg.Guard.WorkspaceDirs) {
		return nil, nil
	}
	g := guard.New(cfg.Guard)
	if skips != nil {
		g.OnSkip = skips.guardSkip
	}
	if scanStaged {
		return g.Check(root)
	}
	return g.CheckDir(root)
}

func printBlockedNames(findings []guard.Finding) {
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)

	fmt.Printf("\nFound %d blocked name reference(s):\n\n", len(findings))

	for _, f := range findings {
		_, _ = red.Printf("  [guard] blocked name reference\n")
		if f.File != "" {
			_, _ = cyan.Printf("    %s:%d\n", f.File, f.Line)
		}
		_, _ = yellow.Printf("    %s\n\n", f.Match)
	}
}

// buildScanner constructs a Scanner from the default rules plus any watch
// rules, exclude-rules filter, and allowlist in cfg, and wires the per-repo
// .suspenders.yaml ignore file under root when present. Both `scan` and the
// pre-commit hook build through here so the two paths detect identically.
func buildScanner(root string, cfg *config.Config) *scanner.Scanner {
	rules := append([]scanner.Rule{}, scanner.DefaultRules...)
	if cfg != nil {
		for _, w := range cfg.Watch {
			r, err := scanner.NewWatchRule(w.ID, w.Description, w.Pattern, w.Severity)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping watch rule %q: %v\n", w.ID, err)
				continue
			}
			rules = append(rules, r)
		}
		rules = applyExcludeRules(rules, cfg.Scan.ExcludeRules)
	}

	s := scanner.New(rules)

	if cfg != nil {
		for _, a := range cfg.Allowlist {
			if err := s.AddAllowance(a.Match, a.Description, a.Paths); err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping allowlist entry %q: %v\n", a.Match, err)
			}
		}
	}

	ignorePath := filepath.Join(root, ".suspenders.yaml")
	if _, err := os.Stat(ignorePath); err == nil {
		s.IgnoreFile = ignorePath
	}
	return s
}

// applyExcludeRules drops rules whose ID appears in excludeIDs.
func applyExcludeRules(rules []scanner.Rule, excludeIDs []string) []scanner.Rule {
	if len(excludeIDs) == 0 {
		return rules
	}
	excluded := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}
	filtered := rules[:0]
	for _, r := range rules {
		if !excluded[r.ID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func printFindings(findings []scanner.Finding, root string) {
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)

	fmt.Printf("\nFound %d potential secret(s):\n\n", len(findings))

	for _, f := range findings {
		rel, err := filepath.Rel(root, f.File)
		if err != nil {
			rel = f.File
		}
		_, _ = red.Printf("  [%s] %s (%s)\n", f.Rule.Severity, f.Rule.Description, f.Rule.ID)
		if f.Line == 0 {
			_, _ = cyan.Printf("    %s\n", rel)
		} else {
			_, _ = cyan.Printf("    %s:%d:%d\n", rel, f.Line, f.Column)
		}
		_, _ = yellow.Printf("    %s\n\n", f.Context)
	}
}
