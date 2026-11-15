# harvest configuration

## Where config lives

harvest has no configuration file. A Claude Code hook gets no flags, so every
setting is an environment variable. Set it in the `env` block of the Claude
Code settings file, or in the shell that starts Claude Code. Every setting is
optional. `harvest run` hands the `HARVEST_*` settings on to each step it
starts.

## Environment variables

### The digest

- `HARVEST_STATE_DIR` (path, default `~/.local/state/harvest`): the log, the
  per-session marks, the hint's stamp and the job dirs. A leading `~`
  expands to `$HOME`. The path must be absolute. The sandbox settings allow
  only the default; grant another path in
  `~/.config/srt/harvest.local.json`.
- `HARVEST_MIN_BYTES` (whole number, default `4000`): a digest shorter than
  this is skipped, with a skip line in the log.
- `HARVEST_MAX_BYTES` (whole number, at least 1, default `200000`): a longer
  digest keeps its last bytes, so the end of the session is kept.

### The model

- `HARVEST_SKILL` (path, default `~/.claude/skills/harvest/SKILL.md`): the
  skill file the model gets as its system prompt. A missing file fails the
  run.
- `HARVEST_BACKEND` (`litellm`, `claude` or `openrouter`, default empty):
  forces a backend. Empty picks the first one present: a LiteLLM proxy, then
  the claude CLI on PATH, then OpenRouter.
- `HARVEST_MODEL` (model id, default empty): the model. Empty is
  `claude-haiku-5-5`. OpenRouter gets a canonical id in its own form; the
  other backends get it as given.
- `HARVEST_EFFORT` (`low`, `medium`, `high`, `xhigh` or `max`, default
  empty): the reasoning effort. Empty keeps the provider's default.

The backends read the provider variables `LITELLM_BASE_URL`,
`LITELLM_API_KEY` and `OPENROUTER_API_KEY`. The hook's launcher,
`~/code/bin/harvest-env`, pulls them from the secrets file. harvest hands
them to the model step alone.

### The sandbox

- `HARVEST_SANDBOX` (`on` or `off`, default `on`): `off` runs every step
  without srt. Any other value fails the run, so a typo never turns the
  sandbox off.
- `HARVEST_LAUNCHER` (path, default `~/code/bin/harvest-step`): the script
  the confined steps start through. The harvest recipe links it; it execs
  `~/code/bin/srt-run` with the settings in `~/.config/srt/harvest.json`.

With the sandbox on, a missing launcher, srt-run or settings file fails the
run.

### The hook

- `HARVEST_OFF` (any value): switches the SessionEnd hook off. The hint
  still runs.
- `HARVEST_CHILD` (any value): marks a process the harvest itself started.
  The hook does nothing in such a process, so a harvest never harvests its
  own session. Set by harvest, not by hand.

A value that does not parse fails the run with a `fail` line in the log
naming the variable, and fails the `settings` check of `harvest doctor`.

## Flags

- `--version` (bool, default `false`): print the build version and exit,
  the standard cobra root flag.
- `version -o json`: print the build metadata object instead of the bare
  version token.

The other commands take positional arguments or stdin only.
