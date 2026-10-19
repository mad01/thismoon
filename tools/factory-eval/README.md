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
pinned commit. The clone loses its origin remote, every branch, every tag
and its reflogs, and git prunes the objects that leaves unreachable. So the
clone holds nothing newer than the pin, and `git fetch` has no remote to
pull it back from. Its local config carries the `factory-eval` git
identity, so no commit prompts. The temp dir is named in resolved form:
`/private/var/folders/...` on macOS, not the `/var/folders` form, so every
path an arm session sees matches the working directory claude records.
The first paid run denied every `cd <worktree> && ...` a worker ran. The
cause is a Claude Code rule, not the path form. In dontAsk mode a cd to a
directory outside the session's working directory, followed by any
command that needs an allow rule, is denied. The lists do not matter.
`git status` needs no rule in the start directory, but every probe denied
it after such a cd; of the probed commands only `pwd` survived the cd. The
worker brief therefore runs `git -C <worktree>`, `make -C` and `go
<subcommand> -C` from the start directory and never cd.
The arm session works in the clone under a fixed permission profile:

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
  Both lists go on the command line and into the run's settings file.
- `--settings` with a settings file written into the run's temp dir. It
  repeats the arm's allow and deny lists, because Claude Code documents
  settings-file rules as reaching subagents such as the factory's workers.
  Its own deny rules keep Edit, Write, `git -C`, `make -C`, `go build -o`,
  and `go build`, `go test` or `go vet` with `-C` out of the user's
  checkouts, as the next section lists.

### What is enforced and what is only detected

The arms do not run in a sandbox. Each arm session runs as the user, with
the user's own settings loaded, and those settings may allow commands this
profile does not list. Claude's permission rules enforce the following,
because a deny rule from any source beats an allow rule from any source:

- `--strict-mcp-config`: the humanizer MCP server is the only one.
- The deny list above blocks push, remote, git config, `gh`, `ralph`,
  installs and the web tools.
- The run's settings file holds Edit deny rules for `~/code` and
  `~/.worktrees`, and for the source repo when it lives elsewhere. Claude
  Code checks the Write tool against Edit rules, so they block Write too;
  it never consults a Write path rule, and the file holds none.
- The same file blocks `curl`, and `git -C`, `make -C`, `go build -o`, and
  `go build`, `go test` or `go vet` with `-C`, aimed into any of those
  directories in absolute or `~/` form. So no
  build in a run can overwrite a binary under `~/code/bin`. Reads stay
  allowed.

The arm's allow and deny lists live in two places: the `--allowedTools`
and `--disallowedTools` flags, and the settings file's `permissions.allow`
and `permissions.deny`. Claude Code's docs say settings-file rules apply
to the whole session, subagents included. They never say the flags reach
custom subagents. Under dontAsk, a factory worker with no allow rule would
be denied every edit and command. The factory arm would then fail for a
reason that has nothing to do with the factory. Flags and settings files
share one rule matcher, so a rule means the same in either place. Each
arm's file carries that arm's lists: the single arm's denies `Agent` and
`SendMessage`, and the factory arm's allows them. The first paid run
(2026-10-19) left open whether the workers obey the file's lists. Every
worker command that passed was one claude runs without a rule, such as
`pwd`. `git status` needs no rule in the start directory too, but every
probe denied it after a cd out of that directory. Arm sessions also load
the user's own settings, and on this machine those allow plain `git
commit` and `make`, so a worker passing those shows nothing either. The
proof is a command only the arm's lists admit, such as `git -C <worktree>
commit`. The flags and the file carry the same lists, so a pass shows they
reached the worker, not which of the two did. Read `permission_denials`
for both arms before trusting a factory result. The factory arm's allow
list reaches every subagent type, not only workers.
A general-purpose subagent the orchestrator spawns could spawn nested
agents, but the `factory-worker-*` definitions list no `Agent` tool, so
workers cannot.

A Bash command can still reach a checkout another way: a relative path
after `-C`, `$HOME` in the path, a symlink, or a command the user's
settings allow. That is detected, not prevented. Before and after each
arm session, factory-eval fingerprints the source checkout: HEAD, a hash
of `git status --porcelain`, the worktree count and every ref. Any change
marks the run tainted, lists the change under `taint`, and counts the run
as errored. Files outside the source checkout, such as other repos under
`~/code` reached through Bash, are not watched.

The fingerprint cannot tell the arm's change from another session's. All
worktrees of a repo share one ref store, so a commit in any worktree of
the source repo taints a run, loom worktrees included. A fetch taints it
too, since it moves the remote-tracking refs. For a paid batch, clone a
dedicated source that no session works in and point `--repo` at it, as
Usage shows.

### Scoring and the pass rule

After the session ends, factory-eval scores every case. Deterministic
checks come first:

- The case branch exists and changes only the allowed paths.
- Both docs follow the shared outline.
- `humanizer detect` finds nothing at warning level outside the two
  ignored rules.
- `make -C <component> build test` passes.

When a branch edits a path outside the case, the humanizer and build checks
fail as skipped instead of running, so a Makefile the arm wrote never runs.

Then one judge call scores the prose from 0 to 10 against a fixed rubric,
a do-not-fail list and the case's glossary files as they read at the pin.
A failed judge call gets one retry. A case's score is the judge score when
every check passed, else 0, and a case whose branch is missing scores 0. A
run passes when it did not error and every case passed its checks.

A run errors in any of these cases:

- factory-eval or claude failed.
- The session stopped before it finished, a budget stop too.
- The result message carries `is_error`. An API error that ends the turn
  reports subtype `success` with `is_error` set.
- The judge failed twice on a case whose branch exists.
- The run is tainted.

A tier is decided only when it is complete: each arm recorded every
expected run and none errored. Errored runs never count toward the means,
so a judge outage or an API error never lowers an arm's score. On a
complete tier, the factory passes only when all of these hold:

- The single arm's mean judge score is above 0, so there is a baseline.
- At least one factory run passed all five checks on every case.
- The factory's mean judge score is at least the single arm's.
- Its mean wall clock is at most half the single arm's, and its mean
  tokens at most 1.5 times. A zero single-arm mean leaves a ratio
  undefined, and the decision says so and fails.

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
  `~/.claude/settings.json` that runs belt. MAD-398 installs the
  definitions and the hook wiring. Without them the factory arm refuses to
  run.
- A signed-off cases file, as Usage describes.
- A temp dir outside `~/code` and `~/.worktrees`, where the arms may edit
  their clone. The default system temp dir qualifies.

## Usage

Rehearse first. A dry run makes no model call: stand-ins replace claude and
the judge, while the clones, the checks and the results file are real. It
prints every prerequisite, the exact argv of both arms and of the judge,
the sign-off digest, and what a paid run would refuse.

```bash
cd tools/factory-eval
go run ./cmd/factory-eval run --dry-run      # one run per arm and tier, about 90 s
go run ./cmd/factory-eval measure            # the deviation table behind the cases
```

To sign off, show the user the cases and the dry run's output. Once they
approve, set the three `sign_off` values in `cases.yaml`:

- `by`: the name of the person who approved.
- `date`: the date, as YYYY-MM-DD.
- `digest`: the value the dry run prints as the sign-off digest.

The digest is the sha256 of `cases.yaml` with each `sign_off` line
rewritten to its key and an empty quoted value (`  by: ""`). Filling in the
three values leaves it unchanged, and any other edit changes it. A paid run
recomputes it and refuses when it no longer matches. In `cases.yaml` as
committed only the `sign_off` lines match this pattern, so it gives the
same digest by hand:

```bash
sed -E 's/^(  (by|date|digest)):.*/\1: ""/' cases.yaml | shasum -a 256
```

Paid runs clone a dedicated source, so no other session can taint them.
Make it once, outside `~/code`, and keep every session out of it. Fetch
in it only between batches, when the pinned commit moves past it:

```bash
git clone ~/code/src/github.com/mad01/thismoon ~/factory-eval-source
```

The first paid run, once the user has signed off `cases.yaml`. Start with
the primary tier alone, one run per arm:

```bash
go run ./cmd/factory-eval run --repo ~/factory-eval-source --tiers mid --runs 1
```

A batch with fewer runs per arm than the cases file's `runs` prints its
decision as provisional. Read it as a first look, not as the verdict.

The full matrix, three runs per arm at both tiers:

```bash
go run ./cmd/factory-eval run --repo ~/factory-eval-source
```

`--runs` must be at least 1. `--arms single` runs the baseline alone, for
example while the factory prerequisites are missing. `--repo` clones another checkout instead of the
one in `cases.yaml`. `--temp-dir` puts the clones somewhere other than the
system temp dir.

### Cost ceilings

| Scope | Ceiling | Where it is set |
|-------|---------|-----------------|
| One arm run, mid tier (Sonnet, medium effort) | $10 | `tiers[].budget_usd`, enforced by `--max-budget-usd` |
| One arm run, top tier (Opus, high effort) | $25 | `tiers[].budget_usd` |
| One judge call (Haiku) | $0.25 | `judge.budget_usd` |
| One batch, worst case | $250 | `batch_ceiling_usd`; a batch whose worst case is higher refuses to start |

The full matrix has a worst case of $234: 12 arm runs at their caps plus
up to 96 judge calls at theirs. That is 48 case scorings, each with one
retry. The expected spend is about half of that; the pilot's sign-off page,
which is not part of this repo, shows the arithmetic. The dollar figures are
claude's own client-side estimates
(`total_cost_usd`), not a bill.

### What this machine's claude confirmed

On claude 2.1.301 and 2.1.302, `claude -p --help` prints the same flags as
`claude --help`. The budget flag is `--max-budget-usd <amount>`, valid only
with `--print`. A value of 0 is rejected before any call ("must be a
positive number greater than 0"). So a zero-budget dry run cannot exist,
and the dry run uses stand-ins instead. `--output-format json` prints one result
message, the Agent SDK's `SDKResultMessage`:

- `type` is `result`, and `subtype` is `success` or an error such as
  `error_max_budget_usd`, with `is_error` and `errors` beside it. A
  `success` result can still carry `is_error` true, with
  `api_error_status` and no `errors`: an API error ended the turn.
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
change to it. The sign-off digest enforces that. The fields:

- `commit` pins the tree every run clones. `repo` is the checkout cloned
  from.
- `tiers` sets the model, effort, worker definition and dollar cap per tier.
  `primary` marks the tier whose decision is the headline.
- `outline` is the shared docs outline (MAD-217), and `humanizer` names the
  severity and the two rules ignored because they fire on text the outline
  itself requires.
- `judge` holds the rubric, the do-not-fail list and the glossary notes
  the judge sees beside each case's glossary files.
- `arms` holds each arm's preamble. The case tasks below it reach both arms
  in identical wording.
- `cases` lists the four components, each with its branch, allowed paths,
  task, done criterion and measured deviation. A case's `glossary` lists
  the glossary files the judge reads; without one it reads the root
  `CONTEXT.md`.

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

Each run also records what could make two runs differ. `installed` holds
the sha256 of every file in the factory and loom skill directories, their
`evals/` suites aside, and of the three worker definitions. It also holds
the belt version. Each value is as it was when the run started. A
`ralph up` in the middle of a batch shows there. `tainted` and `taint` say
whether the source checkout changed during the run. `settings_file` is the
run's settings file. `settings_permissions` at the top holds, per arm, the
permissions block that arm's settings file held. Its `allow` is the arm's
allow list, and its `deny` is the arm's deny list with the isolation rules
at the end.

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
