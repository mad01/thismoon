package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Brief excerpts. A why answer is about one edit, so its subagent's spawn
// brief and last message get more room than a prompt. A turn can run many
// subagents, so each one's brief and last message get their first line
// only, up to turnBriefExcerpt, and their length.
const (
	briefExcerpt     = 8000
	turnBriefExcerpt = 500
)

// agentBrief is one subagent of a turn with the start of its spawn brief
// and of the last message its parent sent it.
type agentBrief struct {
	AgentID       string          `json:"agent_id"`
	Agent         string          `json:"agent"                    jsonschema:"the agent's name, else its type"`
	Brief         string          `json:"brief,omitempty"          jsonschema:"the first line of its spawn brief, the prompt it was spawned with, cut at 500 bytes; moondb_why on a line it wrote gives up to 8000 bytes; absent when unknown (recorded before moondb kept briefs, known only from its edits, or of type internal, which has no transcript)"`
	BriefBytes    int             `json:"brief_bytes,omitempty"    jsonschema:"the length of the whole spawn brief in bytes"`
	LaterMessages int             `json:"later_messages,omitempty" jsonschema:"messages its parent sent it after the spawn brief, which the brief does not hold; absent when none"`
	LastMessage   *messageExcerpt `json:"last_message,omitempty"   jsonschema:"the latest of those messages by the end of its work in the turn; absent when none, or when the record kept no messages"`
	Recorded      bool            `json:"recorded"                 jsonschema:"false when moondb holds no record of the agent, only its edits"`
}

// messageExcerpt is a later message as a turn lists it.
type messageExcerpt struct {
	At    time.Time `json:"at"             jsonschema:"when it reached the agent"`
	From  string    `json:"from,omitempty" jsonschema:"the teammate that sent it; absent for a line with no envelope, such as one the person typed into the agent's pane"`
	Text  string    `json:"text"           jsonschema:"its first line, cut at 500 bytes; moondb_why on a line the agent wrote after it gives up to 8000 bytes"`
	Bytes int       `json:"bytes"          jsonschema:"the length of the whole message in bytes"`
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
			LaterMessages: r.LaterMessages, LastMessage: h.excerpt(ctx, r.LastMessage),
			Recorded: r.Recorded,
		})
	}
	return out
}

// excerpt is a later message's first line and length; nil for none, or
// one whose text the store cannot give.
func (h *handlers) excerpt(ctx context.Context, m *store.AgentMessage) *messageExcerpt {
	if m == nil {
		return nil
	}
	whole := h.chunk(ctx, m.Chunk)
	if whole == "" {
		return nil
	}
	return &messageExcerpt{At: m.At, From: m.From, Text: briefHead(whole), Bytes: len(whole)}
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
// brief is unknown, then the first line of the last message it got before
// the edit when it got one.
func briefLine(a query.WhyAgent, brief, last string) string {
	who := "made by subagent " + a.Label()
	if a.Name != "" && a.Name != a.Type {
		who += " (" + a.Type + ")"
	}
	s := who + ", spawn brief unknown: " + query.MissingBrief(a.Type, a.Recorded)
	if line := trace.PromptLine(brief); line != "" {
		s = fmt.Sprintf("%s, spawn brief: %q", who, line)
		if a.LaterMessages > 0 {
			s += fmt.Sprintf(" (%d later messages from its parent are not in it)", a.LaterMessages)
		}
	}
	if line := trace.PromptLine(last); a.LastMessage != nil && line != "" {
		s += fmt.Sprintf("; last message before the edit%s at %s: %q",
			fromNote(a.LastMessage.From), a.LastMessage.At.UTC().Format(time.RFC3339), line)
	}
	return s
}

// fromNote names a message's sender for a summary, "" when it has none.
func fromNote(from string) string {
	if from == "" {
		return ""
	}
	return ", from " + from + ","
}
