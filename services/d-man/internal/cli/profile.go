package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/d-man/internal/profile"
)

// statusArg prints the active profile like the bare command does. It keeps
// the spelling the earlier generator scripts accepted.
const statusArg = "status"

var profileCmd = &cobra.Command{
	Use:   "profile [name]",
	Short: "Print the active block-list profile, or switch to another",
	Long: `Profile prints the active block-list profile, or switches to another one.

A profile decides which hosts the routes file blocks. Switching writes the
routes file (--config) from inputs in the same directory:

  routes.base.toml        the routes every profile shares (required)
  blocklist.<name>.toml   the profile's top-level blocklist, written above the
                          base so it comes before the first [[route]] table

and records the name in a file called "profile" beside them. The personal
profile needs no block-list file, and blocks nothing without one. Every other
profile needs its file, so a misspelled name fails instead of blocking nothing.

A leading ~/ in games_dir becomes the home directory of the user running the
command, because the root daemon would read ~ as /var/root. The result has to
validate before it replaces the routes file; the running daemon then reloads it
through its file watch, without sudo or a restart. A daemon started while the
routes file was a symlink misses the switch, because its watch resolved the
link once and still follows the old target. Send serve one SIGHUP to load the
new file (see "config and reload" in d-man docs), and one per later switch
until serve restarts.

  d-man profile                      print the active profile (personal when none is recorded)
  d-man profile status               the same
  d-man profile work                 switch to the work profile
  d-man profile "$(d-man profile)"   re-apply the active profile after editing an input`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProfile,
}

func init() {
	rootCmd.AddCommand(profileCmd)
}

func runProfile(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || args[0] == statusArg {
		name, err := profile.Current(flagConfig)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), name)
		return nil
	}
	home, err := confdir.Expand("~")
	if err != nil {
		return err
	}
	if err := profile.Switch(flagConfig, args[0], home); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "d-man profile: %s\n", args[0])
	return nil
}
