package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/scrub"
)

type lintInput struct {
	Text           string `json:"text"                       jsonschema:"the text to scan for invisible/format Unicode watermark carriers"`
	Aggressive     bool   `json:"aggressive,omitempty"       jsonschema:"also flag Cyrillic/fullwidth Latin confusable lookalikes"`
	StripEmojiGlue bool   `json:"strip_emoji_glue,omitempty" jsonschema:"paranoid: also flag load-bearing invisibles (emoji glue, script joiners, flag tags, orthographic Cf)"`
}

type fixInput struct {
	Text                 string `json:"text"                            jsonschema:"the text to scrub"`
	NormalizeSpaces      *bool  `json:"normalize_spaces,omitempty"      jsonschema:"rewrite exotic space homoglyphs to U+0020 (default true; non-intrusive)"`
	NFKC                 bool   `json:"nfkc,omitempty"                  jsonschema:"apply Unicode NFKC normalization after the scrub (risky: alters visible characters)"`
	AggressiveHomoglyphs bool   `json:"aggressive_homoglyphs,omitempty" jsonschema:"map Cyrillic/fullwidth Latin confusables to ASCII (risky)"`
	StripEmojiGlue       bool   `json:"strip_emoji_glue,omitempty"      jsonschema:"paranoid: strip load-bearing invisibles too (risky)"`
}

type fixOutput struct {
	CleanedText string      `json:"cleaned_text"`
	Stats       scrub.Stats `json:"stats"`
}

func registerScrubTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_lint",
		Description: "Report deterministic \"Layer A\" watermark carriers in a block of text: invisible/format Unicode (zero-width, bidi overrides, tag characters, variation selectors) and exotic space homoglyphs. " +
			"Reports what it finds without changing anything — use humanizer_fix to apply the scrub. " +
			"Load-bearing invisibles (emoji ZWJ/variation selectors after an emoji base, script joiners inside complex scripts, flag tags, orthographic Arabic/Syriac marks) are preserved and not flagged unless strip_emoji_glue is set. " +
			"Each hit carries a codepoint, kind, confidence (probable for edit-carriers, informational for spaces), count, and sample offsets. Offline and deterministic.",
	}, handleLint)

	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_fix",
		Description: "Apply the deterministic \"Layer A\" scrub and return the cleaned text plus stats. " +
			"Default (non-intrusive): strip zero-width/format controls, bidi overrides, tag characters and variation selectors, and normalize exotic spaces to U+0020 — none of which changes visible meaning. " +
			"Risky, visibly-altering transforms are opt-in: nfkc (Unicode NFKC), aggressive_homoglyphs (map confusable letters to ASCII), strip_emoji_glue (strip load-bearing invisibles). " +
			"Returns cleaned_text and stats; the caller writes the result. Offline and deterministic.",
	}, handleFix)
}

func handleLint(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in lintInput,
) (*mcp.CallToolResult, scrub.Report, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, scrub.Report{}, fmt.Errorf("humanizer_lint: text is required")
	}
	report := scrub.Inspect(in.Text, scrub.Options{
		AggressiveHomoglyphs: in.Aggressive,
		StripEmojiGlue:       in.StripEmojiGlue,
	})
	return nil, report, nil
}

func handleFix(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in fixInput,
) (*mcp.CallToolResult, fixOutput, error) {
	if in.Text == "" {
		return nil, fixOutput{}, fmt.Errorf("humanizer_fix: text is required")
	}
	normalizeSpaces := true
	if in.NormalizeSpaces != nil {
		normalizeSpaces = *in.NormalizeSpaces
	}
	cleaned, stats := scrub.Clean(in.Text, scrub.Options{
		NormalizeSpaces:      normalizeSpaces,
		NFKC:                 in.NFKC,
		AggressiveHomoglyphs: in.AggressiveHomoglyphs,
		StripEmojiGlue:       in.StripEmojiGlue,
	})
	return nil, fixOutput{CleanedText: cleaned, Stats: stats}, nil
}
