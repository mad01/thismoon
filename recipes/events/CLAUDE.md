# `events` recipe

Builds and installs the `events` CLI + MCP + web service and registers
`events serve` as a launchd user agent via t-man. Source lives in this repo:
`services/events/` (see `services/events/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/events`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/events`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/events` like the other Go tools (before the
  the consuming repo's `recipes/claude-mcp` at wave 1 registers the MCP server).
- **`packages.events.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.events_service`** — `t-man add --name events -- events serve
  --port 7430 --workdir ~/.local/share/events`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` (entry `events`, command
  `events mcp` — unsandboxed first-party code, HTTP client only)
- the `events.this` route: `recipes/d-man/routes.toml` (`events` → 7430)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Architecture

```
events mcp / CLI ─HTTP─►  events serve (t-man agent, port 7430)  ─owns─►  ~/.local/share/events/sources/<source>.jsonl
web page (events.this) ──┘
producers (sandbox-watch, reminder, deps) ─HTTP POST /api/events─┘  (best-effort, fire-and-forget)
```

`serve` is the single writer. The MCP/CLI/web/producers are all clients of its
HTTP API, so they never touch the JSONL files directly and never race.

**Archive only:** `events.this` records and displays events — it does NOT fire
macOS notifications. Producers keep firing their own notifications and *also*
emit an event here.

## Working with it

```bash
ralph up                                       # sync the source, build + install + register agent
t-man status events                            # check the serve agent
t-man restart events                           # after a manual rebuild
t-man logs events --stderr
events emit --source test --level warn --title "hi"   # smoke-test ingest
events list --source test
ralph disable thismoon/events && ralph up --enable-cleanup   # pre_uninstall removes the agent, then binary cleanup
```

The serve agent stores events under `~/.local/share/events/sources/*.jsonl`
(one capped JSONL ring per source) — removing the agent doesn't delete them.
On hosts without d-man the page is still reachable at `http://localhost:7430/`;
the `events.this` route only applies where d-man runs.

## See also

- Source + module notes: `services/events/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
