package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/library"
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
			"A link resolves case-insensitively by stem (or path), then by title or alias; a private vault's links look in that vault first, then in the shared vaults, and a shared vault's links only in shared vaults. " +
			"Inbound: every line in another note with a link that resolves to this note, with its excerpt. " +
			"Outbound: every [[...]] wikilink in the note, with the vault and path it resolves to, or dangling: true. " +
			"Supply either path (preferred) or title; title is matched against frontmatter.title and aliases.",
	}, h.handleBacklinks)
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_graph",
		Description: "Return the wikilink graph of the configured vaults as nodes and edges, resolved like brain_backlinks. " +
			"Nodes: notes (id is id:<note id>, or vault:path for a note without one), dangling link targets (dangling: true) and, with tags_as_nodes, tags (tag:<name>). " +
			"Edges: {source, target, kind, resolved}; kind is link (a wikilink), moc (a wikilink out of a MOC), " +
			"supersedes (to the note superseded_by names), source (to a note a sources[] ref names) or tag. " +
			"degree counts the edges of the returned graph. " +
			"Pass center (a note ref) and depth to get the neighbourhood of one note; pass vault to walk one vault's notes.",
	}, h.handleGraph)
}

type graphInput struct {
	Vault       string `json:"vault,omitempty"         jsonschema:"walk only this vault's notes; notes they link to in other vaults still appear"`
	Center      string `json:"center,omitempty"        jsonschema:"note ref (vault:path, id:<ulid> or a bare path); keep the notes within depth hops of it"`
	Depth       int    `json:"depth,omitempty"         jsonschema:"hops from center, either link direction; default 1"`
	TagsAsNodes bool   `json:"tags_as_nodes,omitempty" jsonschema:"add a node per tag and a tag edge from each note to it"`
}

func (h *handlers) handleGraph(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in graphInput,
) (*mcp.CallToolResult, vault.Graph, error) {
	lib, err := h.open()
	if err != nil {
		return nil, vault.Graph{}, fmt.Errorf("brain_graph: %w", err)
	}
	opts := vault.GraphOptions{
		Sources:     names(in.Vault),
		Depth:       in.Depth,
		TagsAsNodes: in.TagsAsNodes,
	}
	if in.Center != "" {
		ref, err := lib.Locate(names(in.Vault), in.Center)
		if err != nil {
			return nil, vault.Graph{}, fmt.Errorf("brain_graph: %w", err)
		}
		opts.Center = &ref
	}
	g, err := lib.Set().Graph(opts)
	if err != nil {
		return nil, vault.Graph{}, fmt.Errorf("brain_graph: %w", err)
	}
	return nil, g, nil
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
	lib, ref, err := h.backlinksTarget(in)
	if err != nil {
		return nil, vault.Backlinks{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	links, err := lib.Set().Backlinks(ref)
	if err != nil {
		return nil, vault.Backlinks{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	return nil, links, nil
}

// backlinksTarget locates the note named by path, else by title.
func (h *handlers) backlinksTarget(in backlinksInput) (*library.Library, vault.Ref, error) {
	if in.Path != "" {
		return h.locate(in.Vault, in.Path)
	}
	lib, set, err := h.vaults(in.Vault)
	if err != nil {
		return nil, vault.Ref{}, err
	}
	ref, err := set.FindByTitle(in.Title)
	return lib, ref, err
}
