package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type readInput struct {
	Path      string `json:"path"                 jsonschema:"note ref: vault:path, or a vault-relative path such as notes/topic/foo.md found in exactly one vault"`
	Vault     string `json:"vault,omitempty"      jsonschema:"look the path up in this configured vault only"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first body line to return, 1-based; 0 or omit for start of body"`
	EndLine   int    `json:"end_line,omitempty"   jsonschema:"last body line to return, 1-based inclusive; 0 or omit for end of body"`
}

type peekInput struct {
	Path         string `json:"path"                     jsonschema:"note ref: vault:path, or a vault-relative path found in exactly one vault"`
	Vault        string `json:"vault,omitempty"          jsonschema:"look the path up in this configured vault only"`
	MaxBodyLines int    `json:"max_body_lines,omitempty" jsonschema:"number of body lines to include in preview (default 20)"`
}

func registerReadTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_read",
		Description: "Read a note's frontmatter and body (optionally a line slice). " +
			"Use after brain_search finds a relevant note and you need the full text, or when you already know its path. " +
			"Name the note as vault:path (search hits carry both), or by a bare path when it is in one vault only. " +
			"start_line / end_line are 1-based inclusive over the body (frontmatter excluded); omit both to read the whole body.",
	}, h.handleRead)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_peek",
		Description: "Cheap overview of a note: frontmatter, the first N body lines (default 20), and a count of inbound backlinks. " +
			"Use to decide whether a note is worth reading in full or to check its connectivity before diving in.",
	}, h.handlePeek)
}

func (h *handlers) handleRead(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in readInput,
) (*mcp.CallToolResult, vault.Note, error) {
	if in.StartLine < 0 || in.EndLine < 0 {
		return nil, vault.Note{}, fmt.Errorf("brain_read: start_line and end_line must be >= 0")
	}
	if in.StartLine > 0 && in.EndLine > 0 && in.StartLine > in.EndLine {
		return nil, vault.Note{}, fmt.Errorf(
			"brain_read: start_line (%d) must be <= end_line (%d)",
			in.StartLine,
			in.EndLine,
		)
	}

	_, ref, err := h.locate(in.Vault, in.Path)
	if err != nil {
		return nil, vault.Note{}, fmt.Errorf("brain_read: %w", err)
	}
	note, err := ref.Vault.Read(ref.Path, in.StartLine, in.EndLine)
	if err != nil {
		return nil, vault.Note{}, fmt.Errorf("brain_read: %w", err)
	}
	return nil, note, nil
}

func (h *handlers) handlePeek(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in peekInput,
) (*mcp.CallToolResult, vault.Peek, error) {
	_, ref, err := h.locate(in.Vault, in.Path)
	if err != nil {
		return nil, vault.Peek{}, fmt.Errorf("brain_peek: %w", err)
	}
	peek, err := ref.Vault.Peek(ref.Path, in.MaxBodyLines)
	if err != nil {
		return nil, vault.Peek{}, fmt.Errorf("brain_peek: %w", err)
	}
	return nil, peek, nil
}
