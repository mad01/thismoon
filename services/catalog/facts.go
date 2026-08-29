package catalog

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `catalog web` listens on when CATALOG_PORT and
// --port are both unset. It sits outside the platform's 7423+ block on
// purpose — 7575 spells "SYSY", and the catalog is the systems index. The
// server binds to loopback only.
const DefaultPort = 7575

// RegistryFileName is the registry's name inside catalog's config directory.
const RegistryFileName = "registry.yaml"

// DefaultRegistry is the compiled fallback registry location, used when the
// config directory cannot be resolved. Normal resolution goes through
// confdir.Path("catalog", RegistryFileName), which honors XDG_CONFIG_HOME. The
// leading ~ is expanded at runtime, never at build time.
const DefaultRegistry = "~/.config/catalog/" + RegistryFileName

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
		HasDoctor: true,
	}
}
