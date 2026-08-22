package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/services/events"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(events.OperatingDoc, events.Facts()))
}
