package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type readInput struct {
	Path      string `json:"path"                 jsonschema:"vault-relative note path, e.g. notes/topic/foo.md"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first body line to return, 1-based; 0 or omit for start of body"`
	EndLine   int    `json:"end_line,omitempty"   jsonschema:"last body line to return, 1-based inclusive; 0 or omit for end of body"`
}

type peekInput struct {
	Path         string `json:"path"                     jsonschema:"vault-relative note path"`
	MaxBodyLines int    `json:"max_body_lines,omitempty" jsonschema:"number of body lines to include in preview (default 20)"`
}

func registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_read",
		Description: "Read a note's frontmatter and body (optionally a line slice). " +
			"Use after brain_search finds a relevant note and you need the full text, or when you already know a vault-relative path. " +
			"start_line / end_line are 1-based inclusive over the body (frontmatter excluded); omit both to read the whole body.",
	}, handleRead)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_peek",
		Description: "Cheap overview of a note: frontmatter, the first N body lines (default 20), and a count of inbound backlinks. " +
			"Use to decide whether a note is worth reading in full or to check its connectivity before diving in.",
	}, handlePeek)
}

func handleRead(
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

	_, v, err := loadVault()
	if err != nil {
		return nil, vault.Note{}, fmt.Errorf("brain_read: %w", err)
	}
	note, err := v.Read(in.Path, in.StartLine, in.EndLine)
	if err != nil {
		return nil, vault.Note{}, fmt.Errorf("brain_read: %w", err)
	}
	return nil, note, nil
}

func handlePeek(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in peekInput,
) (*mcp.CallToolResult, vault.Peek, error) {
	_, v, err := loadVault()
	if err != nil {
		return nil, vault.Peek{}, fmt.Errorf("brain_peek: %w", err)
	}
	peek, err := v.Peek(in.Path, in.MaxBodyLines)
	if err != nil {
		return nil, vault.Peek{}, fmt.Errorf("brain_peek: %w", err)
	}
	return nil, peek, nil
}
