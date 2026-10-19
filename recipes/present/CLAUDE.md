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

Wave 0 (builds before the consuming repo's `claude-mcp` recipe at wave 1 registers the
MCP server).

- **`packages.present`** — `make build` + `make install` → `~/code/bin/present`
  (codesigned). Build metadata (git sha, commit, release tag, build time) is
  injected via ldflags from the repo-root `buildinfo.mk`.
- **`packages.present.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration). The
  restart ends every live terminal session serve holds, so an apply that
  rebuilds present mid-talk kills the demo.
- **`hooks.builds.present_service`** — registration only: `t-man add …
  present-serve.sh --port 7423 --workdir ~/.config/present`. The launcher
  reads the sharing config from the secrets file and execs `present serve`,
  so the Share button works and the plist holds no secret. Idempotent;
  `run = "always"` self-heals if the agent was removed. Guarded on t-man
  being on PATH.
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
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` — entry `present`,
  command pointing at `present-mcp-sandbox.sh` **in the sources cache**
  (`~/.config/ralph/sources/thismoon/recipes/present/present-mcp-sandbox.sh`)
- the `present.this` route: `recipes/d-man/routes.toml` (`present` → 7423)
- sandbox-denial monitoring (the dotfiles speak recipe's `sandbox-watch` agent
  lists `present` in its watched processes)
- `depends_on = ["packages.t_man"]` references the dotfiles t-man recipe's
  package key — the merged config must include the dotfiles recipes for
  validation to pass.

## Sandbox containment

The MCP server runs inside a seatbelt profile. The dotfiles overlay registers
`present-mcp-sandbox.sh` (not the bare binary). The wrapper `cd`-s to `/`
(getcwd in a read-denied cwd is an EPERM). It scrubs the environment with
`env -i` (only `HOME`, the three `PRESENT_*` config vars, and, when set, the
two sharing vars pass through). Then it execs `~/code/bin/present mcp` under
`sandbox-exec -f present.sb` (the profile lives beside the wrapper in this
directory).

`present.sb` posture:
- **Egress shaped, not zero** — `present mcp` never serves; it dials the local
  events port (`localhost:7430`) and, for `present_share`, outbound `:443` plus
  DNS. Seatbelt can't filter by hostname, so a plain-http shared URL (a kind
  port-forward, say) fails inside the sandbox on purpose. The Share button and
  `present share` run outside it and take any URL.
- **$HOME reads default-denied**; allow-list is `~/code/bin` (the binary) and
  `~/.config/present` (page store).
- **Writes** confined to `~/.config/present` + temp.

## Sharing config

`PRESENT_SHARED_URL` and `PRESENT_AUTHOR_KEY` live in the ralph-managed
secrets file (`~/.config/ralph/secrets.sh`, legacy `~/.secrets.sh` as a
fallback), one `export` each; `present key new` mints the key. Both launchers
source `present-shared-env.sh`, which reads exactly those two exports in a
subshell (a var already in the environment wins) so neither the plist nor
`servers.json` carries a secret. The login shell sources the same file for
`present share`. With the pair unset nothing changes: sharing stays off.

`present_open` execs `/usr/bin/open`, which inherits the sandbox — the browser
is launched by launchd *outside* the sandbox, so it runs unconfined.

Only the MCP server is sandboxed; the CLI and the `present serve` t-man agent
run bare (serve binds the port and is first-party long-running code).

Live terminal posture. serve runs bare, so a terminal session it spawns has
the user's full rights. That is why the live terminal stays behind a switch,
off by default (`docs/adr/0025-present-terminal-sessions-in-serve.md`):
- **Sessions exist only while the switch does.** The switch is the file
  `~/.config/present/terminal.enabled`. serve reads it on every terminal
  request and checks it every 2 s, and `present terminal off` removes it,
  which kills every session within one check. A switch serve cannot read
  counts as off.
- **The CLI flips it, from the login shell.** No MCP tool touches it.
  `present.sb` allows writes under `~/.config/present`, so the profile
  alone would not stop a process inside the sandbox from creating the file.
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
- Seatbelt profile + launchers: `present.sb`, `present-mcp-sandbox.sh`,
  `present-serve.sh`, `present-shared-env.sh` (this dir)
