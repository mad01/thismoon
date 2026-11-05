package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/docs"
)

// docTopics maps each `brain docs` topic to the page it prints; the empty
// topic is the CLI guide.
var docTopics = map[string]string{
	"":          docs.CLI,
	"cli":       docs.CLI,
	"lifecycle": docs.Lifecycle,
}

func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs [topic]",
		Short: "Print the agent-facing CLI guide",
		Long: `Print the brain CLI guide for agents: commands, flags, exit codes, JSON
shapes, zoekt query tips and the capture-then-promote workflow.

Topics:
  cli        the CLI guide (the default)
  lifecycle  statuses, revocation, brain check and keeping notes fresh`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := ""
			if len(args) == 1 {
				topic = args[0]
			}
			page, ok := docTopics[topic]
			if !ok {
				return usageErrorf("unknown docs topic %q; topics: %s", topic, topicList())
			}
			fmt.Fprint(cmd.OutOrStdout(), page)
			return nil
		},
	}
}

func topicList() string {
	var names []string
	for name := range docTopics {
		if name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
