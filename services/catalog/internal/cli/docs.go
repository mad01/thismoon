package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	catalogroot "github.com/mad01/thismoon/services/catalog"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(catalogroot.OperatingDoc, catalogroot.Facts()))
}
