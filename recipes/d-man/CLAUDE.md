# `d-man` recipe

Builds and installs the `d-man` local domain front door. Source lives in this
repo: `services/d-man/` (see `services/d-man/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/d-man`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/d-man`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/d-man` (platform foundation, before wave-1
  recipes). Hard `depends_on` from other recipes onto `packages.d_man` is
  allowed — d-man is a foundation per `docs/adr/0006`.
- **`hooks.post_apply`** — prints the one-time daemon registration command
  until `/Library/LaunchDaemons/d-man.plist` exists. Registration itself is a
  manual sudo step (`SETUP.md`); it must never run unattended.
- **`hooks.pre_uninstall`** — prints the daemon removal command (also sudo).

## Why no `[packages.d_man.service]` restart stanza

d-man is a **root** daemon (binds `:80`, writes `/etc/hosts`). Bouncing it via
`t-man --daemon restart` would prompt for sudo on every `ralph up`. Instead
`d-man serve` watches its own binary and `exit(0)`s when `make install`
overwrites it; launchd `KeepAlive` relaunches the new build. No restart command,
no sudo.

## What stays in dotfiles (private overlay)

Machine-specific wiring is deliberately not here (see `docs/adr/0006`):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- **`routes.toml` + its symlink item** (`dotfiles.d_man_routes` →
  `~/.config/d-man/routes.toml`): the route set (which `.this` names exist on
  a machine) is personal even though the mechanism is platform
- the one-time `sudo t-man --daemon add` registration state (per machine)

## Working with it

```bash
ralph up                         # sync the source, build + install
t-man status d-man               # check the daemon (after one-time SETUP.md registration)
t-man logs d-man
curl http://localhost/__this/sites.json   # live-filtered site list (webkit ⌘K palette)
```

Editing routes needs no ralph run at all: the daemon watches its routes file
(fsnotify) and re-syncs `/etc/hosts` + proxy routes on save.

## See also

- Source + module notes: `services/d-man/CLAUDE.md`
- One-time daemon setup: `SETUP.md`
- Import provenance: `docs/MIGRATED-FROM.md`
