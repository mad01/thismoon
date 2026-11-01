package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/ask"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/mcpserver"
)

var (
	askQueryFlags    []string
	askRepoFlag      string
	askLangFlag      string
	askFileFlag      string
	askMaxFilesFlag  int
	askMaxLinesFlag  int
	askPerRepoFlag   bool
	askFilesFromFlag string
	askJSONFlag      bool
)

var askCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Answer a question over the files a search selects, with a hosted worker model",
	Long: `Answer a question over the files a zoekt search selects. A cheap hosted
worker model reads the files whole and answers, so the files never reach the
caller: the answer prints on stdout and one footer line on stderr, giving the
corpus size, the backend and model, and the list price. It is the CLI form of
the csl_ask MCP tool and builds the same corpus.

Use it for a question over many files, or over one large file, when only the
answer is needed. For lines you will edit, read them with 'csl read' instead.

Selecting the files:
  -q <query>          a zoekt query (csl search syntax); repeat for up to 10.
                      The corpus is the union of the matching files in rank
                      order, taking turns across queries. --repo, --lang and
                      --file narrow every query.
  --files-from -      read org/repo/path lines from stdin instead (the format
                      csl search prints), or from a file named in place of -.
                      Takes no -q, --repo, --lang or --file.

Each file is read whole. The corpus stops at --max-files (default 20, max 100)
and --max-lines (default 6000, max 20000); --json lists the files that did not
fit under corpus.skipped. --per-repo asks once per repo and prints the answers
under an org/repo heading each.

The worker comes from the ask keys of config.yaml (see 'csl config --help').
With no ask.backend set it is a LiteLLM proxy when LITELLM_BASE_URL is set,
else the claude CLI on PATH, else OpenRouter when OPENROUTER_API_KEY is set.
'csl doctor' shows which one resolves (ask-backend).

Examples:
  csl ask "which flags does csl search accept" -q 'sym:searchCmd' --repo thismoon
  csl ask "how does each service set its port" -q 'ResolvedPort' -q 'DefaultPort' --per-repo
  csl search 'retry' --file '\.go$' --limit 5000 | csl ask "how is backoff computed" --files-from -`,
	Args: cobra.ExactArgs(1),
	RunE: runAsk,
}

func init() {
	f := askCmd.Flags()
	f.StringArrayVarP(&askQueryFlags, "query", "q", nil,
		"zoekt query selecting files; repeat for up to 10")
	f.StringVarP(&askRepoFlag, "repo", "r", "",
		"restrict every query to repo names matching this case-insensitive regex")
	f.StringVarP(&askLangFlag, "lang", "l", "", "restrict every query to files of this language")
	f.StringVarP(&askFileFlag, "file", "f", "",
		"restrict every query to file paths matching this regex")
	f.IntVar(&askMaxFilesFlag, "max-files", ask.DefaultMaxFiles,
		fmt.Sprintf("most files the worker reads (max %d)", ask.MaxFilesCeiling))
	f.IntVar(&askMaxLinesFlag, "max-lines", ask.DefaultMaxLines,
		fmt.Sprintf("most lines across those files (max %d)", ask.MaxLinesCeiling))
	f.BoolVar(&askPerRepoFlag, "per-repo", false,
		"ask once per repo and print the answers under an org/repo heading each")
	f.StringVar(&askFilesFromFlag, "files-from", "",
		"read org/repo/path lines from this file, or from stdin for -")
	f.BoolVar(&askJSONFlag, "json", false, "output the answer, corpus and usage as JSON")
	askCmd.MarkFlagsOneRequired("query", "files-from")
	for _, filter := range []string{"query", "repo", "lang", "file"} {
		askCmd.MarkFlagsMutuallyExclusive("files-from", filter)
	}
	rootCmd.AddCommand(askCmd)
}

// askSurface points the malformed-query fix at -q and the zero-result notes
// at the CLI's own commands.
var askSurface = lexical.Surface{
	QueryArg:   "the -q query",
	RepoLookup: cliSurface.RepoLookup,
	Reindex:    cliSurface.Reindex,
}

func runAsk(cmd *cobra.Command, args []string) error {
	req := mcpserver.AskRequest{
		Question: args[0],
		Queries:  askQueryFlags,
		Repo:     askRepoFlag,
		Lang:     askLangFlag,
		File:     askFileFlag,
		Limits:   ask.Limits{MaxFiles: askMaxFilesFlag, MaxLines: askMaxLinesFlag},
		PerRepo:  askPerRepoFlag,
		Surface:  askSurface,
	}
	if askFilesFromFlag != "" {
		lines, err := readFileList(cmd.InOrStdin(), askFilesFromFlag)
		if err != nil {
			return err
		}
		req.FileList = lines
	}

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	// The claude CLI backend runs as a child process; a Ctrl-C cancels the
	// context, which kills it instead of leaving it running.
	ctx, stop := interruptContext(parent)
	defer stop()

	res, err := mcpserver.Ask(ctx, req)
	if err != nil {
		return err
	}
	if askJSONFlag {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Fprintln(cmd.OutOrStdout(), res.Answer)
	fmt.Fprintln(cmd.ErrOrStderr(), res.Footer())
	return nil
}

// readFileList reads the --files-from lines: stdin for "-", else the named
// file. Reading nothing at all is an error here, before any setup runs.
func readFileList(stdin io.Reader, from string) ([]string, error) {
	r := stdin
	if from != "-" {
		f, err := os.Open(from)
		if err != nil {
			return nil, fmt.Errorf("--files-from: %w", err)
		}
		defer f.Close()
		r = f
	}
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("--files-from: %w", err)
	}
	if len(lines) == 0 {
		return nil, errors.New(
			"--files-from read no lines; pipe in csl search output or name a file",
		)
	}
	return lines, nil
}
