package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	status "github.com/mad01/thismoon/services/status"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(status.OperatingDoc, status.Facts()))
}
