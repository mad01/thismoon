package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	kof "github.com/mad01/thismoon/services/keeper-of-facts"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(kof.OperatingDoc, kof.Facts()))
}
