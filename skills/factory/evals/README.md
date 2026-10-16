# Factory skill evals

**Experimental**, like the skill. These are cases for `claude plugin eval`, in
four suites, one directory each. A case is a `prompt.md` with the fixture and
the task, plus `graders/`. The regex graders check the shape of the message.
One llm rubric checks its content, and it repeats the fixture because the
judge sees only the reply. A run has no MCP servers and no SendMessage, so
every prompt asks for the message as the whole reply.

## Suites

| Suite | Input | Output | What it grades |
|-------|-------|--------|----------------|
| `brief/` | a plan unit and run facts | a task message | Shape: the header in order, the routing result, warnings before step 1, no hedges, no sentence over 20 words, the report-format line last. The rubric checks completeness: what, where, the done criterion, the report format, and nothing outside the unit. |
| `report/` | a task and the facts of a finished or blocked unit | `ready` or `blocked` | Shape: the header and the ready or blocked fields, the exact commit range, no sentence over 25 words. The rubric checks faithfulness to the fixture. |
| `round-trip/` | a task and a work log | the worker's message | **Deferred until brief and report pass.** Information: whether the message answers the task's done criterion and every report-format item. It is the only suite that grades information rather than shape. |
| `context/` | the worker brief as the SubagentStart hook delivers it, a task, and what happened | the worker's next message | The brief alone: contract format, no edits outside the worktree, questions parked, a denial reported as `blocked`. |

The sentence-length graders count the words between a period, a question
mark, an exclamation mark or a line end. A dot inside a path or a file name
also ends a count, so the check is lenient by design. The STE gate below
counts sentences properly.

The brief, report and round-trip suites measure the skill text. The runner
adds a baseline arm without the skill and reports the score delta. That
delta means something only if the run reads the skill. So `SKILL.md` carries
the whole contract inline. No run has shown yet that Read reaches
`contract.md` and the other files beside it inside the eval sandbox. The
first signed-off run uses `--keep-temp`, and its traces are checked for
Read calls on `contract.md` to settle it.

The context suite measures `worker-brief.md` alone, because workers have no
Skill tool. Its prompts quote the brief behind the prefix
`belt[factory-worker]:`, which is what belt prints for the custom hint named
`factory-worker` (see `../worker-brief.md`). Its cases grant no tools, so
both arms see the same thing; run it with `--ablation none`.

Three cases are built on the sample messages in `contract.md`:
`brief-spec-sample` (the spec's sample task), `report-ready-unresolved-term`
and `report-blocked-denial`. Each sample must pass every regex grader of its
case, and `scripts/check-graders.mjs` checks that for free.

## Provenance and sign-off

Every case carries `tags: [<suite>, <provenance>]`, and the provenance is
one of three labels:

- `from-spec`: built from the factory spec, including its sample task.
- `from-transcript`: built from a real session, so far the session that
  built this skill.
- `hand-written`: made up for the eval.

The user signs off the cases before the first paid run, and again after any
change to a prompt or a rubric. Never label or rewrite a case because of
what a model under test answered: that trains the eval to reward the
incumbent.

## Running it

Every invocation below passes `-j 4`, the runner's `--concurrency` flag. It
accepts 1 to 8, and all runs share one rate limit. The cost ceiling is
checked before each run starts, so with `-j 4` a run that trips it can
overshoot by up to four runs.

Run the free checks first, from the repo root:

```bash
node skills/factory/evals/scripts/check-graders.mjs
claude plugin eval skills/factory -j 4 --max-cost-usd 0 --runs 1 --ablation none --trust-plugin --no-publish
```

The first checks frontmatter, grader shapes, the samples and the copies of
the sample task. It is optional and needs node from the mise-managed
toolchain; skip it when node is absent.

The second is a dry run. Every start sees the ceiling of 0 as already
reached, the first one included, so it loads and validates every case but
starts none. Its summary line reads `0 case(s) · 0s · $0.00` and the run
exits 2 for the partial result; a case file that fails to load is listed
above that line.

One case, one run, one arm, while editing it:

```bash
claude plugin eval skills/factory -j 4 --case brief-spec-sample --runs 1 --ablation none --trust-plugin --no-publish
```

The matrix runs the brief, report and context suites on each model the
routing table in `../routing.md` allows, which today is the mid tier
(Sonnet) and the top tier (Opus). The round-trip suite stays in the tree but
is deferred: add it to the inner loop once brief and report pass. Each model
and suite writes to its own `--output-dir`, so the two model loops can run in
parallel in the background without colliding:

```bash
stamp=$(date +%Y%m%dT%H%M%S)
for model in sonnet opus; do
  for suite in brief report; do
    claude plugin eval skills/factory -j 4 --tag "$suite" --model "$model" \
      --output-dir "skills/factory/evals/results/$stamp-$model-$suite" --trust-plugin --no-publish
  done
  claude plugin eval skills/factory -j 4 --tag context --ablation none --model "$model" \
    --output-dir "skills/factory/evals/results/$stamp-$model-context" --trust-plugin --no-publish
done
```

Add `--keep-temp` to the first signed-off run, for the trace check above.

Then run the Simplified Technical English (STE) check over each results
directory:

```bash
skills/factory/evals/scripts/ste-pass.sh skills/factory/evals/results/<stamp>-<model>-<suite>
```

It drops fenced code blocks, then exits 1 when a with-arm reply has STE
findings at warning level or above. A fence around the whole reply only
wraps it, so the reply inside is still checked. Briefs get the procedure
profile and the rest get the description profile.

A run costs about 30 cents per case per arm, and each case runs three times
by default:

| Slice | Cases | Arms | Rough cost per model |
|-------|-------|------|----------------------|
| brief, report | 12 | 2 | $22 |
| round-trip, once enabled | 6 | 2 | $11 |
| context | 6 | 1 | $5 |

## Results

This is a local gate, never CI: a run needs a Claude Code binary and paid
credentials. The runner writes to `results/` here, which git ignores. After
a signed-off run, add one row per suite and model to `scores.md` beside
this file and commit it. A row holds the date, the suite, the model, cases
passed out of cases run, the STE result, the cost, and the Claude Code
version. The first row creates the file.

The effort axis is not wired yet. The runner has no effort flag; a case's
`env` frontmatter could set `CLAUDE_CODE_EFFORT_LEVEL`, but that is
untested.
