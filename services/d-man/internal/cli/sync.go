package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/d-man/internal/hosts"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Write the managed /etc/hosts block once and exit",
	Long: `Sync rewrites the d-man managed block in the hosts file from routes.toml,
then exits. It validates before writing and never touches foreign lines.

Writing /etc/hosts needs root, so run it with sudo:
  sudo d-man sync

Note: when the d-man daemon is running it syncs automatically on every
routes.toml change — sync is only needed as a manual / diagnostic one-shot.`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	cfg, err := loadRoutes()
	if err != nil {
		return err
	}
	res, err := hosts.Sync(flagHostsFile, cfg.Hosts())
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf(
				"%w (writing %s needs root — try: sudo d-man sync)",
				err,
				flagHostsFile,
			)
		}
		return err
	}
	for _, warn := range res.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warn)
	}
	if res.Changed {
		fmt.Fprintf(cmd.OutOrStdout(), "synced %d hosts to %s (backup: %s)\n",
			len(cfg.Hosts()), flagHostsFile, res.Backup)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "%s already up to date\n", flagHostsFile)
	}
	return nil
}
