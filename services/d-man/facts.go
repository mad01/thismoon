// Package dman holds the d-man component identity: the embedded operating doc
// and the mechanical facts it renders with. internal/cli reads from here, so
// the doc, the config-resolution order, and the error hints cannot drift from
// the defaults the code actually uses.
package dman

import "github.com/mad01/thismoon/kit/agentdoc"

// DefaultRoutesPath is the per-user routes file when --config and DMAN_CONFIG
// are unset. The leading ~ is expanded at runtime by the CLI, never at build
// time.
const DefaultRoutesPath = "~/.config/d-man/routes.toml"

// SystemRoutesPath is the root-context routes file. A root launchd daemon has
// no useful HOME, so the per-user default resolves to root's home where no
// routes live; this system path is the fallback that makes a bare
// `d-man serve` work there.
const SystemRoutesPath = "/etc/d-man/routes.toml"

// Facts returns the mechanical facts rendered into OperatingDoc and error
// hints. BaseURL stays empty: d-man is the proxy under every .this host and
// has no address of its own.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "d-man",
		Bin:       "d-man",
		Purpose:   "local domain front door routing *.this hostnames to localhost services",
		StorePath: DefaultRoutesPath,
		HasDoctor: true,
	}
}
