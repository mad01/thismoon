# CLAUDE.md — toss-bin recipe

Public-layer recipe for `tools/toss-bin` (see docs/adr/0006 for the
layering). Builds and installs the toss-bin binary from the thismoon sources
cache to `~/.local/bin/toss-bin`, and defines the `rm` shell function that
routes deletes through `toss-bin --safe-mode`, falling back to plain `rm`
when the binary is missing.

Item keys are the dotfiles-era names (`packages.toss_bin` from the packages
recipe, `shell.functions.rm` from shell-aliases) so ralph state carries over
at cutover — do not rename them.

The only machine-private piece is optional: a machine that wants extra
protected paths ships `~/.config/toss-bin/config.yaml` from the consuming
repo (docs/adr/0006); without it the built-in deny-list applies. There is
no MCP registration and no service. The cutover PR in dotfiles deletes the
`toss-bin/` source directory, the `[packages.toss_bin]` stanza in the
packages recipe, and the `rm` function in shell-aliases in one change, plus
the stale `~/.local/bin` copy is simply overwritten by the first
`ralph up` from this source.
