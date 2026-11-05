package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type listInput struct {
	Tier   string `json:"tier"             jsonschema:"vault tier to list: inbox, notes, outputs, or MOCs"`
	Tag    string `json:"tag,omitempty"    jsonschema:"filter by frontmatter tag"`
	Status string `json:"status,omitempty" jsonschema:"filter by frontmatter status"`
	Type   string `json:"type,omitempty"   jsonschema:"filter by frontmatter type"`
	Limit  int    `json:"limit,omitempty"  jsonschema:"maximum number of notes to return (default 100)"`
	Vault  string `json:"vault,omitempty"  jsonschema:"list this configured vault only; default lists every vault"`
}

func registerListTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_list_tier",
		Description: "List notes in a single vault tier (inbox, notes, outputs, or MOCs), across every vault unless vault narrows it. " +
			"Use to survey what's in a tier — 'what's in my inbox', 'list validated notes tagged kubernetes', 'show MOCs'. " +
			"Optionally filter by tag, status, or type. Returns notes sorted by frontmatter.updated descending, then path. Default limit 100.",
	}, h.handleListTier)
}

func (h *handlers) handleListTier(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, vault.Listing, error) {
	if strings.TrimSpace(in.Tier) == "" {
		return nil, vault.Listing{}, fmt.Errorf("brain_list_tier: tier is required")
	}
	tier, err := vault.ParseTier(in.Tier)
	if err != nil {
		return nil, vault.Listing{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	_, set, err := h.vaults(in.Vault)
	if err != nil {
		return nil, vault.Listing{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	listing, err := set.List(vault.ListOptions{
		Tier:   tier,
		Tag:    in.Tag,
		Status: in.Status,
		Type:   in.Type,
		Limit:  in.Limit,
	})
	if err != nil {
		return nil, vault.Listing{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	return nil, listing, nil
}
