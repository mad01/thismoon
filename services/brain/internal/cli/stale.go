package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/drift"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/stale"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newStaleCmd(a *app) *cobra.Command {
	var (
		withDrift, online bool
		limit             int
	)
	c := &cobra.Command{
		Use:   "stale",
		Short: "List the notes that need review, grouped by owner",
		Long: `List the notes that need a human look, computed now and never stored. A
note is in the queue when:

  expired    its review_by has passed, or, for a raw note, its expires_at
  disputed   someone flagged it (brain flag)
  dependent  a sources[] ref rests on a superseded, retracted or withdrawn note
  drift      with --drift: brain drift found its pinned sources changed

Only sources[] refs make a dependent; a wikilink is a mention, and a
dependent's own dependents are not queued. Revoked notes are never in it.
Notes are grouped by domain: the owners the vault's CODEOWNERS (or
.github/CODEOWNERS) gives the note's path, else its steward field, else
unowned. Each group shows its first --limit notes (10), oldest reason
first. --drift runs drift on every note, which is slow; add --online to
fetch external sources too. -o json prints {"total", "drift", "groups":
[{domain, from, total, notes: [{vault, path, title, status, reasons:
[{kind, detail, since}]}]}]}. Exits 0 whatever the queue holds.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit <= 0 {
				return usageErrorf("--limit must be > 0, got %d", limit)
			}
			if online && !withDrift {
				return usageErrorf("--online fetches sources for --drift; pass both")
			}
			lib, sn, notes, err := a.freshnessTargets(nil)
			if err != nil {
				return err
			}
			opts := stale.Options{Snapshot: sn, Notes: notes, Now: time.Now(), Limit: limit}
			if withDrift {
				checker := drift.New(driftOptions(lib, online))
				opts.Drift = func(n *vault.NoteView) []drift.Finding {
					return checker.Sources(
						context.Background(),
						lifecycle.Read(n.Frontmatter).Sources,
					)
				}
			}
			q, err := stale.Build(opts)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), q, func(w io.Writer) error {
				return a.writeStaleText(w, q)
			})
		},
	}
	c.Flags().
		BoolVar(&withDrift, "drift", false, "also queue notes whose pinned sources drifted (slow)")
	c.Flags().BoolVar(&online, "online", false, "with --drift, fetch external sources too")
	c.Flags().IntVar(&limit, "limit", stale.DefaultLimit, "notes shown per group")
	return c
}

func (a *app) writeStaleText(w io.Writer, q stale.Queue) error {
	if q.Total == 0 {
		fmt.Fprintln(w, "no notes need review")
		return nil
	}
	for _, g := range q.Groups {
		shown := ""
		if len(g.Notes) < g.Total {
			shown = fmt.Sprintf(", showing %d", len(g.Notes))
		}
		fmt.Fprintf(w, "%s (%s, %d note(s)%s)\n", g.Domain, g.From, g.Total, shown)
		for _, e := range g.Notes {
			reasons := make([]string, 0, len(e.Reasons))
			for _, r := range e.Reasons {
				reasons = append(reasons, r.Kind+": "+r.Detail)
			}
			fmt.Fprintf(w, "  %s  %s\n", a.show(e.Vault, e.Path), strings.Join(reasons, "; "))
		}
	}
	return nil
}
