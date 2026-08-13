# worklog

Resumable, ticket/topic-keyed cross-session work state: a CLI and MCP server.

Long tasks that span several repos and get picked up days later need a stable
home for "where was I." Claude Code's per-directory memory loses that when you
start in a tmp dir: work across three repos from `/tmp/foo`, stop, and the
memory is orphaned under a tmp slug you can never find again. worklog keys
work on the **task** (a ticket id or a topic) instead, groups per-repo context
underneath it, and keeps the whole thing in a local git history you can
search.

## How it works

The store has no git remote: work state stays on the machine that created it,
so you resume where you left off. The task is the unit, not the directory.
One ticket spanning three repos is one item with three per-repo notes, not
three scattered memories.

## Install

```bash
make install   # builds and installs to ~/code/bin/worklog
```

## Usage

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

# Which config file is read, and the settings in effect:
worklog config
```

## MCP

`worklog mcp` starts the MCP stdio server, exposing the same operations as
tools so an agent can checkpoint and resume without shelling out:
`worklog_checkpoint`, `worklog_list`, `worklog_search`, `worklog_show`,
`worklog_status`.

Registration is machine-private: the consuming repo's companion recipe
registers `worklog mcp` with the MCP host, unsandboxed as first-party code.
See [`CLAUDE.md`](CLAUDE.md) for the two-layer build/install vs. wiring split.

## Where things live

The store lives at `~/code/worklog/` (override with `$WORKLOG_DIR`), one
directory per item:

```
~/code/worklog/ABC-1234/
  CONTEXT.md          # status, repos touched, "Where I am", and a reverse-chron log
  repos/ralph.md      # per-repo notes, created when the task touches that repo
```

## Develop

```bash
make build    # ./worklog
make test     # go test ./...
```

See [`CLAUDE.md`](CLAUDE.md) for the store internals, MCP tool signatures, and
the ticket-scan firewall.
