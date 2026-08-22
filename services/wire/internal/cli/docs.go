package cli

import (
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	wire "github.com/mad01/thismoon/services/wire"
)

func init() {
	rootCmd.AddCommand(agentcli.DocsCommand(wire.OperatingDoc, wire.Facts()))
}
