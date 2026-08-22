package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	dman "github.com/mad01/thismoon/services/d-man"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(dman.OperatingDoc, dman.Facts()))
}
