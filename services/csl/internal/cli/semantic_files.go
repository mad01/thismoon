package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

var (
	semanticFilesSkippedFlag bool
	semanticFilesJSONFlag    bool
)

var semanticFilesCmd = &cobra.Command{
	Use:   "files [path]",
	Short: "List files the semantic indexer would embed under a path",
	Long: `Classify every tracked file under a path (default: the current directory)
with the exact rules 'csl index --semantic' applies, without embedding
anything. Use it to check what a repo contributes to the vector index and
why the rest is filtered out.

Examples:
  csl semantic files                     # indexable files under .
  csl semantic files ~/code/src/app     # ...under another checkout
  csl semantic files --skipped          # what gets filtered, and why
  csl semantic files --json             # full per-file decisions`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSemanticFiles,
}

func init() {
	semanticFilesCmd.Flags().
		BoolVar(&semanticFilesSkippedFlag, "skipped", false, "list skipped files with their skip reason instead")
	semanticFilesCmd.Flags().
		BoolVar(&semanticFilesJSONFlag, "json", false, "output every decision as JSON")
	semanticCmd.AddCommand(semanticFilesCmd)
}

func runSemanticFiles(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a directory", abs)
	}

	decisions, err := semantic.AuditRepoFiles(abs)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if semanticFilesJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(decisions)
	}

	indexed := 0
	for _, d := range decisions {
		if d.Index {
			indexed++
		}
		switch {
		case semanticFilesSkippedFlag && !d.Index:
			fmt.Fprintf(w, "%s — %s\n", d.Path, d.Reason)
		case !semanticFilesSkippedFlag && d.Index:
			fmt.Fprintln(w, d.Path)
		}
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%d of %d files would be indexed\n", indexed, len(decisions))
	return nil
}
