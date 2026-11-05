package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/drift"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newDriftCmd(a *app) *cobra.Command {
	var online bool
	c := &cobra.Command{
		Use:   "drift [notes...]",
		Short: "Check the sources notes pin against what they are now",
		Long: `Check each note's sources[] against what they point at now. A code ref,
ref: repo:path@commit#symbol, is checked in the local checkout of its repo:
host/owner/name (owner/name means github.com) is looked up as
<root>/<host>/<owner>/<name> under each code_roots entry of the brain
config. Findings:

  moved           the path no longer exists at HEAD (moved_to when git sees a rename)
  changed         the path's content at HEAD differs from the pinned commit
  symbol-missing  #symbol no longer appears in the file at HEAD (a word match)
  gone            an external url answers 404 or 410 (archive names a copy)
  hash-changed    an external url no longer matches its content_hash
  unresolved      not checkable: repo not checked out, commit not fetched, fetch failed

External urls are fetched only with --online; by default drift is offline
and checks code refs alone. Each argument is a note ref; with none, every
note of the --vault vaults that is not revoked is checked. -o json prints
{"ok", "online", "checked", "drifted", "offline_skipped", "notes":
[{vault, path, findings: [{kind, source, message, moved_to, archive}]}]}.
Exits 0 when no note drifted (unresolved alone is not drift), 1 when one did.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, _, notes, err := a.freshnessTargets(args)
			if err != nil {
				return err
			}
			r := drift.Run(context.Background(), notes, driftOptions(lib, online))
			if err := a.emit(cmd.OutOrStdout(), r, func(w io.Writer) error {
				return a.writeDriftText(w, r)
			}); err != nil {
				return err
			}
			if !r.OK {
				return &reportedError{
					err: fmt.Errorf("%d of %d note(s) drifted", r.Drifted, r.Checked),
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&online, "online", false, "also fetch external urls and compare content_hash")
	return c
}

func driftOptions(lib *library.Library, online bool) drift.Options {
	return drift.Options{CodeRoots: lib.Config().CodeRoots, Online: online}
}

// freshnessTargets snapshots every vault and resolves the notes drift and
// stale look at: the notes args name, or every tier note of the --vault
// vaults that is not revoked.
func (a *app) freshnessTargets(
	args []string,
) (*library.Library, *vault.Snapshot, []*vault.NoteView, error) {
	lib, scope, err := a.openVaults()
	if err != nil {
		return nil, nil, nil, err
	}
	sn, err := lib.Set().Snapshot()
	if err != nil {
		return nil, nil, nil, err
	}
	var notes []*vault.NoteView
	if len(args) > 0 {
		for _, arg := range args {
			ref, err := lib.Locate(a.vaults, arg)
			if err != nil {
				return nil, nil, nil, withNoteHint(err, refPath(arg))
			}
			n, err := sn.Note(ref)
			if err != nil {
				return nil, nil, nil, err
			}
			notes = append(notes, n)
		}
		return lib, sn, notes, nil
	}
	in := map[*vault.Vault]bool{}
	for _, v := range scope.Vaults() {
		in[v] = true
	}
	for _, n := range sn.Notes() {
		if in[n.Ref.Vault] && n.Tier() != "" && !lifecycle.Revoked(n.Status()) {
			notes = append(notes, n)
		}
	}
	return lib, sn, notes, nil
}

func (a *app) writeDriftText(w io.Writer, r drift.Report) error {
	for _, n := range r.Notes {
		fmt.Fprintln(w, a.show(n.Vault, n.Path))
		for _, f := range n.Findings {
			fmt.Fprintf(w, "  %-14s %s: %s\n", f.Kind, f.Source, f.Message)
		}
	}
	fmt.Fprintf(w, "%d of %d note(s) drifted", r.Drifted, r.Checked)
	if r.Offline > 0 {
		fmt.Fprintf(w, "; %d external source(s) not fetched (pass --online)", r.Offline)
	}
	fmt.Fprintln(w)
	return nil
}
