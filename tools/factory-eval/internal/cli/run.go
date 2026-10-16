package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	factoryeval "github.com/mad01/thismoon/tools/factory-eval"
	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/dryrun"
	"github.com/mad01/thismoon/tools/factory-eval/internal/eval"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

type runOptions struct {
	casesPath string
	dryRun    bool
	arms      []string
	tiers     []string
	runs      int
	out       string
	repo      string
	tempDir   string
}

func runCmd() *cobra.Command {
	var o runOptions
	c := &cobra.Command{
		Use:   "run",
		Short: "Run the matrix (paid), or rehearse it with --dry-run (no model call)",
		Long: `Run every arm at every tier, runs times each, and write one results file.

A paid run needs a signed-off cases file, every prerequisite (the factory arm
also needs the factory skill, the three worker definitions and the belt hook),
and a worst-case cost within the cases file's batch ceiling. A dry run checks
and prints all of that, then rehearses one run per arm and tier with stand-ins
for every model call: same checkouts, same checks, same results file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return agentdoc.Hint(runBatch(cmd.Context(), cmd.OutOrStdout(), o), factoryeval.Facts())
		},
	}
	c.Flags().StringVar(&o.casesPath, "cases", "cases.yaml", "cases file")
	c.Flags().BoolVar(&o.dryRun, "dry-run", false, "rehearse with stand-ins, no model call")
	c.Flags().StringSliceVar(&o.arms, "arms", []string{eval.ArmSingle, eval.ArmFactory},
		"arms to run: single, factory")
	c.Flags().StringSliceVar(&o.tiers, "tiers", nil, "tiers to run (default: every tier)")
	c.Flags().
		IntVar(&o.runs, "runs", 0, "runs per arm and tier (default: the cases file's, 1 in a dry run)")
	c.Flags().StringVar(&o.out, "out", "results", "directory the results file goes in")
	c.Flags().StringVar(&o.repo, "repo", "", "repo to clone (default: the cases file's)")
	c.Flags().
		StringVar(&o.tempDir, "temp-dir", "", "parent of the throwaway checkouts (default: system temp)")
	return c
}

// batch is a run's resolved plan: what runs, at what cost, against what.
type batch struct {
	file    *cases.File
	arms    []string
	tiers   []cases.Tier
	runs    int
	source  string
	version string
	common  []prereq.Result
	factory []prereq.Result
}

func (b batch) prereqs() []prereq.Result { return append(slices.Clone(b.common), b.factory...) }

func runBatch(ctx context.Context, w io.Writer, o runOptions) (err error) {
	b, err := plan(ctx, o)
	if err != nil {
		return err
	}
	printPlan(w, b, o.dryRun)
	if !o.dryRun {
		if err := gate(b); err != nil {
			return err
		}
	}
	judgeDir, err := os.MkdirTemp(o.tempDir, "factory-eval-judge-")
	if err != nil {
		return fmt.Errorf("judge dir: %w", err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(judgeDir)) }()
	res := eval.Run(ctx, config(b, o, w, judgeDir))
	path, err := writeResults(o.out, res)
	if err != nil {
		return err
	}
	decision := res.Summary.Decision
	if res.DryRun {
		decision = "dry run, every number is a stand-in: " + decision
	}
	fmt.Fprintf(w, "\n%s\nresults: %s\n", decision, path)
	return nil
}

func plan(ctx context.Context, o runOptions) (batch, error) {
	f, err := cases.Load(o.casesPath)
	if err != nil {
		return batch{}, err
	}
	b := batch{file: f, runs: o.runs, source: expandHome(f.Repo)}
	if o.repo != "" {
		b.source = expandHome(o.repo)
	}
	if b.arms, err = parseArms(o.arms); err != nil {
		return batch{}, err
	}
	if b.tiers, err = selectTiers(f, o.tiers); err != nil {
		return batch{}, err
	}
	if b.runs == 0 {
		b.runs = f.Runs
		if o.dryRun {
			b.runs = 1
		}
	}
	env := hostEnv()
	_, b.version, _ = claude.Exec{}.Version(ctx)
	b.common = prereq.Common(ctx, env)
	if slices.Contains(b.arms, eval.ArmFactory) {
		b.factory = prereq.Factory(env)
	}
	return b, nil
}

func hostEnv() prereq.Env {
	home, _ := os.UserHomeDir()
	return prereq.Env{Home: home, LookPath: exec.LookPath, ClaudeVersion: claude.Exec{}.Version}
}

func parseArms(arms []string) ([]string, error) {
	for _, a := range arms {
		if a != eval.ArmSingle && a != eval.ArmFactory {
			return nil, fmt.Errorf("arm %q unknown, want single or factory", a)
		}
	}
	if len(arms) == 0 {
		return nil, errors.New("no arms to run")
	}
	return arms, nil
}

func selectTiers(f *cases.File, names []string) ([]cases.Tier, error) {
	if len(names) == 0 {
		return f.Tiers, nil
	}
	var tiers []cases.Tier
	for _, n := range names {
		i := slices.IndexFunc(f.Tiers, func(t cases.Tier) bool { return t.Name == n })
		if i < 0 {
			return nil, fmt.Errorf("tier %q is not in the cases file", n)
		}
		tiers = append(tiers, f.Tiers[i])
	}
	return tiers, nil
}

// gate refuses a paid run before it spends anything: no sign-off, a
// missing prerequisite, or a worst case above the batch ceiling.
func gate(b batch) error {
	if !b.file.SignOff.Signed() {
		return errors.New("the cases file is not signed off: run --dry-run, show the user " +
			"the sign-off digest, and set sign_off.by and sign_off.date once they approve")
	}
	if failed := prereq.Failed(b.common); len(failed) > 0 {
		return errors.New(prereq.Explain("every", failed))
	}
	if failed := prereq.Failed(b.factory); len(failed) > 0 {
		return errors.New(prereq.Explain(eval.ArmFactory, failed))
	}
	worst := eval.WorstCaseUSD(b.file, len(b.arms), b.tiers, b.runs)
	if worst > b.file.BatchCeilingUSD {
		return fmt.Errorf(
			"worst case $%.2f exceeds the batch ceiling $%.2f: run fewer runs or tiers",
			worst,
			b.file.BatchCeilingUSD,
		)
	}
	return nil
}

func config(b batch, o runOptions, w io.Writer, judgeDir string) eval.Config {
	cfg := eval.Config{
		Cases: b.file, CasesPath: o.casesPath, Source: b.source, Arms: b.arms, Tiers: b.tiers,
		Runs: b.runs, TempParent: o.tempDir, DryRun: o.dryRun, ClaudeVersion: b.version,
		Prereqs: b.prereqs(), Now: time.Now, Progress: w,
		Checker: judge.Deterministic{
			Outline: b.file.Outline, Humanizer: b.file.Humanizer, Cmd: judge.ExecCommander{},
		},
	}
	var judgeRunner judge.Runner = claude.Exec{}
	cfg.Agent = claude.Exec{}
	if o.dryRun {
		cfg.Agent = dryrun.Agent{Cases: b.file.Cases, Out: w}
		judgeRunner = dryrun.NewJudge(w)
	}
	cfg.Scorer = judge.Scorer{Runner: judgeRunner, Judge: b.file.Judge, Dir: judgeDir}
	return cfg
}

func writeResults(dir string, res eval.Results) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("results dir: %w", err)
	}
	name := res.StartedAt.Format("20060102T150405Z")
	if res.DryRun {
		name += "-dry-run"
	}
	path := filepath.Join(dir, name+".json")
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode results: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write results: %w", err)
	}
	return path, nil
}
