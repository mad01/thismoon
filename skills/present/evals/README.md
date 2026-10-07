# present skill evals

Cases for `claude plugin eval`. They check that an agent given a work summary writes the right Doc: a chart where there are numbers, the page graph where there is a dependency map, and a deck with one idea per slide. Each case is a `prompt.md` with the fixture and the task, plus `graders/`. The graders are a few regex checks on the reply, a `tool_used` check that the skill fired, and one llm rubric. The rubric repeats the fixture facts, because the judge sees only the reply.

The present MCP is not available inside a run, so every prompt asks for the `present_create` arguments as one JSON object in the reply instead of a tool call.

Run from the repo root:

```bash
claude plugin eval skills/present --trust-plugin --no-publish
```

That runs every case three times with the skill and three times without it, and reports the score delta. While editing a case, run one case once on one arm:

```bash
claude plugin eval skills/present --case chart-for-numbers --runs 1 --ablation none --trust-plugin --no-publish
```

Results land under `results/` here, which git ignores. A run costs about 30 cents per case per arm.
