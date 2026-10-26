# Harness eval

Runs fixed tasks through `claude -p` with your real hooks and MCP servers loaded. Each run is scored on its outcome and on what it cost in turns, tokens, dollars and tool calls. Run it before and after a change to the harness and compare the two runs. `claude plugin eval` can't do this job because MCP tools are unavailable inside its runs.

## Running it

Run from the repo root. Every run is paid: a fresh session on the default model costs about a dollar before it does any work, so start with one case.

```
go run ./evals/harness -list                      # the cases, no claude
go run ./evals/harness -case where-defined        # one case, one rep
go run ./evals/harness -case a,b -reps 3          # several cases and reps
go run ./evals/harness -model haiku -case a       # override the model
go run ./evals/harness -report 20261026-191500    # re-render a summary
go run ./evals/harness -compare <runA> <runB>     # per-case deltas
```

Each case run starts as its own session: the runner strips the variables that would nest it in the calling session. It passes `--no-session-persistence`, `--session-id`, `--max-turns` from the case, and `--max-budget-usd` (5 dollars by default) as a hard cap. Ctrl-C stops the current case and still tears down its workspace.

## Adding a case

One markdown file in `cases/`, named after its id. The frontmatter is YAML and the body is the prompt. Unknown keys fail the load, and `go test ./evals/harness/` loads every case.

```
---
id: where-defined
title: Find where a Go function is defined
workspace: repo:thismoon      # or worktree:thismoon, or tmpdir
timeout: 300                  # seconds
max_turns: 12                 # passed to claude
permission_mode: acceptEdits  # optional; edits outside /tmp need it
disallowed_tools: [Edit, Write, "Bash(git push:*)"]
graders:
  - type: tool_used
    tool: '^mcp__csl__csl_search$'
    input: 'sym:'
    weight: 2                 # default 1
---
Where is `CanonicalRepo` defined? Answer with the path and line only.
```

A `repo:` workspace runs in the canonical checkout, so read-only cases should disallow edits. A bare name resolves under `-src`. `worktree:` cuts a throwaway worktree and branch from the checkout's HEAD under `~/.worktrees/<repo>/eval-<id>-<stamp>` and removes both afterwards. `tmpdir` is an empty temp directory.

Graders run before the workspace is torn down:

- `regex`: `pattern` matches the final answer.
- `file`: `path` exists in the workspace and, when `pattern` is set, its content matches.
- `commit`: the run committed, and every subject matches `pattern`. Worktree only.
- `tool_used`, `tool_not_used`: a call whose name matches `tool`. Optional `input` and `input_without` regexes test the call's input as sorted-key JSON.
- `tool_before`: a `tool` call comes before any `before` call.
- `max_turns`, `max_cost_usd`, `max_tool_calls`: the run stays at or under `max`.
- `llm`: a haiku judge reads `rubric` and the answer.

## Reading a run

A run writes `runs/<timestamp>/`. `run.json` records the claude version and flags, the cases, and the totals. `results.jsonl` has one row per case and rep with the score, each grader's result, the usage numbers, and the session id. `streams/` keeps each raw transcript. `summary.md` repeats the totals, then gives one line per row, the tool calls per row, and every failed grader with the reason.

The score is the passed share of the grader weight, and a row passes only when every grader passes. The cost column is the agent run alone; llm judge calls are counted apart. Rows also carry the moondb session scorecard when moondb serves one, and `unavailable` otherwise.

Git ignores `runs/`. When a run settles a question, copy its `summary.md` to `results/<date>-<label>.md` so later runs have a baseline to compare against.
