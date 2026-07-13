# `reminder` recipe

Builds and installs the `reminder` CLI + MCP + web service and registers
`reminder serve` as a launchd user agent via t-man. Source lives in this repo:
`services/reminder/` (see `services/reminder/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/reminder`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/reminder`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/reminder` like the other Go tools (before the
  dotfiles `recipes/claude-mcp` at wave 1 registers the MCP server).
- **`packages.reminder.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.reminder_service`** — `t-man add --name reminder -- reminder
  serve --port 7428 --workdir ~/.local/share/reminder`. Idempotent;
  `run = "always"` self-heals if the agent was removed. Guarded on t-man being
  on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: `recipes/claude-mcp/servers.json` (entry `reminder`,
  command `reminder mcp` — unsandboxed first-party code, HTTP client only)
- the `reminder.this` route: `recipes/d-man/routes.toml` (`reminder` → 7428)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Architecture

```
reminder mcp / CLI ─HTTP─►  reminder serve (t-man agent, port 7428)  ─owns─►  ~/.local/share/reminder/reminders.json
web page (reminder.this) ───┘            └─ ticker fires osascript notifications at due time
```

`serve` is the single writer. The MCP/CLI/web are all clients of its HTTP API,
so they never touch the JSON file directly and never race.

## Working with it

```bash
ralph up                          # sync the source, build + install + register the agent
t-man status reminder             # check the serve agent
t-man restart reminder            # after a manual rebuild
t-man logs reminder --stderr
ralph disable thismoon/reminder && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The serve agent stores reminders in `~/.local/share/reminder/reminders.json` —
removing the agent doesn't delete that file.

## See also

- Source + module notes: `services/reminder/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
