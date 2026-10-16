# Message contract

Every message between the orchestrator and a worker has the same shape. It
has eight header lines, one blank line, then a body in STE-lite, a light
form of Simplified Technical English (STE). The transport is SendMessage. An
unnamed worker has no SendMessage route back, so its final reply is its
last message and follows the same contract.

The weaver reads the message, not the transcript. Evidence travels with
`ready`: if a fact is not in the message, the weaver does not have it.

## Header

Eight lines, in this order, one `key: value` per line, nothing between them:

| Key | Value |
|-----|-------|
| `kind` | `task`, `status`, `question`, `answer`, `ready`, or `blocked` |
| `unit` | the unit id from the plan, `<plan>/<slug>`, for example `docs-sweep/events` |
| `worktree` | the claimed worktree, `~/.worktrees/<repo>/<slug>` |
| `branch` | the worktree's branch |
| `model` | the model alias the worker runs on, after any override (see `routing.md`) |
| `effort` | `low`, `medium`, or `high` |
| `reply_to` | the id of the message this one answers, or `-` |
| `needs_reply` | `true` or `false` |

A worker copies `unit`, `worktree`, `branch`, `model` and `effort` from its
task. The header always carries the routing result, never the default the
plan started from.

A message id is not a header line. Derive it as `<unit>#<kind>-<n>`, where
`n` counts the messages of that kind for that unit, starting at 1. The first
task for `docs-sweep/events` is `docs-sweep/events#task-1`, and the `ready`
that answers it carries `reply_to: docs-sweep/events#task-1`.

## Kinds

| Kind | Sender | Body profile | `needs_reply` | `reply_to` |
|------|--------|--------------|---------------|------------|
| `task` | orchestrator | procedure | `true` | `-`, or the `blocked` or `question` it settles |
| `answer` | orchestrator | description | `false` | the question's id |
| `status` | worker | description | `false` | the task's id |
| `question` | worker | description | `true` | the task's id |
| `ready` | worker | description | `false` | the task's id |
| `blocked` | worker | description | `true` | the task's id |

## The body

STE is the controlled language of aircraft maintenance manuals; the body
follows a light version of it. Two profiles apply.

A `task` is a **procedure**:

- Imperative mood, one instruction per sentence, at most 20 words per
  sentence.
- Warnings come first, as commands on their own lines: `Do not ...`.
- Steps are numbered. The last step names the done criterion and the message
  to send when it holds.
- The body ends with a report-format line that starts with `Report format:`.
- No hedges: no "you should", no "please", no "try to".

`status`, `question`, `answer`, `ready` and `blocked` are **descriptions**:

- At most 25 words per sentence.
- Simple present or simple past tense, active voice.
- At most six sentences per field.

Vocabulary comes from the repo's `CONTEXT.md` glossaries: use the bold term,
never a synonym its `_Avoid_` line bans. The official STE dictionary is not
used.

Check a message before you send it:

```sh
printf '%s' "$MSG" | humanizer ste --profile procedure --repo <checkout> -
printf '%s' "$MSG" | humanizer ste --profile description --repo <checkout> -
```

Exit 0 is clean, 1 means findings, 2 is an error. The `humanizer_detect` MCP
tool takes the same `profile` and `repo` parameters. A denial or a tool error
goes verbatim into a fenced code block, and the STE rules apply only to the
words outside it. Never paraphrase a denial to make it pass.

## Fields

A field is a label at the start of a line, a colon, then its value. A
single value sits on the label line. A list follows the label, one `- ` item
per line. Fields are separated by one blank line.

`ready` carries three fields, in this order:

- `files changed:` a list of repo-relative paths.
- `gates run:` a list, one gate per item: the command in backticks, a colon,
  then the last line it printed.
- `commit range:` `<base>..<head>` as short hashes, on the label line.

Fields that the task's report-format line asks for come after the three, each
with its own label, for example `unresolved terms:`. An empty answer is
written as `none`, never left out.

`ready` means the work is committed in the worktree, every gate passed, and
the message describes it. Never send `ready` with a dirty tree or a failed
gate.

`blocked` carries three fields, in this order:

- `blocker:` the blocker in the worker's own words, on the label line, in at
  most six sentences. A denial or a tool error follows on the next lines,
  copied exactly as Claude Code printed it, inside a fenced code block.
- `question:` one question for the human, on the label line, ending in `?`.
- `not done:` a list of the steps or actions the worker did not do.

A denied tool call always becomes `blocked`. The worker does not retry the
call and never relays it to another agent or session.

`question` states the open point on the label line `question:` and the
options the worker sees under `options:`. The worker does not guess. It
continues with the steps that do not depend on the answer, then waits.

`answer` carries `answer:` with the decision. `status` is free description
under `progress:`.

## Examples

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

The `ready` that answers it:

```
kind: ready
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: docs-sweep/events#task-1
needs_reply: false

files changed:
- services/events/README.md
- services/events/CLAUDE.md

gates run:
- `humanizer detect services/events/README.md`: no findings
- `humanizer detect services/events/CLAUDE.md`: no findings

commit range: 3f1c2ab..9e04d17

unresolved terms:
- sink: the glossary has no entry for it, so the word stays.
```

A `blocked` after a denial. The outer fence has four backticks so the inner
fenced block shows:

````
kind: blocked
unit: csl/prune-shards
worktree: ~/.worktrees/thismoon/alexander-csl-prune-shards
branch: alexander/csl-prune-shards
model: sonnet
effort: medium
reply_to: csl/prune-shards#task-1
needs_reply: true

blocker: Step 4 was denied, so I stopped and did not retry it.
```
Permission to use Bash with command make -C services/csl install has been denied.
```

question: Do you run the install step yourself, or do I skip it and send ready after the tests?

not done:
- Step 4, the install.
- Step 5, the `/version` check of the installed service.
````
