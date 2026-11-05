# Harness eval

Runs fixed tasks through `claude -p` with your real hooks and MCP servers loaded. Each run is scored on its outcome and on what it cost in turns, tokens, dollars and tool calls. Run it before and after a change to the harness and compare the two runs. `claude plugin eval` can't do this job because MCP tools are unavailable inside its runs.

## Running it

Run from the repo root. Every run is paid: a fresh session on the default model costs about a dollar before it does any work. One rep of all 17 cases should cost $22 to $32. The worst case is $85, 17 cases at the $5 per-run cap. `-max-total-usd` (default $20) stops the run before the next case once it has spent that much, so raise it for a full rep.

```
go run ./evals/harness -list                      # the cases, no claude
go run ./evals/harness -case where-defined        # one case, one rep
go run ./evals/harness -case a,b -reps 3          # several cases and reps
go run ./evals/harness -model haiku -effort low   # override model and effort
go run ./evals/harness -report 20261026-191500    # re-render a summary
go run ./evals/harness -compare <runA> <runB>     # per-case deltas
go run ./evals/harness -profile csl-cli -case fleet-cobra-repos   # a workspace with the csl CLI only
go run ./evals/harness -profile brain-cli                        # the brain cases
go run ./evals/harness -profile brain-cli -include-pending       # and the parked ones
```

A run takes the cases its profile runs: brain workspaces run under `brain-cli` only, and `brain-cli` runs nothing else, so a run with no `-case` under any other profile skips the brain cases. A brain lookup or a single capture costs about $0.07 on opus. A case with `pending:` is parked until brain can do what it grades; it loads and its graders compile, but it runs only with `-include-pending`.

Each case run is its own session: the runner strips the variables that would nest it in the calling session or set effort behind its back. It also strips the Rex ones, whose hook would have the run rename the caller's pane. It passes `--max-turns` from the case and `--max-budget-usd` as a hard cap. The session is kept like any other, so moondb can read its transcript and the row's scorecard has tokens, cost, searches and reads; eval sessions show up in `claude --resume` under the workspace directory. The runner sets `MOONDB_SESSION_TAGS=eval`, so moondb marks every eval session with the `eval` tag. Ctrl-C, SIGTERM and SIGHUP stop the current case, kill claude's whole process group, tear down the workspace and write the rows so far.

## Profiles

`-profile` picks what the runs have for code search, so the same cases measure a workspace with csl against one without it. Every profile but `full` denies the thismoon CLIs (`-deny-clis`, a comma-separated list; add a workspace's own CLI there to keep it out of the measurement), except the one the profile is about.

- `full` (the default) is your session as it is: every MCP server, hook, and CLI.
- `csl-mcp` registers csl alone through `--strict-mcp-config`, keeps the csl CLI on PATH as a set-up machine has it, keeps your hooks, and appends the steer file to the system prompt.
- `csl-cli` registers no MCP server, allows `Bash(csl:*)`, keeps your hooks, and appends the steer file.
- `grep` registers no MCP server, turns every hook off, denies csl, and tells the model it has only the built-in tools: the baseline, where every search is a filesystem walk.
- `brain-cli` runs the brain cases with your setup shut out: no MCP server, no hooks, no auto memory, no setting sources (`--setting-sources ""`, so no CLAUDE.md, output style, permissions or plugins), and no session file. It runs in `dontAsk` mode with `--tools` and `--allowedTools` set to `Bash(brain:*)`, Read, Glob and Grep; a case's `allowed_tools` replaces that list. Bash runs brain alone, and every other command is denied. The profile denies the other thismoon CLIs, csl included, and appends `steer/brain.md`.

The csl steer file (`-steer-file`, default `services/csl/claude-md.md`) is the csl rules fragment a workspace CLAUDE.md imports, the same file the csl recipe links to `~/.claude/CLAUDE.csl.md`; the two csl profiles add one line under it saying which form of csl the session has. `-no-grep` also denies Grep and Bash grep, rg, ugrep and find in the two csl profiles and in `brain-cli`. It compares the steered run with the forced one. For `brain-cli` the default steer file is `steer/brain.md`, which tells the agent the vault sits behind the brain CLI.

Cases grade on outcome whatever the profile. Each row counts `csl` calls (MCP tools and CLI runs, subagents included) and the brain CLI runs that were allowed, with the subcommands used. It also records claude's CPU seconds (its own plus the children it waited for). A summary or compare then shows whether the steer took and what the search cost the machine. The `fleet-*` cases ask questions across every checkout under one directory; they are the ones the profiles are for. The profile, steer file and `-no-grep` are recorded in `run.json`.

## Workspaces

- `repo:<name or path>` runs in the canonical checkout, so it also denies edit tools, git write verbs and the csl repo tools.
- `worktree:<name or path>` cuts a worktree on a throwaway `eval/<id>-<stamp>` branch under `~/.cache/harness-eval/worktrees/<repo>/`, locked with the runner's pid, and removes both afterwards. A run's start removes only the worktrees under that root whose locking runner has died, and their branches.
- `tmpdir` is an empty temp directory.
- `brain:<fixture>` copies `fixtures/brain/<fixture>/` (`-fixtures`) to `<tmp>/vault` and commits it as a git baseline, writes `<tmp>/config.yaml` pointing brain at that vault with semantic search off, and builds the literal index. claude runs in the empty `<tmp>/work` with `BRAIN_CONFIG=<tmp>/config.yaml` and `XDG_STATE_HOME=<tmp>/state`; your own `BRAIN_*` variables are dropped. The runner builds brain from `services/brain` in the working tree once per run and puts it first on PATH, so a run measures the brain beside the harness. After the run it stages the vault and diffs it against the baseline: untracked files count, and a moved file is a delete and an add.

Every case is denied the worklog, brain and GitHub tools that write, on top of its own list. For repo and worktree cases the runner compares the canonical checkout's HEAD, branch and status before and after. Any change is an error on the row.

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
allowed_tools: [Read]         # optional; replaces the profile's allow list
disallowed_tools: [NotebookEdit]
pending: waiting on a fix     # optional; runs only with -include-pending
graders:
  - type: tool_used
    tool: '^mcp__csl__csl_search$'
    input: 'sym:'             # one regex or a list that must all match
    weight: 2                 # default 1
---
Where is `CanonicalRepo` defined? Answer with the path and line only.
```

Graders run before teardown. `regex` matches the final answer. `file` checks that a path exists and, with `pattern`, that one of its files matches; `absent: true` on either turns it around (the answer must not match, no file may match). `commit` checks every new commit subject in a worktree. `tool_used`, `tool_not_used` and `tool_before` match tool names, with `input` and `input_without` regexes on the input as sorted-key JSON. They and `max_tool_calls` see only the main session unless `include_agents: true`. `max_turns`, `max_cost_usd` and `max_tool_calls` are budgets. `llm` has a hookless haiku judge read the `rubric`, with the vault diff after the answer in a brain workspace.

The brain graders resolve paths against the vault. Paths are slash globs where `**` spans directories (`file` takes them too), and `new_only: true` on `file`, `frontmatter` and `wikilinks` looks only at the files the run added.

| Grader | Fields | Passes when |
|---|---|---|
| `frontmatter` | path, key, pattern, new_only | a matching file has the key with a matching value; a list is matched one item per line, so use `(?m)^tag$` |
| `new_notes` | dir, min, max, content_regex | the `.md` files added under dir (the whole vault when unset) number min to max, and each matches content_regex |
| `vault_unchanged_except` | paths | every added, modified or deleted vault path matches one of the globs; with none, nothing may change |
| `wikilinks` | path, min, new_only | every `[[link]]` in the matching files resolves to a note, by stem or by path, and each file links at least min distinct notes |
| `brain_cmd` | pattern, min, max | min (default 1, or 0 when max is set) to max brain calls have a subcommand label matching the pattern |

`brain_cmd` finds brain calls by splitting each Bash command on unquoted `;`, `&`, `|`, parentheses, backticks and newlines. The label is the first word after `brain`, so `brain write --help` is `write --help`; help lookups and denied calls never count. `new_notes`, `vault_unchanged_except` and `new_only` read the vault's baseline diff, so they need a brain workspace.

## The brain fixture

`fixtures/brain/team/` is the vault of a fictional logistics company, Bramblewick Freight: 28 notes in the frontmatter layout brain writes. `notes/` holds runbooks, system facts (ports, timeouts, owners), decision records and incident learnings. It also holds summaries of public docs and papers, as `type: source` with the URL in `source`. `MOCs/` indexes databases, messaging and operations, `outputs/` has two reports, and `inbox/` has three raw captures. Its traps are deliberate:

- `adr-0011` (Kafka) supersedes `adr-0004` (RabbitMQ), and only the newer one says so.
- `inbox/rp-timeouts.md` is ready to promote, three notes link to it, and its title gives it a new stem when promoted.
- Five notes link `[[pg-failover]]`, in the plain, `|alias` and `#heading` forms; three others link only `[[pg-failover-drill]]`, and one names pg-failover in prose.
- `notes/data-retention-policy.md` carries the extra frontmatter keys `owner` and `classification`, which brain edit drops today.

`vault_test.go` checks that every link resolves and that each trap is in place. It also checks that every note has its tier's status and only keys brain keeps, the policy note excepted. Pin a new fact there when a case starts to depend on it. Some brain cases fail today on purpose: `brain-edit-preserve-frontmatter` because brain edit drops keys it does not know, and `brain-backlinks-exact` because brain has no backlinks command. They are the baseline a fix is measured against. The three pending cases name a future fixture, `team-multi`, for shared and private vaults; their `notes` list what it needs.

## Reading a run

A run writes `runs/<timestamp>/`. `run.json` records the claude version, the model and effort asked for and used, the flags, and the totals. `results.jsonl` has one row per case and rep with the score, each grader's result, usage, main and subagent tool counts, csl and brain calls, CPU seconds and the session id. A brain workspace's row also lists the vault paths the run changed. `streams/` keeps each transcript, `diffs/` each brain workspace's vault diff against its baseline, and `summary.md` lists the rows, their tool calls with the brain subcommands, and every failure. The score is the passed share of grader weight. A row passes only when every grader does. An API error result, an unparsed stream line or a changed checkout puts the row in errors. Rows carry the moondb scorecard, or `unavailable`; a `brain-cli` run keeps no session file, so its rows have none.

Git ignores `runs/`. Copy a settled `summary.md` to `results/<date>-<label>.md` as a baseline. `-compare` totals only the cases both runs have.
