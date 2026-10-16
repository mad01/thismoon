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
	runsSet   bool
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

A paid run needs a cases file signed off at its current digest, every
prerequisite (the factory arm also needs the factory skill, the three worker
definitions and the belt hook), and a worst-case cost within the cases file's
batch ceiling. A dry run checks and prints all of that, the digest included,
then rehearses one run per arm and tier with stand-ins for every model call:
same checkouts, same checks, same results file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.runsSet = cmd.Flags().Changed("runs")
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
	env     prereq.Env
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
	b := batch{file: f}
	if b.runs, err = resolveRuns(o, f.Runs); err != nil {
		return batch{}, err
	}
	if b.arms, err = parseArms(o.arms); err != nil {
		return batch{}, err
	}
	if b.tiers, err = selectTiers(f, o.tiers); err != nil {
		return batch{}, err
	}
	if b.env, err = hostEnv(ctx); err != nil {
		return batch{}, err
	}
	if b.source, err = resolveSource(f.Repo, o.repo); err != nil {
		return batch{}, err
	}
	if err := checkTempDir(o.tempDir, b.isolation()); err != nil {
		return batch{}, err
	}
	b.common = prereq.Common(b.env)
	if slices.Contains(b.arms, eval.ArmFactory) {
		b.factory = prereq.Factory(b.env)
	}
	return b, nil
}

func (b batch) isolation() eval.Isolation {
	return eval.Isolation{Home: b.env.Home, Source: b.source}
}

// resolveRuns is --runs when given, which must be at least 1, else the
// cases file's runs, or 1 in a dry run.
func resolveRuns(o runOptions, fileRuns int) (int, error) {
	switch {
	case o.runsSet && o.runs < 1:
		return 0, fmt.Errorf("--runs %d, want at least 1", o.runs)
	case o.runsSet:
		return o.runs, nil
	case o.dryRun:
		return 1, nil
	}
	return fileRuns, nil
}

// resolveSource is the absolute path of the repo every run clones: --repo
// when given, else the cases file's repo.
func resolveSource(fileRepo, flagRepo string) (string, error) {
	src := fileRepo
	if flagRepo != "" {
		src = flagRepo
	}
	abs, err := filepath.Abs(expandHome(src))
	if err != nil {
		return "", fmt.Errorf("repo %s: %w", src, err)
	}
	return abs, nil
}

// checkTempDir refuses a temp parent inside a directory the isolation
// rules deny edits in, where an arm could not edit its own clone.
func checkTempDir(tempDir string, iso eval.Isolation) error {
	dir := tempDir
	if dir == "" {
		dir = os.TempDir()
	}
	abs, err := filepath.Abs(expandHome(dir))
	if err != nil {
		return fmt.Errorf("temp dir %s: %w", dir, err)
	}
	if iso.Covers(abs) {
		return fmt.Errorf("temp dir %s is under a path the arms may not edit "+
			"(~/code, ~/.worktrees or the source repo): pick one outside them", abs)
	}
	return nil
}

// hostEnv probes the host once: the home dir, claude's version, and the
// seams the prerequisite checks and the install snapshot read through.
func hostEnv(ctx context.Context) (prereq.Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return prereq.Env{}, fmt.Errorf("home dir: %w", err)
	}
	v, raw, err := claude.Exec{}.Version(ctx)
	return prereq.Env{
		Home:        home,
		LookPath:    exec.LookPath,
		Claude:      prereq.ClaudeProbe{Version: v, Raw: raw, Err: err},
		BeltVersion: beltVersion,
	}, nil
}

func beltVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "belt", "version").Output()
	if err != nil {
		return "", fmt.Errorf("belt version: %w", err)
	}
	return string(out), nil
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

// gate refuses a paid run before it spends anything: no valid sign-off of
// the file as it is now, a missing prerequisite, or a worst case above the
// batch ceiling.
func gate(b batch) error {
	if err := b.file.CheckSignOff(); err != nil {
		return fmt.Errorf("the cases file is not signed off for a paid run:\n%w\n"+
			"Run --dry-run, show the user the cases and the digest it prints, and fill in "+
			"sign_off.by, sign_off.date and sign_off.digest once they approve", err)
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
		Cases: b.file, CasesPath: o.casesPath, Source: b.source, Home: b.env.Home,
		Arms: b.arms, Tiers: b.tiers, Runs: b.runs, TempParent: o.tempDir, DryRun: o.dryRun,
		ClaudeVersion: b.env.Claude.Raw, Prereqs: b.prereqs(), Now: time.Now, Progress: w,
		Checker: judge.Deterministic{
			Outline: b.file.Outline, Humanizer: b.file.Humanizer, Cmd: judge.ExecCommander{},
		},
		Snapshot: func(ctx context.Context) prereq.Installed { return prereq.Snapshot(ctx, b.env) },
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
