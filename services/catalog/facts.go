package catalog

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `catalog web` listens on when --port is unset. The
// server binds to loopback only.
const DefaultPort = 7575

// DefaultRegistry is the registry location when --registry is unset. The
// leading ~ is expanded at runtime, never at build time.
const DefaultRegistry = "~/.config/catalog/registry.yaml"

// Facts returns the mechanical facts rendered into OperatingDoc and error
// hints. StorePath names the registry: catalog persists no state of its own,
// and the registry is the one file every command reads.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "catalog",
		Bin:       "catalog",
		Purpose:   "self-hosted systems catalog indexing service-info.yaml files from local repos",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultRegistry,
	}
}
