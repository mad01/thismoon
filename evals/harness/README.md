# Harness eval

Runs fixed tasks through `claude -p` with your real hooks and MCP servers loaded. Each run is scored on its outcome and on what it cost in turns, tokens, dollars and tool calls. Run it before and after a change to the harness and compare the two runs. `claude plugin eval` can't do this job because MCP tools are unavailable inside its runs.

## Running it

Run from the repo root. Every run is paid: a fresh session on the default model costs about a dollar before it does any work. One rep of all eight cases should cost $10 to $15. The worst case is $40, eight cases at the $5 per-run cap, and `-max-total-usd` (default $20) stops the run before the next case once it has spent that much.

```
go run ./evals/harness -list                      # the cases, no claude
go run ./evals/harness -case where-defined        # one case, one rep
go run ./evals/harness -case a,b -reps 3          # several cases and reps
go run ./evals/harness -model haiku -effort low   # override model and effort
go run ./evals/harness -report 20261026-191500    # re-render a summary
go run ./evals/harness -compare <runA> <runB>     # per-case deltas
```

Each case run is its own session: the runner strips the variables that would nest it in the calling session or set effort behind its back. It passes `--max-turns` from the case and `--max-budget-usd` as a hard cap. The session is kept like any other, so moondb can read its transcript and the row's scorecard has tokens, cost, searches and reads; eval sessions show up in `claude --resume` under the workspace directory. Ctrl-C, SIGTERM and SIGHUP stop the current case, kill claude's whole process group, tear down the workspace and write the rows so far.

## Workspaces

- `repo:<name or path>` runs in the canonical checkout, so it also denies edit tools, git write verbs and the csl repo tools.
- `worktree:<name or path>` cuts a worktree on a throwaway `eval/<id>-<stamp>` branch under `~/.cache/harness-eval/worktrees/<repo>/`, locked with the runner's pid, and removes both afterwards. A run's start removes only the worktrees under that root whose locking runner has died, and their branches.
- `tmpdir` is an empty temp directory.

Every case is denied the kof, worklog, brain and GitHub tools that write, on top of its own list. For repo and worktree cases the runner compares the canonical checkout's HEAD, branch and status before and after. Any change is an error on the row.

## Adding a case

One markdown file in `cases/`, named after its id: YAML frontmatter, then the prompt. Unknown keys fail the load, and `go test ./evals/harness/` loads every case.

```
---
id: where-defined
title: Find where a Go function is defined
notes: Where the budgets come from.
workspace: repo:thismoon
timeout: 300                  # seconds
max_turns: 12                 # passed to claude
permission_mode: acceptEdits  # optional; edits outside /tmp need it
disallowed_tools: [NotebookEdit]
graders:
  - type: tool_used
    tool: '^mcp__csl__csl_search$'
    input: 'sym:'             # one regex or a list that must all match
    weight: 2                 # default 1
---
Where is `CanonicalRepo` defined? Answer with the path and line only.
```

Graders run before teardown. `regex` matches the final answer. `file` checks a workspace path and its content. `commit` checks every new commit subject in a worktree. `tool_used`, `tool_not_used` and `tool_before` match tool names, with `input` and `input_without` regexes on the input as sorted-key JSON. They and `max_tool_calls` see only the main session unless `include_agents: true`. `max_turns`, `max_cost_usd` and `max_tool_calls` are budgets. `llm` has a hookless haiku judge read the `rubric`.

## Reading a run

A run writes `runs/<timestamp>/`. `run.json` records the claude version, the model and effort asked for and used, the flags, and the totals. `results.jsonl` has one row per case and rep with the score, each grader's result, usage, main and subagent tool counts, and the session id. `streams/` keeps each transcript, and `summary.md` lists the rows, their tool calls and every failure. The score is the passed share of grader weight. A row passes only when every grader does. An API error result, an unparsed stream line or a changed checkout puts the row in errors. Rows carry the moondb scorecard, or `unavailable`.

Git ignores `runs/`. Copy a settled `summary.md` to `results/<date>-<label>.md` as a baseline. `-compare` totals only the cases both runs have.
