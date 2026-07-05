package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var versionOutput string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the t-man version",
	Long: `Print the t-man version (the git commit it was built from).

With -o json, prints {"version":"<sha>"} — the cross-tool convention sibling
tools follow so a single probe can ask any of them what build it is.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
