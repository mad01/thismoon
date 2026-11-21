// Package cli wires the cards commands: list mode and feed mode, plus the
// version, docs and doctor commands every thismoon component ships.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/tools/cards"
)

// ErrUsage marks a command called the wrong way, such as an unknown flag or
// nothing piped to stdin. main exits 2 on it, and 1 on any other error.
var ErrUsage = errors.New("usage")

// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "cards",
		Short:         "Show JSON Lines from stdin as cards in a terminal UI",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	})
	c.AddCommand(
		listCmd(),
		feedCmd(),
		versionCmd(),
		agentcli.DocsCommand(cards.OperatingDoc, cards.Facts()),
		doctorCmd(),
	)
	return c
}
