package cards

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc and the
// doctor's last line. BaseURL and StorePath stay empty: cards has no serve
// process and no store. A run reads stdin, draws on the terminal, and keeps
// its cards in memory until the user quits.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name: "cards",
		Bin:  "cards",
		Purpose: "terminal UI that shows JSON Lines from stdin as cards: " +
			"a list to pick from or a feed to follow",
		HasDoctor: true,
	}
}
