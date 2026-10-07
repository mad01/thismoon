package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/store"
)

var deckCmd = &cobra.Command{
	Use:   "deck <id> <start|stop|next|prev|goto> [slide]",
	Short: "Drive a page's slide deck in the open browser tab",
	Long: `Send a remote command to the slide deck of a page that is open in the
browser (its /p/<id>/deck view). start and stop toggle presenting: the chrome
hides and one slide fills the window. next and prev move one step: a reveal
item, a stepped chart's next series, a stepped diagram's next elements or
focus, or the next slide. goto jumps to the
1-based slide number given as the third argument, with every step taken. Every open
tab of the deck follows within a second.

The command is written into the page's directory in the workdir, so it
works with serve running or not; a tab opened later ignores commands sent
before it loaded.`,
	Args: cobra.RangeArgs(2, 3),
	RunE: runDeck,
}

func init() {
	rootCmd.AddCommand(deckCmd)
}

func runDeck(cmd *cobra.Command, args []string) error {
	id, action := args[0], args[1]
	slide, err := deckSlideArg(action, args[2:])
	if err != nil {
		return err
	}
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	sent, err := st.SendDeckCommand(context.Background(), id, action, slide)
	if err != nil {
		return err
	}
	if sent.Action == store.DeckGoto {
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"%s: %s seq %d slide %d\n",
			id,
			sent.Action,
			sent.Seq,
			sent.Slide,
		)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s seq %d\n", id, sent.Action, sent.Seq)
	return nil
}

// deckSlideArg reads the optional slide argument: goto needs it, every
// other action refuses it, so a stray third word cannot pass silently.
func deckSlideArg(action string, rest []string) (int, error) {
	if action != store.DeckGoto {
		if len(rest) > 0 {
			return 0, fmt.Errorf("%s takes no slide number", action)
		}
		return 0, nil
	}
	if len(rest) == 0 {
		return 0, errors.New("goto needs a slide number (1-based)")
	}
	slide, err := strconv.Atoi(rest[0])
	if err != nil {
		return 0, fmt.Errorf("slide number %q: want an integer", rest[0])
	}
	return slide, nil
}
