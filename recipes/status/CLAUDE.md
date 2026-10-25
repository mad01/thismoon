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
- **`dotfiles.status_srt_settings`**: links `status.srt.json` to
  `~/.config/srt/status.json`, the settings `status serve` runs under in srt
  (anthropics/sandbox-runtime). Rendered from `spec.access` in
  `services/status/service-info.yaml` by `make sandbox-settings`; edit the
  manifest, not the JSON.
- **`hooks.builds.status_service`**: `t-man add --name status -- ~/code/bin/srt-run
  ~/.config/srt/status.json ~/code/bin/status serve --port 7426 --workdir
  ~/.local/share/status`, with the denial ledger as an extra log
  (`t-man logs status --source sandbox`). Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
  `launchctl print` works inside the sandbox, but each exec logs three
  base-profile denials (`kern.iossupportversion`, `kern.singleuser` and
  `com.apple.diagnosticd`), all ignore-listed. That is about 48 IGNORED
  denials a minute on 13 services, which sandbox-watch collapses to one
  ledger line per distinct denial per window.
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
