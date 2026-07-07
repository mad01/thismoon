package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/voice"
)

type detectStatisticalInput struct {
	Text string `json:"text" jsonschema:"the text to scan for statistical AI-writing signals; 200+ words gives the most reliable verdict"`
}

type statFinding struct {
	RuleID    string  `json:"rule_id"`
	RuleName  string  `json:"rule_name"`
	Category  string  `json:"category"`
	Severity  string  `json:"severity"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Message   string  `json:"message"`
}

type detectStatisticalOutput struct {
	Findings []statFinding `json:"findings"`
	Profile  voice.Profile `json:"profile"`
	Summary  detectSummary `json:"summary"`
	Engine   string        `json:"engine"`
}

func registerStatisticalTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_detect_statistical",
		Description: "Scan text for statistical AI-writing signals that span-based rules miss: robotically uniform sentence length, low contraction rate, missing semicolons in long text, low type-token ratio, any em-dash in short text, heading-heavy outline scaffolding, and anaphora (3+ consecutive sentences with the same opening word). " +
			"Complements humanizer_detect (Vale span rules) — run both for full coverage. " +
			"Returns findings (rule_id, severity, metric, value, threshold, message) plus the full voice profile the checks were computed from. " +
			"Most checks gate on a minimum sample size, so very short snippets return few or no findings.",
	}, handleDetectStatistical)
}

func handleDetectStatistical(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in detectStatisticalInput,
) (*mcp.CallToolResult, detectStatisticalOutput, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, detectStatisticalOutput{}, fmt.Errorf(
			"humanizer_detect_statistical: text is required",
		)
	}
	findings := voice.DetectStatistical(in.Text)
	out := detectStatisticalOutput{
		Findings: make([]statFinding, 0, len(findings)),
		Profile:  voice.Compute(in.Text),
		Engine:   "statistical",
	}
	sum := detectSummary{
		BySeverity: map[string]int{},
		ByCategory: map[string]int{},
		ByRule:     map[string]int{},
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, statFinding{
			RuleID:    f.RuleID,
			RuleName:  f.Name,
			Category:  f.Category,
			Severity:  f.Severity,
			Metric:    f.Metric,
			Value:     f.Value,
			Threshold: f.Threshold,
			Message:   f.Message,
		})
		sum.Total++
		sum.BySeverity[f.Severity]++
		if f.Category != "" {
			sum.ByCategory[f.Category]++
		}
		sum.ByRule[f.RuleID]++
	}
	out.Summary = sum
	return nil, out, nil
}
