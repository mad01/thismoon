package eval

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/outline"
)

// ReportSchema is the final report every arm session must return as its
// structured output. The harness records it beside the checks, but the
// checks, not the report, decide pass or fail.
const ReportSchema = `{"type":"object","properties":{"cases":{"type":"array","items":` +
	`{"type":"object","properties":{"component":{"type":"string"},` +
	`"branch":{"type":"string"},"status":{"type":"string","enum":["done","blocked"]},` +
	`"commit":{"type":"string"},"note":{"type":"string"}},` +
	`"required":["component","branch","status"]}}},"required":["cases"]}`

// PromptInput is what an arm's preamble template sees.
type PromptInput struct {
	Repo      string
	Worktrees string
	Commit    string
	Model     string
	Effort    string
	Worker    string
}

// ArmPrompt is an arm session's whole prompt: the arm's preamble, the
// shared outline and checks, then every case's task in the cases file's
// wording, identical for both arms.
func ArmPrompt(f *cases.File, arm cases.Arm, in PromptInput) (string, error) {
	tmpl, err := template.New("preamble").Option("missingkey=error").Parse(arm.Preamble)
	if err != nil {
		return "", fmt.Errorf("eval: parse preamble: %w", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, in); err != nil {
		return "", fmt.Errorf("eval: render preamble: %w", err)
	}
	b.WriteString("\n")
	b.WriteString(sharedBlock(f))
	for i, c := range f.Cases {
		b.WriteString(caseBlock(i+1, len(f.Cases), c))
	}
	return b.String(), nil
}

func sharedBlock(f *cases.File) string {
	var b strings.Builder
	b.WriteString("## The shared outline\n\n")
	b.WriteString(outlineText("CLAUDE.md", f.Outline.ClaudeMD))
	b.WriteString(outlineText("README.md", f.Outline.Readme))
	b.WriteString("A section the outline does not list moves under the nearest listed " +
		"section as a third-level heading.\n\n")
	b.WriteString("## What the harness checks on every case\n\n")
	fmt.Fprintf(&b, "- `humanizer detect --min-severity %s` finds nothing in either file, "+
		"apart from these ignored rules: %s.\n", f.Humanizer.MinSeverity,
		strings.Join(f.Humanizer.IgnoreRules, ", "))
	b.WriteString("- The headings of both files match the outline above.\n")
	b.WriteString("- `make -C <component> build test` passes on the branch.\n")
	b.WriteString("- The branch changes only the allowed paths.\n\n")
	return b.String()
}

func outlineText(file string, spec outline.Spec) string {
	var b strings.Builder
	first := "`# <name>`"
	if spec.Title == outline.TitleNameComma {
		first = "`# <name>, <purpose>`"
	}
	fmt.Fprintf(&b, "%s: the first line is %s, then these second-level sections in this "+
		"order:\n\n", file, first)
	for i, s := range spec.Sections {
		mark := ""
		if s.Required {
			mark = " (required)"
		}
		fmt.Fprintf(&b, "%d. `## %s`%s\n", i+1, s.Heading, mark)
	}
	b.WriteString("\n")
	return b.String()
}

func caseBlock(n, total int, c cases.Case) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Case %d of %d: %s\n\n", n, total, c.Component)
	fmt.Fprintf(&b, "Branch: %s\n\nAllowed paths:\n", c.Branch)
	for _, p := range c.AllowedPaths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	fmt.Fprintf(
		&b,
		"\n%s\n\nDone when: %s\n\n",
		strings.TrimSpace(c.Task),
		strings.TrimSpace(c.Done),
	)
	return b.String()
}
