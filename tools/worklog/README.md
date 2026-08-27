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

The task is the unit, not the directory. One ticket spanning three repos is
one item with three per-repo notes, not three scattered memories. The store
is a git repo, local-first: with no `remote:` config it stays on the machine
that created it, and with one it follows you across machines.

### Sync across machines

Give the store an upstream in `~/.config/worklog/config.yaml`, a private
git repo you create once, empty:

```yaml
remote:
  push: true
  upstreams:
    personal: git@github.com:you/worklog-store.git
```

Upstreams are keyed by machine profile (from ralph's `config.local.toml`;
the machine's first profile with an entry wins), so one shared config can
send different machines to different stores. With a single store, key it
under the one profile all your machines carry. From then on every
checkpoint commits and pushes automatically (a failed push degrades to a
warning; the write always lands locally), a fresh machine clones the store
on first use, and `worklog sync` fast-forward pulls then pushes when you
switch machines. One writer at a time is the assumption: sync before
switching, there is no merge strategy. Full reference:
[config.md](config.md).

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

On a standalone install, register it once:

```sh
claude mcp add --scope user worklog -- worklog mcp
```

On a ralph-managed machine, skip the manual command — registration is
machine-private wiring that ships from the consuming repo's companion
recipe, unsandboxed as first-party code (`docs/adr/0006` at the repo root).
See [`CLAUDE.md`](CLAUDE.md) for the two-layer build/install vs. wiring
split.

No backing service has to be running: the MCP process reads and writes
`~/code/worklog` directly, the same store the CLI uses. Confirm the server
is registered with `claude mcp list`, which should list `worklog` among the
connected servers. There is no `worklog doctor` — `worklog docs` prints the
embedded operating doc (runtime behavior, failure modes, first moves)
instead.

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

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
