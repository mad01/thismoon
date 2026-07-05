# `status` recipe

Builds and installs the `status` CLI and registers `status serve` as a launchd
user agent via t-man. Source lives in this repo: `services/status/` (see
`services/status/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/status`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/status`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/status` like the other Go tools.
- **`packages.status.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.status_service`** — `t-man add --name status -- status serve
  --port 7426`. Idempotent; `run = "always"` self-heals if the agent was
  removed. Guarded on t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the
  binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- the `status.this` route: `recipes/d-man/routes.toml` (`status` → 7426)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Working with it

```bash
ralph up                 # sync the source, build + install + register the agent
t-man status status      # check the serve agent
t-man logs status --stderr
ralph disable thismoon/status && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The serve agent stores uptime history in `~/.local/share/status/history.json`
— removing the agent doesn't delete history. On hosts without d-man the page
is still reachable at `http://localhost:7426/`; the `status.this` route only
applies where d-man runs.

## See also

- Source + module notes: `services/status/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
