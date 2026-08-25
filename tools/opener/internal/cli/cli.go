// Package cli wires the opener commands onto the open command core.
package cli

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/tools/opener"
	"github.com/mad01/thismoon/tools/opener/internal/mcpserver"
	"github.com/mad01/thismoon/tools/opener/internal/sysopen"
)

// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "opener",
		Short:         "macOS open bridge (URLs, files, apps, Finder reveal), CLI + MCP",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(opener.OperatingDoc, opener.Facts()),
		urlCmd(),
		fileCmd(),
		appCmd(),
		withCmd(),
		revealCmd(),
		mcpCmd(),
	)
	return c
}

// report prints the resolved target the open command received, mirroring the
// MCP tools' opened field.
func report(cmd *cobra.Command, opened string, err error) error {
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "opened %s\n", opened)
	return nil
}

func urlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <url>",
		Short: "Open a URL with its default handler (the browser for https)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opened, err := sysopen.New().URL(args[0])
			return report(cmd, opened, err)
		},
	}
}

func fileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "file <path>",
		Short: "Open a file or directory with its default application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opened, err := sysopen.New().File(args[0])
			return report(cmd, opened, err)
		},
	}
}

func appCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "app <name>",
		Short: "Launch or foreground an application by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opened, err := sysopen.New().App(args[0])
			return report(cmd, opened, err)
		},
	}
}

func withCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "with <path> <app>",
		Short: "Open a file with a specific application instead of its default",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opened, err := sysopen.New().With(args[0], args[1])
			return report(cmd, opened, err)
		},
	}
}

func revealCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reveal <path>",
		Short: "Reveal a file or directory in Finder, selected",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opened, err := sysopen.New().Reveal(args[0])
			return report(cmd, opened, err)
		},
	}
}

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the opener MCP stdio server for agent hosts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.New(buildinfo.Get().Version).
				Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}
