# why worklog

## The problem

Long tasks span several repos and get picked up days later, and "where was
I" needs a home that survives the gap. Claude Code's auto-memory keys on
the working directory, which fails in a specific way: start a session in a
tmp directory, work across three repos, stop, and the memory is orphaned
under a tmp slug you can never find again. The state was recorded; it just
can't be found from where you resume. worklog keys work on the task
instead (a ticket id or a topic) and groups per-repo context underneath
it, so one ticket touching three repos is one item with three per-repo
notes rather than three scattered memories.

## Why its own tool

The fix is a different keying scheme, not a patch on directory-keyed
memory, so it needs its own store and its own commands. It is a tool, not
a service: state is plain markdown on disk and every operation is a read
or write of that tree, so nothing has to stay running. The CLI and the MCP
server are two thin frontends over the same store methods, which means an
agent checkpointing mid-task and a human running `worklog show` a week
later see exactly the same state.

## Why this shape

The store is one directory per item under `~/code/worklog/`: a CONTEXT.md
with status frontmatter, a "Where I am" section rewritten on every
checkpoint, an append-only log, and lazily created per-repo notes. Every
checkpoint is a git commit in a local repo with no remote — machine-local
by design, so internal references never leave the machine. Plainness is
deliberate: markdown files stay searchable and readable without the tool.

`scan` digests local session transcripts into compact JSON so past work
can be imported without pulling raw transcripts into context, and it
enforces a ticket firewall: each session is tagged personal or internal
from its path, and extracted ticket ids are filtered to that world so the
two never co-mingle in one item. The firewall's machine-specific strings
ship as a configuration overlay from the consuming repo, per the two-layer
recipe split (docs/adr/0006); the built-in defaults carry only generic
markers.

## Non-goals

No cross-machine sync: the store has no remote, and you resume on the
machine you left. worklog is not a ticket tracker — status is only active,
paused, or done, and tickets live in whatever system issued their keys. It
also skips workflow machinery on purpose: no stage folders and no
per-stage state files, just the item's markdown.
