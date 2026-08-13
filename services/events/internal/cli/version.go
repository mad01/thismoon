package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
)

var versionOutput string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print events version",
	Long: `Print the events version (the git commit it was built from).

Plain output is the bare version token — the cross-tool convention sibling
tools follow so ralph and status can probe any of them for the build they are
running. With -o json, prints the full build metadata object: version, commit,
tag, build_time, with every key present and "" for anything unknown.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := buildinfo.Get()
		if versionOutput == "json" {
			fmt.Fprint(cmd.OutOrStdout(), info.PrettyJSON())
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), info.Version)
		return nil
	},
}

func init() {
	versionCmd.Flags().
		StringVarP(&versionOutput, "output", "o", "text", "Output format: text or json")
	rootCmd.AddCommand(versionCmd)
}
