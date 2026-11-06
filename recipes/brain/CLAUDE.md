# `brain` recipe

Builds and installs the `brain` CLI, MCP server and web view, and registers
`brain serve` as a launchd user agent via t-man under the name
`brain-serve`. Source lives in this repo: `services/brain/` (see
`services/brain/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/brain`. So the
package `working_dir` names the source cache
(`~/.config/ralph/sources/thismoon/services/brain`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/brain` like the other Go tools, before the
  consuming repo's brain overlay (wave 1) links the config and warms the
  indexes.
- `packages.brain`: `make build` and `make install` in the source cache.
  The consuming repo's overlay names this key in its `depends_on`, so keep
  it.
- `packages.brain.service`: restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install, before registration.
- `hooks.builds.brain_service`: `t-man add --name brain-serve --path
  /opt/homebrew/bin:/usr/bin:/bin:/usr/local/bin --extra-log sandbox=… --
  srt-run ~/.config/srt/brain-serve.json brain serve --port 7427`.
  Idempotent; `run = "always"` self-heals if the agent was removed. Guarded
  on t-man being on PATH. `depends_on` is `packages.brain`, `packages.t_man`,
  `builds.speak_chmod_scripts` (keeps srt-run executable) and
  `packages.srtspec` (the launcher srt-run execs).
- `dotfiles.brain_serve_srt_settings`: links `brain-serve.srt.json` to
  `~/.config/srt/brain-serve.json`. The file is rendered from `spec.access`
  in `services/brain/service-info.yaml` by `make sandbox-settings`; edit the
  manifest, not the JSON.
- `pre_uninstall`: removes the t-man agent before cleanup deletes the
  binary.

## Sandbox

serve runs under srt as the label `brain-serve`, loopback only. It binds
7427 and calls one peer, Ollama on `127.0.0.1:11434`, for the query
embedding of a hybrid search. The shipped settings deny `~` and read back
three directories: `~/.config/brain`, `~/code/bin` and the state dir
`~/.local/state/brain`. The state dir is the workdir, the only writable path
besides the temp dirs.

The settings name no machine path, so two reads come from the machine:

- **The vault.** Each vault root in the config needs a read grant, and so
  does `semantic.index_dir` when it moves the semantic stores out of the
  state dir.
- **The config link target.** srt matches resolved paths. When
  `~/.config/brain/config.yaml` is a symlink, the read of `~/.config/brain`
  does not cover the file it points at.

Both go in `~/.config/srt/brain-serve.local.json`, the machine expansion
layer srt-run merges at launch (docs/SANDBOXING.md, "The machine-local
expansion layer"). The consuming repo ships it; it may only widen. Without
it serve starts and reports the unreadable config or vault through
`/api/doctor`, instead of exiting.

The read of `~/.config/brain` is the whole directory, not just the config
file. brain looks for the legacy probe `~/.config/brain/index/state.json`
there to decide its state dir, so serve and the CLI resolve the same one.
An install that still keeps its indexes in `~/.config/brain` would have
serve writing outside its workdir; docs/UPGRADING.md moves them first.

`srtspec explain brain-serve` prints the merged policy, and the sandbox
denials land in the `sandbox` extra log t-man shows.

## What stays in the consuming repo (private overlay)

Machine-specific wiring is deliberately not here (docs/adr/0006):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- the config file and its link to `~/.config/brain/config.yaml`, the vault
  clone, the Ollama model pulls and the `brain index` warm-up
- `~/.config/srt/brain-serve.local.json`, the expansion layer above
- MCP registration: the consuming repo's agent-sync `servers.json` (command
  `brain mcp`, which runs in its own stdio process and never reaches serve)
- the `brain.this` route: the d-man routes file (`brain` → 7427)
- `depends_on = ["packages.t_man"]` references the t-man recipe's package
  key, so the merged config must include the consuming repo's recipes for
  validation to pass.

## Working with it

```bash
ralph up                         # sync the source, build + install + register the agent
t-man status brain-serve         # check the serve agent
t-man restart brain-serve        # after a manual rebuild or a new expansion layer
t-man logs brain-serve --stderr
curl -s 127.0.0.1:7427/version   # the build serve is running
brain doctor                     # includes the serve checks
ralph disable thismoon/brain && ralph up --enable-cleanup   # pre_uninstall removes the agent, then the binary is cleaned up
```

The indexes and the layout cache live under `~/.local/state/brain/`.
Removing the agent doesn't delete them.

## See also

- Source + module notes: `services/brain/CLAUDE.md`
- HTTP API and UI contract: `services/brain/docs/serve.md`
