# `moondb` recipe

Builds and installs the `moondb` capture hook, CLI, MCP server and web
service, and registers `moondb serve` as a launchd user agent via t-man.
Source lives in this repo: `services/moondb/` (see
`services/moondb/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/moondb`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/moondb`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/moondb` like the other Go tools, before the
  consuming repo's agent-sync recipe (wave 1) registers the MCP server and
  the hooks.
- **`packages.moondb.service`**: restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install, before registration.
- **`hooks.builds.moondb_service`**: `t-man add --name moondb -- moondb serve
  --port 7433 --workdir ~/.local/state/moondb`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`pre_uninstall`**: removes the t-man agent before cleanup deletes the
  binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (docs/adr/0006):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- the capture hooks in the Claude Code settings: `moondb record` on
  UserPromptSubmit (async), on PostToolUse for
  `Edit|Write|MultiEdit|NotebookEdit` (sync, timeout 3), and on Stop (async)
- MCP registration: the consuming repo's agent-sync `servers.json` (entry
  `moondb`, command `moondb mcp`, an HTTP client only)
- the `moondb.this` route: `recipes/d-man/routes.toml` (`moondb` → 7433)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key, so the merged config must include the dotfiles recipes for
  validation to pass.

## Architecture

```
hooks ─stdin─► moondb record ─files─► ~/.local/state/moondb/spool/
                                              │ drained by
moondb mcp / CLI ─HTTP─► moondb serve (t-man agent, port 7433) ─owns─► log/ + objects/
web page (moondb.this) ─┘
```

`serve` is the single writer. `record` only writes the spool, so capture
keeps working while serve is down. The MCP, the CLI and the web page are
clients of serve's HTTP API and never touch the store directly.

## Working with it

```bash
ralph up                       # sync the source, build + install + register the agent
t-man status moondb            # check the serve agent
t-man restart moondb           # after a manual rebuild
t-man logs moondb --stderr
ralph disable thismoon/moondb && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The store lives under `~/.local/state/moondb/`. Removing the agent doesn't
delete it.

## See also

- Source + module notes: `services/moondb/CLAUDE.md`
