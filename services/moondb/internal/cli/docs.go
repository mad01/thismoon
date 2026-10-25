package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/services/moondb"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(moondb.OperatingDoc, moondb.Facts()))
}
