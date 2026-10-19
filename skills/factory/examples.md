# Factory examples

**Experimental**, like the skill. This file shows one factory run end to
end, in the order the orchestrator procedure in `SKILL.md` takes. It covers
the plan, the split, the claims, the preflight, the board, the task
messages, what comes back, and the weaver pass that lands the branches. The
plan and its units are invented for the page. Every message follows `contract.md` and
passes `humanizer ste` for its profile, and every command is one the
procedure names. Read `SKILL.md` first; this file shows what its steps look
like when they run.

## The plan

The user hands the orchestrator a plan in an interactive session whose
working directory is the canonical checkout:

```
/factory run readme-doctor

Three units, independent, all in thismoon:
1. deps: tools/deps/README.md names two flags the CLI renamed. Fix the names. Trivial.
2. events: add a section on a full store to services/events/README.md, in the form the other services use. Known domain.
3. agentdoc: decide how a README declares its doctor checks, and write it down in kit/agentdoc. Novel.
```

A unit that comes from a work-on ExecutionPlan already carries a model and
an effort from `routing.md`. A plan typed by hand gets them in step 2.

## Steps 1 and 2: split and route

The orchestrator writes the unit table before it touches git. One change,
one done criterion, one branch and one worker per unit. The table carries
the routing result, never the default the plan started from:

| Unit | work-on tier | Model | Effort | Worker | Branch |
|------|--------------|-------|--------|--------|--------|
| readme-doctor/deps | trivial | sonnet | low | factory-worker-low | alexander/readme-doctor-deps |
| readme-doctor/events | known-domain | sonnet | medium | factory-worker-medium | alexander/readme-doctor-events |
| readme-doctor/agentdoc | novel | opus | high | factory-worker-high | alexander/readme-doctor-agentdoc |

Two units that touch the same file run one after the other. These three
share no file, so they run at the same time.

## Step 3: claim the worktrees

One claim per command, each command its own tool call, run in the
canonical checkout:

```sh
git worktree add ~/.worktrees/thismoon/alexander-readme-doctor-deps -b alexander/readme-doctor-deps
git worktree add ~/.worktrees/thismoon/alexander-readme-doctor-events -b alexander/readme-doctor-events
git worktree add ~/.worktrees/thismoon/alexander-readme-doctor-agentdoc -b alexander/readme-doctor-agentdoc
```

The slug is the branch name with `/` replaced by `-`. Never chain a claim
with a pull of the canonical checkout: a chained command hides which half
failed.

## Step 4: preflight

Five checks, each its own command, before the first spawn. A clean pass
looks like this:

```sh
belt doctor                                  # exit 0
events doctor                                # exit 0, or events_doctor over MCP
present doctor                               # exit 0, or present_doctor over MCP
git var GIT_AUTHOR_IDENT                     # <name> <email> <timestamp>
git var GIT_COMMITTER_IDENT                  # the same name and email
git -C ~/.worktrees/thismoon/alexander-readme-doctor-deps status --porcelain      # prints nothing
git -C ~/.worktrees/thismoon/alexander-readme-doctor-events status --porcelain   # prints nothing
git -C ~/.worktrees/thismoon/alexander-readme-doctor-agentdoc status --porcelain  # prints nothing
```

Then `ListAgents` has to return. A failed check stops the run before any
spawn. Check 4 is one command per worktree: a shell loop over them matches
no allow rule, so an unattended run denies it.

## Step 5: the board

`present_create` opens one page for the run, named after the plan. It has
one row per unit and a Questions section. The page id goes into the first
worklog checkpoint, so a later session finds the board again. At the start
every row reads the same way:

```
unit                     worker                                       model, effort   state     last message
readme-doctor/deps       factory-worker-low-readme-doctor-deps        sonnet, low     claimed   -
readme-doctor/events     factory-worker-medium-readme-doctor-events   sonnet, medium  claimed   -
readme-doctor/agentdoc   factory-worker-high-readme-doctor-agentdoc   opus, high      claimed   -

Questions: none
```

## Step 6: the task messages and the spawns

The task for the events unit. The warnings come first, the steps are
numbered, the last step names the done criterion and the message to send,
and the report-format line closes it:

```
kind: task
unit: readme-doctor/events
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-events
branch: alexander/readme-doctor-events
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit files outside services/events.
Do not change Go source files.
Do not commit outside your worktree.

1. Read services/events/README.md and the READMEs of two other services.
2. Add a section named "when the store is full" under the Gotchas section.
3. Describe the three checks a reader runs, one paragraph each, in the form the other docs use.
4. Run make -C services/events test in your worktree.
5. Run humanizer detect on the file.
6. Commit when both pass, then send ready.

Report format: one ready message. List the files you changed.
Name the two READMEs you used as the form.
```

Check it before it goes out:

```sh
printf '%s' "$TASK" | humanizer ste --profile procedure --repo ~/code/src/github.com/mad01/thismoon -
```

Then spawn the worker with the definition for the unit's effort, the task
as its prompt, and the name the hint gate and the ledger key on:

```
Agent(
  subagent_type: factory-worker-medium,
  name: factory-worker-medium-readme-doctor-events,
  prompt: <the task message above>,
)
```

Pass `model` on the spawn only when the header overrides the definition's
model. All three spawns go in one message, so the workers run at the same
time. belt's `factory-worker` hint prints the worker brief into each
worker before its first prompt, and the ledger tags the start with the
model and effort of its definition.

## Step 7: what comes back

### A status message

```
kind: status
unit: readme-doctor/deps
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-deps
branch: alexander/readme-doctor-deps
model: sonnet
effort: low
reply_to: readme-doctor/deps#task-1
needs_reply: false

progress: I renamed both flags in tools/deps/README.md. The test run is next.
```

The board row changes, and nothing else happens.

### A question and its answer

The agentdoc worker reaches a choice the plan does not settle:

```
kind: question
unit: readme-doctor/agentdoc
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-agentdoc
branch: alexander/readme-doctor-agentdoc
model: opus
effort: high
reply_to: readme-doctor/agentdoc#task-1
needs_reply: true

question: Does the list of doctor checks live in README.md or in a new file beside it?

options:
- A section in README.md, so one file stays the source.
- A doctor.md beside it, so the README stays short.
```

The orchestrator does not guess. It parks the question: a keeper-of-facts
(kof) assertion of kind `open-thread` under the subject
`repo:mad01/thismoon/kit/agentdoc`, and a row under Questions on the board.
The worker goes on with the steps that do not depend on the answer, then
waits. When the user decides, the orchestrator answers and resolves the
open thread:

```
kind: answer
unit: readme-doctor/agentdoc
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-agentdoc
branch: alexander/readme-doctor-agentdoc
model: opus
effort: high
reply_to: readme-doctor/agentdoc#question-1
needs_reply: false

answer: Put the list in README.md, as a section named "doctor checks". One file stays the source.
```

### A blocked message and the round trip after it

The deps worker meets a denial at step 4. It sends `blocked` with the
denial copied exactly, and it does not retry the call. The outer fence has
four backticks so the inner block shows:

````
kind: blocked
unit: readme-doctor/deps
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-deps
branch: alexander/readme-doctor-deps
model: sonnet
effort: low
reply_to: readme-doctor/deps#task-1
needs_reply: true

blocker: Step 4 was denied, so I stopped before the test run.
```
Permission to use Bash with command make -C tools/deps test has been denied.
```

question: Do you allow make -C tools/deps test in this worktree, or do you run it yourself?

not done:
- Step 4, the test run.
- Step 6, the commit.
````

The orchestrator takes the blocker and the question to the user. It never
runs the denied call itself and never hands it to another agent or
session. The user either runs the gate by hand, with `! make -C
~/.worktrees/thismoon/alexander-readme-doctor-deps/tools/deps test` in
the orchestrator's session, or changes the allow list for the next run.
Either way the worker gets a new task that answers the blocked message:

```
kind: task
unit: readme-doctor/deps
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-deps
branch: alexander/readme-doctor-deps
model: sonnet
effort: low
reply_to: readme-doctor/deps#blocked-1
needs_reply: true

Do not retry the denied call.

1. Leave the test gate out. The user ran make -C tools/deps test in your worktree, and it passed.
2. Commit the renamed flags, then send ready.

Report format: one ready message. List the files you changed.
```

### Ready

```
kind: ready
unit: readme-doctor/deps
worktree: ~/.worktrees/thismoon/alexander-readme-doctor-deps
branch: alexander/readme-doctor-deps
model: sonnet
effort: low
reply_to: readme-doctor/deps#task-2
needs_reply: false

files changed:
- tools/deps/README.md

gates run:
- `make -C tools/deps test`: ok github.com/mad01/thismoon/tools/deps 0.412s
- `humanizer detect tools/deps/README.md`: no findings

commit range: 3aa0a4e..b17c0d2
```

The orchestrator checks that the three fields are there and hands the
branch to the weaver. The message is the evidence; the weaver reads it,
not the transcript. Stop the worker as soon as its unit is ready or
blocked. An idle worker still holds its context and its worktree.

## The weaver pass

The orchestrator may act as the weaver, or a second session runs `/loom
weave`. The loom skill's rules apply either way. Per ready branch, oldest
first, the weaver proves the claim from git, rebases, re-runs the gates and
lands the branch by repo policy. thismoon is PR-only, so landing means a
push, a pull request, and a merge when the user says so:

```sh
C=~/code/src/github.com/mad01/thismoon
W=~/.worktrees/thismoon/alexander-readme-doctor-deps
git -C $C fetch origin
git -C $W status --porcelain                                       # prints nothing
git -C $C log --oneline origin/main..alexander/readme-doctor-deps # the commits on offer
git -C $C merge-tree --write-tree origin/main alexander/readme-doctor-deps
git -C $C rev-parse 'origin/main^{tree}'                           # differs from the line above: not integrated yet
git -C $W rebase origin/main
make -C $W/tools/deps test
git -C $W push -u origin alexander/readme-doctor-deps
```

Then the pull request goes up through the GitHub MCP, and the merge waits
for the user. After it, the two tree ids above print the same value, and
teardown follows `loom.md`: `git worktree remove`, then `git branch -d`,
with the proof shown on the lines above the delete. A rebase conflict goes
back to the branch's owner; the weaver moves on to the next branch.

## One unit, no run

`/factory brief readme-doctor/events` writes the task message for one
unit and stops. Use it to hand a unit to a background session or an
unattended worker, or to check a task with `humanizer ste` before a run.
`/factory preflight` runs the five checks and stops.

## An unattended worker

A worker can run outside the orchestrator's session, as `claude -p`
started inside its claimed worktree, with the task as its prompt. It gets
the consuming repo's worker settings file: the dontAsk mode, an allow list
scoped to the working directory, and a deny list. It also gets the agents
file with the three definitions, and the csl, humanizer and kof MCP
servers. It has no SendMessage route back, so its final reply is its
message, in the same contract. That wiring is machine-private (ADR-0006)
and lives in the consuming repo's factory recipe, not here.

## Measuring a run: factory-eval

`tools/factory-eval` is the factory used as an instrument. It runs the
same plan form through two arms at an equal dollar cap. One arm is an
orchestrator session with a worker per unit. The other is one session that
does the units in turn. A dry run costs nothing and rehearses everything
but the model calls. A paid run is started by hand:

```sh
cd tools/factory-eval
factory-eval run --dry-run --repo ~/factory-eval-source --out ~/factory-eval-results
factory-eval run --repo ~/factory-eval-source --tiers mid --runs 1 --out ~/factory-eval-results
```

Read `permission_denials` for both arms before the scores. A worker denied
its first command scores zero for a reason that has nothing to do with the
factory.

## Checking a message before it goes out

```sh
printf '%s' "$TASK"  | humanizer ste --profile procedure   --repo ~/code/src/github.com/mad01/thismoon -
printf '%s' "$READY" | humanizer ste --profile description --repo ~/code/src/github.com/mad01/thismoon -
```

Exit 0 is clean, 1 means findings, 2 is an error. Over MCP,
`humanizer_detect` takes the same `profile` and `repo`. A denial inside a
fenced block is exempt from the rules. Never paraphrase a denial to make it
pass.

## What runs have shown

- A worker starts in the orchestrator's working directory, not in its
  worktree. The brief tells it to run `pwd` first and to put `cd
  <worktree> &&` in front of each command when it has to.
- Spawn every worker of a wave in one message, and name each one. An
  unnamed worker cannot message you; its final reply is its message.
- Messages cross. An answer sent while the worker is mid-turn is read at
  its next tool round. When an idle notice omits the answer, send it again.
- One command per claim and per preflight check. A loop or a variable
  assignment matches no allow rule, so an unattended run denies it.
- Stop a worker the moment its unit is ready or blocked.
