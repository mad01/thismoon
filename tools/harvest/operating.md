# operating harvest

harvest saves what a Claude Code session learned to the brain vault inbox
as raw captures. A SessionEnd hook starts it, and a belt hint reminds the
user, once a day, that captures wait for review.

## how it runs

There is no serve process. Three entry points, each a short process:

- `{{.Bin}} session-end` is the SessionEnd hook. It reads the hook payload
  on stdin, checks its guards, starts `{{.Bin}} run` detached and returns.
  SessionEnd hooks share a budget of about a second and a half, so this
  path does no heavy work. It always exits 0.
- `{{.Bin}} run <transcript> <session-id> <cwd>` is the detached run. It
  starts three steps, each its own process, and logs the outcome.
- `{{.Bin}} hint` is the belt custom hint. It prints one line when raw notes
  tagged `harvest` wait in the brain inbox, at most once a day.

The hook returns with no log line in three cases. `HARVEST_CHILD` is set
(a process the harvest itself started), `HARVEST_OFF` is set, or the payload
carries an `agent_id` (a subagent ended, not the session). It logs a skip
when the payload is not JSON or names no transcript file.

## the steps

`{{.Bin}} run` makes a job dir for the session in the state dir and copies
the skill file into it. Then it starts the steps with `{{.Bin}} step`:

1. `prepare` reads the transcript lines after the session's mark and builds
   the digest. It logs a skip when nothing is new or the digest is under
   `HARVEST_MIN_BYTES`, and the run stops there. Otherwise it writes the
   model's user text: the session id, the working directory and the date,
   then the digest.
2. `complete` sends the skill file as the system prompt and that user text
   to the model, once, with no tools. It writes the reply as it came.
3. `apply` reads the reply, writes each capture with `brain capture`, moves
   the mark and logs the done line.

The run removes the job dir at the end, whatever the outcome.

## the model

The backend is picked in a fixed order: a LiteLLM proxy when
`LITELLM_BASE_URL` is set, then the claude CLI when `claude` is on PATH,
then OpenRouter when `OPENROUTER_API_KEY` is set. `HARVEST_BACKEND` forces
one, and `HARVEST_MODEL` and `HARVEST_EFFORT` steer it. The default model is
Haiku 5.5. A call that runs past 10 minutes fails.

The reply is one JSON object: the captures, each with a title, a body, up to
three tags and an optional source URL, and the candidates the model
dropped. apply keeps at most five captures and adds the `harvest` tag to
each.

## the sandbox

Each step starts through `~/code/bin/harvest-step` (`HARVEST_LAUNCHER`),
which runs it under srt with the settings in `~/.config/srt/harvest.json`.
A step gets a clean environment: `HOME`, `PATH`, `TMPDIR` and the
`HARVEST_*` settings. Only `complete` gets the provider variables, and only
it may reach the network, to the provider's host alone. `prepare` and
`apply` have no network.

The claude CLI backend is the exception. Its login is in the macOS
keychain, which srt does not let it reach, so its `complete` step runs
outside srt. kit/llm turns off tools, hooks and settings for that call.

`HARVEST_SANDBOX=off` runs every step without srt. With the sandbox on and
the launcher, srt-run or the settings file missing, every run fails with a
`fail` line;
it never runs a step without the sandbox instead.

The settings allow the state dir, the transcripts, `~/.config/brain` and
`~/code/bin`. The vault, and the real target of the brain config when it is
a link, differ per machine: they go in `~/.config/srt/harvest.local.json`.

## the digest

A transcript is JSONL. The digest keeps the lines with `type` user or
assistant, drops subagent (`isSidechain`) and Claude Code note (`isMeta`) lines, and
keeps only the text: a string content, or the `text` blocks of a content
array. Tool calls, tool output and thinking stay out. Each entry renders as
`## <type>`, the text, and a blank line. The digest keeps the last
`HARVEST_MAX_BYTES` bytes, so a long session keeps its end. A last line
without a newline is still being written; it is left for the next harvest.

## state

Everything lives in the state dir, `{{.StorePath}}` unless
`HARVEST_STATE_DIR` says otherwise:

- `{{.LogPath}}`: one timestamped line per outcome. `skip <session>: ...`
  for a session with nothing new or a short digest, and
  `fail <session>: <step>: ...` for an error. A harvest logs
  `done <session>: model= digest= cost= backend= sandbox=`, with the report
  indented under it. `cost=?` means the backend reported no cost.
  `sandbox=on` means apply ran under srt.
- `sessions/<session-id>.lines`: how many transcript lines the last
  successful harvest read. A resumed session ends more than once; the next
  harvest reads only the lines after the mark. Delete the file to harvest a
  session again from its start.
- `hint-day`: the day the hint last spoke.
- `jobs/<session-id>/`: the files one run's steps hand on. It exists only
  while the run does.

The report under a done line has one line per decision:

- `captured: <vault>:<path>`: the note was written.
- `dropped: likely duplicate: <title>`: brain found the claim already in the
  vault.
- `dropped: secret found: <title>`: brain refused a body with a secret.
- `dropped: <reason>: <candidate>`: the model, or the cap of five, dropped it.
- `failed: <title>: <error>`: brain failed for another reason.
- `captured: none`: the model found nothing to keep.

## failure modes

Nothing happens at session end: check the log first. No log line at all
means the hook did not run or a guard returned early. Look for
`HARVEST_OFF` in the settings env, or a hook entry missing from the Claude
Code settings.

A `fail` line names the step and its cause. A failed harvest writes no mark,
so the next end of the same session tries the same lines again.
`{{.Bin}} run` with the three values from the hook payload retries one
session by hand. Common causes:

- `prepare: sandbox: launcher: ...`, or a `prepare:` line that names srt-run
  or the settings file: the sandbox cannot start. Install the srtspec and harvest recipes, or set
  `HARVEST_SANDBOX=off`.
- `complete: llm: no backend configured`: no provider is set and `claude`
  is not on PATH. The SessionEnd hook must run `harvest-env`, which pulls
  the provider variables from the secrets file.
- `apply: reply: ...`: the reply was not the JSON object the skill asks
  for.
- `apply: run: all N captures failed`: brain refused every write. A
  sandboxed apply that cannot reach the vault fails this way: the vault
  belongs in `~/.config/srt/harvest.local.json`. `sandbox-watch` shows the
  denials under the `harvest` label.
- `operation not permitted` in a confined step with `HARVEST_STATE_DIR` set:
  the state dir is outside the sandbox. Grant it in the local layer.

The hint stays silent: it speaks once a day at most, and only when `brain
list inbox --tag harvest --status raw` reports a total above zero. Delete
`hint-day` to let it speak again today. A failure (no `brain` on PATH, an
error from brain) is printed to stderr, which belt does not show.

## first moves

1. `{{.Bin}} doctor`: the settings, the state dir, the skill file, brain on
   PATH, the backend and model, the sandbox, and the last log lines.
2. `tail -n 20 {{.LogPath}}`: the recent outcomes.
3. `{{.Bin}} hint`, run by hand, to see the line belt would relay.
