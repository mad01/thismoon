# loom-pane

A Claude Code mod for sessions that follow the loom skill. It draws a pane of the repo's worktrees and a status line naming the session's role, and it turns the skill's rules into guards. The skill is unchanged; without the mod, loom works as before.

## What the pane shows

`/loom-pane` opens it. One row per entry of `git worktree list --porcelain`, read from the canonical checkout:

```
= main              clean    +0  3 days ago
> feat/pane         dirty    +2  5 minutes ago
  detached 3333333  ?         ?  ?
```

`>` is this session's worktree, `=` the canonical checkout. The columns are the branch, `dirty` or `clean` (`git status --porcelain`), the commits ahead of the default branch (`git rev-list --count origin/<default>..HEAD`, `main` when origin names no HEAD), and the age of the last commit (`git log -1 --format=%cr`). A git call that fails or runs past 2 s leaves `?` in its column. The pane re-reads git every `pollMs` while it is open and stops when it closes.

## Role and status line

At `session.start` (and again after a resume or `/clear`) the mod runs `git rev-parse --show-toplevel` and `--git-common-dir`; the canonical checkout is the parent of the common git dir. A session whose top level is `~/.worktrees/<repo>/<slug>` is an owner and the status line reads `loom: owner <slug>`. A session in the canonical checkout becomes the weaver once `/loom weave` was typed, and its line reads `loom: weaver · 3 worktrees, 1 dirty` after a survey. Anything else is `none` and the line is cleared.

## Guards

Active while `guards` is on (the default):

- Owner edits: for an owner, `Write` and `Edit` on a path under the canonical checkout or under another `~/.worktrees/<repo>/<slug>` are denied with `loom-pane: <path> belongs to the canonical checkout; owners edit only their own worktree`. The own worktree and paths outside the repo are allowed.
- Owner shell: a `Bash` command is read for `cd`, `git -C <path>` and redirections. A mutating git verb (`commit`, `add`, `checkout`, `switch`, `reset`, `rebase`, `merge`, `push`, `rm`, `mv`, and `restore`, `stash`, `cherry-pick`, `revert`, `clean`, `pull`, `apply`, `am`) acting in the canonical checkout or another worktree, or a `>` into one, is denied the same way. `git status`, `git log`, `git diff`, `ls`, `cat` are read-only and pass anywhere.
- Removals, any role: `git branch -D <name>` and `git worktree remove <path>` check the target. A dirty tree or commits not on the default branch raise an `AskUserQuestion` dialog, `<target> has 2 unmerged commits and uncommitted changes. Remove anyway?`, with Proceed and Cancel. Cancel denies the call.

Weaver and `none` sessions get no edit guards.

## How they fail open

A guard that can't be sure lets the call through. A command with a substitution, a heredoc, a variable in a path, `bash -c`, or an unbalanced quote isn't parsed and passes. A guard that throws, outruns its 10 s budget, or whose dialog is dismissed logs one debug line and passes. Path checks compare spellings, not inodes, so a symlinked spelling into the canonical checkout slips by. belt remains the fail-closed guard; this mod is a second opinion on top of it.

## Config

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `guards` | boolean | `true` | run the three guards above |
| `pollMs` | number | `30000` | how often the open pane re-reads git (1000 at least) |

## Dev loop

```bash
claude --plugin-dir mods/loom-pane         # from a checkout; a save hot-reloads
claude plugin validate mods/loom-pane      # what the engine would refuse
node --test mods/loom-pane/test/*.test.mjs # the parsers and formatters in lib/
```

The engine writes `.claude-plugin/types/` beside the mod on load; `npx -p typescript tsc -p mods/loom-pane` type-checks against it.
