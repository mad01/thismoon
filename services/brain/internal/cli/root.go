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

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/brain"
)

// app holds what every subcommand shares: the output format chosen with the
// persistent -o flag and the config file chosen with --config.
type app struct {
	output     string
	configPath string
	// vaults are the --vault names; empty means every vault (or the default
	// vault, for write).
	vaults []string
	// multiVault is set once the config is loaded: text output then
	// qualifies paths as vault:path.
	multiVault bool
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

Every command takes -o json for machine-readable output and --vault <name>
to restrict it to configured vaults. A note is named by vault:path or by a
bare path found in one vault. Exit codes: 0 ok, 1 error, 2 usage (and an
ambiguous note), 3 not found, 4 already exists, 5 invalid query, 6 read-only
vault.

The config file is --config, else $BRAIN_CONFIG, else config.yaml under
$XDG_CONFIG_HOME/brain or ~/.config/brain. Indexes live under
$XDG_STATE_HOME/brain or ~/.local/state/brain (index/ and semantic/); an
install that already indexed under ~/.config/brain keeps using it.
brain doctor prints the paths in effect.`,
		Args:          rootArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if a.output != outputText && a.output != outputJSON {
				return usageErrorf("-o must be %s or %s, got %q", outputText, outputJSON, a.output)
			}
			// Env values reach Go without shell expansion, so expand ~ here.
			var err error
			a.configPath, err = confdir.Expand(a.configPath)
			return err
		},
	}
	root.PersistentFlags().
		StringVarP(&a.output, "output", "o", outputText, "output format: text or json")
	root.PersistentFlags().StringVar(&a.configPath, "config",
		envdefault.String(brain.ConfigEnv, defaultConfigPath()),
		"config file (env "+brain.ConfigEnv+")")
	root.PersistentFlags().StringArrayVar(&a.vaults, "vault", nil,
		"restrict to this vault (repeatable); write targets it instead of the default vault")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	root.AddCommand(
		newIndexCmd(a),
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
		newMCPCmd(a),
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

// defaultConfigPath resolves config.yaml in brain's config directory. When the
// home directory cannot be resolved it returns the ~-prefixed form, so the
// failure surfaces in PersistentPreRunE, which can return an error.
func defaultConfigPath() string {
	if path, err := brain.DefaultConfigPath(); err == nil {
		return path
	}
	return "~/.config/" + brain.Component + "/" + brain.ConfigFile
}
