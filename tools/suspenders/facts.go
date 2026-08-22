package suspenders

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc. suspenders
// is CLI-only: no backing service (BaseURL empty) and no data store
// (StorePath empty; the YAML config file is configuration, not a store).
// HasDoctor is true because `suspenders doctor` is the self-diagnosis
// surface: it explains the guard's decisions for a repo.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "suspenders",
		Bin:       "suspenders",
		Purpose:   "offline git secret scanner, internal-reference guard, and hook orchestrator",
		HasDoctor: true,
	}
}
