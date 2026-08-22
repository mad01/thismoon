package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	speak "github.com/mad01/thismoon/services/speak"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(speak.OperatingDoc, speak.Facts()))
}
