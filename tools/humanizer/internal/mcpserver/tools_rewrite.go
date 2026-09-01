package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/rewrite"
)

type rewriteInput struct {
	Text         string `json:"text"                    jsonschema:"the text to build a rewrite prompt for"`
	Strength     string `json:"strength,omitempty"      jsonschema:"paraphrase (default), humanize, code, backtranslate, or structural"`
	Lang         string `json:"lang,omitempty"          jsonschema:"pivot language for backtranslate (default French)"`
	OriginalLang string `json:"original_lang,omitempty" jsonschema:"original language for backtranslate (default English)"`
}

type rewriteOutput struct {
	Prompt string       `json:"prompt"`
	Info   rewrite.Info `json:"info"`
}

func registerRewriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_rewrite",
		Description: "Build a \"Layer B\" rewrite prompt for statistical (token-sampling) watermarks, which the deterministic Layer A scrub can't touch. " +
			"Returns the prompt for the given strength (paraphrase, humanize, code, backtranslate, structural); YOU produce the rewrite by running the prompt. This tool calls no model, so it is offline and sandbox-safe. " +
			"Prefer a rewrite model different from the suspected origin model; rewriting with the origin model can re-stamp the text. " +
			"For running a local/remote model directly, use the `humanizer rewrite` CLI (network backends aren't available under the MCP sandbox).",
	}, handleRewrite)
}

func handleRewrite(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in rewriteInput,
) (*mcp.CallToolResult, rewriteOutput, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, rewriteOutput{}, fmt.Errorf("humanizer_rewrite: text is required")
	}
	strength := rewrite.Strength(in.Strength)
	if in.Strength == "" {
		strength = rewrite.Paraphrase
	}
	lang := in.Lang
	if lang == "" {
		lang = "French"
	}
	origLang := in.OriginalLang
	if origLang == "" {
		origLang = "English"
	}
	prompt, info, err := rewrite.Rewrite(in.Text, rewrite.Options{
		Backend:      rewrite.PrintPrompt,
		Strength:     strength,
		Lang:         lang,
		OriginalLang: origLang,
	})
	if err != nil {
		return nil, rewriteOutput{}, fmt.Errorf("humanizer_rewrite: %w", err)
	}
	return nil, rewriteOutput{Prompt: prompt, Info: info}, nil
}
