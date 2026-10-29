package mcpserver

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// briefExcerpt caps the brief text a why or turn answer carries. A brief
// runs to a few KB and is the reason a subagent's edit was made, so it
// gets more room than a prompt.
const briefExcerpt = 8000

// agentBrief is one subagent of a turn with the prompt it was spawned with.
type agentBrief struct {
	AgentID string `json:"agent_id"`
	Agent   string `json:"agent"           jsonschema:"the agent's name, else its type"`
	Brief   string `json:"brief,omitempty" jsonschema:"the prompt it was spawned with, cut at 8000 bytes; absent when unknown (recorded before moondb kept briefs, or known only from its edits)"`
}

// briefs lists the subagents of a turn's agent rows with their briefs, in
// the rows' order; nil when only the main conversation ran.
func (h *handlers) briefs(ctx context.Context, rows []server.AgentRow) []agentBrief {
	var out []agentBrief
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		out = append(out, agentBrief{
			AgentID: r.ID, Agent: rowLabel(r),
			Brief: h.chunkText(ctx, r.BriefChunk, briefExcerpt),
		})
	}
	return out
}

// rowLabel names an agent row: its name, else its type.
func rowLabel(r server.AgentRow) string {
	if r.Name != "" {
		return r.Name
	}
	return r.Type
}

// briefLine names the subagent behind an edit and gives the first line of
// its brief, or says the brief is unknown.
func briefLine(a query.WhyAgent, brief string) string {
	who := "made by subagent " + a.Label()
	if a.Name != "" && a.Name != a.Type {
		who += " (" + a.Type + ")"
	}
	if line := trace.PromptLine(brief); line != "" {
		return fmt.Sprintf("%s, brief: %q", who, line)
	}
	return who + ", brief unknown"
}
