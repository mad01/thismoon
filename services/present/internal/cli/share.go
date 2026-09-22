package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// errNoSharer is what every share path says when nothing is configured.
var errNoSharer = errors.New(
	"no shared instance configured: set --shared-url and --author-key " +
		"(PRESENT_SHARED_URL, PRESENT_AUTHOR_KEY); mint a key with `present key new`",
)

// sharer returns the shared-instance client the flags describe, or nil when
// sharing is off. Half a configuration is worth a word on stderr: a URL
// without a key (or the reverse) is a typo, not a choice.
func sharer() *sharedclient.Client {
	switch {
	case flagSharedURL != "" && flagAuthorKey != "":
		return sharedclient.New(flagSharedURL, flagAuthorKey)
	case flagSharedURL != "" || flagAuthorKey != "":
		log.Printf(
			"present: sharing needs both --shared-url and --author-key; only one is set, sharing stays off",
		)
	}
	return nil
}

var shareCmd = &cobra.Command{
	Use:   "share <id>",
	Short: "Push a local page to the shared instance and print its link",
	Long: `Push a local page to the shared instance named by --shared-url, signed with
--author-key, and print the link others can open. Sharing a page again
replaces its copy under the same link. With --ephemeral the copy expires 30
days after the last share; otherwise it stays until 'present unshare'.`,
	Args: cobra.ExactArgs(1),
	RunE: runShare,
}

var flagShareEphemeral bool

var unshareCmd = &cobra.Command{
	Use:   "unshare <id>",
	Short: "Remove a local page's copy from the shared instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnshare,
}

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Author keys for the shared instance",
}

var keyNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Mint an author key and print it",
	Long: `Mint a fresh author key. The shared instance stores only its hash, as the
author of every page this key creates or shares; keep the key itself in
PRESENT_AUTHOR_KEY on this machine and in the HTTP MCP registration's
bearer header. A lost key cannot be recovered, only replaced by re-sharing.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), author.NewKey())
		return err
	},
}

func init() {
	shareCmd.Flags().BoolVar(&flagShareEphemeral, "ephemeral", false,
		"expire the shared copy 30 days after the last share")
	keyCmd.AddCommand(keyNewCmd)
	rootCmd.AddCommand(shareCmd, unshareCmd, keyCmd)
}

func runShare(cmd *cobra.Command, args []string) error {
	c := sharer()
	if c == nil {
		return errNoSharer
	}
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	info, err := sharedclient.Share(
		context.Background(),
		st,
		c,
		args[0],
		flagShareEphemeral,
		time.Now(),
	)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), info.URL)
	if info.ExpiresAt != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "expires %s\n", info.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return nil
}

func runUnshare(cmd *cobra.Command, args []string) error {
	c := sharer()
	if c == nil {
		return errNoSharer
	}
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	if err := sharedclient.Unshare(context.Background(), st, c, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "unshared %s\n", args[0])
	return nil
}
