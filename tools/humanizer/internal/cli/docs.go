package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/tools/humanizer"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(humanizer.OperatingDoc, humanizer.Facts()))
}
