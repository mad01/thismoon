package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/cli"
)

var versionOutput string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the suspenders version",
	Long: `Print the suspenders version (the git commit it was built from).

With -o json, prints {"version":"<sha>"} — the cross-tool convention sibling
tools follow so a single probe can ask any of them what build it is.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionOutput == "json" {
			b, err := json.Marshal(map[string]string{"version": cli.Version})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return err
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "suspenders", cli.Version)
		return err
	},
}

func init() {
	versionCmd.Flags().
		StringVarP(&versionOutput, "output", "o", "text", "Output format: text or json")
	rootCmd.AddCommand(versionCmd)
}
