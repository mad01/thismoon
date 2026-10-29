# moondb

moondb records every file edit an agent session makes between commits. Ask
it why a line looks the way it does, and it hands back the prompt that
produced it.

Git keeps commits. Everything between them is gone: which prompt led to an
edit, which of five parallel sessions touched a file, what a formatter changed
after the agent was done. Claude Code's transcripts hold some of it, but they
expire, drop the content of larger files, and file worktree sessions under the
main checkout. moondb keeps that history itself. Each edit is stored with the
file content before and after it, the turn and prompt it came from, and the
commit it landed in.

## How it works

Claude Code hooks pipe their payloads into `moondb record`:

- when you submit a prompt, a turn opens
- after every Edit, Write, MultiEdit or NotebookEdit, the change is recorded
  with the file's content
- after a Bash command, the files it changed are recorded, when Claude Code
  reports them (the `bashEditDiffEnabled` setting). A file git reports clean
  after the command, because a pull or checkout put it back to its committed
  state, is only counted on the turn
- when the session stops, the turn closes, and any new commits in the
  worktrees the session touched are noted
- when a subagent stops, it is recorded with the turns it ran under and
  its own transcript, each message cut to a short head, so its time and
  cost can be traced

At the start and the end of each turn, `record` also takes a **snapshot** of
each worktree in play: the content of its uncommitted files and of the files
the session touched. A file that changed between two turns, with no tool call
to explain it, shows up as an unattributed delta.

`record` writes to a spool on disk and returns within 200 ms, or 2 s for the
async Bash hook. It always exits 0 and never prints to stdout, so a capture
problem can't break your session.
`moondb serve` drains the spool and stores the records.

serve works out each edit's after content from what the tool was asked to
do, not from what is on disk. The disk read is kept as **observed**. When the
two differ, because a formatter ran or you typed in the file, serve records
an **unattributed delta** for the difference. When a commit lands, serve
matches its file contents against the recorded edits by git blob id. moondb
never writes anything into your repository's `.git`.

## Install

It ships from the thismoon monorepo. `ralph up` builds the binary to
`~/code/bin/moondb` and registers the `moondb serve` agent with t-man. The
hooks, the `moondb.this` route and the MCP registration come from your
consuming repo's companion recipe, not from here.

To check the service is up:

```
t-man status moondb
```

Without ralph, run `make install` in `services/moondb` and start `moondb serve`.
Then add the hooks to your Claude Code settings:

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }],
    "PostToolUse": [
      { "matcher": "Edit|Write|MultiEdit|NotebookEdit",
        "hooks": [{ "type": "command", "command": "moondb record", "timeout": 3 }] },
      { "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }
    ],
    "Stop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }],
    "SubagentStop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }]
  },
  "bashEditDiffEnabled": true
}
```

The Edit hook is synchronous on purpose: edits in one message land
milliseconds apart, and a hook that ran later would read the wrong file.
`bashEditDiffEnabled` (Claude Code 2.1.269 or later) makes Claude Code put
the files a Bash command changed into the hook payload. Without it, the Bash
hook records nothing, and snapshots still catch those changes at the next
turn edge.

Claude Code lists every file under the session's repository that a command
changed, including the ones `git pull`, `git checkout` or `git reset`
rewrote. Those aren't the command's own work, so `record` runs one `git
status` over the listed files and records only the ones still modified,
added, deleted, untracked or ignored afterwards. The turn shows how many it
skipped and why: clean after the command, past the budget, or with a status
git could not give. A file a command edits and commits in one go is clean
afterwards and gets no edit either; `git blame` names that commit. The diff covers only
the repository of the session's working directory, so a session started in
the main checkout never sees Bash edits in a linked worktree.

## Usage

```bash
moondb why <path>:<line>     # the edit, turn, prompt, subagent brief and commit behind one line
moondb history <path>        # every recorded edit of a file, in any checkout, newest first
moondb sessions              # recorded sessions, most recently active first, with their titles, active time and wall time
moondb trace <session-id>    # where a session's time went and what it cost
moondb inflight              # files in this checkout other sessions edited that no commit has taken yet
moondb scorecard <session-id>  # how a session spent its context: searches, reads, hints, flags
moondb import --dry-run      # what an import of past session transcripts would record
moondb doctor                # are the hooks wired, is serve up, is the store readable, is the build current
```

For example, `moondb why services/events/internal/store/dedupe.go:42` names
the turn whose prompt led to line 42, the session and agent that made the
edit, and the commit it landed in.

To cover the time before the hooks were wired, import the last 30 days with
`moondb import`. It reads the session transcripts Claude Code keeps under
`~/.claude/projects` and spools the turns, edits and subagents the hooks
would have recorded, so serve must be running to take them in. Sessions the store
already holds are skipped. A transcript has less than the hooks see: it has
no disk read after an edit and no snapshots, and the import skips Bash
commands. It has no HEAD either, so imported edits link to commits only once
a later session moves HEAD. Run it with `--dry-run` first to see the counts.

`moondb trace` splits a session's wall time into model time, tool time,
harness time (hooks and request setup), user time (waiting for input) and
idle time. Idle time is what a turn spends past 30 minutes without a
transcript line. The command also adds up the tokens by kind and the cost at
list prices, then lists the ten longest tool calls and the ten most
expensive model calls with their turns. It reads the transcript lines each
turn keeps and each subagent's own transcript, so the cost covers the main
conversation and every subagent that stopped while moondb recorded or was
imported. The cost line splits the two, an agents table lists each subagent
with its turns, calls and cost, and the rankings include the subagents'
calls. A model moondb has no price for shows its tokens and is named as
unpriced, so the cost is never a silent zero.

`moondb inflight` lists the files of the current checkout that sessions
edited and no commit has taken yet, newest first, with the sessions behind
them. `--exclude-session <id>` leaves one session's own edits out. An edit
stops counting once a commit takes it or a later edit of the same file. A
session sees its own commits land. A commit a person makes after the
session ended never gets a link, so that file stays listed until the 72 hour
window passes. So does a deleted, excluded or oversized file, whose edit
leaves no content to match a commit by. The belt `moondb-inflight` hint
asks this at session start and checks `git status` before it says anything.

`moondb scorecard` scores how a session spent its context. It counts the csl
searches that came back empty, cut at the limit or failed, and the Read
calls that took a whole file of 300 lines or more. It counts the belt hints
shown to the session by id. It also adds the belt ledger's compactions and
permission denials. Flags at the top name the costliest patterns. The hints
come from the events service. When it does not answer, the scorecard leaves
that part out and says so. `--json` prints the same object as
`GET /api/sessions/{id}/scorecard`. The session page links to it under
Session details. Subagent transcripts keep only the head of each result, so
a subagent's reads are mostly unsized and its searches unread.

`why` first asks `git blame` which commit the line came from, then finds the
edit whose content matches that commit. For a line that isn't committed yet,
it matches the line's text against the lines recent edits added and ranks
the candidates.

## MCP + web view

Mostly you'll ask through Claude: "why does this line look like this?", or
"what else did the turn that wrote this function change?". Claude calls the
tools below. They are all read-only.

On a standalone install, register the server once: `claude mcp add --scope user moondb -- moondb mcp`. On a ralph-managed machine, the consuming repo's companion recipe does it instead (`docs/adr/0006` at the repo root). Either way, `moondb serve` must be running: `moondb mcp` is a thin HTTP client to it. Run `moondb doctor` if a tool call fails.

| Tool | Purpose |
|------|---------|
| `moondb_why(path, line)` | The edit, turn, prompt, session and commit behind one line, and the brief of the subagent that made it |
| `moondb_history(path)` | Every recorded edit of a file, newest first |
| `moondb_inflight(worktree)` | The files of a checkout that other sessions edited and no commit has taken yet, with the sessions behind them |
| `moondb_turn(id)` | One turn: prompt, final message, edits, unattributed deltas, snapshots, the agents that worked in it and each subagent's brief |
| `moondb_sessions()` | Recorded sessions, most recently active first, with title, status, counts, subagents, cost and a slice per day |
| `moondb_trace(id)` | One session's time and cost: totals, the subagents, the longest tool calls, the most expensive model calls |
| `moondb_scorecard(id)` | How one session spent its context: searches, reads, hints, agents and flags |
| `moondb_doctor()` | Run the `moondb doctor` checks and return the report |

### Web page

Open `http://moondb.this/` (or `http://localhost:7433/`) for a read-only web
UI. The front page lists the sessions active in the last 7 days under
every day they were active on, newest day first, so a session that has
run for weeks sits under today with today's work. Each row is one session
on one day. It names the session and its repo, or the directory it
started in when it sits in no repo, and says whether it still runs and
when it was last active, or when that day's work ended. It shows the
day's active time: the time inside its turns, less the time it waited for
you or sat idle. A night the session sat open between turns adds nothing.
A turn that wrote no transcript line for hours adds only the first 30
minutes of that stretch. "day 2 of 5" says the session spans several
days. The day's turn, edit, file and agent counts sit beside its cost,
which covers the session and its subagents. Switch the window to 24
hours, 30 days or any time, pick a repo chip to see one repo, or a
directory chip to see the sessions outside any repo that started there.
A strip above the list adds up the sessions, turns, edits, files, active
time and cost in view, and the subagents' part of it.

A bar under the strip splits that cost by repo. Model calls carry no repo,
so each agent's cost goes to one. A subagent's goes to the repo most of
its edits touched, else the repo it worked in, else "no repo (cwd)". The
main conversation's goes to the repo most of its own edits touched, else
to "orchestration". Hover a part for its share and the rule. Each repo
chip counts the sessions that edited its repo or sent it cost, and shows
that cost. Picking it lists those sessions, and the cost column then
shows each session's part for that repo. The hover title gives the
session's whole cost.

A session opens on its title, repos and status, then its active time over
its wall time, such as "active 1h 51m over 23h 05m (2 days)". Its trace comes
next: a strip of totals, then a timeline lane per turn with a bar for each
model call, tool call and subagent, the subagent's from its spawn to its
stop. Hover a bar for its duration, calls, tokens and cost, and click it
to open the turn. An agents table follows, the main conversation first and
then each subagent with its turns, calls and cost. Then the turns, each
with the agents that ran in it and a badge when a background task, a
teammate or another agent started it rather than you. The worktrees come
last, and the session id and paths sit in a closed "Session details" block
at the end.

A turn shows its prompt, each subagent's brief and its final message. Below
them come the agents that worked in it with their calls and cost, every edit
as a diff, and the snapshots taken at its start and end. Click an added line to ask why
it's there. A file view shows the file as it is today with the turn and
agent behind each line, plus its recorded history.

## Configuration

Flags win over environment variables, which win over
`~/.config/moondb/config.toml`, which wins over the compiled defaults. The
keys you're most likely to touch:

- `MOONDB_PORT` (default 7433) and `MOONDB_WORKDIR` (default
  `~/.local/state/moondb`)
- `capture.enabled` in the config file, to switch capture off on one machine
- `capture.exclude`, the globs whose contents are never stored. The default
  covers `.env` files, keys, certificates, Terraform state and variable files,
  credential JSON, kubeconfig and `.npmrc`. An excluded file is recorded by
  path only.
- `[prices."<model id>"]`, to price a model the built-in table lacks or to
  change a price

Every key, with its default, is in [config.md](config.md).

## Where things live

- Store: `~/.local/state/moondb/` (`log/` for the records, `objects/` for
  file contents, `spool/` for what serve hasn't ingested yet)
- Config: `~/.config/moondb/config.toml` (optional)
- Binary: `~/code/bin/moondb`
- Web + API: `http://moondb.this/` (or `http://localhost:7433/`)

Every file moondb writes is readable by your user only. Removing the service
doesn't delete the store.

## What comes next

Later releases add:

- the Bash diffs a transcript keeps, read by `moondb import`
- a cloud backend: the log on a Kubernetes StatefulSet, file contents in
  object storage
- a shared mode that a local serve replicates into

## Develop

```bash
make test    # go test ./...  (hermetic: temp dirs and throwaway git repos, never your real store)
make build   # ./moondb
```

Architecture, the data model, the HTTP API and the capture contract are in
[`CLAUDE.md`](CLAUDE.md).

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
