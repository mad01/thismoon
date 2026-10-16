// Package judge scores one case of one run. Deterministic checks come
// first: the branch exists, it edits only the allowed paths, both docs
// follow the outline, humanizer finds nothing at the configured severity,
// and the component builds and tests green. Then one LLM judge call scores
// prose quality against a fixed rubric and a do-not-fail list.
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
	"github.com/mad01/thismoon/tools/factory-eval/internal/outline"
)

// Check names, in the order every case reports them.
const (
	CheckBranch       = "branch"
	CheckAllowedPaths = "allowed_paths"
	CheckOutline      = "outline"
	CheckHumanizer    = "humanizer"
	CheckBuildTest    = "build_test"
)

// Check is one deterministic check's outcome.
type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// AllPass reports whether every check passed.
func AllPass(checks []Check) bool {
	for _, c := range checks {
		if !c.Pass {
			return false
		}
	}
	return len(checks) > 0
}

// Commander runs an external command in dir and returns its stdout. A
// non-zero exit is an error that carries the command's output.
type Commander interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// Deterministic runs the checks that need no model.
type Deterministic struct {
	Outline   cases.Outline
	Humanizer cases.Humanizer
	Cmd       Commander
}

// TargetFiles are the two docs every case brings to the outline.
func TargetFiles(c cases.Case) []string {
	return []string{c.Component + "/CLAUDE.md", c.Component + "/README.md"}
}

// Run checks one case's branch in co and returns all five checks in order.
// A missing branch fails every check, so each case always reports the same
// five names.
func (d Deterministic) Run(ctx context.Context, co *checkout.Checkout, c cases.Case) []Check {
	if !co.BranchExists(ctx, c.Branch) {
		return failAll("branch " + c.Branch + " missing")
	}
	changed, err := co.ChangedFiles(ctx, c.Branch)
	if err != nil {
		return failAll(err.Error())
	}
	dir, remove, err := co.Worktree(ctx, c.Branch)
	if err != nil {
		return failAll(err.Error())
	}
	defer remove()
	return []Check{
		{Name: CheckBranch, Pass: true, Detail: fmt.Sprintf("%d files changed", len(changed))},
		allowedPaths(changed, c.AllowedPaths),
		d.outlineCheck(dir, c),
		d.humanizerCheck(ctx, dir, c, changed),
		d.buildTest(ctx, dir, c.Component),
	}
}

func failAll(detail string) []Check {
	names := []string{CheckBranch, CheckAllowedPaths, CheckOutline, CheckHumanizer, CheckBuildTest}
	checks := make([]Check, len(names))
	for i, n := range names {
		checks[i] = Check{Name: n, Detail: detail}
	}
	return checks
}

func allowedPaths(changed, allowed []string) Check {
	var outside []string
	for _, f := range changed {
		if !slices.Contains(allowed, f) {
			outside = append(outside, f)
		}
	}
	if len(outside) > 0 {
		return Check{
			Name:   CheckAllowedPaths,
			Detail: "outside allowlist: " + strings.Join(outside, ", "),
		}
	}
	return Check{Name: CheckAllowedPaths, Pass: true}
}

func (d Deterministic) outlineCheck(dir string, c cases.Case) Check {
	var details []string
	for _, rel := range TargetFiles(c) {
		problems, err := d.OutlineProblems(dir, rel, c.Name())
		switch {
		case err != nil:
			details = append(details, err.Error())
		case len(problems) > 0:
			details = append(details, fmt.Sprintf("%s: %s", rel, joinProblems(problems)))
		}
	}
	if len(details) > 0 {
		return Check{Name: CheckOutline, Detail: strings.Join(details, "; ")}
	}
	return Check{Name: CheckOutline, Pass: true}
}

// OutlineProblems reads rel under dir and checks it against the outline
// for its file name: CLAUDE.md or README.md.
func (d Deterministic) OutlineProblems(dir, rel, component string) ([]outline.Problem, error) {
	spec := d.Outline.Readme
	if path.Base(rel) == "CLAUDE.md" {
		spec = d.Outline.ClaudeMD
	}
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return nil, fmt.Errorf("judge: read %s: %w", rel, err)
	}
	return outline.Check(string(raw), component, spec), nil
}

func joinProblems(ps []outline.Problem) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

func (d Deterministic) humanizerCheck(ctx context.Context, dir string, c cases.Case,
	changed []string,
) Check {
	var details []string
	for _, rel := range proseFiles(dir, c, changed) {
		counts, err := d.Findings(ctx, dir, rel)
		switch {
		case err != nil:
			details = append(details, err.Error())
		case len(counts) > 0:
			details = append(details, fmt.Sprintf("%s: %s", rel, formatCounts(counts)))
		}
	}
	if len(details) > 0 {
		return Check{Name: CheckHumanizer, Detail: strings.Join(details, "; ")}
	}
	return Check{Name: CheckHumanizer, Pass: true}
}

// proseFiles is the two target docs plus every other changed markdown file
// that still exists on the branch.
func proseFiles(dir string, c cases.Case, changed []string) []string {
	files := TargetFiles(c)
	for _, f := range changed {
		if slices.Contains(files, f) || !strings.HasSuffix(f, ".md") {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			files = append(files, f)
		}
	}
	return files
}

type detectReport struct {
	Findings []struct {
		RuleID string `json:"rule_id"`
	} `json:"findings"`
}

// Findings runs humanizer detect on rel under dir at the configured
// severity and counts the findings per rule, ignored rules left out.
func (d Deterministic) Findings(ctx context.Context, dir, rel string) (map[string]int, error) {
	out, err := d.Cmd.Run(ctx, dir, "humanizer", "detect", "--json",
		"--min-severity", d.Humanizer.MinSeverity, rel)
	if err != nil {
		return nil, fmt.Errorf("judge: humanizer %s: %w", rel, err)
	}
	var rep detectReport
	if err := json.Unmarshal(out, &rep); err != nil {
		return nil, fmt.Errorf("judge: humanizer %s output: %w", rel, err)
	}
	counts := map[string]int{}
	for _, f := range rep.Findings {
		if !slices.Contains(d.Humanizer.IgnoreRules, f.RuleID) {
			counts[f.RuleID]++
		}
	}
	return counts, nil
}

func formatCounts(counts map[string]int) string {
	rules := make([]string, 0, len(counts))
	for r := range counts {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	parts := make([]string, len(rules))
	for i, r := range rules {
		parts[i] = fmt.Sprintf("%s=%d", strings.TrimPrefix(r, "Humanizer."), counts[r])
	}
	return strings.Join(parts, ", ")
}

func (d Deterministic) buildTest(ctx context.Context, dir, component string) Check {
	if _, err := d.Cmd.Run(ctx, dir, "make", "-C", component, "build", "test"); err != nil {
		return Check{Name: CheckBuildTest, Detail: lastLine(err.Error())}
	}
	return Check{Name: CheckBuildTest, Pass: true}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
