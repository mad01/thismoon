package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/bionic/internal/transform"
)

type renderInput struct {
	Text string `json:"text" jsonschema_description:"markdown text to render in bionic reading format"`
}

type renderOutput struct {
	Text string `json:"text"`
}

func registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "bionic_render",
		Description: "Render text in bionic reading format. The first ~50% of each word is bolded " +
			"using markdown **bold** syntax, making text easier to speed-read. " +
			"Preserves markdown structure: headers, code blocks, inline code, links, and existing bold/italic. " +
			"Numbers are left untouched. " +
			"Use this tool whenever the user asks for bionic reading, half-bold, or invokes /bionic.",
	}, handleRender)
}

func handleRender(
	_ context.Context,
	_ *mcp.CallToolRequest,
	params renderInput,
) (*mcp.CallToolResult, renderOutput, error) {
	out := transform.Bionic(params.Text)
	return nil, renderOutput{Text: out}, nil
}
