# Example: consuming thismoon from a dotfiles repo

A minimal dotfiles repo layout that pulls this monorepo's recipes in as a
remote source and layers machine-private wiring on top. This is the two-layer
model from `docs/adr/0006`: thismoon ships the portable recipes (build,
install, hooks, skills); the consuming repo owns everything machine-private.

```
your-dotfiles/
  config.toml            # ralph config: [[recipe_sources]] points at thismoon
  recipes/
    d-man/               # overlay: machine-personal routes for the d-man front door
      recipe.toml
      routes.toml
```

`config.local.toml` sits beside the ralph config on each machine (outside the
repo, not committed) and selects that machine's profiles.

## The split

| Layer | Lives in | Examples |
|-------|----------|----------|
| Portable recipes | thismoon `recipes/` | build + install, service registration hooks, skills, SETUP docs |
| Machine-private wiring | your dotfiles repo | the `[[recipe_sources]]` pin, d-man routes, per-machine configs (watch lists, host gates), MCP registration, secrets |

Hard `depends_on` across the boundary is allowed only onto the platform
foundations (`packages.t_man`, `packages.d_man`) — see `docs/adr/0006`.

## How the pieces fit

1. `config.toml` declares the thismoon source. ralph clones it to
   `~/.config/ralph/sources/thismoon` and discovers every `recipes/*/recipe.toml`
   there, merged under the identity `thismoon/<recipe>`.
2. Your local `recipes/` load as usual. Overlay recipes keep only the items the
   machine owns — the example `recipes/d-man/` symlinks a personal
   `routes.toml` into place while the d-man binary ships from `thismoon/d-man`.
3. Item keys are global across all loaded recipes: an overlay must not
   redeclare a key the thismoon recipe already defines (ralph rejects it).
4. To disable or host-scope a remote recipe, use the namespaced override key:

```toml
[recipes_config.overrides."thismoon/reminder"]
enable = false
```

## Trying it

Copy `config.toml` and `recipes/` into a fresh repo, adjust
`dotfiles_repo_path`, put a `config.local.toml` with your machine's profiles
next to your ralph config, and run `ralph up --dry-run`.

One-time steps stay manual by design: the d-man daemon registration needs sudo
once per machine (`recipes/d-man/SETUP.md` in this repo).
