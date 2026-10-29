package mcpserver

import (
	"context"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/present/internal/export"
)

// exportsDir is the folder in the page store present_export writes to. The
// MCP sandbox lets present write its page store and nothing else on disk,
// so the file lands there and the caller copies it on.
const exportsDir = "exports"

type exportInput struct {
	ID   string `json:"id"             jsonschema:"page id to export"`
	Deck bool   `json:"deck,omitempty" jsonschema:"export the page's slide deck instead of its content"`
}

type exportOutput struct {
	Path      string   `json:"path"                jsonschema:"absolute path of the written file, under exports/ in the page store"`
	Bytes     int      `json:"bytes"`
	Rendition string   `json:"rendition"           jsonschema:"brief or deck"`
	Libraries []string `json:"libraries,omitempty" jsonschema:"CDN URLs the file loads, each pinned by an integrity hash"`
	Notes     []string `json:"notes,omitempty"     jsonschema:"what the file does without, such as fonts missing from the asset cache"`
}

// registerExportTool adds present_export. It needs the workdir, which holds
// the asset cache, the image store, and the exports folder, so it is
// registered locally and only with one.
func registerExportTool(s *mcp.Server, h *handlers) {
	if h.mode != ModeLocal || h.workdir == "" {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "present_export",
		Description: "Write a page as one standalone HTML file that opens in any browser, offline included, with no present server. " +
			"The page's markup, styles, scripts, and local images are inside the file, so prose reads with scripts off; " +
			"graph, chart, and diagram libraries load from cdn.jsdelivr.net, pinned by integrity hash, and only the ones the page uses. " +
			"A terminal block shows a note instead of a session. Set `deck` to export the slide deck instead of the content; " +
			"a page with only a deck exports its deck. The file lands in exports/ in the page store, the one place the sandbox lets " +
			"this server write: return the path to the user or copy the file on. Exporting again overwrites the file.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, withHint(h.handleExport))
}

func (h *handlers) handleExport(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in exportInput,
) (*mcp.CallToolResult, exportOutput, error) {
	res, err := export.New(h.store, h.workdir).Export(ctx, export.Request{ID: in.ID, Deck: in.Deck})
	if err != nil {
		return nil, exportOutput{}, err
	}
	// The store found the page, so the id is a valid page id and safe in a
	// path.
	path := filepath.Join(h.workdir, exportsDir, export.FileName(in.ID, res.Rendition))
	if err := export.WriteFile(path, res.HTML); err != nil {
		return nil, exportOutput{}, err
	}
	return nil, exportOutput{
		Path: path, Bytes: len(res.HTML), Rendition: res.Rendition,
		Libraries: res.Libraries, Notes: res.Notes,
	}, nil
}
