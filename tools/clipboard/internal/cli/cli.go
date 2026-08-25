// Package cli wires the clipboard commands onto the pasteboard.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/tools/clipboard"
	"github.com/mad01/thismoon/tools/clipboard/internal/clip"
	"github.com/mad01/thismoon/tools/clipboard/internal/mcpserver"
)

// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "clipboard",
		Short:         "macOS system clipboard bridge (pbcopy/pbpaste), CLI + MCP",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(clipboard.OperatingDoc, clipboard.Facts()),
		copyCmd(),
		pasteCmd(),
		mcpCmd(),
	)
	return c
}

func copyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "copy [text]",
		Short: "Write text (or stdin when no argument) to the clipboard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if len(args) == 1 {
				text = args[0]
			} else {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				text = string(b)
			}
			if text == "" {
				return fmt.Errorf("nothing to copy: pass text or pipe it on stdin")
			}
			if err := clip.New().Copy(text); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "copied %d bytes\n", len(text))
			return nil
		},
	}
}

func pasteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paste",
		Short: "Print the current clipboard contents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := clip.New().Paste()
			if err != nil {
				return err
			}
			// Verbatim, exactly as pbpaste prints it: no newline appended.
			fmt.Fprint(cmd.OutOrStdout(), text)
			return nil
		},
	}
}

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the clipboard MCP stdio server for agent hosts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.New(buildinfo.Get().Version).
				Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}
