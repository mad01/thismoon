# `pr` recipe

Builds and installs the `pr` CLI and registers `pr serve` as a launchd user
agent via t-man. Source lives in this repo: `services/pr/` (see
`services/pr/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/pr`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/pr`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/pr` like the other Go tools.
- **`packages.pr.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.pr_service`** — `t-man add --name pr -- pr serve --port 7427`.
  Idempotent; `run = "always"` self-heals if the agent was removed. Guarded on
  t-man being on PATH.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- **`~/.config/pr/config.toml` symlinks** — host-gated per-machine repo watch
  lists. These are private (they name the repos each machine polls), so the
  config files and their `[dotfiles.pr_config_*]` stanzas live in the dotfiles
  overlay, not beside this recipe.
- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- the `pr.this` route: `recipes/d-man/routes.toml` (`pr` → 7427)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Working with it

```bash
ralph up                 # sync the source, build + install + register the agent
t-man status pr          # check the serve agent
t-man logs pr --stderr
ralph disable thismoon/pr && ralph up --enable-cleanup   # pre_uninstall removes the agent, then binary cleanup
```

pr shells out to `gh` (GitHub CLI) for all API calls and relies on `gh` being
authenticated per host. Without a `~/.config/pr/config.toml` the dashboard
starts with no sources. On hosts without d-man the page is still reachable at
`http://localhost:7427/`; the `pr.this` route only applies where d-man runs.

## See also

- Source + module notes: `services/pr/CLAUDE.md`
- Import provenance: `docs/MIGRATED-FROM.md`
