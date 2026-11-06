package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// migrateResult is what brain migrate prints with -o json.
type migrateResult struct {
	DryRun  bool             `json:"dry_run"`
	Changed []vault.IDChange `json:"changed"`
	Skipped []vault.IDSkip   `json:"skipped,omitempty"`
}

func newMigrateCmd(a *app) *cobra.Command {
	var ids, dryRun bool
	c := &cobra.Command{
		Use:   "migrate --ids",
		Short: "Backfill note ids",
		Long: `Give every note that has no id a fresh one (a ULID in the id frontmatter
field). Nothing else in the note changes, updated included, and a second run
changes nothing. --dry-run prints the ids it would assign and writes nothing.
A shared vault is read-only: a run over every vault skips it, and naming it
with --vault exits 6 unless the run is a dry run. Note ids are what id:<ulid>
note refs name. With -o json, prints
{dry_run, changed: [{vault, path, id}], skipped: [{vault, path, reason}]}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !ids {
				return usageErrorf("migrate needs --ids, the one migration brain has")
			}
			res, err := a.migrateIDs(dryRun)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				a.writeMigrateText(w, res)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&ids, "ids", false, "give every note without an id a fresh one")
	c.Flags().
		BoolVar(&dryRun, "dry-run", false, "print the ids that would be assigned, write nothing")
	return c
}

// migrateIDs runs the id migration over the selected vaults. A shared vault
// the user did not name is skipped, dry run or not, so a dry run predicts the
// real one; a shared vault the user named refuses a real run with
// ErrReadOnly.
func (a *app) migrateIDs(dryRun bool) (migrateResult, error) {
	res := migrateResult{DryRun: dryRun, Changed: []vault.IDChange{}}
	_, set, err := a.openVaults()
	if err != nil {
		return res, err
	}
	for _, v := range set.Vaults() {
		if v.Shared && len(a.vaults) == 0 {
			res.Skipped = append(res.Skipped, vault.IDSkip{
				Vault:  v.Name,
				Reason: "shared vault: ids reach it through a pull request",
			})
			continue
		}
		report, err := v.MigrateIDs(dryRun, time.Now())
		res.Changed = append(res.Changed, report.Changed...)
		res.Skipped = append(res.Skipped, report.Skipped...)
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func (a *app) writeMigrateText(w io.Writer, res migrateResult) {
	verb := "assigned"
	if res.DryRun {
		verb = "would assign"
	}
	for _, c := range res.Changed {
		fmt.Fprintf(w, "%s %s to %s\n", verb, c.ID, a.show(c.Vault, c.Path))
	}
	for _, s := range res.Skipped {
		where := s.Vault
		if s.Path != "" {
			where = a.show(s.Vault, s.Path)
		}
		fmt.Fprintf(w, "skipped %s: %s\n", where, s.Reason)
	}
	fmt.Fprintf(w, "%s %d id(s)\n", verb, len(res.Changed))
}
