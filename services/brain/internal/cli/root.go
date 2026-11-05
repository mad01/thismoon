package cli

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
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
  write             Create a new note in a tier (inbox/notes/outputs/MOCs).
  edit              Edit body or frontmatter of an existing note.
  promote           Move an inbox note into notes/, rewriting backlinks.
  list              List notes in a tier, optionally filtered.
  mcp               Start the MCP stdio server for Claude Code.

Config lives at ~/.config/brain/config.yaml. Indexes live at ~/.config/brain/index/
(literal) and ~/.config/brain/semantic/ (when semantic.enabled=true).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}
