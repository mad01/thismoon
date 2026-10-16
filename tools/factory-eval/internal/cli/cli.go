// Package cli wires the factory-eval commands: run (the matrix, or a dry
// run of it), measure (the deviation table that picks the cases), docs,
// and version.
package cli

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	factoryeval "github.com/mad01/thismoon/tools/factory-eval"
)

// Execute runs the root command. An interrupt cancels the batch after the
// run in flight, and the results written so far still land on disk.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return root().ExecuteContext(ctx)
}

func root() *cobra.Command {
	c := &cobra.Command{
		Use:   "factory-eval",
		Short: "Experimental: the factory against one agent at equal budget, on the docs-sweep pilot",
		Long: `factory-eval runs the MAD-402 pilot unattended. Two arms work the same docs-sweep
cases under the same dollar cap: one claude session doing every case in turn,
and one orchestrator session running the factory skill with a worker per case.
Each run works in a throwaway clone at the pinned commit. Deterministic checks
and one LLM judge score every case, and the pass rule compares the arms.

Experimental, like the factory itself.`,
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(factoryeval.OperatingDoc, factoryeval.Facts()),
		runCmd(),
		measureCmd(),
	)
	return c
}

// expandHome resolves a leading ~ against the home directory.
func expandHome(p string) string {
	rest, ok := strings.CutPrefix(p, "~")
	if !ok || (rest != "" && rest[0] != '/') {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, rest)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
