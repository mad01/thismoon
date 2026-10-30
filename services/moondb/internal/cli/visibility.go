package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// How long hide and show wait for serve to apply what they spooled, and
// how often they ask. serve drains on a file watch with a 1 s poll behind
// it, so the change normally shows on the first or second look.
const (
	visibilityWait = 5 * time.Second
	visibilityPoll = 50 * time.Millisecond
)

const visibilityLong = `%s takes session ids, or unique prefixes of them, as moondb sessions
prints them. A hidden session drops out of moondb sessions, the web page
and the moondb_sessions MCP tool, and out of the page's totals and repo
chips. Its records stay: moondb trace, scorecard, why and history still
reach it, and moondb sessions --all lists it. moondb sessions show puts it
back.

The change goes through the spool, as every record does, and serve applies
it. The command waits up to %s for serve to report the new state.`

var sessionsHideCmd = &cobra.Command{
	Use:   "hide <session-id>...",
	Short: "Hide sessions from the session listings",
	Long:  fmt.Sprintf(visibilityLong, "hide", visibilityWait),
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVisibility(cmd, args, true)
	},
}

var sessionsShowCmd = &cobra.Command{
	Use:   "show <session-id>...",
	Short: "Show hidden sessions in the session listings again",
	Long:  fmt.Sprintf(visibilityLong, "show", visibilityWait),
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVisibility(cmd, args, false)
	},
}

func init() {
	sessionsCmd.AddCommand(sessionsHideCmd, sessionsShowCmd)
}

func runVisibility(cmd *cobra.Command, ids []string, hidden bool) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	v := visibilityRun{
		client: client.New(apiURL()),
		spool:  &spool.Writer{Dir: filepath.Join(flagWorkdir, moondb.SpoolDirName)},
		out:    cmd.OutOrStdout(),
		wait:   visibilityWait,
	}
	return v.set(ctx, ids, hidden)
}

// visibilityRun is one hide or show: the serve client that resolves the
// ids and reports the result, the spool the request goes through, and how
// long to wait for serve to apply it.
type visibilityRun struct {
	client *client.Client
	spool  *spool.Writer
	out    io.Writer
	wait   time.Duration
}

// set resolves every id before it spools anything, so a typo changes no
// session. It then spools one visibility envelope per session and waits
// for serve to report each in the state asked for, printing one line per
// session.
func (v visibilityRun) set(ctx context.Context, ids []string, hidden bool) error {
	var full []string
	for _, id := range ids {
		f, err := resolveSession(ctx, v.client, id)
		if err != nil {
			return err
		}
		if !slices.Contains(full, f) {
			full = append(full, f)
		}
	}
	for _, id := range full {
		if err := v.spool.Write(visibilityEnvelope(id, hidden, time.Now()), nil); err != nil {
			return err
		}
	}
	verb := "shown"
	if hidden {
		verb = "hidden"
	}
	for _, id := range full {
		title, err := v.await(ctx, id, hidden)
		if err != nil {
			return err
		}
		fmt.Fprintf(v.out, "%s %s  %s\n", verb, short(id), title)
	}
	return nil
}

// await asks serve for the session until it reports hidden as asked, and
// returns the session's title. serve answered for the session when its id
// was resolved, so an error now is returned at once. After v.wait it gives
// up and says the request is still in the spool.
func (v visibilityRun) await(ctx context.Context, id string, hidden bool) (string, error) {
	deadline := time.Now().Add(v.wait)
	tick := time.NewTicker(visibilityPoll)
	defer tick.Stop()
	for {
		view, err := v.client.Session(ctx, id)
		if err != nil {
			return "", err
		}
		if view.Summary.Hidden == hidden {
			return view.Summary.Title, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("moondb: serve has not applied the change to session %s "+
				"after %s; it stays in the spool until serve drains it (moondb doctor)",
				short(id), v.wait)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
		}
	}
}

// visibilityEnvelope asks serve to hide or show session id. Its id is
// unique per request, so asking twice spools twice.
func visibilityEnvelope(id string, hidden bool, now time.Time) spool.Envelope {
	return spool.Envelope{
		Version:    spool.Version,
		Kind:       spool.KindVisibility,
		ID:         fmt.Sprintf("visibility:%s:%d", id, now.UnixNano()),
		At:         now,
		Session:    spool.SessionRef{ID: id},
		Visibility: &spool.Visibility{Hidden: hidden},
	}
}
