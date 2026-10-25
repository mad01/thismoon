package srtspec

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc. srtspec has
// no serve process, no store and no MCP server: every run reads manifests or
// settings files and exits.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:    "srtspec",
		Bin:     "srtspec",
		Purpose: "renders sandbox access manifests to srt settings and explains the effective policy",
	}
}
