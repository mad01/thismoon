# `wire` recipe

Builds and installs the `wire` CLI + MCP + web service and registers
`wire serve` as a launchd user agent via t-man. Source lives in this repo:
`services/wire/` (see `services/wire/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/wire`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/wire`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/wire` like the other Go tools (before the
  consuming repo's `recipes/claude-mcp` at wave 1 registers the MCP server).
- **`packages.wire.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.wire_service`** — `t-man add --name wire -- wire serve
  --port 7432 --workdir ~/.local/share/wire`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (docs/adr/0006):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` (entry `wire`,
  command `wire mcp` — unsandboxed first-party code, HTTP client only)
- the `wire.this` route: `recipes/d-man/routes.toml` (`wire` → 7432)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

**The route needs streaming to work.** The transcript page follows a
server-sent event stream and reads can hold a connection open for two minutes,
so the `wire.this` route must not buffer responses or impose a short proxy read
timeout. `http://localhost:7432/` always works regardless, and the MCP talks to
localhost directly.

## Architecture

```
wire mcp / CLI ─HTTP─►  wire serve (t-man agent, port 7432)  ─owns─►  ~/.local/share/wire/*.jsonl
web page (wire.this) ─SSE─┘
```

`serve` is the single writer and the only place a blocking read parks. The MCP,
the CLI, and the web page are all clients of its HTTP API, so they never touch
the log files directly and never race.

## Working with it

```bash
ralph up                       # sync the source, build + install + register the agent
t-man status wire              # check the serve agent
t-man restart wire             # after a manual rebuild
t-man logs wire --stderr
ralph disable thismoon/wire && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

Restarting the agent drops every in-flight blocking read and event stream.
Callers see a request failure and retry, so it is safe — but a session sitting
in a 120-second wait will notice.

The serve agent stores transcripts under `~/.local/share/wire/` — removing the
agent doesn't delete them.

## See also

- Source + module notes: `services/wire/CLAUDE.md`
