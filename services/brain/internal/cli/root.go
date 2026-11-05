// Package cli builds the brain command tree. Every command prints its result
// as text or, with -o json, as the same JSON document the matching MCP tool
// returns. Errors print once as "brain <cmd>: <message>" on stderr and map to
// a documented exit code (see errors.go).
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// app holds what every subcommand shares: the output format chosen with the
// persistent -o flag.
type app struct {
	output string
}

// Execute runs the brain CLI on os.Args and returns the process exit code.
func Execute() int {
	return run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// run executes one CLI invocation against the given streams and returns its
// exit code. Tests call it directly.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	a := &app{output: outputText}
	root := newRootCmd(a)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd, err := root.ExecuteC()
	if err == nil {
		return exitOK
	}
	if a.output == outputText && outputFromArgs(args) == outputJSON {
		a.output = outputJSON
	}
	return a.report(cmd, err, stdout, stderr)
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "brain",
		Short: "Personal knowledge brain CLI + MCP server",
		Long: `brain — a local CLI and MCP server over an Obsidian-style markdown vault.

Subcommands:
  index             Build or refresh the literal (zoekt) and optional semantic indexes.
  search            Literal/regex query via zoekt.
  semantic-search   Vector search via Ollama embeddings + chromem-go.
  hybrid-search     Fused literal + semantic via RRF (with optional LLM rerank).
  count             Count matches of a zoekt query.
  query             Validate a zoekt query.
  read              Read a note (optionally a slice of lines).
  peek              Frontmatter, first body lines and backlink count of a note.
  backlinks         Inbound and outbound wikilinks of a note.
  write             Create a new note in a tier (inbox/notes/outputs/MOCs).
  edit              Edit body or frontmatter of an existing note.
  promote           Move an inbox note into notes/, rewriting backlinks.
  list              List notes in a tier, optionally filtered.
  docs              Print the agent-facing CLI guide.
  doctor            Check config, vault, index and Ollama.
  mcp               Start the MCP stdio server for Claude Code.

Every command takes -o json for machine-readable output. Exit codes: 0 ok,
1 error, 2 usage, 3 not found, 4 already exists, 5 invalid query.

Config lives at ~/.config/brain/config.yaml. Indexes live at ~/.config/brain/index/
(literal) and ~/.config/brain/semantic/ (when semantic.enabled=true).`,
		Args:          rootArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if a.output != outputText && a.output != outputJSON {
				return usageErrorf("-o must be %s or %s, got %q", outputText, outputJSON, a.output)
			}
			return nil
		},
	}
	root.PersistentFlags().
		StringVarP(&a.output, "output", "o", outputText, "output format: text or json")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	root.AddCommand(
		newIndexCmd(),
		newSearchCmd(a),
		newSemanticSearchCmd(a),
		newHybridSearchCmd(a),
		newCountCmd(a),
		newQueryCmd(a),
		newReadCmd(a),
		newPeekCmd(a),
		newBacklinksCmd(a),
		newWriteCmd(a),
		newEditCmd(a),
		newPromoteCmd(a),
		newListCmd(a),
		newDocsCmd(),
		newDoctorCmd(a),
		newMCPCmd(),
		newVersionCmd(a),
	)
	return root
}

// rootArgs rejects an unknown subcommand as a usage error. Cobra would
// otherwise print help and exit 0, which hides the typo from scripts.
func rootArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q; see brain --help or brain docs", args[0])
	if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
		msg += "; did you mean " + strings.Join(s, " or ") + "?"
	}
	return usageErrorf("%s", msg)
}

// exactArgs is cobra.ExactArgs reported as a usage error naming the usage line.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usageErrorf(
				"want %d argument(s), got %d; usage: %s",
				n,
				len(args),
				cmd.UseLine(),
			)
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs reported as a usage error.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return usageErrorf(
				"want at most %d argument(s), got %d; usage: %s",
				n,
				len(args),
				cmd.UseLine(),
			)
		}
		return nil
	}
}
