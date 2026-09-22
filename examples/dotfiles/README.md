# Example: consuming thismoon from a dotfiles repo

A minimal dotfiles repo layout that pulls this monorepo's recipes in as a
remote source and layers machine-private wiring on top. This is the two-layer
model from `docs/adr/0006`: thismoon ships the portable recipes (build,
install, hooks, skills); the consuming repo owns everything machine-private.

```
your-dotfiles/
  config.toml            # ralph config: [[recipe_sources]] points at thismoon,
                         #   plus a host-scoping override example
  recipes/
    d-man/               # overlay: machine-personal routes for the d-man front door
    mcp-registration/    # overlay: register MCP servers (+ seatbelt sandbox variant)
    config-overlay/      # overlay: per-machine service config (watch list, host gate)
    csl-config/          # overlay: which directories csl indexes on this machine
    claude-hooks/        # overlay: belt config + the Claude Code hooks block
    agent-memory/        # overlay: clone the shared memory store belt injects
    secrets-env/         # overlay: secret/env wiring without committing values
```

`config.local.toml` sits beside the ralph config on each machine and selects
that machine's profiles. It is machine-local and gitignored — this repo tracks
only `config.local.toml.example` as a template; you copy it into place per
machine.

## The split

| Layer | Lives in | Examples |
|-------|----------|----------|
| Portable recipes | thismoon `recipes/` | build + install, service registration hooks, skills, SETUP docs |
| Machine-private wiring | your dotfiles repo | the `[[recipe_sources]]` pin, and the overlay patterns below |

Hard `depends_on` across the boundary is allowed only onto the platform
foundations (`packages.t_man`, `packages.d_man`) — see `docs/adr/0006`.

Each kind of machine-private wiring has a worked example in `recipes/`:

| Pattern | Example | What it owns |
|---------|---------|--------------|
| Routes + block list | [`recipes/d-man/`](recipes/d-man/) | which `.this` names resolve on this machine, and which hosts the work profile blocks |
| MCP registration | [`recipes/mcp-registration/`](recipes/mcp-registration/) | which MCP servers Claude Code sees, and the seatbelt sandbox untrusted ones run under |
| Config overlay | [`recipes/config-overlay/`](recipes/config-overlay/) | per-machine service config — watch lists, host gates, local paths |
| csl index list | [`recipes/csl-config/`](recipes/csl-config/) | which directories csl walks for git checkouts, and the remote-host allowlist if you need one |
| Agent hooks (belt) | [`recipes/claude-hooks/`](recipes/claude-hooks/) | the belt config overlay and the `~/.claude/settings.json` hooks block that registers belt's guards and hints |
| Agent memory store | [`recipes/agent-memory/`](recipes/agent-memory/) | the private facts repo cloned to `~/.config/agent-memory`, with the index and fact-file format belt injects at session start |
| Secrets / env | [`recipes/secrets-env/`](recipes/secrets-env/) | a service's secret, resolved at runtime or from a gitignored env-file — never committed |
| Host-scoping | [`config.toml`](config.toml) | enabling a remote recipe on a subset of machines (`hosts = [...]`) |

## How the pieces fit

1. `config.toml` declares the thismoon source. ralph clones it to
   `~/.config/ralph/sources/thismoon` and discovers every `recipes/*/recipe.toml`
   there, merged under the identity `thismoon/<recipe>`.
2. Your local `recipes/` load as usual. Overlay recipes keep only the items the
   machine owns — the binary always ships from the thismoon source, the overlay
   layers wiring on top: `recipes/d-man/` symlinks a personal `routes.toml`,
   `recipes/config-overlay/` a service's config, `recipes/secrets-env/` a
   secret resolved at runtime, `recipes/mcp-registration/` an MCP server (with a
   seatbelt profile for the untrusted ones).
3. Item keys are global across all loaded recipes: an overlay must not
   redeclare a key the thismoon recipe already defines (ralph rejects it).
4. To disable or host-scope a remote recipe, use the namespaced override key —
   `enable = false` turns it off everywhere, `hosts = [...]` runs it only on the
   listed machines. `config.toml` carries a worked block; the short form is:

```toml
[recipes_config.overrides."thismoon/catalog"]
enable = false
```

   Recipe-level `hosts`/`profiles` gate a recipe you own (see
   `recipes/secrets-env/recipe.toml`, which only applies on a `personal` host);
   the override above gates a recipe that ships from a source. A second,
   profile-gated source (a work overlay repo, say) can carry the same table in
   an `overrides.toml` at its root, so its machines get their own answer
   without this repo restating it.

## Runtime profiles: block-list switching (d-man)

`recipes/d-man/` also shows a runtime toggle, distinct from the build-time
`hosts`/`profiles` gates above. The d-man block list lives in the work profile
only, driven by macOS Focus:

- `routes.base.toml` holds the shared routes; `blocklist.work.toml` holds the
  blocked hosts. The active `~/.config/d-man/routes.toml` is **generated** (not
  symlinked) by the `d-man-profile` script, which prepends the block list for
  the work profile and omits it for personal.
- `d-man-profile work` / `personal` rewrites the active file atomically; the
  running daemon reloads it via fsnotify, so no sudo or restart is needed. State
  lives in `~/.config/d-man/profile`, and a build hook re-applies the current
  profile on every `ralph up`.
- Wire it to macOS **Work Focus** with a Shortcuts automation (run
  `d-man-profile work` when Work turns on, `personal` when it turns off) so
  blocking follows work mode. Blocking HTTPS hosts also needs the local CA
  trusted once: `sudo d-man ca install`.

## Updates and releases

How a thismoon change reaches a machine wired up like this:

1. A fix merges to thismoon's main.
2. On the next `ralph up`, the source checkout pulls main (the example
   `config.toml` sets `ref = "main"`, `update = true`), the changed component
   rebuilds from source, and its service restarts through t-man.
3. Verify with the service's `/version` endpoint (or `<tool> version` for
   CLIs). ralph can report ok while an old binary keeps running, so check the
   version, not the run report.

Merging to main is the deploy; there is nothing to bump in the consuming
repo. To move slower than main, pin `ref` to a tag or commit:

```toml
[[recipe_sources]]
name = "thismoon"
url = "git@github.com:mad01/thismoon.git"
ref = "present/v1.2.3"   # any git ref pins the whole checkout at that commit
update = true
```

The pin stays put until you edit the config; changing it moves the checkout
on the next run. Note the ref pins the entire monorepo checkout, not just the
named component.

thismoon's per-component GitHub Releases (`present/v1.2.3` tags, tarballs,
cosign signatures — see `docs/RELEASING.md` in the repo root) serve
`go install` and manual downloads. A fleet wired through ralph builds from
source and never touches those artifacts.

## Trying it

Copy `config.toml` and `recipes/` into a fresh repo, adjust
`dotfiles_repo_path`, copy `config.local.toml.example` to `config.local.toml`
next to your ralph config and set your machine's profiles, then run
`ralph up --dry-run`.

One-time steps stay manual by design: the d-man daemon registration needs sudo
once per machine (`recipes/d-man/SETUP.md` in this repo), trusting the d-man
block-page CA needs `sudo d-man ca install` once (only if you use a block
list), and the `secrets-env` pattern needs a `~/.config/myservice/secrets.env`
created by hand from the committed `.example` (the recipe never writes the
value).
