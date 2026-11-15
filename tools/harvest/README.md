# harvest

Save what a Claude Code session learned to the brain vault inbox as raw
captures, at the moment the session ends. A once-a-day hint then tells the
next session that captures wait for review.

## How it works

A session ends and its context is gone. harvest runs as a SessionEnd hook
and starts a background run over that session. The run has three steps,
each its own process:

1. **prepare** cuts the transcript down to what the user and the assistant
   wrote, leaving out tool calls, tool output and subagent turns. It reads
   only the lines added since the last harvest of the same session, and
   skips a session with too little text.
2. **complete** sends the harvest skill and that digest to a model, once,
   with no tools. The model answers with the captures worth keeping, as
   JSON.
3. **apply** checks the reply and writes each capture to the brain inbox
   with `brain capture`. brain refuses a secret or a likely duplicate.

Each step runs in an srt sandbox. prepare and apply have no network. Only
complete gets the provider key, and it may reach only the provider's host.
The claude CLI backend is the one exception: its login is in the macOS
keychain, so its model call runs outside the sandbox, with no tools, hooks
or settings.

The model is picked from what the machine has: a LiteLLM proxy, the claude
CLI, or OpenRouter, in that order. The default model is Haiku 5.5.

A capture is unverified and expires 30 days after it was written unless a
person keeps or promotes it. `harvest hint` is how the captures get that
review: a belt custom hint that prints one line at session start when raw
harvest captures wait in the inbox.

## Install

With ralph, the `harvest` recipe builds the binary, links the launcher and
the sandbox settings, and ships the skill. By hand:

```bash
make install   # builds ~/code/bin/harvest and codesigns it
```

Then wire it in your own config:

- A SessionEnd hook in the Claude Code settings file that runs
  `~/code/bin/harvest-env session-end`. The launcher
  (`recipes/harvest/harvest-env.sh`) pulls the provider variables from the
  secrets file. Without it, run `~/code/bin/harvest session-end` with the
  provider variables in the settings env.
- A belt custom hint with `event: session-start` and
  `command: [harvest, hint]`. belt does not expand `~` in `command`, so name
  the binary on `PATH` or give its absolute path.
- `~/.config/srt/harvest.local.json`, which grants read and write on the
  vault. Or set `HARVEST_SANDBOX=off` to run the steps without srt.

## Usage

```bash
harvest session-end < payload.json                       # what the SessionEnd hook runs
harvest run ~/.claude/projects/p/s1.jsonl s1 /work/repo  # harvest one session by hand
harvest hint                                             # the line belt would relay
harvest doctor                                           # checks and the last log lines
harvest docs                                             # the operating doc
```

## Configuration

Environment variables only, all optional. The digest:
`HARVEST_STATE_DIR`, `HARVEST_MIN_BYTES` and `HARVEST_MAX_BYTES`. The
model: `HARVEST_SKILL`, `HARVEST_BACKEND`, `HARVEST_MODEL` and
`HARVEST_EFFORT`. The sandbox: `HARVEST_SANDBOX` and `HARVEST_LAUNCHER`.
The hook: `HARVEST_OFF`. See
[config.md](config.md).

## Where things live

`~/.local/state/harvest` holds `harvest.log` (one line per outcome),
`sessions/<session-id>.lines` (how much of each transcript was harvested),
`hint-day` (the day the hint last spoke), `jobs/<session-id>/` while a run
is on, and `locks/<session-id>.lock` (the lock a run of the session holds).
Only its owner can read any of it.

## Develop

```bash
make build    # ./harvest
make test     # go test ./...
make lint     # golangci-lint run ./...
```

See [CLAUDE.md](CLAUDE.md) for the module layout and the steps, and
[operating.md](operating.md) for runtime behavior.
