# worklog

Resumable, ticket/topic-keyed cross-session work state — a CLI and MCP server.

Long tasks that span several repos and get picked up days later need a stable
home for "where was I." Claude Code's per-directory memory loses that when you
start in a tmp dir. worklog keys work on the **task** (a ticket id or a topic),
groups per-repo context underneath it, and keeps the whole thing in a local
git history you can search.

## Install

```bash
make install   # builds and installs to ~/code/bin/worklog
```

## Use

```bash
# Save where you are (creates the item, auto-detects the current repo):
worklog checkpoint ABC-1234 --where "auth middleware wired, tests red" \
  --note "next: fix the 401 path in handler_test.go"

# Find your way back:
worklog list --status active
worklog list --repo ralph          # items that touched this repo
worklog search "401"
worklog show ABC-1234

# Close it out:
worklog status ABC-1234 done
```

The store lives at `~/code/worklog/` (override with `$WORKLOG_DIR`), one
directory per item:

```
~/code/worklog/ABC-1234/
  CONTEXT.md          # status, repos touched, "Where I am", and a reverse-chron log
  repos/ralph.md      # per-repo notes, created when the task touches that repo
```

## MCP

`worklog mcp` exposes the same operations as MCP tools (`worklog_checkpoint`,
`worklog_list`, `worklog_search`, `worklog_show`, `worklog_status`) so an agent
can checkpoint and resume without shelling out.

## Design notes

- **Local only.** The store has no git remote — work state stays on the machine
  it was created on. Resume where you left off.
- **Task is the unit.** One ticket spanning three repos is one item with three
  per-repo notes, not three scattered memories.
