package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type backlinksInput struct {
	Path  string `json:"path,omitempty"  jsonschema:"note ref: vault:path, or a vault-relative path found in exactly one vault; one of path or title is required"`
	Title string `json:"title,omitempty" jsonschema:"note title or alias to look up; matched against frontmatter.title and aliases"`
	Vault string `json:"vault,omitempty" jsonschema:"look the note up in this configured vault only"`
}

func registerGraphTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_backlinks",
		Description: "Return inbound and outbound wikilinks for a note. " +
			"Inbound: every note that contains [[stem]] or [[title]] pointing at this note (with the matching line and excerpt). " +
			"Outbound: every [[...]] wikilink in the note itself, resolved to a vault-relative path when the target matches an existing note. " +
			"Supply either path (preferred) or title; title is matched against frontmatter.title and aliases.",
	}, h.handleBacklinks)
}

func (h *handlers) handleBacklinks(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in backlinksInput,
) (*mcp.CallToolResult, vault.Backlinks, error) {
	if in.Path == "" && in.Title == "" {
		return nil, vault.Backlinks{}, fmt.Errorf(
			"brain_backlinks: one of path or title is required",
		)
	}
	ref, err := h.backlinksTarget(in)
	if err != nil {
		return nil, vault.Backlinks{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	links, err := ref.Vault.Backlinks(ref.Path)
	if err != nil {
		return nil, vault.Backlinks{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	return nil, links, nil
}

// backlinksTarget locates the note named by path, else by title.
func (h *handlers) backlinksTarget(in backlinksInput) (vault.Ref, error) {
	if in.Path != "" {
		_, ref, err := h.locate(in.Vault, in.Path)
		return ref, err
	}
	_, set, err := h.vaults(in.Vault)
	if err != nil {
		return vault.Ref{}, err
	}
	return set.FindByTitle(in.Title)
}
