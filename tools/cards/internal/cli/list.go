package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/cards/internal/listview"
	"github.com/mad01/thismoon/tools/cards/internal/order"
	"github.com/mad01/thismoon/tools/cards/internal/render"
)

// listStyle is list mode's default card style: a blank row between cards.
const listStyle = render.CardSpace

func listCmd() *cobra.Command {
	var out string
	cfg := listview.Config{Style: listStyle}
	c := &cobra.Command{
		Use:   "list",
		Short: "Pick from cards piped to stdin: filter, mark, and press enter to act",
		Long: `Read card messages as JSON Lines on stdin and show them as a list.

Type to filter, space to mark, enter to run the enter action of the marked
cards (or of the cursor card when none is marked), esc to clear the marks and
then the filter, ctrl+c to quit. With --esc-quits, esc also quits once there
are no marks and no filter left to clear, as in a popup. Keys come from the
terminal, so stdin must be a pipe: producer | cards list.

A card with a new id goes at the bottom, or at the top with --newest top. A
card whose id is shown replaces it in place, and a snapshot keeps its own
order.

A run action starts its argv directly, with no shell. An emit action appends
{"action": NAME, "id": CARD_ID} as one JSON line to the --out file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.InOrStdin(), out, cfg)
		},
	}
	c.Flags().StringVar(&out, "out", "", "append emit actions to `FILE` as JSON lines")
	addCardStyleFlag(c, &cfg.Style)
	addNewestFlag(c, &cfg.Newest, "put a card with a new id at the `END` of the list")
	c.Flags().BoolVar(&cfg.EscQuits, "esc-quits", false,
		"quit on esc when there are no marks and no filter to clear")
	addWrapLastFlag(c, &cfg.WrapLast)
	return c
}

// addWrapLastFlag adds --wrap-last to c, setting wrap. It is off by default,
// so a card draws as it always has.
func addWrapLastFlag(c *cobra.Command, wrap *bool) {
	c.Flags().BoolVar(wrap, "wrap-last", false,
		"give a card's last line a second row when it is too wide, broken at a space")
}

// addCardStyleFlag adds --card-style to c, setting style, which holds the
// mode's default. An unknown style is a flag error, so a usage error.
func addCardStyleFlag(c *cobra.Command, style *render.CardStyle) {
	c.Flags().TextVar(style, "card-style", *style,
		"set cards apart with `STYLE`: "+strings.Join(render.CardStyleNames(), ", "))
}

// addNewestFlag adds --newest to c, setting newest, which holds the default.
// usage says what goes at the end the flag names. An unknown end is a flag
// error, so a usage error.
func addNewestFlag(c *cobra.Command, newest *order.Newest, usage string) {
	c.Flags().TextVar(newest, "newest", *newest, usage+": "+strings.Join(order.Names(), ", "))
}

// runList runs list mode on stdin, with the card style, the newest end,
// whether Esc quits and whether a long last line wraps in cfg, until the
// user quits.
func runList(stdin io.Reader, outPath string, cfg listview.Config) error {
	if err := checkPipe(stdin, "list"); err != nil {
		return err
	}
	cfg.Starter = listview.ProcessStarter{}
	if outPath != "" {
		f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("open --out: %w", err)
		}
		defer f.Close()
		cfg.Emitter = listview.NewJSONEmitter(f)
	}
	if err := runTUI(stdin, listview.New(cfg)); err != nil {
		return fmt.Errorf("list: %w", err)
	}
	return nil
}
