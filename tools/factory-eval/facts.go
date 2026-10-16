package factoryeval

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc and the
// error hints. BaseURL and StorePath stay empty: factory-eval has no serve
// process and no store. Each batch writes one results file and exits.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:    "factory-eval",
		Bin:     "factory-eval",
		Purpose: "experimental eval harness: the factory against one agent at equal budget",
	}
}
