package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	factoryeval "github.com/mad01/thismoon/tools/factory-eval"
	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/outline"
)

func measureCmd() *cobra.Command {
	var casesPath, repo string
	c := &cobra.Command{
		Use:   "measure",
		Short: "Print every component's deviation from the outline and the humanizer pack",
		Long: `Clone the repo at the cases file's pinned commit into a throwaway dir and,
for every component with a CLAUDE.md and a README.md, count the outline
problems and the humanizer findings (ignored rules left out) in each file.
The table, sorted by total deviation, is the measured provenance behind the
cases file's choice of components. No model call.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := measure(cmd.Context(), cmd.OutOrStdout(), casesPath, repo)
			return agentdoc.Hint(err, factoryeval.Facts())
		},
	}
	c.Flags().StringVar(&casesPath, "cases", "cases.yaml", "cases file")
	c.Flags().StringVar(&repo, "repo", "", "repo to clone (default: the cases file's)")
	return c
}

// deviation is one component's distance from the outline and the pack.
type deviation struct {
	component string
	outlines  map[string][]outline.Problem
	findings  map[string]map[string]int
}

func (d deviation) total() int {
	n := 0
	for _, ps := range d.outlines {
		n += len(ps)
	}
	for _, counts := range d.findings {
		for _, c := range counts {
			n += c
		}
	}
	return n
}

func measure(ctx context.Context, w io.Writer, casesPath, repo string) (err error) {
	f, err := cases.Load(casesPath)
	if err != nil {
		return err
	}
	source := expandHome(f.Repo)
	if repo != "" {
		source = expandHome(repo)
	}
	co, err := checkout.Create(ctx, source, f.Commit, "")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, co.Remove()) }()
	d := judge.Deterministic{Outline: f.Outline, Humanizer: f.Humanizer, Cmd: judge.ExecCommander{}}
	var rows []deviation
	for _, comp := range components(co.Repo) {
		row, merr := measureComponent(ctx, d, co.Repo, comp)
		if merr != nil {
			return merr
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].total() > rows[j].total() })
	printDeviations(w, f.Commit, rows)
	return nil
}

// components lists every component dir that carries both docs.
func components(repo string) []string {
	var out []string
	for _, pattern := range []string{"services/*/Makefile", "tools/*/Makefile"} {
		matches, _ := filepath.Glob(filepath.Join(repo, pattern))
		for _, m := range matches {
			dir := filepath.Dir(m)
			if exists(filepath.Join(dir, "CLAUDE.md")) && exists(filepath.Join(dir, "README.md")) {
				rel, _ := filepath.Rel(repo, dir)
				out = append(out, rel)
			}
		}
	}
	return out
}

func measureComponent(
	ctx context.Context,
	d judge.Deterministic,
	repo, comp string,
) (deviation, error) {
	row := deviation{
		component: comp,
		outlines:  map[string][]outline.Problem{},
		findings:  map[string]map[string]int{},
	}
	for _, file := range []string{"CLAUDE.md", "README.md"} {
		rel := comp + "/" + file
		problems, err := d.OutlineProblems(repo, rel, filepath.Base(comp))
		if err != nil {
			return row, err
		}
		counts, err := d.Findings(ctx, repo, rel)
		if err != nil {
			return row, err
		}
		row.outlines[file], row.findings[file] = problems, counts
	}
	return row, nil
}

func printDeviations(w io.Writer, commit string, rows []deviation) {
	fmt.Fprintf(
		w,
		"deviation at %s (outline problems + humanizer findings, ignored rules left out)\n\n",
		commit,
	)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(
		tw,
		"component\tCLAUDE outline\tCLAUDE humanizer\tREADME outline\tREADME humanizer\ttotal",
	)
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\n", r.component,
			len(r.outlines["CLAUDE.md"]), sum(r.findings["CLAUDE.md"]),
			len(r.outlines["README.md"]), sum(r.findings["README.md"]), r.total())
	}
	_ = tw.Flush() // the rows were written to w; a flush error repeats a write error
	fmt.Fprintln(w)
	for _, r := range rows {
		for _, file := range []string{"CLAUDE.md", "README.md"} {
			if detail := describe(r.outlines[file], r.findings[file]); detail != "" {
				fmt.Fprintf(w, "%s/%s: %s\n", r.component, file, detail)
			}
		}
	}
}

func describe(problems []outline.Problem, counts map[string]int) string {
	var parts []string
	for _, p := range problems {
		parts = append(parts, p.String())
	}
	rules := make([]string, 0, len(counts))
	for rule := range counts {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	for _, rule := range rules {
		parts = append(
			parts,
			fmt.Sprintf("%s=%d", strings.TrimPrefix(rule, "Humanizer."), counts[rule]),
		)
	}
	return strings.Join(parts, "; ")
}

func sum(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}
