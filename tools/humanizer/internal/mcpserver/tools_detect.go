package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/glossary"
	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

type detectInput struct {
	Text        string   `json:"text"                   jsonschema:"the text to scan for AI-writing patterns"`
	Rules       []string `json:"rules,omitempty"        jsonschema:"restrict detection to these rule IDs (e.g. Humanizer.AIVocabulary); empty = all; use humanizer_rules_list to discover IDs"`
	MinSeverity string   `json:"min_severity,omitempty" jsonschema:"filter findings below this severity: suggestion, warning, or error (default suggestion)"`
	Profile     string   `json:"profile,omitempty"      jsonschema:"experimental: run the STE-lite rules instead of the Humanizer pack: procedure for task briefs and steps, description for status reports and summaries; empty runs the Humanizer pack"`
	Repo        string   `json:"repo,omitempty"         jsonschema:"experimental: absolute repo path whose CONTEXT.md glossaries become STE.Glossary substitutions at suggestion level; needs profile"`
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

type detectOutput struct {
	Findings []detectFinding `json:"findings"`
	Summary  rules.Summary   `json:"summary"`
	Engine   string          `json:"engine"`
}

func registerDetectTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_detect",
		Description: "Scan a block of text for AI-writing patterns from Wikipedia's \"Signs of AI writing\" (em-dash overuse, smart quotes, filler phrases, sycophancy, signposting, bold overuse, AI vocabulary, promotional language, copula avoidance, and more). " +
			"Backed by the vale CLI running an embedded Humanizer style pack. " +
			"Returns findings with rule_id (e.g. Humanizer.AIVocabulary), severity (suggestion|warning|error), line, column, matched text, and message. " +
			"Use before rewriting AI-heavy text so the rewrite targets are concrete. " +
			"Filter with rules=[...] to check specific patterns, or min_severity=error for only the highest-confidence tells. " +
			"To check a message one agent writes for another (experimental), pass profile=procedure (task briefs, steps) or profile=description (status reports): that runs the STE-lite rules (STE.* IDs, shaped on Simplified Technical English) instead of the Humanizer pack. " +
			"Add repo=<absolute path> to flag the synonyms the repo's CONTEXT.md glossaries ban.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, handleDetect)

	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_detect_file",
		Description: "Scan a file on disk for AI-writing patterns. Use instead of humanizer_detect when the text is already in a file: saves a read and lets vale use the real extension for format detection. " +
			"Takes an absolute path. Under the MCP sandbox only prose files (.md/.markdown/.txt) and Go sources (.go) beneath the sandbox profile's workspace roots, plus /tmp paths, are readable; this tool still only makes sense on prose, so use humanizer_scan_go for Go source. " +
			"For anything else pass the text via humanizer_detect. Returns the same findings shape as humanizer_detect.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
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
	opts, err := detectOptions(in)
	if err != nil {
		return nil, detectOutput{}, fmt.Errorf("humanizer_detect: %w", err)
	}
	findings, err := rules.Detect(ctx, in.Text, opts)
	if err != nil {
		return nil, detectOutput{}, fmt.Errorf("humanizer_detect: %w", err)
	}
	return nil, buildDetectOutput(findings), nil
}

// detectOptions validates the humanizer_detect input and resolves it into
// detect options, generating the repo glossary when repo is set.
func detectOptions(in detectInput) (rules.DetectOptions, error) {
	for _, id := range in.Rules {
		if _, ok := rules.Get(id); !ok {
			return rules.DetectOptions{}, fmt.Errorf(
				"unknown rule %q (use humanizer_rules_list to see valid IDs)",
				id,
			)
		}
	}
	opts := rules.DetectOptions{Rules: in.Rules, MinSeverity: in.MinSeverity}
	if in.Profile == "" {
		if in.Repo != "" {
			return rules.DetectOptions{}, fmt.Errorf(
				"repo needs profile (procedure or description)",
			)
		}
		return opts, nil
	}
	profile, err := rules.ParseProfile(in.Profile)
	if err != nil {
		return rules.DetectOptions{}, err
	}
	opts.Profile = profile
	if in.Repo != "" {
		if !filepath.IsAbs(in.Repo) {
			return rules.DetectOptions{}, fmt.Errorf(
				"repo must be an absolute path, got %q",
				in.Repo,
			)
		}
		gloss, err := glossary.FromRepo(in.Repo)
		if err != nil {
			return rules.DetectOptions{}, repoGlossaryError(err, in.Repo, profile)
		}
		opts.Glossary = gloss
	}
	return opts, nil
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

// detectFileError translates a sandbox permission denial into a structured
// error with a machine-readable use_instead field. The seatbelt profile
// (recipes/humanizer/humanizer.sb, ADR-0016) only exposes prose files under
// the code roots; everything else under $HOME is denied — including every
// sibling of a denied path, so the hint says to switch tools, not retry
// with the next file from the same directory.
func detectFileError(err error, path string) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	redirect, _ := json.Marshal(struct {
		Error      string `json:"error"`
		UseInstead string `json:"use_instead"`
		Hint       string `json:"hint"`
	}{
		Error:      path + " is not readable under the MCP sandbox",
		UseInstead: "humanizer_detect",
		Hint: "read the file yourself and pass its content as the text param to humanizer_detect. " +
			"Do not retry humanizer_detect_file on other paths in the same directory — the whole directory is outside the sandbox. " +
			"Readable here: .md/.markdown/.txt under the sandbox profile's workspace roots, and /tmp paths.",
	})
	return fmt.Errorf("%s — use humanizer_detect instead: %w", redirect, err)
}

// repoGlossaryError translates a sandbox denial on the repo walk into a
// structured error whose use_instead field names the unsandboxed CLI form.
// Which roots the sandbox lets the walk read is the consuming repo's choice.
func repoGlossaryError(err error, repo string, profile rules.Profile) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	redirect, _ := json.Marshal(struct {
		Error      string `json:"error"`
		UseInstead string `json:"use_instead"`
		Hint       string `json:"hint"`
	}{
		Error:      "the CONTEXT.md files under " + repo + " are not readable under the MCP sandbox",
		UseInstead: "humanizer ste --profile " + string(profile) + " --repo " + repo,
		Hint: "run the CLI from a shell, which is unsandboxed, and pipe the text on stdin. " +
			"Or call humanizer_detect again without repo to run the STE rules without the glossary.",
	})
	return fmt.Errorf("%s: %w", redirect, err)
}

func buildDetectOutput(findings []rules.Finding) detectOutput {
	out := detectOutput{
		Findings: make([]detectFinding, 0, len(findings)),
		Summary:  rules.Summarize(findings),
		Engine:   "vale",
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
	}
	return out
}
