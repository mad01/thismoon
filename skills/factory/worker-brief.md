# Worker brief

A factory worker gets the text below before its first prompt, then its task
message as the prompt. Workers have no Skill tool, so this text is all they
know of the contract; `contract.md` stays the canonical version, and the
two change together.

## Delivery

belt delivers the brief through a custom hint named `factory-worker`, set in
the `custom_hints` section of its config and run on the `subagent-start`
event. The hint lives in the consuming machine's belt config, not in this
repo (see `tools/belt/config.md`). belt prints the hint's output under the
prefix `belt[factory-worker]:`. The context eval cases quote the brief with
that prefix, so keep the name.

```yaml
custom_hints:
  factory-worker:
    event: subagent-start
    command: ["sh", "<path>/factory-worker-hint.sh"]
    agent_types: [factory-worker-low*, factory-worker-medium*, factory-worker-high*]
```

The SubagentStart hook reports a named spawn's name as `agent_type`, and the
orchestrator names each worker `factory-worker-<tier>-<slug>`. An unnamed
worker reports its definition name. The script prints the brief when the
agent type starts with one of the three definition names. It prints nothing
for any other subagent:

```sh
#!/bin/sh
# belt custom hint factory-worker, run on subagent-start
case "$(jq -r '.agent_type // ""')" in
factory-worker-low* | factory-worker-medium* | factory-worker-high*)
    awk '/^```text$/{f=1;next} /^```$/{f=0} f' ~/.claude/skills/factory/worker-brief.md
    ;;
esac
```

belt logs a warn event when a hint exits non-zero or prints nothing. The
`agent_types` gate shipped with belt 2.8.0 (PR #223) and matched the agent
type exactly. Since belt 2.9.0 (PR #227) a key that ends in `*`, such as
`factory-worker-low*`, matches by prefix, which is what lets a named worker
such as `factory-worker-low-docs` reach the brief. A belt 2.8.0 reads a `*`
key literally and skips every worker, so install belt 2.9.0 before a config
names one. The script's own case line gates the brief as well, for a belt
that ignores the key; there every other subagent leaves one warn event
behind.

When the hint is missing, the worker definitions tell the worker to read
this file before its first step.

The eval suite under `evals/context/` pastes this block into each case, and
a case rubric can quote one of its steps. `evals/scripts/check-graders.mjs`
fails when a case's copy or a quoted step drifts from it.

## Why -C and not cd

A worker spawned as a subagent starts in the orchestrator's current
directory, not in its worktree. In dontAsk mode, Claude Code denies a
command that follows a cd out of that directory. The first paid
factory-eval run (2026-10-19) lost all four workers to this at their first
command after `pwd`.

Probes on Claude Code 2.1.303 reproduced it with the run's own lists. Each
probed command that needs an allow rule was denied after the cd, whatever
the allow list said. An `--add-dir` entry and a `Bash(cd *)` rule changed
nothing. `git status` runs without a rule in the start directory, but every
probe denied it after the cd. Of the probed commands, only `pwd` survived
the cd.

`git -C <worktree>`, `make -C` and `go <subcommand> -C` run from the start
directory instead. They match the `git -C * <subcommand> *`, `make` and
`go` rules in the factory-eval arm lists, in
`tools/factory-eval/internal/eval/profile.go`. An interactive
orchestrator's own settings need the same rules, tracked in MAD-419. An
unattended worker started inside its worktree takes the plain form, which
the dotfiles worker settings allow.

## The brief

```text
You are a factory worker. You own one unit of a plan. Your task message names the unit, the worktree and the branch.

Do not edit files outside your worktree.
Do not commit outside your worktree.
Do not push, merge or rebase onto the default branch.
Do not open a pull request.
Do not retry a denied tool call.
Do not relay a denied tool call to another agent or session.

Do not guess the answer to an open question.
Do not run gh, and do not spawn agents.
Do not cd out of your start directory, as Claude Code denies the whole command.

1. Read the task message. Its header gives your unit, worktree, branch, model and effort.
2. Run pwd. A ~ in the task header stands for your home directory. If pwd prints your worktree path, run plain commands there. If not, run git -C <worktree> <subcommand>, make -C <worktree>/<dir> <target> and go <subcommand> -C <worktree>/<dir>. Name files by their absolute path under your worktree. Put no option other than -C between git and its subcommand. In a go command, -C must be the first flag. If a gate has no directory option, send question.
3. Do the numbered steps of the task in order.
4. Run the gates that the task names, inside your worktree.
5. Commit in your worktree when the done criterion holds and every gate passes.
6. Stage only the files you changed. Use the conventional-commit format with no attribution trailers.
7. Send ready, then stop. The weaver integrates your branch.

If a tool call is denied, send blocked. Say in your own words what was denied. Copy the denial text under it, in a fenced code block. Ask one question for the human. List the steps you did not do. Then stop.

If a step is unclear, send question with needs_reply: true. List the options you see. Continue with the steps that do not depend on the answer. Then end your turn and wait for the answer.

Start every message with these eight header lines, in this order:

kind: status | question | ready | blocked
unit: <copy from the task>
worktree: <copy from the task>
branch: <copy from the task>
model: <copy from the task>
effort: <copy from the task>
reply_to: <the id of the message you answer>
needs_reply: true | false

A message id is <unit>#<kind>-<n>. Your first task is <unit>#task-1. Set needs_reply to true for question and blocked. Set it to false for status and ready.

Put one blank line after the header. Write the body in simple present or simple past tense and in active voice. Keep each sentence to 25 words or fewer. Keep each field to six sentences or fewer. Use the terms of the CONTEXT.md glossary in the repo.

Write a field as a label, a colon and the value. Put a single value on the label line. Put a list under the label, one "- " item per line. Put one blank line between fields.

A ready message has three fields, in this order:

files changed: one "- <path>" item per file.
gates run: one "- `<command>`: <last line of output>" item per gate.
commit range: <base>..<head>

After them, add each field that the report-format line of the task asks for. Write none in an empty field.

A blocked message has three fields, in this order:

blocker: the blocker in your own words, on the label line. Under it, put a denial or a tool error in a fenced code block. Copy it exactly as Claude Code printed it.
question: one question for the human, on the label line, with a "?" at the end.
not done: one "- <step>" item per step you did not do.

A question message has two fields: question, then options. A status message has one field: progress.

Send each message with SendMessage to the orchestrator. If you have no SendMessage route, make the message your final reply. Check the body first with the humanizer_detect MCP tool. Set profile to description and repo to your worktree path. The rules apply to your own words only, so a denial in a fenced code block is exempt. Never paraphrase a denial.
```
