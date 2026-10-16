package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
)

// VerdictSchema is the structured output the judge call must return.
const VerdictSchema = `{"type":"object","properties":{` +
	`"score":{"type":"integer","minimum":0,"maximum":10},` +
	`"reasons":{"type":"array","items":{"type":"string"},"minItems":1},` +
	`"do_not_fail_applied":{"type":"array","items":{"type":"string"}}},` +
	`"required":["score","reasons"]}`

// Runner makes one headless claude call.
type Runner interface {
	Run(ctx context.Context, inv claude.Invocation) (claude.Output, error)
}

// Scorer makes the one LLM judge call per case.
type Scorer struct {
	Runner Runner
	Judge  cases.Judge
	// Dir is the call's working dir: an empty dir, so no repo CLAUDE.md
	// reaches the judge.
	Dir string
}

// FilePair is one doc before and after the sweep.
type FilePair struct {
	Path   string
	Before string
	After  string
}

// Doc is one glossary file as it reads at the pinned commit.
type Doc struct {
	Path    string
	Content string
}

// Request is everything one judge call grades: the component's docs before
// and after, and the glossaries the case names.
type Request struct {
	Component string
	Files     []FilePair
	Glossary  []Doc
}

// Verdict is the judge's score for one case.
type Verdict struct {
	Score            int      `json:"score"`
	Reasons          []string `json:"reasons"`
	DoNotFailApplied []string `json:"do_not_fail_applied,omitempty"`
	CostUSD          float64  `json:"cost_usd"`
}

// Score asks the judge model for a 0 to 10 score on the request's docs.
func (s Scorer) Score(ctx context.Context, req Request) (Verdict, error) {
	out, err := s.Runner.Run(ctx, claude.Invocation{
		Dir:   s.Dir,
		Args:  claude.JudgeArgs(s.Judge.Model, s.Judge.BudgetUSD, VerdictSchema),
		Stdin: Prompt(s.Judge, req),
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: %s: %w", req.Component, err)
	}
	rep, err := claude.ParseReport(out.Stdout)
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: %s: %w: %s",
			req.Component, err, tail(string(out.Stderr), 3))
	}
	v, err := parseVerdict(rep)
	v.CostUSD = rep.TotalCostUSD
	if err != nil {
		return v, fmt.Errorf("judge: %s: %w", req.Component, err)
	}
	return v, nil
}

func parseVerdict(rep claude.Report) (Verdict, error) {
	if !rep.Finished() {
		return Verdict{}, fmt.Errorf(
			"call ended %s: %s",
			rep.Subtype,
			strings.Join(rep.Errors, "; "),
		)
	}
	if len(rep.StructuredOutput) == 0 {
		return Verdict{}, errors.New("no structured output")
	}
	var v Verdict
	if err := json.Unmarshal(rep.StructuredOutput, &v); err != nil {
		return Verdict{}, fmt.Errorf("verdict: %w", err)
	}
	if v.Score < 0 || v.Score > 10 || len(v.Reasons) == 0 {
		return Verdict{}, fmt.Errorf("verdict score %d with %d reasons is out of contract",
			v.Score, len(v.Reasons))
	}
	return v, nil
}

// Prompt is the judge's whole input: the rubric, the do-not-fail list, the
// case's glossary files and the glossary notes, and each file before and
// after the sweep. The judge sees nothing else, so everything it grades
// against is in here.
func Prompt(j cases.Judge, req Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You grade the documentation of the %s component after a docs sweep.\n\n",
		req.Component)
	fmt.Fprintf(&b, "<rubric>\n%s\n</rubric>\n\n", strings.TrimSpace(j.Rubric))
	b.WriteString("<do_not_fail>\nNever take points off for any of these:\n")
	for _, item := range j.DoNotFail {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	b.WriteString("</do_not_fail>\n\n")
	for _, g := range req.Glossary {
		fmt.Fprintf(&b, "<glossary path=%q>\n%s</glossary>\n\n", g.Path, g.Content)
	}
	if notes := strings.TrimSpace(j.GlossaryNotes); notes != "" {
		fmt.Fprintf(&b, "<glossary_notes>\n%s\n</glossary_notes>\n\n", notes)
	}
	for _, f := range req.Files {
		fmt.Fprintf(&b, "<file path=%q version=\"before\">\n%s</file>\n\n", f.Path, f.Before)
		fmt.Fprintf(&b, "<file path=%q version=\"after\">\n%s</file>\n\n", f.Path, f.After)
	}
	b.WriteString("Return the score from 0 to 10, one reason per point you took off " +
		"(or one reason for a 10), and the do-not-fail items you applied.\n")
	return b.String()
}
