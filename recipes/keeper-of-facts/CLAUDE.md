# `keeper-of-facts` recipe

Builds and installs the keeper-of-facts (`kof`) CLI + MCP + web service and registers
`kof serve` as a launchd user agent via t-man. Source lives in this repo:
`services/keeper-of-facts/` (see `services/keeper-of-facts/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/keeper-of-facts`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/keeper-of-facts`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/kof` like the other Go tools (before the
  the consuming repo's `recipes/claude-mcp` at wave 1 registers the MCP server).
- **`packages.keeper_of_facts.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.keeper_of_facts_service`** — `t-man add --name keeper-of-facts -- kof serve
  --port 7431 --workdir ~/.local/share/kof`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (docs/adr/0006):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` (entry `kof`,
  command `kof mcp` — unsandboxed first-party code, HTTP client only)
- the `kof.this` route: `recipes/d-man/routes.toml` (`kof` → 7431)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Architecture

```
kof mcp / CLI ─HTTP─►  kof serve (t-man agent, port 7431)  ─owns─►  ~/.local/share/kof/assertions.jsonl
web page (kof.this) ──┘
```

`serve` is the single writer. The MCP, the CLI, and the web page are all
clients of its HTTP API, so they never touch the JSON file directly and never
race.

## Working with it

```bash
ralph up                       # sync the source, build + install + register the agent
t-man status keeper-of-facts              # check the serve agent
t-man restart keeper-of-facts             # after a manual rebuild
t-man logs keeper-of-facts --stderr
ralph disable thismoon/keeper-of-facts && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The serve agent stores assertions in `~/.local/share/kof/assertions.jsonl` —
removing the agent doesn't delete that file.

## See also

- Source + module notes: `services/keeper-of-facts/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
