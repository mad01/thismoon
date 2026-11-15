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
when the payload is not JSON or names no transcript file, or when its
session id cannot name a file: empty, a path, or with a control character.
That skip line quotes the id, so it stays one line.

## the steps

`{{.Bin}} run` takes the session's lock and holds it to the end. A session
resumed and ended again while its harvest still runs starts a second run;
that one logs `skip <session>: a harvest of this session is running` and
touches nothing. Its new lines wait for the next end of the session. The
run then removes the job dirs that killed runs left behind, makes a job dir
for the session and copies the skill file into it. Then it starts the steps
with `{{.Bin}} step`:

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
dropped. apply reads the first object in the reply and ignores the text
after it. It keeps at most five captures and adds the `harvest` tag to
each. A capture with a title over 200 bytes or a body over 16 KiB is
dropped. Of the model's drops it keeps only the count and the first 20
reasons, each cut to 160 bytes. It never keeps a candidate: the model
drops secrets, so a candidate can hold one.

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
`HARVEST_STATE_DIR` says otherwise. The dir and the log are readable by
their owner only; harvest narrows a wider mode an older version left.

- `{{.LogPath}}`: one timestamped line per outcome. `skip <session>: ...`
  for a session with nothing new, a short digest or a harvest already
  running, and `fail <session>: <step>: ...` for an error, on one line.
  `fail: state: session id is not a plain file name: "<id>"` is a run
  started by hand with a bad id. A harvest logs
  `done <session>: model= digest= cost= backend= sandbox=`, with the report
  indented under it. `cost=?` means the backend reported no cost.
  `sandbox=on` means apply ran under srt.
- `sessions/<session-id>.lines`: how many transcript lines the last
  successful harvest read. A resumed session ends more than once; the next
  harvest reads only the lines after the mark. Delete the file to harvest a
  session again from its start.
- `hint-day`: the day the hint last spoke.
- `jobs/<session-id>/`: the files one run's steps hand on. It exists only
  while the run does. A run that was killed leaves it, and the next run of
  any session removes it.
- `locks/<session-id>.lock`: the lock a run of the session holds. The file
  stays; the lock goes with the process that held it, so a killed run
  blocks nothing.

The report under a done line has one line per decision:

- `captured: <vault>:<path>`: the note was written.
- `dropped: likely duplicate: <title>`: brain found the claim already in the
  vault.
- `dropped: secret found`: brain refused a capture with a secret. The title
  is left out: brain scans it too, so it can be where the secret is.
- `dropped: <reason>: <title>`: harvest dropped the capture: no title, no
  body, a title or body over its cap, or past the cap of five.
- `failed: <title>: <error>`: brain failed for another reason.
- `captured: none`: the model found nothing to keep.
- `dropped by the model: <n>`: how many candidates the model dropped, with
  the reason for each indented under it. Past 20 the line adds
  `(the first 20 reasons)`.

## failure modes

Nothing happens at session end: check the log first. No log line at all
means the hook did not run or a guard returned early. Look for
`HARVEST_OFF` in the settings env, or a hook entry missing from the Claude
Code settings.

A `fail` line names the step and its cause: the error the step printed,
from its start, on one line and cut to 300 bytes. A failed harvest writes no mark,
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
