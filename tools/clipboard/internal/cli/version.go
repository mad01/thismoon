package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
)

func versionCmd() *cobra.Command {
	var output string
	c := &cobra.Command{
		Use:   "version",
		Short: "Print the clipboard version",
		Long: `Print the clipboard version (the git commit it was built from).

Plain output is the bare version token — the cross-tool convention sibling
tools follow so ralph and status can probe any of them for the build they are
running. With -o json, prints the full build metadata object: version, commit,
tag, build_time, with every key present and "" for anything unknown.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Get()
			if output == "json" {
				fmt.Fprint(cmd.OutOrStdout(), info.PrettyJSON())
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), info.Version)
			return nil
		},
	}
	c.Flags().StringVarP(&output, "output", "o", "text", "Output format: text or json")
	return c
}
