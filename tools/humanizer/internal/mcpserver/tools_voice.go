package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/voice"
)

type voiceProfileInput struct {
	Text string `json:"text" jsonschema:"text sample to profile; 500+ words gives the most reliable metrics"`
}

type voiceDiffInput struct {
	Draft  string `json:"draft"  jsonschema:"the AI-generated or AI-edited draft"`
	Sample string `json:"sample" jsonschema:"a reference sample of the target voice (e.g. the user's own prior writing)"`
}

type voiceDiffOutput struct {
	DraftProfile  voice.Profile `json:"draft_profile"`
	SampleProfile voice.Profile `json:"sample_profile"`
	Diff          voice.Diff    `json:"diff"`
}

func registerVoiceTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_voice_profile",
		Description: "Compute a quantitative voice profile: sentence-length distribution (mean/stddev/p50/p90), punctuation density (em-dash, semicolon, colon, paren, comma per 100 words), contraction rate, hyphenated-pair density, bold density, type-token ratio, Flesch reading ease, and top bigrams/trigrams. " +
			"Use on a user's writing sample to capture their voice, or on an AI draft to diagnose tells like low contraction rate or uniform sentence length.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleVoiceProfile)

	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_voice_diff",
		Description: "Profile a draft and a sample, then return a metric-by-metric delta sorted by magnitude. " +
			"Use when the user provides their own writing as a voice reference: the diff shows which metrics to match during rewrite (e.g. \"sample is shorter, sample uses contractions 3x more, draft has too many em-dashes\").",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleVoiceDiff)
}

func handleVoiceProfile(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in voiceProfileInput,
) (*mcp.CallToolResult, voice.Profile, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, voice.Profile{}, fmt.Errorf("humanizer_voice_profile: text is required")
	}
	return nil, voice.Compute(in.Text), nil
}

func handleVoiceDiff(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in voiceDiffInput,
) (*mcp.CallToolResult, voiceDiffOutput, error) {
	if strings.TrimSpace(in.Draft) == "" {
		return nil, voiceDiffOutput{}, fmt.Errorf("humanizer_voice_diff: draft is required")
	}
	if strings.TrimSpace(in.Sample) == "" {
		return nil, voiceDiffOutput{}, fmt.Errorf("humanizer_voice_diff: sample is required")
	}
	draft := voice.Compute(in.Draft)
	sample := voice.Compute(in.Sample)
	return nil, voiceDiffOutput{
		DraftProfile:  draft,
		SampleProfile: sample,
		Diff:          voice.DiffProfiles(draft, sample),
	}, nil
}
