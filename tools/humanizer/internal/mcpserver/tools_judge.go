package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/backend"
	"github.com/mad01/thismoon/tools/humanizer/internal/judge"
)

const judgeTimeout = 90 * time.Second

type judgeInput struct {
	Text    string `json:"text"              jsonschema:"the passage to judge; 50-2000 words works best"`
	Backend string `json:"backend,omitempty" jsonschema:"force an LLM backend (openrouter); default auto-detects from env"`
	Model   string `json:"model,omitempty"   jsonschema:"override the backend's default model id"`
}

func registerJudgeTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_judge",
		Description: "Holistic LLM judgment on a whole passage: does it read as machine-written? " +
			"Returns {verdict: likely_ai|likely_human|mixed, confidence, signals[], summary} plus the backend and model used. " +
			"This is the fuzzy complement to humanizer_detect (span rules) and humanizer_detect_statistical (sample metrics) — the three cover disjoint failure modes, so run them together. " +
			"Feed whole sections or paragraphs, never isolated words; verdicts are advisory rewrite targets, not ground truth. " +
			"Calls out to a configured LLM provider (OPENROUTER_API_KEY -> openrouter, default model anthropic/claude-haiku-4.5), so it needs network egress and a key in the server's environment; " +
			"when it errors with no backend configured or a network denial, fall back to the `humanizer judge` CLI in a shell that has the key.",
	}, handleJudge)
}

func handleJudge(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in judgeInput,
) (*mcp.CallToolResult, *judge.Verdict, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, nil, fmt.Errorf("humanizer_judge: text is required")
	}
	b, err := backend.Select(in.Backend, in.Model)
	if err != nil {
		if errors.Is(err, backend.ErrNoBackend) {
			return nil, nil, fmt.Errorf(
				"humanizer_judge: %w; the deterministic tools (humanizer_detect, humanizer_detect_statistical) work without one",
				err,
			)
		}
		return nil, nil, fmt.Errorf("humanizer_judge: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	v, err := judge.Run(ctx, b, in.Text)
	if err != nil {
		return nil, nil, fmt.Errorf("humanizer_judge: %w", err)
	}
	return nil, v, nil
}
