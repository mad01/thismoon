package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	present "github.com/mad01/thismoon/services/present"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(present.OperatingDoc, present.Facts()))
}
