# `deps` recipe

Builds and installs the `deps` CLI + MCP + web service, symlinks its discovery
config, and registers `deps serve` as a launchd user agent via t-man. Source
lives in this repo: `services/deps/` (see `services/deps/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/deps`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/deps`), not a dev checkout. The
`[dotfiles.deps_config]` source stays relative — ralph resolves it against the
recipe's directory inside the cache.

## What this recipe does

- `wave = 0`: builds `~/code/bin/deps` like the other Go tools (before the
  the consuming repo's `recipes/claude-mcp` at wave 1 registers the MCP server).
- `profiles = ["personal"]`: personal Mac only — it depends on d-man and scans
  the personal repos. The MCP entry and d-man route carry the same gate.
- **`directories.deps_config`** + **`dotfiles.deps_config`** — creates
  `~/.config/deps/` and symlinks `config.toml` (the `exclude_repos` /
  `exclude_paths` discovery config) into it. Editing it takes effect on the next
  scan; no restart.
- **`packages.deps.service`** — restarts the agent only when the installed binary
  changed (ralph hashes `install_paths`); `t-man status` guard skips the restart
  on first install.
- **`hooks.builds.deps_service`** — `t-man add --name deps -- deps serve --port
  7429 --workdir ~/.local/share/deps` (with PATH + GOPATH/GOMODCACHE env for the
  `go list` discovery). Idempotent; `run = "always"` self-heals.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` (entry `deps`, command
  `deps mcp`, `profiles: ["personal"]` — unsandboxed first-party code, HTTP
  client only)
- the `deps.this` route: `recipes/d-man/routes.toml` (`deps` → 7429)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Architecture

```
deps mcp / CLI ─HTTP─►  deps serve (t-man agent, port 7429)  ─owns─►  ~/.local/share/deps/scan.json
web page (deps.this) ───┘            ├─ daily catch-up scan loop (OSV) + offline backoff
                                     └─ coalesced osascript notifications on newly-flagged deps
```

`serve` is the single writer and the only process that reaches OSV. The
MCP/CLI/web are clients of its HTTP API.

## Discovery config

`config.toml` (here, tracked in git) → `~/.config/deps/config.toml`. The repo
*set* comes from the catalog registry (`~/.config/catalog/registry.yaml`); this
file trims it via `exclude_repos` / `exclude_paths` globs. Git worktrees and
nested checkouts are skipped automatically.

## Working with it

```bash
ralph up                 # sync the source, build + install + register the agent + symlink config
t-man status deps        # check the serve agent
t-man logs deps --stderr
ralph disable thismoon/deps && ralph up --enable-cleanup   # pre_uninstall removes the agent, then binary cleanup
```

The serve agent stores scan results in `~/.local/share/deps/` — removing the
agent doesn't delete them. On hosts without d-man the page is still reachable
at `http://localhost:7429/`; the `deps.this` route only applies where d-man runs.

## See also

- Source + module notes: `services/deps/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
