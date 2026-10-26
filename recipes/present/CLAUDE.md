# `present` recipe

Builds and installs the `present` CLI + MCP server, and registers
`present serve` as a launchd user agent via t-man. Source lives in this repo:
`services/present/` (see `services/present/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/present`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/present`), not a dev checkout.

## What this recipe does

Wave 0 (builds before the consuming repo's agent-sync recipe at wave 1 registers the
MCP server).

- **`packages.present`** — `make build` + `make install` → `~/code/bin/present`
  (codesigned). Build metadata (git sha, commit, release tag, build time) is
  injected via ldflags from the repo-root `buildinfo.mk`.
- **`packages.present.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration). The
  restart ends every live terminal session serve holds, so an apply that
  rebuilds present mid-talk kills the demo.
- **`hooks.builds.present_service`**: registration only: `t-man add …
  --extra-log sandbox=… present-serve.sh --port 7423 --workdir
  ~/.config/present`. The launcher reads the sharing config from the
  secrets file, outside the sandbox, and execs `present serve` through
  `srt-run` (see Sandbox containment), so the Share button works and the
  plist holds no secret. Idempotent; `run = "always"` self-heals if the
  agent was removed. Guarded on t-man being on PATH.
- **`dotfiles.present_serve_srt_settings`**: links `present-serve.srt.json`
  to `~/.config/srt/present-serve.json`, the serve agent's srt settings,
  rendered from `spec.access` (label `present-serve`) in
  `services/present/service-info.yaml` by `make sandbox-settings`. Edit the
  manifest, not the JSON.
- **`hooks.builds.present_chmod_mcp_wrapper`** — keeps `present-mcp-sandbox.sh`
  and `present-serve.sh` executable inside the sources cache (the overlay
  registers the wrapper, not the bare binary).
- **`post_apply` cache hook** — `make cache` fetches fonts/JS into
  `~/.config/present` (d3 included) so pages have zero CDN dependencies,
  and cache-assets.sh exits early once the assets exist. A failed or
  checksum-mismatched download now fails the make target and leaves no
  partial file; the hook's `|| true` still swallows that, so an offline
  apply stays safe. See `docs/adr/0022-vendored-browser-assets.md`.
- **`dotfiles.present_skill`** — symlinks the repo-root `skills/present/` dir
  into `~/.claude/skills/` so the Claude skill ships with the service.
- **`dotfiles.present_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/` so Codex and other agents discover it too.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: the consuming repo's agent-sync `servers.json`: entry `present`,
  command pointing at `present-mcp-sandbox.sh` **in the sources cache**
  (`~/.config/ralph/sources/thismoon/recipes/present/present-mcp-sandbox.sh`)
- the `present.this` route: `recipes/d-man/routes.toml` (`present` → 7423)
- the MCP server's sandbox settings, `~/.config/srt/present.json` (see
  Sandbox containment below); the serve agent's ship here
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Sandbox containment

The MCP server runs under anthropics/sandbox-runtime (srt). The dotfiles
overlay registers `present-mcp-sandbox.sh` (not the bare binary). The wrapper
`cd`-s to `/` (getcwd in a read-denied cwd is an EPERM). It scrubs the
environment with `env -i` (only `HOME`, the three `PRESENT_*` config vars,
and, when set, the two sharing vars, `TMPDIR`, `SRT_DEBUG` and
`SRT_RUN_ALLOW_DOMAINS` pass through). Then it
execs `~/code/bin/srt-run ~/.config/srt/present.json ~/code/bin/present mcp`.
`srt-run` ships with the speak recipe; it tags the process with the label
`present`, which is how `sandbox-watch` attributes its denials.

The settings file is not in this repo. The consuming repo provides
`~/.config/srt/present.json` beside the MCP registration (docs/adr/0006).
It sits outside the page store on purpose: the sandbox may write the store
and must never reach its own policy. Without the file srt exits 1 and the
server never starts. The posture it should carry:
- **Egress is a hostname allow-list.** `present mcp` never serves. It dials
  the local events port (`allowLocalBinding: true` covers `127.0.0.1:7430`)
  and, for `present_share`, the shared instance. The static file keeps
  `network.allowedDomains` empty: `present-shared-env.sh` derives
  `host[:port]` from `PRESENT_SHARED_URL` as `SRT_RUN_ALLOW_DOMAINS`, the
  wrapper passes it through, and `srt-run` merges it in at launch. Any
  other host is refused at srt's proxy. `present share` runs outside the
  sandbox and reaches any URL.
- **$HOME reads denied** (`denyRead: ["~"]`); `allowRead` carves out
  `~/code/bin` (the binary) and the page store (`~/.config/present`,
  `~/.local/state/present`).
- **Writes** confined to the page store plus the temp dirs
  (`/private/tmp`, `/private/var/folders`): `srt-run` points the child's
  `TMPDIR` there, and srt does not allow it on its own.
- Nothing present reads under its store is a symlink, so `allowRead` needs
  no link targets (seatbelt checks the resolved path).

## Sharing config

`PRESENT_SHARED_URL` and `PRESENT_AUTHOR_KEY` live in the ralph-managed
secrets file (`~/.config/ralph/secrets.sh`, legacy `~/.secrets.sh` as a
fallback), one `export` each; `present key new` mints the key. Both launchers
source `present-shared-env.sh`, which reads exactly those two exports in a
subshell (a var already in the environment wins) so neither the plist nor
`servers.json` carries a secret. The login shell sources the same file for
`present share`. With the pair unset nothing changes: sharing stays off.
After adding the exports, `t-man restart present` picks them up for the
Share button, and a new MCP session (`/mcp` reconnect) for `present_share`;
`ralph up` is needed only when the recipe changed. `present doctor` checks
the pair against the instance.

`present_open` execs `/usr/bin/open`, which inherits the sandbox: the browser
is launched by launchd *outside* the sandbox, so it runs unconfined. Launch
Services needs `allowAppleEvents: true` in the MCP settings (the manifest's
`extra` block, since the schema has no field for it), or `present_open` fails
under the sandbox.

The `present serve` t-man agent runs under srt as well, with the label
`present-serve` so its denials stay apart from the MCP server's.
`present-serve.sh` sources `present-shared-env.sh` outside the sandbox, `cd`-s
to `/`, and execs `~/code/bin/srt-run ~/.config/srt/present-serve.json
~/code/bin/present serve`. Its settings ship here as `present-serve.srt.json`,
rendered from the component's `spec.access`, since nothing in them differs
per machine:
- **Loopback plus the shared host.** serve binds 7423 and posts to events
  (`allowLocalBinding: true`). The Share button pushes to the shared
  instance, whose host arrives through `SRT_RUN_ALLOW_DOMAINS` like the MCP
  server's. That push is Go TLS, which needs trustd for certificate checks,
  so the settings set `enableWeakerNetworkIsolation`.
- **$HOME reads denied** except `~/code/bin` and the page store
  (`~/.config/present`, the workdir the recipe passes).
- **Writes** to the page store and the temp dirs.

Only the CLI runs bare.

Live terminal posture. serve's sandbox grants no pseudo-terminal, so a
terminal session doesn't start while serve runs under srt: openpty is
denied (`docs/adr/0025-present-terminal-sessions-in-serve.md`, 2026-10-25
amendment). Turning sessions back on is a future opt-in: a machine-local
settings expansion that adds `allowPty` and the reads a shell needs.
`allowPty` alone isn't enough, since the shell would inherit serve's
confinement. The switch and the rules below
still hold for a serve run bare by hand:
- **Sessions exist only while the switch does.** The switch is the file
  `~/.config/present/terminal.enabled`. serve reads it on every terminal
  request and checks it every 2 s, and `present terminal off` removes it,
  which kills every session within one check. A switch serve cannot read
  counts as off.
- **The CLI flips it, from the login shell.** No MCP tool touches it.
  The sandbox settings allow writes under `~/.config/present`, so the
  sandbox alone would not stop a process inside it from creating the file.
- **A session runs what the stored page names.** serve takes the command,
  directory, and prefill from the page's own terminal block, never from the
  socket, so script injected into a page cannot pick what runs.
- **The shared instance has no terminal.** `serve --shared` builds no
  session manager and registers no terminal route; its pages show a
  placeholder in each terminal block.
- **`present.this` is on the origin allow-list.** The socket accepts
  loopback, `localhost`, and `present.this`. d-man proxies the WebSocket
  upgrade but rewrites the Host to `127.0.0.1:7423`, so the library's
  Origin-equals-Host rule never covers a page opened at `http://present.this`.

## Architecture

```
present_create/update (MCP)  ─writes files─►  ~/.config/present/pages/<id>/
                                                  meta.json, content.html, graph.js
present serve (t-man agent)  ─reads files─►   http://localhost:7423/p/<id>
```

- MCP tools and the HTTP server share `~/.config/present` (the workdir) and the
  port; they communicate purely through the filesystem.
- Pages are created, read, updated, and listed via MCP; delete is a button in
  the web index, and deleting a page that was shared removes the shared copy
  first.

## Working with it

```bash
ralph up                  # sync the source, build + install, register the agent
t-man status present      # check the serve daemon
t-man logs present        # serve logs
ralph disable thismoon/present && ralph up --enable-cleanup   # pre_uninstall removes the agent, then binary cleanup
```

If you build the binary outside ralph, restart manually so the running agent
picks up the new code:
`make -C ~/.config/ralph/sources/thismoon/services/present install && t-man restart present`.

## See also

- Source + module notes: `services/present/CLAUDE.md`
- Skill: repo-root `skills/present/SKILL.md` (linked into `~/.claude/skills/`)
- Launchers: `present-mcp-sandbox.sh`, `present-serve.sh`,
  `present-shared-env.sh` (this dir); the sandbox pattern and a worked
  settings file: `docs/SANDBOXING.md`
