package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/narrative"
)

type narrativeRubricInput struct{}

func registerNarrativeTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_narrative_rubric",
		Description: "Return the StoryScope narrative rubric: 30 discourse-level features (thematic over-explanation, " +
			"plot linearity, embodied emotion, intertextual reference) that separate human-written from AI-generated " +
			"fiction. These features need a reader's judgment, not a regex. For fiction or story-shaped long-form " +
			"prose, feed the returned prompt plus the passage to an LLM judge (e.g. the humanizer skill's headless " +
			"claude -p pass on Sonnet or better). Complements humanizer_detect, which only catches surface style.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleNarrativeRubric)
}

func handleNarrativeRubric(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ narrativeRubricInput,
) (*mcp.CallToolResult, narrative.Rubric, error) {
	return nil, narrative.BuildRubric(), nil
}
