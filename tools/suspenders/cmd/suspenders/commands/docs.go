package commands

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	suspenders "github.com/mad01/thismoon/tools/suspenders"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(suspenders.OperatingDoc, suspenders.Facts()))
}
