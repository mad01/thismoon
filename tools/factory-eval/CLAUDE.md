# factory-eval, experimental eval of the factory against one agent

**Experimental.** The MAD-402 eval: it runs the factory arm and the
single-agent arm on the docs-sweep cases at the same dollar cap, scores
them, and applies the pass rule. A paid batch starts only by hand, against
a cases file the user signed off.

## Module layout

Part of the thismoon module, with no go.mod of its own.

```
tools/factory-eval/
  cmd/factory-eval/main.go   thin shim over internal/cli.Execute
  embed.go, facts.go         operating.md embed and agentdoc facts
  operating.md               the agent-facing operating doc (factory-eval docs)
  cases.yaml                 the signed-off cases file
  schema/                    results JSON Schema and the test-written fixture
  results/                   batch output, ignored by git
  internal/
    cli/       run, measure, docs, version; the paid-run gate
    cases/     cases.yaml types, decoding with known fields, validation
    outline/   pure heading check against the shared outline
    checkout/  throwaway clone at the pin, branch reads, check worktrees
    claude/    headless argv builders, the result-message parser, the exec runner
    judge/     the five deterministic checks and the LLM judge call
    prereq/    tool, skill, worker definition and hook checks
    eval/      the run loop, prompts, permission profile, results, pass rule
    dryrun/    stand-ins for the arm session and the judge
```

## How it works

`run` resolves a batch from cases.yaml and the flags, then prints the plan
and every prerequisite. It refuses a paid run with no sign-off, a missing
prerequisite, or a worst case above `batch_ceiling_usd`. `eval.Run` then
loops run index, tier, arm. Each run clones the source with
`checkout.Create`, invokes the arm through the `AgentRunner` seam, and
scores every case with the `CaseChecker` and `CaseScorer` seams. It keeps
the clone when the run fails. `Summarize` takes the per-arm means and
`Decide` applies the pass rule per tier; the primary tier's decision is the
headline.

`--dry-run` swaps `dryrun.Agent` and `dryrun.Judge` in for `claude.Exec`.
Everything else is the paid path.

### Decisions that are easy to undo by accident

- The judge always runs when the branch exists, but a case scores 0 unless
  every deterministic check passed. A broken branch never wins on prose.
- Tokens are the sum over `modelUsage`, cache reads included, so subagent
  work counts. The pass rule compares `tokens_total`.
- Wall clock is factory-eval's own measure around the claude process, not
  `duration_ms`.
- Two humanizer rules are ignored, measured as firing on every component doc
  on text the outline requires. Changing the list changes what the done
  criterion means, so it needs a new sign-off.

## Data model & storage

No store. Input is cases.yaml. Output is one JSON document per batch under
`results/`, shaped by `schema/results.schema.json` (`factory-eval/results/v1`).
The clones live under the system temp dir, or `--temp-dir`, as
`factory-eval-*`; passing runs delete theirs.

## Build / install / test

```bash
make build      # ./factory-eval with build metadata
make test       # go test ./...
make lint       # golangci-lint
make install    # ~/code/bin/factory-eval
```

`TestResultsFixtureMatchesTheSchemaAndTheCode` regenerates
`schema/results.example.json` with `-update` and validates it against the
schema. Review the diff before committing it.

## Commands

- `factory-eval run [--dry-run] [--arms single,factory] [--tiers mid,top] [--runs N] [--out results] [--repo PATH] [--temp-dir DIR]`
- `factory-eval measure [--repo PATH]`: the deviation table, no model call
- `factory-eval docs`, `factory-eval version [-o json]`

## Gotchas

- **Never make a paid run to test a change.** Use `--dry-run` and the
  stand-in tests. The user signs off cases.yaml before every first paid run.
- **`--max-budget-usd 0` is rejected by claude**, so there is no free real
  call; the dry run exists because of that.
- **Variadic claude flags swallow what follows them.** The prompt goes on
  stdin for that reason, and a test checks that each variadic flag is
  followed by another flag.
- **Arm sessions load the user's settings** (hooks, skills, agent
  definitions, permission rules), because the factory arm needs them. The
  deny list in `internal/eval/profile.go` beats any allow rule they add.
- **The source repo is only read.** The clone's origin remote is removed
  right after cloning, so no session can push back.

## See also

- `README.md`: usage, cost ceilings, the confirmed claude flags and output shape
- `skills/factory/SKILL.md`: what the factory arm runs
- `tools/humanizer/evals/judge/`: the judge eval this one borrows its shape from
