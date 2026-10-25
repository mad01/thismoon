package ingest

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Transcript line types that name a session.
const (
	// lineAgentName carries agentName, the name the person gave the
	// session; it outranks a generated title.
	lineAgentName = "agent-name"
	// lineAITitle carries aiTitle, the title the harness generated.
	lineAITitle = "ai-title"
)

// titleEntry is the part of a transcript line that names the session.
type titleEntry struct {
	Type      string `json:"type"`
	AgentName string `json:"agentName"`
	AITitle   string `json:"aiTitle"`
}

// titleOffer is a title a turn offers its session and where it came from;
// the zero value offers none.
type titleOffer struct {
	title  string
	source store.TitleSource
}

// transcriptTitle is the title a turn's transcript lines offer the session,
// by store.OfferTitle over the lines in order: the first name the person
// gave it, else the first title the harness generated. Lines that mention
// neither type are not decoded.
func transcriptTitle(lines []byte) titleOffer {
	var o titleOffer
	if !namesSession(lines) {
		return o
	}
	for raw := range bytes.Lines(lines) {
		if !namesSession(raw) {
			continue
		}
		var e titleEntry
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		switch e.Type {
		case lineAgentName:
			store.OfferTitle(&o.title, &o.source, strings.TrimSpace(e.AgentName), store.TitleNamed)
		case lineAITitle:
			store.OfferTitle(
				&o.title,
				&o.source,
				strings.TrimSpace(e.AITitle),
				store.TitleGenerated,
			)
		}
	}
	return o
}

// namesSession reports whether b mentions a line type that names the
// session, so lines without one skip the decode.
func namesSession(b []byte) bool {
	return bytes.Contains(b, []byte(lineAgentName)) || bytes.Contains(b, []byte(lineAITitle))
}
