# `prs` recipe

Builds and installs the prs open-PR dashboard (CLI + MCP + web service) and
registers `prs serve` as a launchd user agent via t-man. Source lives in this
repo: `services/prs/` (see `services/prs/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/prs`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/prs`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/prs` like the other Go tools (before the
  consuming repo's `recipes/claude-mcp` at wave 1 registers the MCP server).
- **`packages.prs.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.prs_service`** — `t-man add --name prs -- prs serve
  --port 7427 --workdir ~/.local/share/prs`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (docs/adr/0006):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- **the polling config `~/.config/prs/config.yaml`** (dirs to scan, excludes,
  host allowlist) — without it serve runs but polls nothing
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json`
  (entry `prs`, command `prs mcp` — unsandboxed first-party code, HTTP client only)
- the `prs.this` route: `recipes/d-man/routes.toml` (`prs` → 7427)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

Auth is not wiring at all: serve mints tokens from the machine's existing
`gh auth login` sessions at runtime.

## Architecture

```
prs mcp / CLI ─HTTP─►  prs serve (t-man agent, port 7427)  ─owns─►  ~/.local/share/prs/repos.jsonl
web page (prs.this) ──┘        └─polls─►  GitHub hosts (gh-minted token per host)
```

`serve` is the single writer. The MCP, the CLI, and the web page are all
clients of its HTTP API, so they never touch the cache file or GitHub
directly and never race.

## Working with it

```bash
ralph up                    # sync the source, build + install + register the agent
t-man status prs            # check the serve agent
t-man restart prs           # after a manual rebuild
t-man logs prs --stderr
prs doctor                  # reachability, store, version skew, config, gh
ralph disable thismoon/prs && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The serve agent caches PR state in `~/.local/share/prs/repos.jsonl` —
removing the agent doesn't delete that file, and deleting the file only
costs one poll cycle.

## See also

- Source + module notes: `services/prs/CLAUDE.md`
