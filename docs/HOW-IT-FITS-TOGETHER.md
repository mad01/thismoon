# How it fits together

Every component in this repo works standalone: `brew install` one and stop
reading. This page is for the other path: what the pieces add up to when they
run as a fleet, the vocabulary the recipes are written in, and the order to
roll it out. The step-by-step install lives in
[GETTING-STARTED.md](GETTING-STARTED.md); a worked private-config repo lives
in [`examples/dotfiles/`](../examples/dotfiles/).

## Why a fleet instead of a pile of tools

The components are designed around one idea: **you and your agent should work
against the same local data, and the connections between tools should fire
without anyone remembering to use them.** Standalone, each tool is a thing
you invoke. Together, they invoke each other.

A concrete session on a fully wired machine:

1. You open Claude Code in a repo. Before you type anything, belt's
   SessionStart hooks inject two things: the shared agent-memory index (facts
   you told any agent to remember, on any machine) and the repo's stored
   keeper-of-facts (kof) assertions: conclusions previous sessions derived
   about this code, each pinned to the lines that prove it.
2. The agent searches with `csl_search`, the same zoekt index you query in
   the browser at `csl.this`. belt's search hint surfaces any kof assertions
   about the code the search just hit, and marks the ones whose pinned lines
   have changed since they were written.
3. The agent slips into habit and runs a recursive grep instead. belt hands
   back the equivalent `csl_search` call with the pattern already
   translated. Not a lecture, a replacement.
4. The agent tries `git push origin main`. Denied, with the
   branch-and-push-upstream fix in the denial reason. It writes a doc that
   mentions an internal hostname into a public repo. Denied at the Write
   call; and had that slipped through, the suspenders pre-commit hook (same
   name derivation, same config) would have caught it at commit.
5. The agent posts a PR comment through an MCP server. belt nudges it once:
   run `humanizer_detect` before more text leaves the machine.
6. Thirty tool calls in, belt asks the session to deposit what it derived as
   a kof assertion, so the next session starts where this one ended instead
   of re-deriving it.
7. Every deny and every hint landed in the events timeline (`events.this`),
   so you can see what the guardrails actually did this week. `status.this`
   watches the services; `deps` scans the dependencies your repos pin;
   the agent publishes its findings as a briefing page you read at
   `present.this`.

None of that needed an instruction in a prompt. That is the pitch: prose
instructions get skimmed under context pressure; hooks, hints, and shared
services fire deterministically. Each connection is small, and they
compound.

## The three layers

```
thismoon (public)      components + recipes/: the code and how to install it
   ↑ consumed by
ralph (public)         the installer: reconciles machines against TOML recipes
   ↑ configured by
your config repo       machine-private wiring: secrets, hosts, hooks, overlays
(private)
```

The split between the bottom two layers is a rule, not a habit
([ADR-0006](adr/0006-recipe-layering-and-platform-deps.md)): thismoon's
recipes carry only what is portable: build, install, service registration.
Anything that names your machines, your internal checkouts, your MCP set, or
your agent settings lives in your own private repo as small companion
recipes layered on top. That is why the repo can be public while the setup
it describes is personal.

Two components are the platform's foundations rather than peers: **t-man**
runs every service as a launchd agent, and **d-man** gives each one its
`<name>.this` address. They are the only recipes anything else may hard
depend on across the source boundary.

## The ralph vocabulary

The recipes are written in ralph's terms. Five of them carry the whole
model; ralph's own docs
([configuration](https://github.com/mad01/ralph/blob/main/docs/configuration.md),
[recipes](https://github.com/mad01/ralph/blob/main/docs/recipes.md)) have the
detail.

- **Recipe**: a `recipe.toml` declaring items to converge, packages to
  build, directories to create, files to symlink, hooks to run. thismoon
  ships one per component under `recipes/`.
- **Recipe source**: a `[[recipe_sources]]` stanza in your ralph config
  pointing at a git repo of recipes. ralph clones it to
  `~/.config/ralph/sources/<name>` and merges each recipe under the identity
  `<source>/<recipe>` (so this repo's belt recipe becomes `thismoon/belt`).
  With `ref = "main"` and `update = true`, merging to main is the deploy.
- **Item key**: the name of one item (`packages.belt`,
  `builds.csl_register`). Keys are global across all loaded sources, which
  is what lets your private recipe declare
  `depends_on = ["packages.suspenders"]` on a package the public source
  defines.
- **Wave**: coarse ordering. Lower waves complete before higher ones start.
  thismoon's recipes are all wave 0 so every binary exists before your
  wave-1 companion recipes wire things up to them; within a wave,
  `depends_on` orders items. The wave is the only ordering primitive that
  works across the public/private boundary, since cross-source `depends_on`
  is reserved for the foundations.
- **Profile**: a freeform label (`personal`, `work`) a machine declares in
  its git-ignored `~/.config/ralph/config.local.toml`, set with
  `ralph profile set`. A recipe or source listing profiles applies only on
  machines that carry one of them. Profiles answer "what kind of machine is
  this", and not only for ralph: belt reads the same file at runtime, so
  its `git-push-main` guard steps aside on `personal` machines and blocks on
  everything else. **Set profiles before the first `ralph up`**: a machine
  with none silently skips every profile-gated recipe, and `ralph doctor` is
  the only thing that will tell you.

## The rollout, in order

Each step links to where it is actually documented; this list exists so the
order is written down somewhere.

1. **Install ralph** and run `ralph init`
   ([GETTING-STARTED.md](GETTING-STARTED.md), "The fleet with ralph").
2. **Create your private config repo** and point `ralph init`'s
   `dotfiles_repo_path` at it. Start from
   [`examples/dotfiles/`](../examples/dotfiles/); it is a working layout,
   not a sketch.
3. **Declare your profiles**: `ralph profile set personal` (or `work`, or
   both). This writes `config.local.toml`, which stays out of git.
4. **Add the thismoon source** (the `[[recipe_sources]]` stanza in your
   `config.toml`) and run **`ralph up`**. Everything builds from source
   into `~/code/bin`; services register with t-man.
5. **The one sudo step**: write your `routes.toml` and register the d-man
   daemon ([GETTING-STARTED.md](GETTING-STARTED.md), step 5). Verify with
   `ralph doctor`, `t-man list`, and `status.this`.
6. **Layer your companion recipes**, one pattern at a time, each with a
   worked example in `examples/dotfiles/recipes/`: MCP registration (which
   servers your agent sees), the belt config plus the Claude Code hooks
   block (`claude-hooks/`, which is what turns the guards and hints on),
   per-service config overlays, and secrets.
7. **Give the agent its instructions**: adapt
   [`examples/dotfiles/CLAUDE.md.example`](../examples/dotfiles/CLAUDE.md.example)
   into your `~/.claude/CLAUDE.md`. It is an anonymized version of a real
   daily-driven file, the instruction-layer counterpart of everything
   above.

From then on: a change merges to thismoon's main, the next `ralph up`
rebuilds it, t-man restarts the service. Verify with the component's
`/version` endpoint; ralph can report ok while an old binary keeps running.

## The guardrail pair: belt and suspenders

Both exist because of the same retrospective: a session pushed straight to
master, and internal names reached `git commit` twice before anything caught
them. They cover the same risks at different moments:

| | belt | suspenders |
|---|---|---|
| Sits at | the agent session (Claude Code hooks) | git (pre-commit hook) |
| Blocks | risky tool calls before they run | secrets and internal names before they commit |
| Sees | only what the agent does | everything that reaches git, human edits included |
| Reference | [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md) | [tools/suspenders/README.md](../tools/suspenders/README.md) |

Each tool reads only its own config (docs/adr/0010), but the two
internal-name sections share one shape and one derivation: both build the
blocked set from your checkouts rather than enumerating names in a file (a
list of internal names would itself be the leak), so identical config blocks
produce identical block lists. belt is the early, agent-only layer;
suspenders is the backstop that also covers you. Two things to know going
in: belt's hooks do nothing until your settings register them (step 6
above), and both name guards ship with nothing to match until you configure
their sections: belt's `internal_names` and suspenders' `guard:`. Both
layers also ship a documented escape ladder (override switches, allowlists,
per-repo files, kill switches) in the two references above, because a guard
you cannot get past when it is wrong teaches you to bypass the right blocks
too.

## The memory stack

Four stores, one question each. Hooks prompt every one of them; none depend
on remembering to use them.

- **agent-memory**: durable facts about *you*, portable across agents and
  machines (a git repo of one-fact files). belt injects its index at session
  start.
- **keeper-of-facts (kof)**: claims about *code*, each pinned to file
  lines. The pins let `kof check` flip a claim stale when the code moves, so
  the memory decays detectably instead of silently going wrong. belt
  surfaces assertions at session start and after searches, and nudges
  deposits.
- **worklog**: where a *task* stands, keyed by ticket or topic rather than
  directory, so a fresh session can resume mid-task.
- **events**: what actually *happened*. Every guard deny, every hint, every
  service's audit trail, queryable on one timeline.

## Skills

Six agent skills ship in `skills/` at the repo root, one directory per
skill; each recipe symlinks its skill into `~/.claude/skills` and
`~/.agents/skills`, so a provisioned machine has them in every session (for
Claude Code and Codex both). Invoke one by name (`/golang-style`, `/handoff`,
`/humanizer`, `/loom`, `/present`, `/worklog`) or let the agent load it when
a task matches its description.

| Skill | Use it when | Needs |
|---|---|---|
| `golang-style` | writing or reviewing Go in this codebase's idiom | nothing |
| `handoff` | a session ends mid-task and the next one starts cold | nothing |
| `humanizer` | prose is about to land in docs, a PR description, or a commit body | the humanizer MCP registered |
| `loom` | several sessions work one repo in parallel, each in its own git worktree | nothing |
| `present` | a work summary or research result deserves a readable briefing page | the present service running |
| `worklog` | a long task needs checkpointing across sessions | the worklog MCP registered |

The skill is the workflow; the MCP server is its hands. Without the backing
MCP a skill still loads, but its tool-backed steps have nothing to call;
MCP registration is machine-private wiring (step 6 above).

## Where to go next

- [GETTING-STARTED.md](GETTING-STARTED.md): the install walkthrough both
  for one tool and for the fleet.
- [`examples/dotfiles/`](../examples/dotfiles/): the private-repo layout,
  every overlay pattern worked, and the example global CLAUDE.md.
- [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md): every guard and
  hint in depth.
- [ADR-0006](adr/0006-recipe-layering-and-platform-deps.md): why the
  public/private split is a rule; [ADR-0008](adr/0008-belt-hints-alongside-guards.md):
  why hints advise instead of deny.
- [RELEASING.md](RELEASING.md): per-component releases, for the
  non-fleet path.
