# moondb

The edit history moondb records between commits. These terms apply to the
files under `services/moondb/`; the repo-wide terms are in the root
CONTEXT.md.

## Language

**Session**:
One Claude Code session, identified by the hook payload's `session_id`. moondb creates it lazily from the first turn it sees.
_Avoid_: conversation, chat

**Turn**:
One user prompt and everything up to the Stop that ends it, identified by the hook's `prompt_id` (the transcript's `promptId`; an import of a transcript without one uses the prompt entry's uuid). It carries the prompt text, the final assistant message, and the turn's raw transcript lines.
_Avoid_: exchange, step

**Agent**:
The `agent_id` and `agent_type` a hook payload carries. A payload without them belongs to the main conversation.
_Avoid_: subagent as a separate concept, worker

**Edit**:
One recorded change to one file by one tool call. It carries the tool, repo, worktree, branch and path, the patch hunks, the before and after chunk ids, the after content's git blob id, and HEAD at the time. Append-only; its identity is the `tool_use_id` plus the path.
_Avoid_: change, operation, op-log entry

**Observed**:
The file bytes `moondb record` read from disk right after the tool ran. Evidence, not truth: serve derives the after content from the tool payload and compares it with the observed bytes.
_Avoid_: actual, disk state

**Unattributed delta**:
A change between two known contents of a path that no recorded tool call explains. A formatter hook, a human edit, or a Bash command without a diff leaves one. serve synthesizes it when the observed bytes differ from the derived after content.
_Avoid_: delta on its own, drift, diff

**Snapshot**:
Reserved for a later release: the dirty files plus the session's touched paths in one worktree at a turn boundary, stored as a chunk map.
_Avoid_: checkpoint (another tool's term), backup

**Chunk**:
Content-addressed bytes, named by the SHA-256 of the uncompressed content and stored gzipped: file contents, patches, prompt text, transcript lines.
_Avoid_: blob (git's term, used only for git blob ids), object

**Commit link**:
A recorded after-content blob id matched to a blob in a new commit, tying an Edit to the commit it landed in. Never a commit-message trailer.
_Avoid_: attribution trailer, tag

**Backend**:
An implementation of the store interface: `local` today, `cloud` in a later release.
_Avoid_: driver, provider

**Import**:
A backfill from session transcripts: `moondb import` writes the envelopes the hooks would have written, with their ids, for sessions the store does not hold yet. With `--force` it backfills held sessions too, filling only what the store has empty. An earlier prompt time still moves a start back.
_Avoid_: migration, replay (ingest's word for an envelope read again)

**Spool**:
The on-disk inbox `moondb record` writes and `moondb serve` drains. Not the store: an envelope lives in the spool only until serve ingests it.
_Avoid_: queue, buffer, ledger (belt's term), event (the events service's term)
