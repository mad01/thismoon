package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/services/reminder"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(reminder.OperatingDoc, reminder.Facts()))
}
