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

Three Claude Code hooks pipe their payloads into `moondb record`:

- when you submit a prompt, a turn opens
- after every Edit, Write, MultiEdit or NotebookEdit, the change is recorded
  with the file's content
- when the session stops, the turn closes, and any new commits in the
  worktrees the session touched are noted

`record` writes to a spool on disk and returns within 200 ms. It always exits
0 and never prints to stdout, so a capture problem can't break your session.
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
    "PostToolUse": [{ "matcher": "Edit|Write|MultiEdit|NotebookEdit",
                      "hooks": [{ "type": "command", "command": "moondb record", "timeout": 3 }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }]
  }
}
```

The Edit hook is synchronous on purpose: edits in one message land
milliseconds apart, and a hook that ran later would read the wrong file.

## Usage

```bash
moondb why <path>:<line>     # the edit, turn, prompt and commit behind one line
moondb history <path>        # every recorded edit of a file, newest first
moondb sessions              # recorded sessions, newest first
moondb doctor                # is serve up, is the store readable, is the build current
```

For example, `moondb why services/events/internal/store/dedupe.go:42` names
the turn whose prompt led to line 42, the session and agent that made the
edit, and the commit it landed in.

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
| `moondb_why(path, line)` | The edit, turn, prompt, session and commit behind one line |
| `moondb_history(path)` | Every recorded edit of a file, newest first |
| `moondb_turn(id)` | One turn: prompt, final message, edits, unattributed deltas |
| `moondb_sessions()` | Recorded sessions, newest first |
| `moondb_doctor()` | Run the `moondb doctor` checks and return the report |

### Web page

Open `http://moondb.this/` (or `http://localhost:7433/`) for a read-only web
UI. It lists sessions, filtered by repo and age. A session shows its turns,
and a turn shows its prompt, final message, agents and every edit as a diff.
Click an added line to ask why it's there. A file view shows the file as it
is today with the turn and agent behind each line, plus its recorded
history.

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

This release records the file tools only. Later releases add:

- a snapshot of each worktree at turn boundaries
- capture of the file changes Bash commands make
- an importer that backfills the last 30 days from Claude Code transcripts
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
