package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/cards/internal/feedstore"
	"github.com/mad01/thismoon/tools/cards/internal/feedview"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// feedStyle is feed mode's default card style: entries dense.
const feedStyle = render.CardNone

func feedCmd() *cobra.Command {
	cfg := feedview.Config{Style: feedStyle}
	c := &cobra.Command{
		Use:   "feed",
		Short: "Follow cards piped to stdin as a log, newest at the bottom",
		Long: `Read card messages as JSON Lines on stdin and show them as a log of
entries, newest at the bottom (or at the top with --newest top), each with
its time: the card's at in local time, or the time its line arrived.

A card whose id is already in the feed replaces that entry in place, so a
producer that sends an event again does not show it twice. A card with no
id always adds an entry. Only the newest --max entries are kept.

The feed follows new entries. Up and down scroll one entry and page up and
page down one screen, and each stops following. With the newest at the
bottom, home goes to the oldest entry and end follows again; with --newest
top, end goes to the oldest and home follows again. Type to filter, esc to
clear the filter, ctrl+c to quit. Keys come from the terminal, so stdin must
be a pipe: producer | cards feed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFeed(cmd.InOrStdin(), cfg)
		},
	}
	c.Flags().IntVar(&cfg.Max, "max", feedstore.DefaultMax, "keep the newest `N` entries")
	addCardStyleFlag(c, &cfg.Style)
	addNewestFlag(c, &cfg.Newest, "show the newest entry at the `END` of the feed")
	addWrapLastFlag(c, &cfg.WrapLast)
	return c
}

// runFeed runs feed mode on stdin, with the entry limit, the card style, the
// newest end and whether a long last line wraps in cfg, until the user quits.
func runFeed(stdin io.Reader, cfg feedview.Config) error {
	if cfg.Max < 1 {
		return fmt.Errorf("%w: --max must be at least 1, got %d", ErrUsage, cfg.Max)
	}
	if err := checkPipe(stdin, "feed"); err != nil {
		return err
	}
	model := feedview.New(cfg)
	if err := runTUI(stdin, model); err != nil {
		return fmt.Errorf("feed: %w", err)
	}
	return nil
}
