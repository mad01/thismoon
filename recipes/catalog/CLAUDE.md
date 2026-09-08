# CLAUDE.md — catalog recipe

Public-layer recipe for `services/catalog` (see docs/adr/0006 for the
layering). Builds and installs the `catalog` binary from the thismoon sources
cache and registers the `catalog-web` t-man agent on port 7575.

Item keys are the dotfiles-era names (`packages.catalog`,
`hooks.builds.catalog_web_service`) so ralph state carried over at cutover —
do not rename them.

No `profiles` gate: public recipes here never carry one (docs/adr/0006,
2026-09-08 amendment). Which machine classes run the catalog is decided in
the consuming layers, keyed on the namespaced recipe name
`[recipes_config.overrides."thismoon/catalog"]` (quoted; remote recipes get
`<source>/<name>` identities): the base config leaves it on, and a
profile-gated role source withholds it for its machines with the same table
in its `overrides.toml`.

What stays in the consuming repo (machine wiring, per ADR-0006):

- `~/.config/catalog/registry.yaml` symlink — which repos a machine catalogs
  is per-machine. The sample registry ships beside the service
  (`services/catalog/registry.yaml`); keep the two in sync.
