package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newFlagCmd(a *app) *cobra.Command {
	var reason string
	c := &cobra.Command{
		Use:   "flag <note> --reason <text>",
		Short: "Mark a note disputed, with the reason",
		Long: `Flag a note whose claims you doubt: status becomes disputed, --reason is
stored as status_reason and status_at records the day. read and peek then
show a DISPUTED banner until someone settles the note (edit --status
validated, or retract it). Flagging a disputed note again replaces the
reason. A superseded, retracted or withdrawn note refuses with exit code 2.

A shared vault refuses with exit code 6: open an issue on the vault's repo
naming the note and the reason instead. With -o json, prints {"vault",
"path", "sha"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return usageErrorf("--reason <text> is required")
			}
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			sha, err := ref.Vault.Flag(ref.Abs(), reason, time.Now())
			if errors.Is(err, vault.ErrReadOnly) {
				err = withRepoHint(err, ref.Vault)
			}
			return a.emitRevoked(cmd, ref, sha, err)
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "why the note is in doubt (required)")
	return c
}

// withRepoHint adds the shared vault's origin remote to a read-only error,
// so the message names where to open the issue. A vault without one, or
// without git, gets the error unchanged.
func withRepoHint(err error, v *vault.Vault) error {
	if !v.Shared {
		return err
	}
	out, gitErr := exec.Command("git", "-C", v.Root, "remote", "get-url", "origin").Output()
	remote := strings.TrimSpace(string(out))
	if gitErr != nil || remote == "" {
		return err
	}
	return fmt.Errorf("%w (repo: %s)", err, remote)
}
