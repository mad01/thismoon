# moondb

The edit history moondb records between commits. These terms apply to the
files under `services/moondb/`; the repo-wide terms are in the root
CONTEXT.md.

## Language

**Session**:
One Claude Code session, identified by the hook payload's `session_id`. moondb creates it lazily from the first turn it sees. Its title is the name the harness gives it (the hook's session title, or a transcript `agent-name` or `ai-title` line), else the first line of the first prompt serve saw, else the last segment of its cwd. One rule picks the title on every path. A name the person gave (`agent-name`) outranks a generated title (`ai-title`, or the hook's session title), which outranks the prompt line. Within the same rank the first title seen wins. The prompt line, once set, is never replaced.
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

**Skipped**:
The files of one Bash call that `record` counted instead of recording as Edits. Each count names its reason. `clean`: git status reports the file clean after the command, as a pull, checkout, reset or stash leaves it. `timeout`: the hook's budget or a git deadline ran out first. `failed`: git could not give the file's status, or the file is gone and no worktree holds it. serve keeps the counts on the Turn, one entry per call. A Snapshot's skipped counts are a different thing: paths the snapshot left out or listed without content.
_Avoid_: dropped, not recorded

**Observed**:
The file bytes `moondb record` read from disk right after the tool ran. Evidence, not truth: serve derives the after content from the tool payload and compares it with the observed bytes.
_Avoid_: actual, disk state

**Unattributed delta**:
A change between two known contents of a path that no recorded tool call explains. A formatter hook, a human edit, or a Bash command without a diff leaves one. serve synthesizes it when the observed bytes differ from the derived after content (cause `observed`), or when a Snapshot holds content for a path that differs from the latest content known of it before (cause `snapshot`).
_Avoid_: delta on its own, drift, diff

**Snapshot**:
The dirty files plus the session's touched paths in one worktree, read at a turn edge: when the prompt arrives (`pre`) and at Stop (`post`). Its file map (path, chunk, blob id, size, HEAD blob) is one Chunk, and each file's content is its own Chunk. A path it cannot read (excluded, a symlink, unreadable, too large) is listed without content and with the reason. A snapshot its budget or file cap cut short is truncated and counts what it skipped. A path missing from a snapshot that is not truncated matched HEAD. Git status does not show ignored files or files marked skip-worktree or assume-unchanged, so for those the rule does not hold.
_Avoid_: checkpoint (another tool's term), backup

**Chunk**:
Content-addressed bytes, named by the SHA-256 of the uncompressed content and stored gzipped: file contents, patches, prompt text, transcript lines.
_Avoid_: blob (git's term, used only for git blob ids), object

**Commit link**:
A recorded after-content blob id matched to a blob in a new commit, tying an Edit to the commit it landed in. Never a commit-message trailer.
_Avoid_: attribution trailer, tag

**Trace**:
The account of what took time in a turn or a session and what it cost, computed on read from a Turn's transcript lines. A turn's trace is its Spans plus its wall time split by what was running: model, tool, harness, or user time spent waiting for input. A session's trace adds the user time between one turn's Stop and the next prompt.
_Avoid_: event, log entry, metric, profile

**Turn totals**:
A closed Turn's Trace totals, kept on the Turn record so a listing never reads a transcript. They hold the time per class, the call counts and the tokens per model and rate. Never a cost: the tokens are priced when read. A turn stored before totals existed, or under an older trace version, gets them from the totals backfill.
_Avoid_: stats, metrics, summary (the listing row's word)

**Span**:
One timed piece of a Trace: the turn itself, one model call (an assistant message, with its tokens and cost), one tool call (from the line asking for it to its result) or one subagent's recorded edits. A model whose price moondb does not know has tokens and no cost, never a zero cost.
_Avoid_: event, log entry, metric

**Backend**:
An implementation of the store interface: `local` today, `cloud` in a later release.
_Avoid_: driver, provider

**Import**:
Bringing past sessions in from their transcripts: `moondb import` writes the envelopes the hooks would have written, with their ids, for sessions the store does not hold yet. With `--force` it imports held sessions too, filling only what the store has empty; a session title follows the Session's title rule. An earlier prompt time still moves a start back.
_Avoid_: migration, replay (ingest's word for an envelope read again), backfill (the totals backfill's word)

**Spool**:
The on-disk inbox `moondb record` writes and `moondb serve` drains. Not the store: an envelope lives in the spool only until serve ingests it.
_Avoid_: queue, buffer, ledger (belt's term), event (the events service's term)
