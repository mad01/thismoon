# why moondb

## The problem

Git records commits. Everything that happens between two commits is gone
once the second one lands. That covers which prompt led to an edit, what the
agent was told, and which of several parallel sessions touched a file. It
also covers what a formatter or a human changed after the agent was done.
With agents writing most of the code, that in-between history is where the
reasons live.

The only existing record is Claude Code's transcript files. Their format is
undocumented, and they expire after a configurable cleanup period. They drop
the full content of larger files. They are filed by working directory, so an
edit made in a worktree lands under the main checkout. None of that makes
them a history you can query by file and line.

moondb keeps that history itself. Every file edit a session makes is stored
with its before and after content, its turn and prompt, its session and
agent, and later the commit it landed in. `moondb why <path>:<line>` turns
that into an answer.

## Why its own service

Capture has to run inside the session, on every edit, without slowing it
down or ever failing it. Storage needs one process that can derive content,
deduplicate it, and link it to commits without racing other writers. Those
pull in opposite directions, so moondb splits them. A hook writes a spool
file and returns. A single `moondb serve` process drains the spool and owns
the store. A generic log or notes store has neither half: it cannot sit on a
synchronous hook path, and it knows nothing about patches, blob ids or turns.

## Why this shape

**After content is derived, not read from disk.** The tool payload says what
the agent asked for; the disk says what is there now, which a formatter hook
or a second edit may already have changed. serve derives the after content
from the payload and keeps the disk read as observed. A mismatch is recorded
as an unattributed delta, so the gap stays visible instead of being blamed
on the agent.

**The Edit hook is synchronous.** Edits in one assistant message land
milliseconds apart. An asynchronous hook would read the file after the next
edit had already changed it. `record` bounds itself to a 200 ms budget
instead, and spools whatever it has when the budget runs out.

**Commits are linked by content.** Every after content carries its git blob
id. When a worktree's HEAD moves, the new commit's blobs are matched against
recorded edits. A trailer in the commit message would be dropped by an
amend, forged by anyone, and lost on a fast-forward merge. A blob id
survives a squash merge and a deleted worktree.

**Nothing is written into the user's `.git`.** No shadow commits, refs or
notes. Loose objects written behind git's back can let an automatic garbage
collection corrupt a worktree's index. moondb reads git without taking the
index lock and keeps everything in its own store.

**Content-addressed chunks plus an append-only log.** File contents repeat
across edits, so chunks are stored once, named by their SHA-256. The records
are facts that never change, so an append-only JSONL log fits them and
replays into an in-memory index on start.

## Non-goals

moondb does not rewind or restore files, and it never edits the working copy.
It does not record shell commands themselves. It records the files a command
changed, as Claude Code reports them, and what turn-edge snapshots find. It
does not judge whether an edit was good, and `why` answers which turn wrote
a line, not whether the line is right. The web page is read-only: the hooks
are the only way records get in.
