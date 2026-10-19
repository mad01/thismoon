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
    cases/     cases.yaml types, decoding with known fields, validation, sign-off digest
    outline/   pure heading check against the shared outline
    checkout/  throwaway clone at the pin, branch reads, check worktrees, fingerprint
    claude/    headless argv builders, the result-message parser, the exec runner
    judge/     the five deterministic checks and the LLM judge call
    prereq/    tool, skill, worker definition and hook checks; the install snapshot
    eval/      the run loop, prompts, permission profile, isolation settings, results, pass rule
    dryrun/    stand-ins for the arm session and the judge
```

## How it works

`run` resolves a batch from cases.yaml and the flags, then prints the plan,
the sign-off digest and every prerequisite. It refuses a paid run when the
sign-off is missing or does not match `cases.File.Digest`, a prerequisite
is missing, or the worst case is above `batch_ceiling_usd`. The worst case
counts every arm run at its cap and `judgeAttempts` judge calls per case. `eval.Run` then
loops run index, tier, arm. Each run records `prereq.Snapshot`, clones the
source with `checkout.Create`, and writes its settings file from
`eval.Isolation`. It fingerprints the source checkout, invokes the arm
through the `AgentRunner` seam, and fingerprints again; a change taints
the run. It scores every case with the `CaseChecker` and `CaseScorer`
seams, and keeps the clone when the run fails. `Summarize` takes the
per-arm stats over the runs that did not error, and `Decide` applies the
pass rule per tier. The primary tier's decision is the headline, marked
provisional when `--runs` is below the cases file's `runs`.

`--dry-run` swaps `dryrun.Agent` and `dryrun.Judge` in for `claude.Exec`.
Everything else is the paid path.

### Decisions that are easy to undo by accident

- The judge always runs when the branch exists, but a case scores 0 unless
  every deterministic check passed. A broken branch never wins on prose.
  A judge that fails both attempts on an existing branch errors the run
  instead of scoring 0, so it never lowers the arm's mean.
- Tokens are the sum over `modelUsage`, cache reads included, so subagent
  work counts. The pass rule compares `tokens_total`.
- Wall clock is factory-eval's own measure around the claude process, not
  `duration_ms`.
- Two humanizer rules are ignored, measured as firing on every component doc
  on text the outline requires. Changing the list changes what the done
  criterion means, so it needs a new sign-off.
- `Decide` decides a tier only when it is complete: every expected run of
  both arms recorded, none errored. `RunResult.Errored` covers an error, a
  stop other than success (a budget stop too) and a taint. A result with
  `is_error` set sets the error even under subtype success, the shape an
  API error that ends the turn takes; `claude.Report.ErrorText` words it. On a complete
  tier it still fails without a single-arm baseline (mean judge above 0)
  or without one factory run that passed all five checks.
- humanizer and build_test fail as skipped when allowed_paths fails, so a
  Makefile the arm edited never runs.
- `prereq.Snapshot` hashes every file in the factory and loom skill dirs,
  not only `SKILL.md`, since the worker brief and routing table reach the
  workers. It skips `evals/`, where an eval run writes its results.
- `cases.ContentDigest` blanks each `sign_off` line to its key and `""`
  before hashing; README documents the rule for people. Changing the rule
  voids every existing sign-off.
- The arm's allow and deny lists go on the command line and into the
  run's settings file (`armPermissions`). It looks like duplication, but
  Claude Code documents settings-file rules as reaching subagents and never
  says that of `--allowedTools`. Without the settings copy a factory worker
  under dontAsk could be denied every edit and command, failing the arm for
  a reason unrelated to the factory.

## Data model & storage

No store. Input is cases.yaml. Output is one JSON document per batch under
`results/`, shaped by `schema/results.schema.json` (`factory-eval/results/v2`).
v2 replaced v1's `settings_deny` with `settings_permissions`, so a v1 file
fails the current schema.
The clones live under the system temp dir, or `--temp-dir`, as
`factory-eval-*`, each with the run's read-only `settings.json` beside the
clone. Passing runs delete theirs.

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
  definitions, permission rules), because the factory arm needs them. A
  deny rule beats every allow rule wherever it comes from: the deny list in
  `internal/eval/profile.go`, on the command line and in the run's settings
  file, and the isolation rules from `internal/eval/isolation.go`, in the
  settings file only. That prevents only the paths those rules
  name. It is not a sandbox: a Bash command can still reach a checkout
  through a relative `-C` path or `$HOME`, and only the fingerprint
  detects that.
- **Edit rules cover the Write tool.** Claude Code checks file writes
  against Edit rules and never consults a Write path rule, so the isolation
  rules are Edit rules only, and a test keeps Write rules out. Their `git -C`,
  `make -C`, `go build -o` and `go build|test|vet -C` rules deny the same
  dirs: `~/code`, `~/.worktrees`, and the source when it lives outside
  both. The `go build -o` rule is what keeps a run from overwriting a
  binary in `~/code/bin`.
- **Worker obedience to the settings lists is unverified.** Claude Code
  documents settings-file rules as reaching subagents. The first paid run
  (2026-10-19) left it open. Every worker command that passed was one
  claude runs without a rule, such as `pwd`. `git status` needs no rule in
  the start directory too, but every probe denied it after a cd out of
  that directory. Arm sessions load the user's own settings too, and this
  machine's allow plain `git commit` and `make`, so those passing shows
  nothing. The proof is a command only the arm's lists admit, such as
  `git -C <worktree> commit`. The flags and the file carry the same lists,
  so a pass does not say which carrier reached the worker. Read
  `permission_denials` for both arms before trusting a factory result.
- **Every path an arm session sees is the resolved one.** `checkout.Create`
  runs `EvalSymlinks` on the temp root, so the clone, the worktrees dir,
  the settings file, the prompt and `--add-dir` all name
  `/private/var/folders/...` on macOS, the form claude records the session
  cwd in. The first paid run lost every worker at `cd <worktree> && git
  status`, the command after `pwd`. That is a Claude Code rule, not the
  path form. In dontAsk mode a cd outside the session cwd followed by any
  command that needs an allow rule is denied. The lists do not matter, and
  `--add-dir` does not widen it. `git status` needs no rule in the start
  directory, but every probe denied it after such a cd; of the probed
  commands only `pwd` survived the cd. The worker brief runs `git -C
  <worktree>` instead.
- **The fingerprint taints a run on any change to the source checkout**,
  whoever made it. All worktrees share the ref store, so a commit in any
  worktree of the source repo (loom ones included) taints a run, and so
  does a fetch. Paid batches pass `--repo` pointing at a dedicated clone
  no session works in; README's Usage shows the command.
- **The clone holds nothing past the pin.** Branches, tags, reflogs and the
  origin remote go right after cloning and git prunes what that leaves
  unreachable. No newer main drifts in, and no session can push back.
- **The factory preamble runs preflight check 4 only.** Check 3 reads the
  identity from the repo's git settings, a call the deny list blocks, so
  the clone's local config carries the identity instead. A prompt test ties
  the preamble to that deny rule.

## See also

- `README.md`: usage, cost ceilings, the confirmed claude flags and output shape
- `skills/factory/SKILL.md`: what the factory arm runs
- `tools/humanizer/evals/judge/`: the judge eval this one borrows its shape from
