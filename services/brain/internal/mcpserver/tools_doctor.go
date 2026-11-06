package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/doctor"
)

func registerDoctorTool(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_doctor",
		Description: "Diagnose brain itself: does the config load, does the vault exist, can brain write its " +
			"index directory, is the literal index fresh, and (only with semantic.enabled) does Ollama answer. " +
			"Returns one result per check with a fix for each failure, and `ok` false if any failed. " +
			"Read-only: it probes and leaves nothing behind. " +
			"Call this when another brain tool errors or comes back empty.",
		// Reads the config, the vault and the index state; the Ollama probe
		// stays on the configured local URL.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleDoctor)
}

// handleDoctor runs the same checks as `brain doctor` and returns the report.
// A failing check is a result, not a tool error: the caller asked what is
// wrong, and an error would hide the answer.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Run(ctx, h.configPath), nil
}
