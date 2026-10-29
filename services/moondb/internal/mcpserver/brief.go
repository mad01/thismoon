package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Brief excerpts. A why answer is about one edit, so its subagent's spawn
// brief gets more room than a prompt. A turn can run many subagents, so
// each of its briefs gets its first line only, up to turnBriefExcerpt, and
// its length.
const (
	briefExcerpt     = 8000
	turnBriefExcerpt = 500
)

// agentBrief is one subagent of a turn with the start of its spawn brief.
type agentBrief struct {
	AgentID       string `json:"agent_id"`
	Agent         string `json:"agent"                    jsonschema:"the agent's name, else its type"`
	Brief         string `json:"brief,omitempty"          jsonschema:"the first line of its spawn brief, the prompt it was spawned with, cut at 500 bytes; moondb_why on a line it wrote gives up to 8000 bytes; absent when unknown (recorded before moondb kept briefs, known only from its edits, or of type internal, which has no transcript)"`
	BriefBytes    int    `json:"brief_bytes,omitempty"    jsonschema:"the length of the whole spawn brief in bytes"`
	LaterMessages int    `json:"later_messages,omitempty" jsonschema:"messages its parent sent it after the spawn brief, which the brief does not hold; absent when none"`
	Recorded      bool   `json:"recorded"                 jsonschema:"false when moondb holds no record of the agent, only its edits"`
}

// briefs lists the subagents of a turn's agent rows with the first line of
// their spawn briefs, in the rows' order; nil when only the main
// conversation ran.
func (h *handlers) briefs(ctx context.Context, rows []server.AgentRow) []agentBrief {
	var out []agentBrief
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		whole := h.chunk(ctx, r.BriefChunk)
		out = append(out, agentBrief{
			AgentID: r.ID, Agent: rowLabel(r), Brief: briefHead(whole), BriefBytes: len(whole),
			LaterMessages: r.LaterMessages, Recorded: r.Recorded,
		})
	}
	return out
}

// briefHead is a brief's first line that holds text, cut at
// turnBriefExcerpt bytes.
func briefHead(brief string) string {
	for line := range strings.Lines(brief) {
		if line = strings.TrimSpace(line); line != "" {
			return truncate(line, turnBriefExcerpt)
		}
	}
	return ""
}

// rowLabel names an agent row: its name, else its type.
func rowLabel(r server.AgentRow) string {
	if r.Name != "" {
		return r.Name
	}
	return r.Type
}

// briefLine names the subagent behind an edit and gives the first line of
// its spawn brief and the count of messages sent after it, or says why the
// brief is unknown.
func briefLine(a query.WhyAgent, brief string) string {
	who := "made by subagent " + a.Label()
	if a.Name != "" && a.Name != a.Type {
		who += " (" + a.Type + ")"
	}
	line := trace.PromptLine(brief)
	if line == "" {
		return who + ", spawn brief unknown: " + query.MissingBrief(a.Type, a.Recorded)
	}
	s := fmt.Sprintf("%s, spawn brief: %q", who, line)
	if a.LaterMessages > 0 {
		s += fmt.Sprintf(" (%d later messages from its parent are not in it)", a.LaterMessages)
	}
	return s
}
