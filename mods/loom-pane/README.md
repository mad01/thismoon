# loom-pane

A Claude Code mod for sessions that follow the loom skill. It names the session's role in the status line under the prompt, lists the factory members beside it, and turns the skill's rules into guards. The skill is unchanged; without the mod, loom works as before.

The name is historical. The mod once drew a pane of the repo's worktrees; the pane is gone, and the name stays because `enabledPlugins` on every machine keys on it.

## Role and status line

At `session.start` (and again after a resume or `/clear`) the mod runs `git rev-parse --show-toplevel` and `--git-common-dir`; the canonical checkout is the parent of the common git dir. It then reads `git worktree list --porcelain` from the canonical checkout, once, for the paths the guards judge by, and reads it again when the session claims a worktree. Nothing polls git on a timer. The role follows from there:

- owner: the session's top level is `~/.worktrees/<repo>/<slug>`, or a Bash command in the main loop enters one (`cd ~/.worktrees/<repo>/<slug>`, `git worktree add ~/.worktrees/<repo>/<slug> ...`) and runs; a command that a guard, belt or the person denies claims nothing. The skill's own flow claims from the canonical checkout and commits with `cd <worktree> && git commit`, so this is how the role usually turns up. The status line reads `owner <slug>`.
- weaver: `/loom weave` was typed, run as the skill, or sent as a slash command, in any checkout of the repo. A weaver stays a weaver when it `cd`s into a worktree to rebase. The line reads `weaver · 3 worktrees`, counting the linked worktrees git listed.
- none: anything else; the line is cleared.

The engine prefixes a plugin's status line with its name, so on screen the line reads `loom-pane: owner feat-pane`; the text itself never repeats the name.

## Members

After the role, the line lists who works beside this session, each as name, role, worktree, model and effort, with a session's state after them: `weaver · 3 worktrees · members w1 (worker-high, agent-a1b2, -, -), band fixer (owner, fix-band, -, -) blocked`. With no role the members stand alone; with neither the line is cleared.

- Who: this session's live agents that run a factory worker definition, or work in a loom worktree (`~/.worktrees/<repo>/<slug>`) or an isolation worktree (`.claude/worktrees/agent-<id>`). Then each session `claude agents --json --all` lists in such a worktree, unless it is done or is this session. A plain named teammate is not a member.
- Role: `worker-low`, `worker-medium` or `worker-high` from the `factory-worker-*` definition, `owner` for an agent or session in a worktree. This session's own role, `owner` or `weaver`, is the part before `members`.
- Worktree: the isolation worktree git lists for the agent's id. Without one, the `worktree` tag of the agent's newest subagent-start entry in belt's ledger. That tag comes from the hook payload's cwd, which can be the parent's, so it never counts when it names this session's own worktree. For a session, the worktree its cwd is in.
- Model and effort: the `model` and `effort` tags of the agent's newest subagent-start entry, never the agent type. belt 2.9.0 and later tag both when `ledger.agent_types` in its config maps the agent's type. Its keys end in `*`, because a named spawn reports its name as its agent type. An unmapped agent shows dashes.

A field no source recorded shows `-`. The line is read again at session start, after a spawn starts its agent, and when a main-loop turn ends. Only one refresh runs at a time and none starts within 5 s of the last; one trailing refresh at the 5 s mark catches up whatever was skipped. `claude agents` and the events service (`localhost:7430`, source `belt-ledger`) get 400 ms each. A source that fails, refuses or runs out of time reads as empty, so the line falls back to the role alone and never errors. With `members` off nothing is probed, and members stored earlier are dropped at the next session start.

## Guards

Active while `guards` is on (the default), and only for the main loop: a tool call with an `agentId` (a subagent, which owns its own isolation worktree) passes untouched.

- Owner edits: for an owner, `Write` and `Edit` on a path under the canonical checkout or under any other worktree of the repo are denied. Other worktrees mean the `~/.worktrees/<repo>/*` layout and nested ones such as `.claude/worktrees/*`. The reason reads `loom-pane: <path> belongs to the canonical checkout; owners edit only their own worktree`. The path is normalized first, so `..` doesn't slip through. Its own worktree and paths outside the repo are allowed.
- Owner shell: a `Bash` command is read for `cd` (a subshell's `cd` ends with it), `git -C <path>` and redirections. A mutating git verb acting in the canonical checkout or another worktree, or a `>` into one, is denied the same way. Mutating verbs: `commit`, `add`, `checkout`, `switch`, `reset`, `rebase`, `merge`, `push`, `rm`, `mv`, `restore`, `stash` (except `stash list` and `stash show`), `cherry-pick`, `revert`, `clean`, `pull`, `apply`, `am`. Path operands count for `add`, `rm`, `mv`, `checkout`, `restore` and `clean` only; `-m` and `-F` values never do. `command git`, `time git` and `/usr/bin/git` read as git.
- Claims land after the command: a claim counts only once its command has run, so an owner moving to another worktree claims it in one command and works there in the next.
- Removals, any role: `git worktree remove <path>` asks only for a forced removal of a dirty tree, or a detached head with commits ahead of the default branch. Git itself refuses a dirty tree without `--force`, and a merged branch's tree is a clean teardown. `git branch -D <name>` asks only when the branch's commits exist nowhere else, or its worktree is dirty. Nowhere else means ahead of the default branch, not on `origin/<name>` (or with no upstream at all), and not integrated. Integrated is factory-band's check: a merged tree equal to the default branch's tree (`git merge-tree --write-tree`, git 2.38 or later), so a squash-merged branch goes without a question. A failed or conflicting merge-tree still asks. The dialog reads `<target> has 2 unmerged commits and uncommitted changes. Remove anyway?` with Proceed and Cancel; Cancel denies. A dismissed dialog denies too in an interactive session; headless (`-p`) nobody can be asked and the call passes. A branch of some other repo (`git -C /elsewhere branch -D x`) isn't judged.

Weaver and `none` sessions get no edit guards.

## How they fail open

A guard that can't be sure lets the call through. A command with a substitution, a heredoc, a variable in a path, `bash -c`, `sudo`, `xargs`, `GIT_DIR=` or `GIT_WORK_TREE=`, or an unbalanced quote isn't parsed and passes. A guard that throws or outruns its 10 s budget logs one debug line and passes. Writes the parser doesn't see: `cp`, `mv`, `rm`, `tee`, `sed -i`, `python3 -c`, and `NotebookEdit`. Path checks compare spellings, not inodes, so a symlinked spelling into the canonical checkout slips by. belt remains the fail-closed guard; this mod is a second opinion on top of it.

## Config

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `guards` | boolean | `true` | run the three guards above |
| `members` | boolean | `true` | list the members beside the role |

## Dev loop

```bash
claude --plugin-dir mods/loom-pane         # from a checkout; a save hot-reloads
claude plugin validate mods/loom-pane      # what the engine would refuse
node --test mods/loom-pane/test/*.test.mjs # the parsers, the member line and the status text in lib/
claude plugin test mods/loom-pane          # the guards and the member line against the engine's test kit
```

The engine writes `.claude-plugin/types/` beside the mod on load; `npx -p typescript tsc -p mods/loom-pane` type-checks against it.
