package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

type detectInput struct {
	Text        string   `json:"text"                   jsonschema:"the text to scan for AI-writing patterns"`
	Rules       []string `json:"rules,omitempty"        jsonschema:"restrict detection to these rule IDs (e.g. Humanizer.AIVocabulary); empty = all; use humanizer_rules_list to discover IDs"`
	MinSeverity string   `json:"min_severity,omitempty" jsonschema:"filter findings below this severity: suggestion, warning, or error (default suggestion)"`
}

type detectFileInput struct {
	Path        string   `json:"path"                   jsonschema:"absolute path to a file to lint"`
	Rules       []string `json:"rules,omitempty"        jsonschema:"restrict detection to these rule IDs; empty = all"`
	MinSeverity string   `json:"min_severity,omitempty" jsonschema:"filter findings below this severity: suggestion, warning, or error"`
}

type detectFinding struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name,omitempty"`
	Category string `json:"category,omitempty"`
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Match    string `json:"match,omitempty"`
	Message  string `json:"message"`
	Link     string `json:"link,omitempty"`
}

type detectSummary struct {
	Total      int            `json:"total"`
	BySeverity map[string]int `json:"by_severity"`
	ByCategory map[string]int `json:"by_category"`
	ByRule     map[string]int `json:"by_rule"`
}

type detectOutput struct {
	Findings []detectFinding `json:"findings"`
	Summary  detectSummary   `json:"summary"`
	Engine   string          `json:"engine"`
}

func registerDetectTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_detect",
		Description: "Scan a block of text for AI-writing patterns from Wikipedia's \"Signs of AI writing\" (em-dash overuse, smart quotes, filler phrases, sycophancy, signposting, bold overuse, AI vocabulary, promotional language, copula avoidance, and more). " +
			"Backed by the vale CLI running an embedded Humanizer style pack. " +
			"Returns findings with rule_id (e.g. Humanizer.AIVocabulary), severity (suggestion|warning|error), line, column, matched text, and message. " +
			"Use before rewriting AI-heavy text so the rewrite targets are concrete. " +
			"Filter with rules=[...] to check specific patterns, or min_severity=error for only the highest-confidence tells.",
	}, handleDetect)

	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_detect_file",
		Description: "Scan a file on disk for AI-writing patterns. Use instead of humanizer_detect when the text is already in a file — saves a read and lets vale use the real extension for format detection. " +
			"Takes an absolute path. Under the MCP sandbox only prose files (.md/.markdown/.txt) beneath ~/code/src and ~/workspace, plus /tmp paths, are readable — " +
			"for anything else pass the text via humanizer_detect. Returns the same findings shape as humanizer_detect.",
	}, handleDetectFile)
}

func handleDetect(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in detectInput,
) (*mcp.CallToolResult, detectOutput, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, detectOutput{}, fmt.Errorf("humanizer_detect: text is required")
	}
	for _, id := range in.Rules {
		if _, ok := rules.Get(id); !ok {
			return nil, detectOutput{}, fmt.Errorf(
				"humanizer_detect: unknown rule %q (use humanizer_rules_list to see valid IDs)",
				id,
			)
		}
	}
	findings, err := rules.Detect(ctx, in.Text, rules.DetectOptions{
		Rules:       in.Rules,
		MinSeverity: in.MinSeverity,
	})
	if err != nil {
		return nil, detectOutput{}, fmt.Errorf("humanizer_detect: %w", err)
	}
	return nil, buildDetectOutput(findings), nil
}

func handleDetectFile(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in detectFileInput,
) (*mcp.CallToolResult, detectOutput, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, detectOutput{}, fmt.Errorf("humanizer_detect_file: path is required")
	}
	for _, id := range in.Rules {
		if _, ok := rules.Get(id); !ok {
			return nil, detectOutput{}, fmt.Errorf("humanizer_detect_file: unknown rule %q", id)
		}
	}
	findings, err := rules.DetectFile(ctx, in.Path, rules.DetectOptions{
		Rules:       in.Rules,
		MinSeverity: in.MinSeverity,
	})
	if err != nil {
		return nil, detectOutput{}, fmt.Errorf(
			"humanizer_detect_file: %w",
			detectFileError(err, in.Path),
		)
	}
	return nil, buildDetectOutput(findings), nil
}

// detectFileError translates a sandbox permission denial into an error that
// tells the agent what is readable and what to do instead. The seatbelt
// profile (recipes/humanizer/humanizer.sb, ADR-0016) only exposes prose
// files under the code roots; everything else under $HOME is denied.
func detectFileError(err error, path string) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	return fmt.Errorf(
		"%s is not readable under the MCP sandbox (readable: .md/.markdown/.txt under ~/code/src and ~/workspace, and /tmp paths) — pass the text via humanizer_detect instead: %w",
		path,
		err,
	)
}

func buildDetectOutput(findings []rules.Finding) detectOutput {
	out := detectOutput{
		Findings: make([]detectFinding, 0, len(findings)),
		Engine:   "vale",
	}
	sum := detectSummary{
		BySeverity: map[string]int{},
		ByCategory: map[string]int{},
		ByRule:     map[string]int{},
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, detectFinding{
			RuleID:   f.RuleID,
			RuleName: f.Name,
			Category: f.Category,
			Severity: f.Severity,
			Line:     f.Line,
			Column:   f.Column,
			Match:    f.Match,
			Message:  f.Message,
			Link:     f.Link,
		})
		sum.Total++
		sum.BySeverity[f.Severity]++
		if f.Category != "" {
			sum.ByCategory[f.Category]++
		}
		sum.ByRule[f.RuleID]++
	}
	out.Summary = sum
	return out
}
