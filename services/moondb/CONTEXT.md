# moondb

The edit history moondb records between commits. These terms apply to the
files under `services/moondb/`; the repo-wide terms are in the root
CONTEXT.md.

## Language

**Session**:
One Claude Code session, identified by the hook payload's `session_id`. moondb creates it lazily from the first turn it sees. Its title is the name the harness gives it (the hook's session title, or a transcript `agent-name` or `ai-title` line), else the first line of the first prompt serve saw, else the last segment of its cwd. One rule picks the title on every path. A name the person gave (`agent-name`) outranks a generated title (`ai-title`, or the hook's session title), which outranks the prompt line. Within the same rank the first title seen wins. The prompt line, once set, is never replaced. It also keeps where its cwd sits in git (its location), the entrypoint the harness was started by (`cli`, or `sdk-cli` for a headless `claude -p` run), and its Session tags.
_Avoid_: conversation, chat

**Session tag**:
A short lowercase token the launcher of a Session gave it through `MOONDB_SESSION_TAGS`, such as `eval` for an eval run. The hook reads it from its environment, so an import cannot recover it. A session keeps the union of the tags its envelopes carried, at most eight.
_Avoid_: label, category

**Turn**:
One user prompt and everything up to the Stop that ends it, identified by the hook's `prompt_id` (the transcript's `promptId`; an import of a transcript without one uses the prompt entry's uuid). It carries the prompt text, the final assistant message, the turn's raw transcript lines and its Turn origin.
_Avoid_: exchange, step

**Turn origin**:
What opened a Turn: `person` (a typed prompt or a slash command), `task` (a background task's notification that it finished), `teammate` (a teammate's message), `subagent` (a message from a subagent or another session), `continuation` (the summary that resumes a session after compaction), or `none` (no prompt text). record and import read it from the head the harness gives a prompt it wrote itself. A message relayed from another session counts by the wrapper inside it. The web page reads a turn recorded before origins existed the same way.
_Avoid_: source, trigger, kind

**Agent**:
A subagent of a session, identified by the `agent_id` its hook payloads carry; a payload without one belongs to the main conversation. When it stops, or an import reads its transcript file, serve keeps its record: type, name, model, the Turns it ran under, its start and stop, its Agent transcript and its totals. A teammate runs under every Turn that sent it a message. An agent known only from its edits has no record. Listings show the main conversation beside the agents as `main`.
_Avoid_: worker, subagent as a concept apart from Agent

**Main segment**:
A stretch of the main conversation the harness runs under an agent id of its own. A teammate's message or a queued prompt that wakes the session while it waits on background agents starts one. The harness fires SubagentStop at its end with no `agent_type` and a transcript path that never exists, since the lines go to the main transcript. It is not an Agent: record spools nothing for it, and ingest drops an older spool's stop of one. Every listing leaves out a record of one stored before (type `internal`, no line read, no Agent transcript). An `internal` Agent with lines read is a real Agent.
_Avoid_: phantom agent, ghost agent, internal agent (an `internal` Agent with lines is a real one)

**Agent transcript**:
An Agent's transcript file as moondb keeps it: the timeline lines only (user, assistant, API request, system), with the fields a Trace reads, and each text, thinking block, tool input and tool result cut to a head of `AgentHeadBytes`. Usage, ids and times survive the cut, so its Trace prices the same as the whole file's. Past `MaxAgentTranscriptBytes` whole lines go from its head; the Agent counts them, and their calls add no tokens or cost.
_Avoid_: subagent log, sidechain (the transcript field, not a moondb term)

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

**In-flight edit**:
An Edit no Commit link covers: neither it nor a later Edit of the same path in that worktree has a Commit link. In-flight edits are the work between commits that a commit, reset or rebase in the checkout would sweep up or lose.
_Avoid_: uncommitted edit (git's state, which moondb cannot see for a commit no session watched), pending, dirty

**Trace**:
The account of what took time in a turn, an Agent or a session and what it cost, computed on read from a Turn's transcript lines and its Agents' transcripts. A turn's trace is its Spans plus its wall time split by what was running: model, tool, harness, or user time spent waiting for input. Past a point, a stretch with no line is Idle time. Its cost adds the work its Agents did under it. A session's trace adds the user time between one turn's Stop and the next prompt, and every Agent's whole run to its cost.
_Avoid_: event, log entry, metric, profile

**Idle**:
The part of a stretch inside a Turn or an Agent's run that goes on past `IdleGapAfter` (30 minutes) with no transcript line. A laptop asleep, a session resumed days later or a request that hung leaves one. A Trace classes each stretch by the line that ends it, so without this rule the whole stretch would count as model, tool or harness time. The stretch keeps its first `IdleGapAfter` in its own class, and the rest is idle time. Waiting for input is user time and never idle, inside a turn or between turns. Active time leaves idle time out. The listing's `SessionIdleAfter`, which calls a quiet session ended, is a different rule.
_Avoid_: sleep, pause

**Turn totals**:
A closed Turn's Trace totals, kept on the Turn record so a listing never reads a transcript. They hold the time per class, the call counts and the tokens per model and rate. Never a cost: the tokens are priced when read. An Agent keeps totals of the same shape for its run. A turn or Agent stored before totals existed, or under an older trace version, gets them from the totals backfill.
_Avoid_: stats, metrics, summary (the listing row's word)

**Active time**:
The time a Session spent inside its Turns, less the time a turn waited for input or sat Idle. A turn with Turn totals gives its wall time less its user and idle time. The time between turns never counts, so a session left open overnight adds nothing for the night. Its counterpart is the wall time, from the first prompt to the last turn's end with the time between turns included, and the active time never exceeds it. A session row carries both, as `active_ms` and `duration_ms`.
_Avoid_: duration (say active time or wall time), run time, uptime

**Attributed cost**:
The part of a Session's cost the attribution rule gives one repository. It is computed on read from the Agent and Edit records and the cost of the Turn totals and of each Agent's totals. Model calls carry no repository, so the rule goes by who spent the money. A subagent's cost goes to the repository most of its Edits touched, a tie to the one it edited first. Without an Edit in a repository it goes to the repository of its cwd, else to `no_repo`. The main conversation's cost goes to the repository most of its own Edits touched, else to `orchestration`. An Edit outside any repository counts toward none. An agent or main conversation without priced tokens adds nothing. Never stored: a change to the rule changes every past session. It has nothing to do with an Unattributed delta, which is about Edits, not cost.
_Avoid_: allocation, charge-back, and "repo cost" or "cost split" as prose names for it (the fields `repo_cost` and `repo_costs` keep theirs)

**Scorecard**:
One Session's report of how it spent its context, computed on read. It holds the Trace totals and tool calls, the csl searches by how they came back, and the Read calls by size. It also holds the belt hints shown to the session, the Agents, and flags naming the costliest patterns. For the kof hints it says whether any Agent of the session read or edited the files the shown assertions are pinned to later, since a hint serves the whole session. The hints and the belt ledger's counts come from the events service, and the pins from keeper-of-facts. Each input reports whether it answered, and a part whose input did not is null rather than zero. It is versioned and only grows, since an eval suite scores against it.
_Avoid_: report card, score, stats

**Span**:
One timed piece of a Trace: the turn itself, one model call (an assistant message, with its tokens and cost), one tool call (from the line asking for it to its result) or one Agent's work under the turn. An Agent's span runs from its first line under the turn (its spawn, in the turn that spawned it) to its last there (its stop, in its last turn), with its calls, tokens and cost. An agent without a record gets a span from its first recorded edit to its last. A model whose price moondb does not know has tokens and no cost, never a zero cost.
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
