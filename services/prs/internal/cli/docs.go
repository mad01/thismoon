package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	prs "github.com/mad01/thismoon/services/prs"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(prs.OperatingDoc, prs.Facts()))
}
