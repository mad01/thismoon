package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var versionOutput string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print deps version",
	Long: `Print the deps version (the git commit it was built from).

With -o json, prints {"version":"<sha>"} — the cross-tool convention sibling
tools follow so ralph can probe any of them for the build they are running.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if versionOutput == "json" {
			b, err := json.Marshal(map[string]string{"version": Version})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), Version)
		return nil
	},
}

func init() {
	versionCmd.Flags().
		StringVarP(&versionOutput, "output", "o", "text", "Output format: text or json")
	rootCmd.AddCommand(versionCmd)
}
