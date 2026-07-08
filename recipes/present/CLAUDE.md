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

Wave 0 (builds before the dotfiles `claude-mcp` recipe at wave 1 registers the
MCP server).

- **`packages.present`** — `make build` + `make install` → `~/code/bin/present`
  (codesigned). Version (git sha) is injected via ldflags.
- **`packages.present.service`** — restarts the agent only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.present_service`** — registration only: `t-man add … present
  serve --port 7423 --workdir ~/.config/present`. Idempotent; `run = "always"`
  self-heals if the agent was removed. Guarded on t-man being on PATH.
- **`hooks.builds.present_chmod_mcp_wrapper`** — keeps `present-mcp-sandbox.sh`
  executable inside the sources cache (the overlay registers the wrapper, not
  the bare binary).
- **`post_install` cache hook** — `make cache` fetches fonts/JS into
  `~/.config/present` so pages have zero CDN dependencies. Offline-safe.
- **`dirs_mirror.present_skill`** — symlinks `skills/present/` into
  `~/.claude/skills/` so the Claude skill ships with the service.
- **`pre_uninstall`** — removes the t-man agent before cleanup deletes the binary.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (MAD-199 tracks the pattern):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- MCP registration: `recipes/claude-mcp/servers.json` — entry `present`,
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
`present-mcp-sandbox.sh` (not the bare binary); the wrapper `cd`-s to `/`
(getcwd in a read-denied cwd is an EPERM), scrubs the environment with `env -i`
(only `HOME` + the three `PRESENT_*` vars pass through), and execs
`~/code/bin/present mcp` under `sandbox-exec -f present.sb` (the profile lives
beside the wrapper in this directory).

`present.sb` posture:
- **No network at all** — `present mcp` never serves or dials; it shares the
  page store on disk with the separate (unsandboxed) `present serve` daemon.
- **$HOME reads default-denied**; allow-list is `~/code/bin` (the binary) and
  `~/.config/present` (page store).
- **Writes** confined to `~/.config/present` + temp.

`present_open` execs `/usr/bin/open`, which inherits the sandbox — the browser
is launched by launchd *outside* the sandbox, so it runs unconfined.

Only the MCP server is sandboxed; the CLI and the `present serve` t-man agent
run bare (serve binds the port and is first-party long-running code).

## Architecture

```
present_create/update (MCP)  ─writes files─►  ~/.config/present/pages/<id>/
                                                  meta.json, content.html, graph.js
present serve (t-man agent)  ─reads files─►   http://localhost:7423/p/<id>
```

- MCP tools and the HTTP server share `~/.config/present` (the workdir) and the
  port; they communicate purely through the filesystem.
- Pages are create/read/update/list only via MCP — **no delete**.

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
- Skill: `skills/present/SKILL.md` (mirrored into `~/.claude/skills/`)
- Seatbelt profile + wrapper: `present.sb`, `present-mcp-sandbox.sh` (this dir)
- Import provenance: `docs/MIGRATED-FROM.md`
