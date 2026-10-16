# factory-eval

**Experimental.** factory-eval runs the eval for the software factory
pilot (MAD-402). It runs two arms on the same docs-sweep cases at the same
dollar cap and writes one results file. One arm is a single `claude -p`
session that does every case in turn. The other is an orchestrator session
that runs the factory skill with one worker subagent per case. Then
factory-eval applies the pass rule: the factory must score at least as well,
take at most half the wall clock, and use at most 1.5 times the tokens.
Anything else means the factory stops here.

## How it works

Each run gets a fresh clone of thismoon under a temp dir, detached at the
pinned commit, with its origin remote removed. The arm session works there
under a fixed permission profile:

- `--max-budget-usd` with the tier's cap, and `--output-format json` plus
  `--json-schema` for a structured final report.
- `--permission-mode dontAsk` and `--permission-prompts none`: anything
  that would prompt is denied, and the denial is counted in the results.
- `--strict-mcp-config` with the humanizer MCP server only, so no GitHub,
  Linear or present tool exists inside a run.
- An allow list for the file tools, the git commands a sweep needs, `make
  build`/`make test`, `go build/test/vet` and `humanizer`. A deny list for
  `git push`, `git remote`, `git config`, `gh`, `ralph`, `make install` and
  `go install`. Only the factory arm may use `Agent` and `SendMessage`.

After the session ends, factory-eval scores every case. Deterministic
checks come first:

- The case branch exists and changes only the allowed paths.
- Both docs follow the shared outline.
- `humanizer detect` finds nothing at warning level outside the two
  ignored rules.
- `make -C <component> build test` passes.

Then one judge call scores the prose from 0 to 10
against a fixed rubric and a do-not-fail list. A case's score is the judge
score when every check passed, else 0. A run passes when the session
finished and every case passed its checks.

Runs go one at a time, interleaved by run index, tier and arm. A passing
run's clone is deleted; a failing run's clone stays, and its path is in the
results.

The judge is `claude -p --model haiku` with no tools, no settings files and
no MCP servers, run from an empty directory. The humanizer judge eval goes
through OpenRouter instead. Using claude here keeps one login for the arms
and the judge, and its rubric is this eval's own, not the humanizer's
AI-or-human question.

## Install

```bash
make -C tools/factory-eval install    # builds and installs ~/code/bin/factory-eval
```

Prerequisites for a paid run, all checked before anything is spent:

- claude 2.1.259 or newer (for `--permission-prompts`), plus git, make, go
  and humanizer on PATH.
- For the factory arm: the factory and loom skills in `~/.claude/skills`,
  the three worker definitions `factory-worker-low`, `-medium` and `-high`
  in `~/.claude/agents`, and a SubagentStart hook in
  `~/.claude/settings.json` that runs belt. The definitions and the hook
  wiring land with MAD-398. Until then the factory arm refuses to run.
- A signed-off cases file: `sign_off.by` and `sign_off.date` set.

## Usage

Rehearse first. A dry run makes no model call: stand-ins replace claude and
the judge, while the clones, the checks and the results file are real. It
prints every prerequisite, the exact argv of both arms and of the judge,
and what a paid run would refuse.

```bash
cd tools/factory-eval
go run ./cmd/factory-eval run --dry-run      # one run per arm and tier, about 90 s
go run ./cmd/factory-eval measure            # the deviation table behind the cases
```

The first paid run, once the user has signed off `cases.yaml`. Start with
the primary tier alone, one run per arm:

```bash
go run ./cmd/factory-eval run --tiers mid --runs 1
```

The full matrix, three runs per arm at both tiers:

```bash
go run ./cmd/factory-eval run
```

`--arms single` runs the baseline alone, for example while the factory
prerequisites are missing. `--repo` clones another checkout instead of the
one in `cases.yaml`. `--temp-dir` puts the clones somewhere other than the
system temp dir.

### Cost ceilings

| Scope | Ceiling | Where it is set |
|-------|---------|-----------------|
| One arm run, mid tier (Sonnet, medium effort) | $10 | `tiers[].budget_usd`, enforced by `--max-budget-usd` |
| One arm run, top tier (Opus, high effort) | $25 | `tiers[].budget_usd` |
| One judge call (Haiku) | $0.25 | `judge.budget_usd` |
| One batch, worst case | $250 | `batch_ceiling_usd`; a batch whose worst case is higher refuses to start |

The full matrix has a worst case of $222: 12 arm runs at their caps plus 48
judge calls at theirs. The expected spend is about half of that; the
sign-off digest shows the arithmetic. The dollar figures are claude's own
client-side estimates (`total_cost_usd`), not a bill.

### What this machine's claude confirmed

On claude 2.1.301 and 2.1.302, `claude -p --help` prints the same flags as
`claude --help`. The budget flag is `--max-budget-usd <amount>`, valid only
with `--print`. A value of 0 is rejected before any call ("must be a
positive number greater than 0"). So a zero-budget dry run cannot exist,
and the dry run uses stand-ins instead. `--output-format json` prints one result
message, the Agent SDK's `SDKResultMessage`:

- `type` is `result`, and `subtype` is `success` or an error such as
  `error_max_budget_usd`, with `is_error` and `errors` beside it.
- `total_cost_usd`, `duration_ms`, `num_turns` and `session_id`.
- `usage` (main thread: `input_tokens`, `output_tokens`,
  `cache_creation_input_tokens`, `cache_read_input_tokens`) and
  `modelUsage` (per model and session-wide, subagents included:
  `inputTokens`, `outputTokens`, `cacheReadInputTokens`,
  `cacheCreationInputTokens`, `costUSD`).
- `permission_denials`, and `structured_output` when `--json-schema` is set.

factory-eval sums `modelUsage` for tokens, so a factory run counts its
workers. That shape comes from the SDK reference; no paid call has
confirmed it on this machine yet.

## Configuration

Everything a batch needs is in `cases.yaml`. Every value there carries one
of three provenance labels:

- `measured`: computed from the repo at the pinned commit.
- `spec`: from the ticket, the factory spec, or the shared docs outline.
- `judgement`: a choice made for this eval.

The user signs the file off before the first paid run, and again after any
change to a task, the rubric, a tier or a cap. The fields:

- `commit` pins the tree every run clones. `repo` is the checkout cloned
  from.
- `tiers` sets the model, effort, worker definition and dollar cap per tier.
  `primary` marks the tier whose decision is the headline.
- `outline` is the shared docs outline (MAD-217), and `humanizer` names the
  severity and the two rules ignored because they fire on text the outline
  itself requires.
- `judge` holds the rubric, the do-not-fail list and the glossary the judge
  sees.
- `arms` holds each arm's preamble. The case tasks below it reach both arms
  in identical wording.
- `cases` lists the four components, each with its branch, allowed paths,
  task, done criterion and measured deviation.

## Where things live

- `cases.yaml`: the cases file the user signs off.
- `schema/results.schema.json`: the results document's JSON Schema.
- `schema/results.example.json`: a results file the end-to-end test writes
  with stand-ins for every call. It holds two cases, one run per arm and
  tier, a pass at the primary tier and a failed run with its clone kept.
- `results/<UTC stamp>.json`, or `<stamp>-dry-run.json`: one file per
  batch. Git ignores the directory.

Reading a results file: `summary.decision` is the verdict line, and
`summary.decisions` holds one entry per tier with the three ratios behind
it. `runs[]` has one entry per arm session, with tokens, dollars, wall clock,
the stop reason, the judge's mean score and reasons, and per case the five
checks with their details. A failed run names its kept clone in `kept_dir`.
Open it to see what the session left on each branch.

## Develop

```bash
make -C tools/factory-eval build test lint
go test ./tools/factory-eval/internal/eval -run TestResultsFixture -update   # rewrite the fixture
```

The tests never call claude, humanizer or make: each sits behind a small
interface with a stand-in. The checkout tests use real git against temp
repos.

## Docs

- `CLAUDE.md`: module layout and the rules for changing the code.
- `operating.md`, printed by `factory-eval docs`: failure modes and first
  moves.
- The factory skill: `skills/factory/SKILL.md`.
