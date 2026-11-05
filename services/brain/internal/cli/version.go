package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
)

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the brain version",
		Long: `Print the brain version (the git commit it was built from).

Plain output is the bare version token: the cross-tool convention sibling
tools follow so ralph and status can probe any of them for the build they are
running. With -o json, prints the full build metadata object: version, commit,
tag, build_time, with every key present and "" for anything unknown.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Get()
			return a.emit(cmd.OutOrStdout(), info, func(w io.Writer) error {
				fmt.Fprintln(w, info.Version)
				return nil
			})
		},
	}
}
