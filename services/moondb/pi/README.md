# moondb pi package

This package records the edits the [pi](https://github.com/earendil-works/pi)
coding agent makes, so `moondb why` and `moondb history` cover them. It is
the pi counterpart of the Claude Code hook lines that feed `moondb record`.
Its one extension turns pi's events into the Claude-shaped payloads `moondb
record` already reads, and pipes each one into a `moondb record` child.
moondb itself doesn't change.

## Enable it

1. Make sure `moondb` is on `PATH` and `moondb serve` is running
   (`t-man status moondb`).
2. Add the package to `packages` in pi's `settings.json`
   (`~/.pi/agent/settings.json`, or `$PI_CODING_AGENT_DIR/settings.json`):

   ```json
   { "source": "~/.config/ralph/sources/thismoon/services/moondb/pi" }
   ```

   That path is the thismoon checkout ralph keeps in its sources cache. On a
   ralph-managed machine the settings file belongs to the consuming repo, so
   make the edit there. Write the entry by hand: `pi install` rewrites a
   `~/` source to a relative path.
3. Start a new pi session.

With no `moondb` on `PATH`, every handler does nothing for the life of the
pi process.

## What it records

| pi event | moondb record payload | Waited for |
|----------|-----------------------|------------|
| `before_agent_start` | `UserPromptSubmit`: opens a turn under a new `prompt_id` | no |
| `tool_result` of `edit` | `PostToolUse` `Edit`, or `MultiEdit` for several replacements | yes, up to 3 s |
| `tool_result` of `write` | `PostToolUse` `Write` | yes, up to 3 s |
| `tool_result` of `bash` | `PostToolUse` `Bash` with a `bashEditDiff`, when the command changed files | no |
| `agent_settled` | `Stop`: closes the turn with the last assistant text | no |
| `session_shutdown` | `Stop` for a turn still open, then waits for every record in flight | yes |

A run that an extension starts with a custom message opens a turn too, with
an empty prompt, which moondb gives no origin. Every payload carries
`session_id` (pi's session id), `transcript_path` (pi's session file, or
empty for a session without one), `cwd`, `prompt_id` while a turn is open,
and `session_title` when the session has a name.

The edit and write records are waited for, as the Claude Edit hook is.
moondb reads the file from disk, and a later tool call must not change it
first. The other records run on their own, so pi never waits for them until
shutdown. A shutdown in print mode (`pi -p`) waits for them, so its last
`Stop` isn't lost. A failed edit or write records nothing. A bash call is
recorded whatever its exit status, since a failed command can still have
changed files.

### How pi's tools become Claude payloads

| pi tool and input | `tool_name` and `tool_input` | `tool_response` |
|-------------------|------------------------------|-----------------|
| `edit` `{path, edits: [{oldText, newText}]}`, one replacement | `Edit` `{file_path, old_string, new_string, replace_all: false}` | `filePath`, `userModified: false`, and `structuredPatch` parsed from pi's `details.patch` |
| `edit` with several replacements | `MultiEdit` `{file_path, edits: [{old_string, new_string, replace_all}]}` | same |
| `write` `{path, content}` | `Write` `{file_path, content}` | `type` `create` or `update`, `content`, and for an update `originalFile` with the `structuredPatch` from it |
| `bash` `{command}` | `Bash` `{command}` | `bashEditDiff` `{files, changedFiles}` |

Paths are resolved the way pi's tools resolve them: a leading `@` is
dropped, `~` is the home directory, and a relative path sits under the
session's cwd. `tool_use_id` is pi's `toolCallId`. The legacy edit input
with one top-level `oldText` and `newText` pair is read too.

pi doesn't say whether a write created its file, and it returns no
original. So the extension reads the file in `tool_call`, before the write
runs, and computes the hunks itself. moondb's `why` reads an update's added
lines from its hunks, so an edit without them would never be named.

pi has no `bashEditDiff` of its own. The extension takes a `git status` of
the cwd's repository in `tool_call` and another in `tool_result`, with each
listed file stamped by status, size and times. Every file whose stamp
changed goes in `changedFiles`, at most 200. The first five also get hunks
in `files`, when the extension can work out the file's state before the
command:

- a file that is new after the command gets all its lines as added
- a tracked file git listed clean before the command gets `git diff HEAD`

moondb's own status check then skips the files a command put back to their
committed state, as it does for Claude Code.

### Session entrypoint

moondb reads a session's entrypoint from `CLAUDE_CODE_ENTRYPOINT`. The
extension sets it for the `moondb record` child to `pi-<mode>`: `pi-tui`,
`pi-print`, `pi-json` or `pi-rpc`. A pi started from a Claude Code shell
inherits Claude's value, and this replaces it. The web page shows the
entrypoint as a badge. `MOONDB_SESSION_TAGS` passes through unchanged.

## Limits

- The session's harness stays `claude-code`. moondb's `record` sets it from
  a constant, so a pi session can't say otherwise until moondb changes
  (Phase B).
- Traces and costs are empty. `Stop` spools the tail of pi's session file,
  and moondb's Claude transcript parser skips pi's lines (Phase B).
- A Bash change to a file that was already uncommitted is recorded by path
  only. `history` lists it, but `why` can't name its lines, because the
  extension doesn't know what the file held before the command.
- Bash capture covers only the repository of the session's cwd, as Claude
  Code's does. Ignored files aren't listed. A repository that git can't
  list within 1 s, or that holds over 5,000 uncommitted files, gets no
  Bash capture. The turn-edge snapshots still see those changes.
- Each bash call costs two `git status` runs, which take tens of
  milliseconds in a large repository.
- pi runs the tool calls of one assistant message in parallel. A bash call
  then sees the changes of sibling bash calls too. Files that pi's own edit
  and write tools wrote while the command ran are left to those tools'
  records. A write that runs in parallel with another change to the same
  file can read a stale original.
- The extension can't see moondb's `capture.enabled = false`, so it still
  runs its git calls when capture is off.

## Develop

```bash
node --test services/moondb/pi/test/*.test.mjs   # or: make -C services/moondb test-js
```

The tests run the extension against a fake pi and a fake `moondb` on
`PATH` that keeps each run's argv, stdin and entrypoint.

## Phase B: what moondb needs from pi's session format

Traces, costs and the harness need moondb to read pi's session file in Go.
The format is pi's `docs/session-format.md` and `docs/message-types.md`.
moondb would need these pieces:

- A harness value of `pi` on the envelopes that `record` writes, for
  example from an environment variable this extension sets.
- A JSONL parser for format version 3:
  - The header line (`type: session`) gives `id`, `cwd`, `version`, and
    `parentSession` for a fork or clone.
  - `message` entries carry roles `user`, `assistant`, `toolResult`,
    `system` and `custom`. Each entry has an ISO `timestamp`, and its
    message a millisecond one.
- Assistant fields:
  - `provider`, `model` and `responseModel`, `stopReason`, and
    `errorMessage`.
  - `usage`: `input`, `output`, `cacheRead`, `cacheWrite`, `cacheWrite1h`,
    `reasoning` (already counted in `output`), `totalTokens`, and
    `cost.total`.
  - `toolCall` content blocks with `id`, `name` and `arguments`.
- Tool results: `toolCallId`, `toolName` and `isError`. A result's `usage`
  holds nested model work, and its `nestedCalls` the tools that a tool
  called.
- Other entries:
  - `usage` entries such as `cache_warm`, and the `usage` of `compaction`
    and `branch_summary` entries, all count toward totals.
  - `session_info.name` is the title, and `model_change` and
    `thinking_level_change` mark switches.
  - `custom`, `custom_message`, `label` and `context_edit` carry no cost.
- The tree: entries link by `id` and `parentId`, and `/tree` leaves
  abandoned branches in the same file. pi only appends, so a turn's lines
  are still the bytes after its offset.
- Prices: pi records each message's `cost.total`. moondb can take that, or
  map pi's provider and model ids (OpenRouter names such as
  `anthropic/claude-haiku-5.5`) onto its own price table.
- For `moondb import`: sessions live at
  `<agent dir>/sessions/--<cwd with / as ->--/<timestamp>_<id>.jsonl`. The
  agent dir is `~/.pi/agent` or `$PI_CODING_AGENT_DIR`. pi creates the
  file at the first user or assistant message, so a new session's first
  turn opens at offset 0.
- Subagents: pi has no SubagentStop event. A pi subagent is another pi
  process with its own session file. Linking the two needs
  `parentSession` or a tag the parent sets.
