package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

type statusInput struct{}

type statusOutput struct {
	Installed   bool   `json:"installed"`
	Binary      string `json:"binary"`
	Version     string `json:"version,omitempty"`
	CacheDir    string `json:"cache_dir"`
	RuleCount   int    `json:"rule_count"`
	StylePack   string `json:"style_pack"`
	Error       string `json:"error,omitempty"`
	InstallHint string `json:"install_hint,omitempty"`
}

func registerStatusTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_status",
		Description: "Report humanizer MCP health: whether the vale binary is installed, its version, the cache directory, and how many rules are bundled. " +
			"Call first when humanizer_detect fails unexpectedly.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleStatus)
}

func handleStatus(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ statusInput,
) (*mcp.CallToolResult, statusOutput, error) {
	s := rules.CheckStatus(ctx, rules.DetectOptions{})
	return nil, statusOutput{
		Installed:   s.Installed,
		Binary:      s.Binary,
		Version:     s.Version,
		CacheDir:    s.CacheDir,
		RuleCount:   s.RuleCount,
		StylePack:   s.StylePack,
		Error:       s.Error,
		InstallHint: s.InstallHint,
	}, nil
}
