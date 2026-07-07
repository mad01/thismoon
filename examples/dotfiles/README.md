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
| Routes | [`recipes/d-man/`](recipes/d-man/) | which `.this` names resolve on this machine |
| MCP registration | [`recipes/mcp-registration/`](recipes/mcp-registration/) | which MCP servers Claude Code sees, and the seatbelt sandbox untrusted ones run under |
| Config overlay | [`recipes/config-overlay/`](recipes/config-overlay/) | per-machine service config — watch lists, host gates, local paths |
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
[recipes_config.overrides."thismoon/reminder"]
enable = false
```

   Recipe-level `hosts`/`profiles` gate a recipe you own (see
   `recipes/secrets-env/recipe.toml`, which only applies on a `personal` host);
   the override above gates a recipe that ships from a source.

## Trying it

Copy `config.toml` and `recipes/` into a fresh repo, adjust
`dotfiles_repo_path`, copy `config.local.toml.example` to `config.local.toml`
next to your ralph config and set your machine's profiles, then run
`ralph up --dry-run`.

One-time steps stay manual by design: the d-man daemon registration needs sudo
once per machine (`recipes/d-man/SETUP.md` in this repo), and the `secrets-env`
pattern needs a `~/.config/myservice/secrets.env` created by hand from the
committed `.example` (the recipe never writes the value).
