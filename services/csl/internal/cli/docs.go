package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/services/csl"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(csl.OperatingDoc, csl.Facts()))
}
