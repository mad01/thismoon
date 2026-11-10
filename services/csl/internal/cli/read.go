package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/repo/repopath"
)

var (
	readfileRepoFlag      string
	readfileStartLineFlag int
	readfileEndLineFlag   int
	readfileJSONFlag      bool
	readfileWorktree      worktreeFlags
)

var readfileCmd = &cobra.Command{
	Use:   "read <file>",
	Short: "Read a file from a repository",
	Long: `Read a file from a repository with line numbers.

The --repo flag names the repository the file belongs to. The file path is
relative to the repository root.

--worktree reads from a linked git worktree instead (--worktree . for the
one you are in), and replaces --repo. Inside a worktree, a --repo that
selects only its repo reads from the worktree too, and a stderr line says
so; --no-worktree reads the main checkout.

Examples:
  csl read internal/cli/root.go --repo thismoon
  csl read main.go --repo thismoon --start-line 10 --end-line 30
  csl read README.md --repo thismoon --json
  csl read README.md --worktree ~/.worktrees/thismoon/my-branch
  csl read README.md --worktree .`,
	Args: cobra.ExactArgs(1),
	RunE: runReadfile,
}

func init() {
	readfileCmd.Flags().
		StringVarP(&readfileRepoFlag, "repo", "r", "", "repository name to read from (required unless --worktree is set)")
	readfileCmd.Flags().
		IntVar(&readfileStartLineFlag, "start-line", 0, "first line to show (1-based, 0 = start of file)")
	readfileCmd.Flags().
		IntVar(&readfileEndLineFlag, "end-line", 0, "last line to show (1-based, 0 = end of file)")
	readfileCmd.Flags().BoolVar(&readfileJSONFlag, "json", false, "output as JSON")
	readfileWorktree.register(readfileCmd)
	rootCmd.AddCommand(readfileCmd)
}

// readTarget returns the checkout csl read reads from: the worktree the
// worktree flags choose, announced on w, else the first discovered repo
// whose name holds --repo.
func readTarget(cfg *config.Config, w io.Writer) (*finder.Repo, error) {
	choice, err := readfileWorktree.choose(cfg, readfileRepoFlag)
	if err != nil {
		return nil, err
	}
	if choice != nil {
		choice.announce(w)
		return &choice.repo, nil
	}
	if readfileRepoFlag == "" {
		return nil, errors.New(
			"--repo is required, or --worktree for a linked worktree (--worktree . for the one you are in)",
		)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if strings.Contains(r.Name, readfileRepoFlag) {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("no repo matching %q found", readfileRepoFlag)
}

type readfileLineJSON struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type readfileJSON struct {
	Repo  string             `json:"repo"`
	Path  string             `json:"path"`
	Lines []readfileLineJSON `json:"lines"`
}

func runReadfile(cmd *cobra.Command, args []string) error {
	relPath := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	matched, err := readTarget(cfg, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	absPath, err := repopath.Resolve(matched.Path, "file", relPath)
	if err != nil {
		return err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", absPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []readfileLineJSON
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if readfileStartLineFlag > 0 && lineNum < readfileStartLineFlag {
			continue
		}
		if readfileEndLineFlag > 0 && lineNum > readfileEndLineFlag {
			break
		}
		lines = append(lines, readfileLineJSON{Number: lineNum, Text: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", absPath, err)
	}

	w := cmd.OutOrStdout()

	if readfileJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(readfileJSON{
			Repo:  matched.Name,
			Path:  relPath,
			Lines: lines,
		})
	}

	for _, l := range lines {
		fmt.Fprintf(w, "%6d\t%s\n", l.Number, l.Text)
	}

	return nil
}
