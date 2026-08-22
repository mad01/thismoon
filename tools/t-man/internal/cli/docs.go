package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	tman "github.com/mad01/thismoon/tools/t-man"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(tman.OperatingDoc, tman.Facts()))
}
