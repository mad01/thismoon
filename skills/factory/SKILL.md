---
name: factory
description: Run a plan with worker subagents (experimental). One orchestrator session splits the plan into units, claims one loom worktree per unit, and sends each worker a task message in a fixed contract. Workers commit in their worktree and answer ready or blocked; the weaver integrates. Use when the user says "factory", "run this plan with workers", "orchestrate workers", or hands over a plan with several independent units to build in parallel.
version: 0.1.0
argument-hint: "run <plan> | brief <unit> | preflight"
---

**Experimental.** The contract, the routing table and the worker definitions
are version one and will change as runs teach us what breaks.

A factory run takes a plan with several independent units through worker
subagents. Each unit gets its own loom worktree, its own branch and its own
worker. The orchestrator and the workers talk only in messages that follow
one contract, so the weaver can land a branch from the message alone.

This file holds everything an action needs. Four files beside it hold the
detail and the reasons:

- `contract.md`: the full message contract, with a sample ready and a sample
  blocked message.
- `routing.md`: the routing table with tier definitions and the override
  rules.
- `worker-brief.md`: the text every worker gets before its first prompt, and
  how belt delivers it.
- `worker-definitions.md`: the three worker agent definitions.

Pick the action from the arguments. `run` takes a plan through the whole
procedure. `brief` writes the task message for one unit and stops.
`preflight` runs the checks and stops.

## Roles

- Orchestrator: the session that holds the plan. It splits, routes,
  claims worktrees, writes tasks and spawns workers. It answers or parks
  questions and owns the board and the worklog checkpoints.
- Worker: a subagent from `worker-definitions.md` that owns one unit and
  one worktree. It commits there and reports. It never integrates.
- Weaver: the loom role that lands ready branches. The orchestrator may
  act as the weaver, or a separate session may; the loom skill's weaver
  rules apply either way.

A run is depth one: the orchestrator spawns every worker itself. The one
exception is a run that exceeds one context window. Then a wave lead, a
general-purpose subagent with this skill, runs steps 6 and 7 for one wave of
units. It spawns its workers unnamed, so their final messages return to it
rather than to the main session, and it returns one message per unit.

## The contract

The transport is SendMessage. An unnamed worker has no SendMessage route
back, so its final reply is its last message. Evidence travels with `ready`:
the weaver reads the message, not the transcript.

Every message starts with these eight header lines, in this order, then one
blank line, then the body:

```
kind: task | status | question | answer | ready | blocked
unit: <plan>/<slug>
worktree: ~/.worktrees/<repo>/<slug>
branch: <branch>
model: <model alias after routing>
effort: low | medium | high
reply_to: <message id> | -
needs_reply: true | false
```

The worktree slug is the branch name with `/` replaced by `-`. A message id
is derived, never a header line: `<unit>#<kind>-<n>`, where `n` counts the
messages of that kind for that unit from 1. A worker copies `unit`,
`worktree`, `branch`, `model` and `effort` from its task.

| Kind | Sender | Body profile | `needs_reply` |
|------|--------|--------------|---------------|
| `task` | orchestrator | procedure | `true` |
| `answer` | orchestrator | description | `false` |
| `status` | worker | description | `false` |
| `question` | worker | description | `true` |
| `ready` | worker | description | `false` |
| `blocked` | worker | description | `true` |

The body follows a light form of Simplified Technical English (STE). A
procedure, the body of a `task`, uses the imperative with one instruction
per sentence and at most 20 words per sentence. Its warnings come first, as
`Do not ...` lines. Its steps are numbered, and the last step names the done
criterion and the message to send. It ends with a line that starts with
`Report format:`, and it never says "you should", "please" or "try to".

A description, the body of every other kind, keeps each sentence to 25 words
or fewer. It uses simple present or simple past tense and active voice, with
at most six sentences per field. Terms come from the repo's `CONTEXT.md`
glossaries, never a synonym their `_Avoid_` lines ban.

A field is a label at the start of a line, then a colon. A single value sits
on the label line; a list follows it, one `- ` item per line.

- `ready` carries `files changed:`, `gates run:` (each item is the command in
  backticks, a colon, then its last output line) and `commit range:`
  (`<base>..<head>`), in that order. Fields the task's report-format line
  asks for follow, each with its own label, and an empty one says `none`.
  `ready` means committed, gated and described.
- `blocked` carries `blocker:` (the blocker in the worker's own words, on the
  label line), `question:` (one question for the human, ending in `?`) and
  `not done:`, in that order. A denial or a tool error goes under the
  `blocker:` line, verbatim, in a fenced code block.
- `question` carries `question:` and `options:`. `answer` carries `answer:`,
  and `status` carries `progress:`.

A denied tool call always becomes `blocked` with the denial text. The worker
does not retry the call and never relays it to another agent or session.

The canonical task, from the factory spec:

```
kind: task
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.
Do not commit outside your worktree.

1. Open README.md and CLAUDE.md of the events service.
2. Reorder the sections to match the shared outline.
3. Replace each banned synonym with the glossary term.
4. Run the humanizer rule pack on both files.
5. Send ready when both files pass.

Report format: one ready message. List the files you changed.
List each term you could not resolve.
```

## Routing

| work-on tier | Model tier | Model today | Effort | Worker definition |
|--------------|------------|-------------|--------|-------------------|
| trivial | mid | Sonnet (`sonnet`) | low | `factory-worker-low` |
| known-domain | mid | Sonnet (`sonnet`) | medium | `factory-worker-medium` |
| novel | top | Opus (`opus`) | high | `factory-worker-high` |

The cheap tier is Haiku (`haiku`); no default route uses it. The orchestrator
may override the model or the effort for one unit, and the header then
carries the override. Reviewers run on the model the panel-policy mod sets.

## Orchestrator procedure

1. Split the plan into units. A unit is one change with one done criterion,
   one branch and one worker. Two units that touch the same file run one
   after the other, never at the same time.
2. Route each unit with the table above. The task header carries the
   result.
3. Claim one loom worktree per unit, each claim as its own command:
   `git -C <canonical> worktree add ~/.worktrees/<repo>/<slug> -b <branch>`.
   Never chain a claim with a pull of the canonical checkout. A chained
   command hides which half failed.
4. Run the preflight below. A failed check stops the run before any spawn.
5. Open the board with `present_create`: one page for the run, one row per
   unit (unit, worker, model and effort, state, last message), and a
   Questions section.
6. Write each task by the contract and check it with
   `humanizer ste --profile procedure`. Spawn the worker with the definition
   for the unit's effort and the task as the prompt. Pass the model on the
   spawn when it overrides the definition's. Name the worker after the unit
   slug so it can message you and receive answers.
7. Read each message as it arrives and update its board row:
   - `ready`: check that it has the three fields, then hand the branch to
     the weaver. The message is the evidence.
   - `blocked`: take the blocker and its question to the user. Never run the
     denied call yourself, and never pass it to another agent or session.
   - `question`: send an `answer` when the plan settles it. Otherwise park
     it.
   - `status`: update the board and nothing else.
8. To park a question, record it in keeper-of-facts (kof) as an
   `open-thread` assertion and add it under Questions on the board. Send the
   worker an `answer` once the user decides.
9. Checkpoint the run in worklog at each wave boundary and before a pause.
   The orchestrator is the only worklog writer.

## Preflight

Run all five checks before the first spawn:

1. `belt doctor` exits 0.
2. The events and present services answer: `events doctor` and
   `present doctor`, or `events_doctor` and `present_doctor` over MCP.
3. The git identity matches: `git -C <worktree> config user.name` and
   `user.email` give the identity the user commits with.
4. Each unit's worktree exists and is clean:
   `git -C <worktree> status --porcelain` prints nothing.
5. The agents listing answers: `ListAgents` returns.

## Rules

- Workers never integrate. `ready` means committed, gated and described.
  From there the weaver owns the branch, per the loom skill.
- One worktree per unit and one worker per worktree. A worker never edits
  or commits outside its worktree.
- A denied tool call becomes a `blocked` message that carries the denial
  text. Nobody retries the call, and nobody relays it to another agent or
  session.
- Questions are parked, never guessed: a kof open-thread plus a row under
  Questions on the board.
- worklog has one writer, the orchestrator.
- Every message passes the STE check for its profile before it goes out.

## Where to find things

- belt hooks, in `tools/belt/README.md` and
  `docs/adr/0024-belt-ledger-and-watcher.md`:
  - The `factory-worker` custom hint runs on subagent-start and delivers
    `worker-brief.md` into each worker's context before its first prompt.
    It is machine-private config; `worker-brief.md` shows the script.
  - `belt record` posts one ledger entry per lifecycle event to the events
    service under `belt-ledger`: subagent start and stop, denials, idle,
    compaction. Each entry carries the session, agent, worktree and branch,
    so a query shows what a worker did.
  - `belt watch` is opt-in. It wakes an idle orchestrator when an item
    addressed to its session lands under `belt-inbox`.
- The STE check, in the STE section of `tools/humanizer/README.md`:
  `humanizer ste --profile procedure|description [--repo <path>] [--json]`.
  Exit 0 is clean, 1 means findings, 2 is an error. Over MCP, call
  `humanizer_detect` with `profile` and `repo`. It is experimental too.
- loom, in `skills/loom/`: claims, the weaver, and teardown.
- panel-policy, in `mods/panel-policy/README.md`: the reviewer model.
- The eval suites for this skill, in `evals/README.md` beside this file.

Every path above is relative to the thismoon repo root, except
`worker-brief.md` and `evals/README.md`, which sit beside this file.
