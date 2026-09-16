package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/outline"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

var (
	outlineKindsFlag        []string
	outlineLimitFlag        int
	outlineIncludeTestsFlag bool
	outlineMaxFilesFlag     int
	outlineJSONFlag         bool
)

var outlineCmd = &cobra.Command{
	Use:   "outline <repo> [path]",
	Short: "Rank a repo's definitions by how many other files reference them",
	Long: `Outline a repo, or a directory inside it: every definition the
tree-sitter extractor finds (types, functions, methods, fields, headings),
ranked by how many other files in the repo mention the name as a whole
identifier. The important types and functions surface first, so one call
answers "how is this service structured".

The repo is a case-insensitive regex that must match exactly one discovered
repo. The optional path narrows which files contribute definitions;
references are always counted across the whole repo, so a package's public
API ranks by repo-wide use. Test files (_test.go, test_*.py, *.test.ts,
__tests__/, ...) stay out of both sides unless --include-tests is set, and
fields, enumerators, and markdown headings stay out unless --kinds names
them. A lowercase Go name counts references only inside its own directory,
and a name defined in several files shares its mentions among them, so
common names don't crowd the top.

Output groups the ranked symbols by file, one line per symbol:
LINE kind name (parent)  refs=N. The MCP tool csl_outline prints the same.

Examples:
  csl outline thismoon
  csl outline thismoon services/csl/internal
  csl outline thismoon --kinds struct,interface --limit 20
  csl outline thismoon docs --kinds section
  csl outline thismoon services/csl --json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runOutline,
}

func init() {
	outlineCmd.Flags().StringSliceVar(&outlineKindsFlag, "kinds", nil,
		"keep only these kinds (comma-separated; default all but field, enumerator, section): "+
			strings.Join(outline.Kinds(), ", "))
	outlineCmd.Flags().IntVar(&outlineLimitFlag, "limit", outline.DefaultLimit,
		fmt.Sprintf("maximum symbols to print (max %d)", outline.MaxLimit))
	outlineCmd.Flags().BoolVar(&outlineIncludeTestsFlag, "include-tests", false,
		"include test files in definitions and reference counts")
	outlineCmd.Flags().IntVar(&outlineMaxFilesFlag, "max-files", outline.DefaultMaxFiles,
		"stop the walk after this many files and mark the result truncated")
	outlineCmd.Flags().BoolVar(&outlineJSONFlag, "json", false, "output as JSON")
	rootCmd.AddCommand(outlineCmd)
}

func runOutline(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return errors.New(cfg.EmptyResultHint())
	}
	repo, err := finder.MatchOne(repos, args[0])
	if err != nil {
		return err
	}

	opts := outline.Options{
		Kinds:        outlineKindsFlag,
		Limit:        outlineLimitFlag,
		IncludeTests: outlineIncludeTestsFlag,
		MaxFiles:     outlineMaxFilesFlag,
	}
	if len(args) == 2 {
		opts.Path = args[1]
	}
	res, err := outline.Build(repo, opts)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if outlineJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	_, err = fmt.Fprintln(w, outline.Render(res))
	return err
}
