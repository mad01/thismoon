package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newSupersedeCmd(a *app) *cobra.Command {
	var by string
	c := &cobra.Command{
		Use:   "supersede <note> --by <note>",
		Short: "Mark a note superseded by another",
		Long: `Mark a note superseded: status becomes superseded, superseded_by names the
replacement as id:<ulid> (so the pointer survives a rename) and status_at
records the day. The replacement may live in any configured vault and must
have an id (brain migrate --ids gives one). Default search hides superseded
notes; read shows a banner naming the replacement. A shared vault refuses
with exit code 6. With -o json, prints {"vault", "path", "sha"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(by) == "" {
				return usageErrorf("--by <note> is required")
			}
			lib, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			byRef, err := replacementRef(lib, ref, by)
			if err != nil {
				return err
			}
			sha, err := ref.Vault.Supersede(ref.Abs(), byRef, time.Now())
			return a.emitRevoked(cmd, ref, sha, err)
		},
	}
	c.Flags().
		StringVar(&by, "by", "", "the note that replaces it (vault:path, id:<ulid> or a bare path)")
	return c
}

// replacementRef resolves the --by note in every vault and returns it as
// the id ref superseded_by stores.
func replacementRef(lib *library.Library, target vault.Ref, by string) (string, error) {
	repl, err := lib.Locate(nil, by)
	if err != nil {
		return "", withNoteHint(err, refPath(by))
	}
	if repl.String() == target.String() {
		return "", usageErrorf("a note cannot supersede itself: %s", repl)
	}
	fm, _, err := repl.Vault.ReadFrontmatter(repl.Abs())
	if err != nil {
		return "", err
	}
	if fm.ID == "" {
		return "", usageErrorf(
			"--by note %s has no id, and superseded_by stores one (give it one with: brain migrate --ids)",
			repl,
		)
	}
	return "id:" + fm.ID, nil
}

func newRetractCmd(a *app) *cobra.Command {
	var reason, note string
	c := &cobra.Command{
		Use:   "retract <note> --reason <reason>",
		Short: "Mark a note retracted, keeping its body under a banner",
		Long: `Mark a note retracted: status becomes retracted, with retracted_reason,
retracted_at and status_at set, and --note stored as status_reason. The body
stays so the record of what was wrong survives; read shows a banner above it
and default search hides it. --reason is one of:

  error             the note's claim was wrong
  source-retracted  the source it rests on was retracted
  unverifiable      the claim cannot be checked any more

A shared vault refuses with exit code 6. With -o json, prints {"vault",
"path", "sha"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !slices.Contains(lifecycle.RetractReasons, reason) {
				return usageErrorf(
					"--reason must be one of %s, got %q",
					strings.Join(lifecycle.RetractReasons, ", "), reason,
				)
			}
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			sha, err := ref.Vault.Retract(ref.Abs(), reason, note, time.Now())
			return a.emitRevoked(cmd, ref, sha, err)
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "why: "+strings.Join(lifecycle.RetractReasons, ", "))
	c.Flags().StringVar(&note, "note", "", "free-text detail, stored as status_reason")
	return c
}

func newWithdrawCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "withdraw <note>",
		Short: "Replace a note with a tombstone and purge it from the indexes",
		Long: `Withdraw a note: only its id, status withdrawn and status_at remain, the
body and every other field are removed, and the note's chunks leave the
literal and semantic indexes at once, so no search returns its old content.
Links to it still resolve to the tombstone by its filename. Use it for a
note that must not be read any more; retract keeps the body instead. A
shared vault refuses with exit code 6. With -o json, prints {"vault",
"path", "sha"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			sha, err := ref.Vault.Withdraw(ref.Abs(), time.Now())
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			if err := lib.Purge(context.Background(), ref.Vault, ref.Path); err != nil {
				return fmt.Errorf(
					"%s is withdrawn, but purging it from the indexes failed: %w (retry with: brain index)",
					ref,
					err,
				)
			}
			return a.emitRevoked(cmd, ref, sha, nil)
		},
	}
	return c
}

// emitRevoked prints the result of a revocation command, or returns its
// error with a hint for finding the note.
func (a *app) emitRevoked(cmd *cobra.Command, ref vault.Ref, sha string, err error) error {
	if err != nil {
		return withNoteHint(err, ref.Path)
	}
	res := vault.PathResult{Vault: ref.Vault.Name, Path: ref.Path, SHA: sha}
	return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
		fmt.Fprintln(w, a.show(res.Vault, res.Path))
		return nil
	})
}
