package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	deps "github.com/mad01/thismoon/services/deps"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(deps.OperatingDoc, deps.Facts()))
}
