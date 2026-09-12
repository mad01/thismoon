package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

type ruleSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	DefaultSeverity string `json:"default_severity"`
	Summary         string `json:"summary"`
}

type rulesListInput struct {
	Category string `json:"category,omitempty" jsonschema:"optional: filter by category (content, language, style, communication)"`
}

type rulesListOutput struct {
	Rules []ruleSummary `json:"rules"`
	Total int           `json:"total"`
}

type rulesExplainInput struct {
	RuleID string `json:"rule_id" jsonschema:"the rule ID to explain (e.g. Humanizer.AIVocabulary)"`
}

type rulesExplainOutput struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	DefaultSeverity string `json:"default_severity"`
	Summary         string `json:"summary"`
	Rationale       string `json:"rationale,omitempty"`
	Before          string `json:"before,omitempty"`
	After           string `json:"after,omitempty"`
	Reference       string `json:"reference,omitempty"`
	File            string `json:"file,omitempty"`
}

func registerRulesTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_rules_list",
		Description: "List every supported humanizer detection rule with its ID, category, default severity, and a one-line summary. " +
			"Use to discover what humanizer_detect can find, or to build a targeted rules=[...] filter. " +
			"Categories: content (significance inflation, promo language), language (AI vocab, negative parallelism), style (em-dashes, bold), communication (sycophancy, filler).",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleRulesList)

	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_rules_explain",
		Description: "Return full metadata for a single humanizer rule: rationale, before/after example, and reference. " +
			"Use when a humanizer_detect finding is ambiguous or you need to explain a rewrite choice to the user. " +
			"Rule IDs come from humanizer_rules_list (e.g. Humanizer.Sycophancy).",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleRulesExplain)
}

func handleRulesList(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in rulesListInput,
) (*mcp.CallToolResult, rulesListOutput, error) {
	all := rules.All()
	out := rulesListOutput{Rules: make([]ruleSummary, 0, len(all))}
	cat := strings.ToLower(strings.TrimSpace(in.Category))
	for _, r := range all {
		if cat != "" && strings.ToLower(r.Category) != cat {
			continue
		}
		out.Rules = append(out.Rules, ruleSummary{
			ID:              r.ID,
			Name:            r.Name,
			Category:        r.Category,
			DefaultSeverity: r.DefaultSeverity,
			Summary:         r.Summary,
		})
	}
	out.Total = len(out.Rules)
	return nil, out, nil
}

func handleRulesExplain(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in rulesExplainInput,
) (*mcp.CallToolResult, rulesExplainOutput, error) {
	if strings.TrimSpace(in.RuleID) == "" {
		return nil, rulesExplainOutput{}, fmt.Errorf("humanizer_rules_explain: rule_id is required")
	}
	r, ok := rules.Get(in.RuleID)
	if !ok {
		return nil, rulesExplainOutput{}, fmt.Errorf(
			"humanizer_rules_explain: unknown rule %q",
			in.RuleID,
		)
	}
	return nil, rulesExplainOutput{
		ID:              r.ID,
		Name:            r.Name,
		Category:        r.Category,
		DefaultSeverity: r.DefaultSeverity,
		Summary:         r.Summary,
		Rationale:       r.Rationale,
		Before:          r.Before,
		After:           r.After,
		Reference:       r.Reference,
		File:            r.File,
	}, nil
}
